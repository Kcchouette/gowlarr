package service

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/Kcchouette/gowlarr/internal/config"
	"github.com/Kcchouette/gowlarr/internal/model"
	"github.com/Kcchouette/gowlarr/internal/newznab"
	"github.com/Kcchouette/gowlarr/internal/search"
	"github.com/Kcchouette/gowlarr/internal/search/providers/apibay"
	"github.com/Kcchouette/gowlarr/internal/store"
)

// SearchParams contains the parameters for a search.
type SearchParams struct {
	Keywords        string
	Categories      []int
	SearchType      string
	Season, Episode int
	IMDbID, TMDBID  string
	NewznabURL      string
	NewznabAPIKey   string
	NewznabName     string
	// IndexerID restricts the search to one configured indexer ("" = all).
	IndexerID string
	// Protocol restricts the results to one protocol ("" = all).
	Protocol model.Protocol
}

// SearchResult contains the results of a search.
type SearchResult struct {
	Releases []model.ReleaseInfo
	Errors   []*search.ProviderError
}

// SearchService orchestrates multi-indexer searches.
type SearchService struct {
	store *store.Store
	cfg   config.Config
}

// NewSearchService creates a new SearchService.
func NewSearchService(st *store.Store, cfg config.Config) *SearchService {
	return &SearchService{store: st, cfg: cfg}
}

// Search executes a search across all active indexers.
func (s *SearchService) Search(ctx context.Context, params SearchParams) (SearchResult, error) {
	purgeStart := time.Now()
	if err := s.store.PurgeExpiredResults(); err != nil {
		slog.Warn("failed to purge expired results", "duration", time.Since(purgeStart), "err", err)
	} else {
		slog.Debug("purge expired results", "duration", time.Since(purgeStart))
	}

	providers := []search.Provider{apibay.New()}

	if params.IndexerID == "" {
		// Generic providers (apibay, ad-hoc newznab) only participate when
		// no specific configured indexer is requested.
		if params.NewznabURL != "" && params.NewznabAPIKey != "" {
			name := params.NewznabName
			if name == "" {
				name = "newznab-generic"
			}
			providers = append(providers, newznab.New(name, name, params.NewznabURL, params.NewznabAPIKey))
		}
	} else {
		// No generic providers when a specific indexer is targeted.
		providers = nil
	}

	configured, err := BuildConfiguredProviders(s.store, s.cfg)
	if err != nil {
		slog.Warn("loading configured indexers", "err", err)
	} else {
		for _, p := range configured {
			if params.IndexerID == "" || p.ID() == params.IndexerID {
				providers = append(providers, p)
			}
		}
	}

	engine := search.NewEngine(providers)
	engine.MaxResultsPerProvider = s.cfg.MaxResultsPerIndexer // 0 -> default (10) inside Search
	result := engine.Search(ctx, search.Query{
		Keywords:   params.Keywords,
		Categories: params.Categories,
		SearchType: params.SearchType,
		Season:     params.Season,
		Episode:    params.Episode,
		IMDbID:     params.IMDbID,
		TMDBID:     params.TMDBID,
	})

	releases := result.Releases
	if params.Protocol != "" {
		filtered := releases[:0]
		for _, r := range releases {
			if r.Protocol == params.Protocol {
				filtered = append(filtered, r)
			}
		}
		releases = filtered
	}

	saved, err := s.store.SaveResults(releases, 30*time.Minute)
	if err != nil {
		return SearchResult{}, fmt.Errorf("persisting search results: %w", err)
	}

	return SearchResult{Releases: saved, Errors: result.Errors}, nil
}
