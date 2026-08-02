package store

import (
	"database/sql"
	"fmt"
	"time"
)

// DefinitionMeta describes a cached Cardigann definition (without the raw YAML).
type DefinitionMeta struct {
	ID           string
	Version      string
	YAMLSHA      string
	DownloadedAt time.Time
}

// SaveDefinition upserts a raw Cardigann definition (YAML) into the local cache,
// indexed by (id, version). The YAML is never redistributed: it is only
// cached for the current user (see legal disclaimer).
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

// GetDefinitionYAML retrieves the cached raw YAML for a given id/version.
func (s *Store) GetDefinitionYAML(id, version string) (string, error) {
	var raw string
	err := s.db.QueryRow(`SELECT raw_yaml FROM indexer_definitions WHERE id = ? AND version = ?`, id, version).Scan(&raw)
	if err == sql.ErrNoRows {
		return "", fmt.Errorf("%w: definition %q (version %s) not found in cache; run `gowlarr defs sync` first", ErrDefinitionNotFound, id, version)
	}
	if err != nil {
		return "", fmt.Errorf("reading definition %s/%s: %w", version, id, err)
	}
	return raw, nil
}

// ListDefinitions lists cached definitions for a given version
// ("" for all versions).
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

// DefinitionFull is a cached definition with its raw YAML content,
// used for fuzzy resolution where the YAML must be parsed to extract
// name, links, and other metadata.
type DefinitionFull struct {
	ID      string
	Version string
	YAML    string
}

// ListDefinitionsWithYAML returns all cached definitions with their raw YAML.
// Pass version="" to include all versions.
func (s *Store) ListDefinitionsWithYAML(version string) ([]DefinitionFull, error) {
	query := `SELECT id, version, raw_yaml FROM indexer_definitions`
	args := []any{}
	if version != "" {
		query += ` WHERE version = ?`
		args = append(args, version)
	}
	query += ` ORDER BY id`

	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("listing definitions with yaml: %w", err)
	}
	defer rows.Close()

	var defs []DefinitionFull
	for rows.Next() {
		var d DefinitionFull
		if err := rows.Scan(&d.ID, &d.Version, &d.YAML); err != nil {
			return nil, fmt.Errorf("scanning definition row: %w", err)
		}
		defs = append(defs, d)
	}
	return defs, rows.Err()
}

// versionsByPriority returns versions in priority order (v11 first, then v10-v1).
var versionsByPriority = []string{"v11", "v10", "v9", "v8", "v7", "v6", "v5", "v4", "v3", "v2", "v1"}

// GetDefinitionYAMLFallback returns the highest-priority version of a
// definition (v11 first, then v10-v1) with a single indexed query (range
// scan on the (id, version) unique index) instead of probing each version
// individually.
func (s *Store) GetDefinitionYAMLFallback(id string) (rawYAML, version string, err error) {
	rows, err := s.db.Query(`SELECT raw_yaml, version FROM indexer_definitions WHERE id = ?`, id)
	if err != nil {
		return "", "", fmt.Errorf("listing definition versions for %q: %w", id, err)
	}
	defer rows.Close()

	byVersion := make(map[string]string, 11)
	for rows.Next() {
		var raw, v string
		if err := rows.Scan(&raw, &v); err != nil {
			return "", "", fmt.Errorf("scanning definition version: %w", err)
		}
		byVersion[v] = raw
	}
	if err := rows.Err(); err != nil {
		return "", "", fmt.Errorf("iterating definition versions for %q: %w", id, err)
	}

	for _, v := range versionsByPriority {
		if raw, ok := byVersion[v]; ok {
			return raw, v, nil
		}
	}
	return "", "", fmt.Errorf("%w: definition %q not found in any version", ErrDefinitionNotFound, id)
}
