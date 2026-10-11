package service

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/Kcchouette/cardigann-go/definition"

	"github.com/Kcchouette/gowlarr/internal/config"
	"github.com/Kcchouette/gowlarr/internal/download"
	"github.com/Kcchouette/gowlarr/internal/model"
	"github.com/Kcchouette/gowlarr/internal/newznab"
	"github.com/Kcchouette/gowlarr/internal/store"
)

// DownloadService manages download resolution.
type DownloadService struct {
	store *store.Store
	cfg   config.Config
}

// NewDownloadService creates a new DownloadService.
func NewDownloadService(st *store.Store, cfg config.Config) *DownloadService {
	return &DownloadService{store: st, cfg: cfg}
}

// Resolve resolves a search result into a downloadable artifact.
func (s *DownloadService) Resolve(ctx context.Context, resultID int64) (download.Artifact, error) {
	release, err := s.store.GetResult(resultID)
	if err != nil {
		return download.Artifact{}, err
	}

	httpClient, authHeaders := s.buildClient(release.IndexerID)

	resolver := download.NewResolverWithClient(httpClient, authHeaders)
	artifact, err := resolver.Resolve(ctx, release)
	if err != nil {
		return download.Artifact{}, fmt.Errorf("resolving download for %q: %w", release.Title, err)
	}

	return artifact, nil
}

// GetRelease retrieves a search result by its ID.
func (s *DownloadService) GetRelease(resultID int64) (model.ReleaseInfo, error) {
	return s.store.GetResult(resultID)
}

// buildClient returns an HTTP client and authentication headers
// for the given indexer.
func (s *DownloadService) buildClient(indexerID string) (interface {
	Do(*http.Request) (*http.Response, error)
}, map[string]string) {
	defaultClient := &http.Client{Timeout: 30 * time.Second}
	if indexerID == "" {
		return defaultClient, nil
	}

	cfg, err := s.store.GetIndexerConfig(indexerID, nil)
	if err != nil {
		slog.Warn("download: loading indexer config, falling back to default client", "indexer_id", indexerID, "err", err)
		return defaultClient, nil
	}

	// Native Newznab configs have no cached definition: reuse the native
	// client so the configured proxy/DNS overrides apply to the NZB fetch.
	// Newznab enclosure URLs embed the API key, so no auth headers are
	// needed (the resolver attaches them only to the trusted host anyway).
	if cfg.DefinitionID == store.NewznabDefinitionID {
		client, err := newznab.NewWithTransport(cfg.ID, cfg.ID,
			cfg.Settings["baseurl"], cfg.Settings["apikey"],
			effectiveProxyURL(cfg.ProxyURL, s.cfg), s.cfg.DNSOverrides)
		if err != nil {
			slog.Warn("download: building native newznab client, falling back to default client", "indexer_id", indexerID, "err", err)
			return defaultClient, nil
		}
		return client.HTTPClient, nil
	}

	def, err := loadDefinition(s.store, cfg.DefinitionID, "")
	if err != nil {
		slog.Warn("download: loading indexer definition, falling back to default client", "indexer_id", indexerID, "definition_id", cfg.DefinitionID, "err", err)
		return defaultClient, nil
	}
	if len(def.Links) == 0 {
		// Malformed/incomplete Cardigann definition: no base link
		// available, cannot build an authenticated provider.
		slog.Warn("download: definition has no links[], falling back to default client", "indexer_id", indexerID, "definition_id", cfg.DefinitionID)
		return defaultClient, nil
	}

	// Unlike the search/test paths, download does NOT apply DNS overrides or
	// the browser UA mask, and does not wrap FlareSolverr — this call-site
	// behavior is preserved deliberately (see clientOptions). Login stays
	// lazy in the cardigann engine, so no provider is constructed here: the
	// resolver only needs the HTTP client (with its shared cookie jar) plus
	// the definition-derived auth headers below.
	client, err := newIndexerHTTPClient(s.store, s.cfg, cfg, clientOptions{})
	if err != nil {
		slog.Warn("download: building indexer HTTP client, falling back to default client", "indexer_id", indexerID, "err", err)
		return defaultClient, nil
	}

	return client, authHeadersFromDef(def, cfg.Settings)
}

// authHeadersFromDef renders the definition's search request headers,
// substituting each {{ .Config.<key> }} placeholder with the indexer's
// configured setting value.
func authHeadersFromDef(def definition.IndexerDefinition, settings map[string]string) map[string]string {
	if def.Search.Headers == nil {
		return nil
	}
	authHeaders := make(map[string]string)
	for header, vals := range def.Search.Headers {
		if len(vals) > 0 {
			tmplStr := vals[0]
			for k, v := range settings {
				tmplStr = strings.ReplaceAll(tmplStr, "{{ .Config."+k+" }}", v)
			}
			authHeaders[header] = tmplStr
		}
	}
	return authHeaders
}
