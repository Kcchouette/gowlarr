package store

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/Kcchouette/gowlarr/internal/crypt"
)

// IndexerConfig représente une instance configurée d'indexeur (association
// d'une définition Cardigann en cache avec les identifiants/options de
// l'utilisateur), persistée pour survivre entre invocations CLI.
type IndexerConfig struct {
	ID           string
	DefinitionID string
	Protocol     string // torrent | usenet
	Enabled      bool
	Settings     map[string]string
	ProxyURL     string
}

// SaveIndexerConfig upsert une configuration d'indexeur.
// Si key est non nil, les settings sont chiffrés en BLOB (settings_enc).
// Sinon, ils restent en clair en TEXT (settings_json) pour rétrocompatibilité.
func (s *Store) SaveIndexerConfig(cfg IndexerConfig, key []byte) error {
	settingsJSON, err := json.Marshal(cfg.Settings)
	if err != nil {
		return fmt.Errorf("marshaling settings: %w", err)
	}
	now := time.Now().UTC().Format(time.RFC3339)

	if key != nil {
		encrypted, err := crypt.Encrypt(settingsJSON, key)
		if err != nil {
			return fmt.Errorf("encrypting settings: %w", err)
		}
		_, err = s.db.Exec(`INSERT INTO indexer_configs
			(id, definition_id, protocol, enabled, settings_json, settings_enc, proxy_url, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
			ON CONFLICT(id) DO UPDATE SET definition_id = excluded.definition_id,
				protocol = excluded.protocol, enabled = excluded.enabled,
				settings_json = excluded.settings_json, settings_enc = excluded.settings_enc,
				proxy_url = excluded.proxy_url, updated_at = excluded.updated_at`,
			cfg.ID, cfg.DefinitionID, cfg.Protocol, boolToInt(cfg.Enabled),
			"", encrypted, nullableString(cfg.ProxyURL), now, now,
		)
		if err != nil {
			return fmt.Errorf("saving indexer config %s: %w", cfg.ID, err)
		}
	} else {
		_, err = s.db.Exec(`INSERT INTO indexer_configs
			(id, definition_id, protocol, enabled, settings_json, proxy_url, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?)
			ON CONFLICT(id) DO UPDATE SET definition_id = excluded.definition_id,
				protocol = excluded.protocol, enabled = excluded.enabled,
				settings_json = excluded.settings_json, proxy_url = excluded.proxy_url,
				updated_at = excluded.updated_at`,
			cfg.ID, cfg.DefinitionID, cfg.Protocol, boolToInt(cfg.Enabled),
			string(settingsJSON), nullableString(cfg.ProxyURL), now, now,
		)
		if err != nil {
			return fmt.Errorf("saving indexer config %s: %w", cfg.ID, err)
		}
	}
	return nil
}

// GetIndexerConfig récupère une configuration d'indexeur par son ID.
func (s *Store) GetIndexerConfig(id string, key []byte) (IndexerConfig, error) {
	row := s.db.QueryRow(`SELECT id, definition_id, protocol, enabled, settings_json, settings_enc, proxy_url
		FROM indexer_configs WHERE id = ?`, id)
	return scanIndexerConfig(row, key)
}

// ListIndexerConfigs liste toutes les configurations, ou uniquement les
// activées si onlyEnabled est vrai.
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
		cfg, err := scanIndexerConfigRows(rows, key)
		if err != nil {
			return nil, err
		}
		configs = append(configs, cfg)
	}
	return configs, rows.Err()
}

// DeleteIndexerConfig supprime une configuration d'indexeur.
func (s *Store) DeleteIndexerConfig(id string) error {
	res, err := s.db.Exec(`DELETE FROM indexer_configs WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("deleting indexer config %s: %w", id, err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("no indexer config with id %q", id)
	}
	return nil
}

// SetIndexerEnabled active/désactive une configuration d'indexeur existante.
func (s *Store) SetIndexerEnabled(id string, enabled bool) error {
	res, err := s.db.Exec(`UPDATE indexer_configs SET enabled = ?, updated_at = ? WHERE id = ?`,
		boolToInt(enabled), time.Now().UTC().Format(time.RFC3339), id)
	if err != nil {
		return fmt.Errorf("updating indexer config %s: %w", id, err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("no indexer config with id %q", id)
	}
	return nil
}

type scanner interface {
	Scan(dest ...any) error
}

func scanIndexerConfig(row *sql.Row, key []byte) (IndexerConfig, error) {
	return doScanIndexerConfig(row, key)
}

func scanIndexerConfigRows(rows *sql.Rows, key []byte) (IndexerConfig, error) {
	return doScanIndexerConfig(rows, key)
}

func doScanIndexerConfig(s scanner, key []byte) (IndexerConfig, error) {
	var cfg IndexerConfig
	var enabled int
	var settingsJSON string
	var settingsEnc []byte
	var proxyURL sql.NullString

	if err := s.Scan(&cfg.ID, &cfg.DefinitionID, &cfg.Protocol, &enabled, &settingsJSON, &settingsEnc, &proxyURL); err != nil {
		if err == sql.ErrNoRows {
			return IndexerConfig{}, fmt.Errorf("indexer config not found")
		}
		return IndexerConfig{}, fmt.Errorf("scanning indexer config: %w", err)
	}
	cfg.Enabled = enabled != 0
	cfg.ProxyURL = proxyURL.String
	cfg.Settings = make(map[string]string)

	// Try encrypted settings first (if key provided and data exists)
	if key != nil && len(settingsEnc) > 0 {
		decrypted, err := crypt.Decrypt(settingsEnc, key)
		if err != nil {
			return IndexerConfig{}, fmt.Errorf("decrypting settings: %w", err)
		}
		_ = json.Unmarshal(decrypted, &cfg.Settings)
	} else if settingsJSON != "" {
		_ = json.Unmarshal([]byte(settingsJSON), &cfg.Settings)
	}
	return cfg, nil
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}
