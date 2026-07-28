package store

import (
	"database/sql"
	"fmt"
	"time"
)

// DefinitionMeta décrit une définition Cardigann en cache (sans le YAML brut).
type DefinitionMeta struct {
	ID           string
	Version      string
	YAMLSHA      string
	DownloadedAt time.Time
}

// SaveDefinition upsert une définition Cardigann brute (YAML) en cache local,
// indexée par (id, version). Le YAML n'est jamais redistribué : il est
// uniquement mis en cache pour l'utilisateur courant (cf. disclaimer légal).
func (s *Store) SaveDefinition(id, version, sha, rawYAML string) error {
	_, err := s.db.Exec(`INSERT INTO indexer_definitions (id, version, yaml_sha, downloaded_at, raw_yaml)
		VALUES (?, ?, ?, ?, ?)
		ON CONFLICT(id, version) DO UPDATE SET yaml_sha = excluded.yaml_sha,
			downloaded_at = excluded.downloaded_at, raw_yaml = excluded.raw_yaml`,
		id, version, sha, time.Now().UTC().Format(time.RFC3339), rawYAML)
	if err != nil {
		return fmt.Errorf("saving definition %s/%s: %w", version, id, err)
	}
	return nil
}

// GetDefinitionYAML récupère le YAML brut mis en cache pour un id/version donnés.
func (s *Store) GetDefinitionYAML(id, version string) (string, error) {
	var raw string
	err := s.db.QueryRow(`SELECT raw_yaml FROM indexer_definitions WHERE id = ? AND version = ?`, id, version).Scan(&raw)
	if err == sql.ErrNoRows {
		return "", fmt.Errorf("definition %q (version %s) not found in cache; run `gowlarr defs sync` first", id, version)
	}
	if err != nil {
		return "", fmt.Errorf("reading definition %s/%s: %w", version, id, err)
	}
	return raw, nil
}

// ListDefinitions liste les définitions en cache pour une version donnée
// ("" pour toutes versions confondues).
func (s *Store) ListDefinitions(version string) ([]DefinitionMeta, error) {
	query := `SELECT id, version, yaml_sha, downloaded_at FROM indexer_definitions`
	args := []any{}
	if version != "" {
		query += ` WHERE version = ?`
		args = append(args, version)
	}
	query += ` ORDER BY id`

	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("listing definitions: %w", err)
	}
	defer rows.Close()

	var metas []DefinitionMeta
	for rows.Next() {
		var m DefinitionMeta
		var downloadedAt string
		if err := rows.Scan(&m.ID, &m.Version, &m.YAMLSHA, &downloadedAt); err != nil {
			return nil, fmt.Errorf("scanning definition row: %w", err)
		}
		if t, err := time.Parse(time.RFC3339, downloadedAt); err == nil {
			m.DownloadedAt = t
		}
		metas = append(metas, m)
	}
	return metas, rows.Err()
}
