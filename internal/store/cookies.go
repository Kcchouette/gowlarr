package store

import (
	"database/sql"
	"fmt"
	"time"
)

// LoadCookies implémente httpclient.CookiePersister : renvoie "" si aucun
// cookie n'est encore stocké pour cet indexeur.
func (s *Store) LoadCookies(indexerID string) (string, error) {
	var cookieJSON string
	err := s.db.QueryRow(`SELECT cookie_json FROM cookies WHERE indexer_id = ?`, indexerID).Scan(&cookieJSON)
	if err == sql.ErrNoRows {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("loading cookies for %s: %w", indexerID, err)
	}
	return cookieJSON, nil
}

// SaveCookies implémente httpclient.CookiePersister : upsert des cookies
// courants pour cet indexeur.
func (s *Store) SaveCookies(indexerID string, cookieJSON string) error {
	_, err := s.db.Exec(`INSERT INTO cookies (indexer_id, cookie_json, updated_at)
		VALUES (?, ?, ?)
		ON CONFLICT(indexer_id) DO UPDATE SET cookie_json = excluded.cookie_json, updated_at = excluded.updated_at`,
		indexerID, cookieJSON, time.Now().UTC().Format(time.RFC3339))
	if err != nil {
		return fmt.Errorf("saving cookies for %s: %w", indexerID, err)
	}
	return nil
}
