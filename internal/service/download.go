package service

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/Kcchouette/cardigann-go/definition"
	cardigannengine "github.com/Kcchouette/cardigann-go/engine"
	"github.com/Kcchouette/cardigann-go/httpclient"

	"github.com/Kcchouette/gowlarr/internal/config"
	"github.com/Kcchouette/gowlarr/internal/download"
	"github.com/Kcchouette/gowlarr/internal/model"
	"github.com/Kcchouette/gowlarr/internal/store"
)

// DownloadService gère la résolution des téléchargements.
type DownloadService struct {
	store *store.Store
	cfg   config.Config
}

// NewDownloadService crée un nouveau DownloadService.
func NewDownloadService(st *store.Store, cfg config.Config) *DownloadService {
	return &DownloadService{store: st, cfg: cfg}
}

// Resolve résout un résultat de recherche en un artifact téléchargeable.
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

// GetRelease récupère un résultat de recherche par son ID.
func (s *DownloadService) GetRelease(resultID int64) (model.ReleaseInfo, error) {
	return s.store.GetResult(resultID)
}

// buildClient retourne un client HTTP et les headers d'authentification
// pour l'indexeur donné.
func (s *DownloadService) buildClient(indexerID string) (interface {
	Do(*http.Request) (*http.Response, error)
}, map[string]string) {
	defaultClient := &http.Client{Timeout: 30 * time.Second}
	if indexerID == "" {
		return defaultClient, nil
	}

	cfg, err := s.store.GetIndexerConfig(indexerID, nil)
	if err != nil {
		return defaultClient, nil
	}

	raw, _, err := s.store.GetDefinitionYAMLFallback(cfg.DefinitionID)
	if err != nil {
		return defaultClient, nil
	}

	def, err := definition.Parse([]byte(raw))
	if err != nil {
		return defaultClient, nil
	}

	client, err := httpclient.New(httpclient.Options{
		IndexerID: cfg.ID,
		Persister: store.NewCookiePersisterAdapter(s.store, nil),
		ProxyURL:  cfg.ProxyURL,
	})
	if err != nil {
		return defaultClient, nil
	}

	_ = cardigannengine.NewAuthenticatedProvider(def, def.Links[0], cfg.Settings, client)

	var authHeaders map[string]string
	if def.Search.Headers != nil {
		authHeaders = make(map[string]string)
		for header, vals := range def.Search.Headers {
			if len(vals) > 0 {
				tmplStr := vals[0]
				for k, v := range cfg.Settings {
					tmplStr = strings.ReplaceAll(tmplStr, "{{ .Config."+k+" }}", v)
				}
				authHeaders[header] = tmplStr
			}
		}
	}

	return client, authHeaders
}
