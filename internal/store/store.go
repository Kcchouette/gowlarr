// Package store gère l'accès à la base SQLite embarquée (modernc.org/sqlite,
// pur Go sans cgo — choix retenu pour rester facilement cross-compilable,
// notamment depuis Windows).
package store

import (
	"database/sql"
	"embed"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

//go:embed migrations/*.sql
var migrationsFS embed.FS

// Store encapsule la connexion SQLite et les opérations de persistance.
type Store struct {
	db *sql.DB
}

// Open ouvre (ou crée) la base SQLite au chemin donné, applique les
// migrations en attente et configure les pragmas recommandés pour un usage
// CLI mono-processus (busy_timeout, WAL, foreign_keys).
func Open(path string) (*Store, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, fmt.Errorf("creating database dir: %w", err)
	}

	dsn := fmt.Sprintf("file:%s?_pragma=busy_timeout(5000)&_pragma=foreign_keys(1)&_pragma=journal_mode(WAL)", path)
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("opening sqlite database %s: %w", path, err)
	}
	db.SetMaxOpenConns(1) // SQLite + writer unique : évite les erreurs "database is locked" en CLI.

	s := &Store{db: db}
	if err := s.migrate(); err != nil {
		_ = db.Close()
		return nil, err
	}
	return s, nil
}

// Close ferme la connexion à la base.
func (s *Store) Close() error {
	return s.db.Close()
}

// DB expose la connexion SQL brute pour les packages qui en ont besoin
// (recherche, définitions, etc.).
func (s *Store) DB() *sql.DB {
	return s.db
}

func (s *Store) migrate() error {
	if _, err := s.db.Exec(`CREATE TABLE IF NOT EXISTS schema_migrations (
		version INTEGER PRIMARY KEY,
		applied_at TEXT NOT NULL
	)`); err != nil {
		return fmt.Errorf("creating schema_migrations table: %w", err)
	}

	entries, err := fs.ReadDir(migrationsFS, "migrations")
	if err != nil {
		return fmt.Errorf("reading embedded migrations: %w", err)
	}

	var names []string
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".sql") {
			names = append(names, e.Name())
		}
	}
	sort.Strings(names)

	for _, name := range names {
		version, err := versionFromFilename(name)
		if err != nil {
			return err
		}

		var applied int
		row := s.db.QueryRow(`SELECT COUNT(1) FROM schema_migrations WHERE version = ?`, version)
		if err := row.Scan(&applied); err != nil {
			return fmt.Errorf("checking migration %d: %w", version, err)
		}
		if applied > 0 {
			continue
		}

		content, err := migrationsFS.ReadFile("migrations/" + name)
		if err != nil {
			return fmt.Errorf("reading migration %s: %w", name, err)
		}

		tx, err := s.db.Begin()
		if err != nil {
			return fmt.Errorf("beginning transaction for migration %s: %w", name, err)
		}
		if _, err := tx.Exec(string(content)); err != nil {
			_ = tx.Rollback()
			return fmt.Errorf("applying migration %s: %w", name, err)
		}
		if _, err := tx.Exec(`INSERT INTO schema_migrations (version, applied_at) VALUES (?, ?)`,
			version, time.Now().UTC().Format(time.RFC3339)); err != nil {
			_ = tx.Rollback()
			return fmt.Errorf("recording migration %s: %w", name, err)
		}
		if err := tx.Commit(); err != nil {
			return fmt.Errorf("committing migration %s: %w", name, err)
		}
	}
	return nil
}

func versionFromFilename(name string) (int, error) {
	prefix := strings.SplitN(name, "_", 2)[0]
	v, err := strconv.Atoi(prefix)
	if err != nil {
		return 0, fmt.Errorf("migration filename %q must start with a numeric version: %w", name, err)
	}
	return v, nil
}
