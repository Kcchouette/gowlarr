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
	// ResolveLinks runs each result's download chain (download.before, e.g.
	// a "thanks" POST unlocking hidden DDL links) after the search. The
	// chain is side-effecting and must only run on explicit request.
	ResolveLinks bool
}

// QueryFromParams maps the transport-agnostic search parameters onto the
// engine's query type. It is the single conversion point between the two, so
// every entry path (CLI flags, HTTP handler) shares one mapping.
func QueryFromParams(p SearchParams) search.Query {
	return search.Query{
		Keywords:   p.Keywords,
		Categories: p.Categories,
		SearchType: p.SearchType,
		Season:     p.Season,
		Episode:    p.Episode,
		IMDbID:     p.IMDbID,
		TMDBID:     p.TMDBID,
	}
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

	// No default indexers: only explicitly configured (enabled) indexers and
	// an ad-hoc newznab are queried.
	var providers []search.Provider
	// NewznabAPIKey is optional (Spotweb public /api answers without a key).
	if params.IndexerID == "" && params.NewznabURL != "" {
		name := params.NewznabName
		if name == "" {
			name = "newznab-generic"
		}
		providers = append(providers, newznab.New(name, name, params.NewznabURL, params.NewznabAPIKey))
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
	result := engine.Search(ctx, QueryFromParams(params))

	releases := result.Releases
	if params.ResolveLinks {
		releases = s.resolveLinks(ctx, providers, releases)
	}
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

// linkResolver is implemented by providers whose download chain (e.g. a
// "thanks" POST) can be run on demand to reveal the actual DDL links.
type linkResolver interface {
	ResolveDownloadLink(ctx context.Context, rel model.ReleaseInfo) (model.ReleaseInfo, error)
}

// resolveLinks runs each result's download chain through its provider,
// revealing the actual links. Failures are logged and the raw link is kept:
// a broken chain must never fail the whole search.
func (s *SearchService) resolveLinks(ctx context.Context, providers []search.Provider, releases []model.ReleaseInfo) []model.ReleaseInfo {
	byID := make(map[string]linkResolver, len(providers))
	for _, p := range providers {
		if lr, ok := p.(linkResolver); ok {
			byID[p.ID()] = lr
		}
	}
	out := make([]model.ReleaseInfo, 0, len(releases))
	for _, rel := range releases {
		resolver, ok := byID[rel.IndexerID]
		if !ok {
			out = append(out, rel)
			continue
		}
		resolved, err := resolver.ResolveDownloadLink(ctx, rel)
		if err != nil {
			slog.Warn("resolving download link", "indexer", rel.IndexerID, "title", rel.Title, "err", err)
			out = append(out, rel)
			continue
		}
		out = append(out, resolved)
	}
	return out
}
