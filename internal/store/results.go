package store

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/Kcchouette/gowlarr/internal/model"
)

// SaveResults persists a list of ReleaseInfo from a search, with an
// expiration duration, so that `gowlarr download <id>` works in a
// separate CLI invocation from `gowlarr search` (the two are separate
// processes: in-memory state does not survive between them — fix
// following independent plan review).
func (s *Store) SaveResults(results []model.ReleaseInfo, ttl time.Duration) ([]model.ReleaseInfo, error) {
	now := time.Now().UTC()
	expiresAt := now.Add(ttl).Format(time.RFC3339)
	createdAt := now.Format(time.RFC3339)

	tx, err := s.db.Begin()
	if err != nil {
		return nil, fmt.Errorf("beginning transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	stmt, err := tx.Prepare(`INSERT INTO search_results
		(indexer_id, indexer_name, title, details_url, download_link, info_hash,
		 size_bytes, publish_date, seeders, peers, grabs, categories, protocol,
		 created_at, expires_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`)
	if err != nil {
		return nil, fmt.Errorf("preparing insert: %w", err)
	}
	defer stmt.Close()

	saved := make([]model.ReleaseInfo, 0, len(results))
	for _, r := range results {
		categoriesJSON, err := json.Marshal(r.Categories)
		if err != nil {
			return nil, fmt.Errorf("marshaling categories: %w", err)
		}

		res, err := stmt.Exec(
			r.IndexerID, r.IndexerName, r.Title, nullableString(r.Details), r.DownloadLink,
			nullableString(r.InfoHash), r.Size, r.PublishDate.UTC().Format(time.RFC3339),
			r.Seeders, r.Peers, r.Grabs, string(categoriesJSON), string(r.Protocol),
			createdAt, expiresAt,
		)
		if err != nil {
			return nil, fmt.Errorf("inserting search result: %w", err)
		}
		id, err := res.LastInsertId()
		if err != nil {
			return nil, fmt.Errorf("reading last insert id: %w", err)
		}
		r.ID = id
		saved = append(saved, r)
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("committing search results: %w", err)
	}
	return saved, nil
}

// GetResult retrieves a persisted search result by its ID, if not expired.
func (s *Store) GetResult(id int64) (model.ReleaseInfo, error) {
	row := s.db.QueryRow(`SELECT id, indexer_id, indexer_name, title, details_url,
		download_link, info_hash, size_bytes, publish_date, seeders, peers, grabs,
		categories, protocol, expires_at
		FROM search_results WHERE id = ?`, id)

	var r model.ReleaseInfo
	var details, infoHash sql.NullString
	var publishDate, expiresAt, protocol string
	var categoriesJSON string

	if err := row.Scan(&r.ID, &r.IndexerID, &r.IndexerName, &r.Title, &details,
		&r.DownloadLink, &infoHash, &r.Size, &publishDate, &r.Seeders, &r.Peers, &r.Grabs,
		&categoriesJSON, &protocol, &expiresAt); err != nil {
		if err == sql.ErrNoRows {
			return model.ReleaseInfo{}, fmt.Errorf("no search result with id %d (expired or unknown, run `search` again)", id)
		}
		return model.ReleaseInfo{}, fmt.Errorf("reading search result %d: %w", id, err)
	}

	if t, err := time.Parse(time.RFC3339, expiresAt); err == nil && time.Now().UTC().After(t) {
		return model.ReleaseInfo{}, fmt.Errorf("search result %d has expired, run `search` again", id)
	}

	r.Details = details.String
	r.InfoHash = infoHash.String
	r.Protocol = model.Protocol(protocol)
	if pd, err := time.Parse(time.RFC3339, publishDate); err == nil {
		r.PublishDate = pd
	}
	if err := json.Unmarshal([]byte(categoriesJSON), &r.Categories); err != nil {
		return model.ReleaseInfo{}, fmt.Errorf("unmarshaling categories for result %d: %w", id, err)
	}

	return r, nil
}

// PurgeExpiredResults deletes expired search results.
func (s *Store) PurgeExpiredResults() error {
	_, err := s.db.Exec(`DELETE FROM search_results WHERE expires_at < ?`,
		time.Now().UTC().Format(time.RFC3339))
	if err != nil {
		return fmt.Errorf("purging expired search results: %w", err)
	}
	return nil
}

func nullableString(v string) sql.NullString {
	if v == "" {
		return sql.NullString{}
	}
	return sql.NullString{String: v, Valid: true}
}
