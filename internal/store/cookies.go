package store

import (
	"database/sql"
	"fmt"
	"time"

	"github.com/Kcchouette/gowlarr/internal/crypt"
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

	// Try encrypted first
	if key != nil && len(cookieEnc) > 0 {
		decrypted, err := crypt.Decrypt(cookieEnc, key)
		if err != nil {
			return "", fmt.Errorf("decrypting cookies for %s: %w", indexerID, err)
		}
		return string(decrypted), nil
	}
	return cookieJSON, nil
}

// SaveCookies implements httpclient.CookiePersister: upserts current
// cookies for this indexer.
// If key is non-nil, encrypts into cookie_enc (BLOB).
// Otherwise, stores in cookie_json (TEXT) for backward compatibility.
func (s *Store) SaveCookies(indexerID string, cookieJSON string, key []byte) error {
	now := time.Now().UTC().Format(time.RFC3339)

	if key != nil {
		encrypted, err := crypt.Encrypt([]byte(cookieJSON), key)
		if err != nil {
			return fmt.Errorf("encrypting cookies for %s: %w", indexerID, err)
		}
		_, err = s.db.Exec(`INSERT INTO cookies (indexer_id, cookie_json, cookie_enc, updated_at)
			VALUES (?, ?, ?, ?)
			ON CONFLICT(indexer_id) DO UPDATE SET cookie_json = excluded.cookie_json,
				cookie_enc = excluded.cookie_enc, updated_at = excluded.updated_at`,
			indexerID, "", encrypted, now)
		if err != nil {
			return fmt.Errorf("saving cookies for %s: %w", indexerID, err)
		}
	} else {
		_, err := s.db.Exec(`INSERT INTO cookies (indexer_id, cookie_json, updated_at)
			VALUES (?, ?, ?)
			ON CONFLICT(indexer_id) DO UPDATE SET cookie_json = excluded.cookie_json, updated_at = excluded.updated_at`,
			indexerID, cookieJSON, now)
		if err != nil {
			return fmt.Errorf("saving cookies for %s: %w", indexerID, err)
		}
	}
	return nil
}
