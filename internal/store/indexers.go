package store

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"time"
)

// NewznabDefinitionID is the reserved DefinitionID of native Newznab
// indexers (settings: baseurl + apikey). It is NOT a cached Cardigann
// definition: configs with this ID are served by the native newznab client,
// so every code path that would load a definition for this ID must branch
// on it BEFORE the definition lookup.
const NewznabDefinitionID = "newznab"

// IndexerConfig represents a configured indexer instance (association of a
// cached Cardigann definition with user credentials/options), persisted to
// survive between CLI invocations.
type IndexerConfig struct {
	ID           string
	DefinitionID string
	Protocol     string // torrent | usenet
	Enabled      bool
	Settings     map[string]string
	ProxyURL     string
}

// SaveIndexerConfig upserts an indexer configuration.
// If key is non-nil, settings are encrypted into BLOB (settings_enc).
// Otherwise, they remain plaintext in TEXT (settings_json) for backward compatibility.
//
// Note: in Gowlarr's current state, callers always pass key=nil:
// settings_json (including any indexer credentials) is therefore stored
// in plaintext in SQLite. The encryption infrastructure exists in
// internal/crypt but is not wired up by default.
func (s *Store) SaveIndexerConfig(cfg IndexerConfig, key []byte) error {
	settingsJSON, err := json.Marshal(cfg.Settings)
	if err != nil {
		return fmt.Errorf("marshaling settings: %w", err)
	}
	now := time.Now().UTC().Format(time.RFC3339)
	plain, encrypted, err := marshalSecret(settingsJSON, key)
	if err != nil {
		return fmt.Errorf("saving indexer config %s: %w", cfg.ID, err)
	}

	// The two column couples stay on separate upserts: the plaintext branch
	// never touches settings_enc (and vice versa), so a key toggled off cannot
	// silently revive a stale encrypted blob.
	if key != nil {
		_, err = s.db.Exec(`INSERT INTO indexer_configs
			(id, definition_id, protocol, enabled, settings_json, settings_enc, proxy_url, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
			ON CONFLICT(id) DO UPDATE SET definition_id = excluded.definition_id,
				protocol = excluded.protocol, enabled = excluded.enabled,
				settings_json = excluded.settings_json, settings_enc = excluded.settings_enc,
				proxy_url = excluded.proxy_url, updated_at = excluded.updated_at`,
			cfg.ID, cfg.DefinitionID, cfg.Protocol, boolToInt(cfg.Enabled),
			plain, encrypted, nullableString(cfg.ProxyURL), now, now,
		)
	} else {
		_, err = s.db.Exec(`INSERT INTO indexer_configs
			(id, definition_id, protocol, enabled, settings_json, proxy_url, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?)
			ON CONFLICT(id) DO UPDATE SET definition_id = excluded.definition_id,
				protocol = excluded.protocol, enabled = excluded.enabled,
				settings_json = excluded.settings_json, proxy_url = excluded.proxy_url,
				updated_at = excluded.updated_at`,
			cfg.ID, cfg.DefinitionID, cfg.Protocol, boolToInt(cfg.Enabled),
			plain, nullableString(cfg.ProxyURL), now, now,
		)
	}
	if err != nil {
		return fmt.Errorf("saving indexer config %s: %w", cfg.ID, err)
	}
	return nil
}

// GetIndexerConfig retrieves an indexer configuration by its ID.
//
// Note: as long as the caller passes key=nil (the current default everywhere
// in the project), settings are read back from plaintext settings_json;
// do not assume that active encryption already protects credentials.
func (s *Store) GetIndexerConfig(id string, key []byte) (IndexerConfig, error) {
	row := s.db.QueryRow(`SELECT id, definition_id, protocol, enabled, settings_json, settings_enc, proxy_url
		FROM indexer_configs WHERE id = ?`, id)
	return doScanIndexerConfig(row, key)
}

// ListIndexerConfigs lists all configurations, or only enabled ones
// if onlyEnabled is true.
func (s *Store) ListIndexerConfigs(onlyEnabled bool, key []byte) ([]IndexerConfig, error) {
	query := `SELECT id, definition_id, protocol, enabled, settings_json, settings_enc, proxy_url FROM indexer_configs`
	if onlyEnabled {
		query += ` WHERE enabled = 1`
	}
	query += ` ORDER BY id`

	rows, err := s.db.Query(query)
	if err != nil {
		return nil, fmt.Errorf("listing indexer configs: %w", err)
	}
	defer rows.Close()

	var configs []IndexerConfig
	for rows.Next() {
		cfg, err := doScanIndexerConfig(rows, key)
		if err != nil {
			return nil, err
		}
		configs = append(configs, cfg)
	}
	return configs, rows.Err()
}

// DeleteIndexerConfig deletes an indexer configuration.
func (s *Store) DeleteIndexerConfig(id string) error {
	res, err := s.db.Exec(`DELETE FROM indexer_configs WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("deleting indexer config %s: %w", id, err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("checking delete result: %w", err)
	}
	if n == 0 {
		return fmt.Errorf("%w: no indexer config with id %q", ErrIndexerConfigNotFound, id)
	}
	return nil
}

// SetIndexerEnabled enables or disables an existing indexer configuration.
func (s *Store) SetIndexerEnabled(id string, enabled bool) error {
	res, err := s.db.Exec(`UPDATE indexer_configs SET enabled = ?, updated_at = ? WHERE id = ?`,
		boolToInt(enabled), time.Now().UTC().Format(time.RFC3339), id)
	if err != nil {
		return fmt.Errorf("updating indexer config %s: %w", id, err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("checking update result: %w", err)
	}
	if n == 0 {
		return fmt.Errorf("%w: no indexer config with id %q", ErrIndexerConfigNotFound, id)
	}
	return nil
}

type scanner interface {
	Scan(dest ...any) error
}

func doScanIndexerConfig(s scanner, key []byte) (IndexerConfig, error) {
	var cfg IndexerConfig
	var enabled int
	var settingsJSON string
	var settingsEnc []byte
	var proxyURL sql.NullString

	if err := s.Scan(&cfg.ID, &cfg.DefinitionID, &cfg.Protocol, &enabled, &settingsJSON, &settingsEnc, &proxyURL); err != nil {
		if err == sql.ErrNoRows {
			return IndexerConfig{}, fmt.Errorf("%w: indexer config not found", ErrIndexerConfigNotFound)
		}
		return IndexerConfig{}, fmt.Errorf("scanning indexer config: %w", err)
	}
	cfg.Enabled = enabled != 0
	cfg.ProxyURL = proxyURL.String
	cfg.Settings = make(map[string]string)
	decrypted, err := unmarshalSecret(settingsJSON, settingsEnc, key)
	if err != nil {
		return IndexerConfig{}, fmt.Errorf("loading settings for %s: %w", cfg.ID, err)
	}
	if len(decrypted) > 0 {
		if err := json.Unmarshal(decrypted, &cfg.Settings); err != nil {
			return IndexerConfig{}, fmt.Errorf("unmarshaling settings: %w", err)
		}
	}
	return cfg, nil
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}
