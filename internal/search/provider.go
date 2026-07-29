// Package search defines the common search contract, implemented by both
// Cardigann providers (torrent/usenet) and the native generic Newznab
// client — the search engine/CLI must never know the implementation
// details of a concrete provider.
package search

import (
	"context"

	"github.com/Kcchouette/gowlarr/internal/model"
)

// Query represents a unified search request, independent of the provider.
type Query struct {
	Keywords   string
	Categories []int
	SearchType string // "search" | "tvsearch" | "movie" | "music" | "book"
	Season     int    // for tvsearch
	Episode    int    // for tvsearch
	IMDbID     string // for movie/tvsearch
	TMDBID     string // for movie
}

// Provider is the contract that any indexer search method must implement
// (Cardigann torrent, Cardigann usenet, native generic Newznab).
type Provider interface {
	// ID returns the stable identifier of the indexer (e.g. "1337x", "nzbgeek").
	ID() string
	// Name returns the human-readable name of the indexer.
	Name() string
	// Protocol returns the distribution protocol of this indexer.
	Protocol() model.Protocol
	// Search executes the search and returns normalized results.
	Search(ctx context.Context, q Query) ([]model.ReleaseInfo, error)
}
