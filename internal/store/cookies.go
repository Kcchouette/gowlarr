package store

import (
	"database/sql"
	"fmt"
	"time"
)

// LoadCookies implements httpclient.CookiePersister: returns "" if no
// cookies are stored yet for this indexer.
// If key is non-nil, attempts to decrypt cookie_enc (BLOB).
// Otherwise, reads cookie_json (TEXT) for backward compatibility.
func (s *Store) LoadCookies(indexerID string, key []byte) (string, error) {
	var cookieJSON string
	var cookieEnc []byte
	err := s.db.QueryRow(`SELECT cookie_json, cookie_enc FROM cookies WHERE indexer_id = ?`, indexerID).Scan(&cookieJSON, &cookieEnc)
	if err == sql.ErrNoRows {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("loading cookies for %s: %w", indexerID, err)
	}

	cookies, err := unmarshalSecret(cookieJSON, cookieEnc, key)
	if err != nil {
		return "", fmt.Errorf("loading cookies for %s: %w", indexerID, err)
	}
	return string(cookies), nil
}

// SaveCookies implements httpclient.CookiePersister: upserts current
// cookies for this indexer.
// If key is non-nil, encrypts into cookie_enc (BLOB).
// Otherwise, stores in cookie_json (TEXT) for backward compatibility.
func (s *Store) SaveCookies(indexerID string, cookieJSON string, key []byte) error {
	now := time.Now().UTC().Format(time.RFC3339)
	plain, encrypted, err := marshalSecret([]byte(cookieJSON), key)
	if err != nil {
		return fmt.Errorf("saving cookies for %s: %w", indexerID, err)
	}

	// The two column couples stay on separate upserts: the plaintext branch
	// never touches cookie_enc (and vice versa), so a key toggled off cannot
	// silently revive a stale encrypted blob.
	if key != nil {
		_, err = s.db.Exec(`INSERT INTO cookies (indexer_id, cookie_json, cookie_enc, updated_at)
			VALUES (?, ?, ?, ?)
			ON CONFLICT(indexer_id) DO UPDATE SET cookie_json = excluded.cookie_json,
				cookie_enc = excluded.cookie_enc, updated_at = excluded.updated_at`,
			indexerID, plain, encrypted, now)
	} else {
		_, err = s.db.Exec(`INSERT INTO cookies (indexer_id, cookie_json, updated_at)
			VALUES (?, ?, ?)
			ON CONFLICT(indexer_id) DO UPDATE SET cookie_json = excluded.cookie_json, updated_at = excluded.updated_at`,
			indexerID, plain, now)
	}
	if err != nil {
		return fmt.Errorf("saving cookies for %s: %w", indexerID, err)
	}
	return nil
}
