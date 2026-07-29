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

// SearchParams contient les paramètres d'une recherche.
type SearchParams struct {
	Keywords        string
	Categories      []int
	SearchType      string
	Season, Episode int
	IMDbID, TMDBID  string
	NewznabURL      string
	NewznabAPIKey   string
	NewznabName     string
}

// SearchResult contient les résultats d'une recherche.
type SearchResult struct {
	Releases []model.ReleaseInfo
	Errors   []*search.ProviderError
}

// SearchService orchestre les recherches multi-indexeurs.
type SearchService struct {
	store *store.Store
	cfg   config.Config
}

// NewSearchService crée un nouveau SearchService.
func NewSearchService(st *store.Store, cfg config.Config) *SearchService {
	return &SearchService{store: st, cfg: cfg}
}

// Search exécute une recherche sur tous les indexeurs actifs.
func (s *SearchService) Search(ctx context.Context, params SearchParams) (SearchResult, error) {
	purgeStart := time.Now()
	if err := s.store.PurgeExpiredResults(); err != nil {
		slog.Warn("échec purge résultats expirés", "duration", time.Since(purgeStart), "err", err)
	} else {
		slog.Debug("purge expired results", "duration", time.Since(purgeStart))
	}

	providers := []search.Provider{apibay.New()}

	if params.NewznabURL != "" && params.NewznabAPIKey != "" {
		name := params.NewznabName
		if name == "" {
			name = "newznab-generic"
		}
		providers = append(providers, newznab.New(name, name, params.NewznabURL, params.NewznabAPIKey))
	}

	configured, err := BuildConfiguredProviders(s.store, s.cfg)
	if err != nil {
		slog.Warn("chargement des indexeurs configurés", "err", err)
	} else {
		providers = append(providers, configured...)
	}

	engine := search.NewEngine(providers)
	result := engine.Search(ctx, search.Query{
		Keywords:   params.Keywords,
		Categories: params.Categories,
		SearchType: params.SearchType,
		Season:     params.Season,
		Episode:    params.Episode,
		IMDbID:     params.IMDbID,
		TMDBID:     params.TMDBID,
	})

	saved, err := s.store.SaveResults(result.Releases, 30*time.Minute)
	if err != nil {
		return SearchResult{}, fmt.Errorf("persisting search results: %w", err)
	}

	return SearchResult{Releases: saved, Errors: result.Errors}, nil
}
