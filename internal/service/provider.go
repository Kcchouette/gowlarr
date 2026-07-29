// Package service implémente la couche métier de Gowlarr, séparant la logique
// d'orchestration des commandes CLI et du serveur HTTP.
package service

import (
	"fmt"
	"log/slog"

	"github.com/Kcchouette/cardigann-go/definition"
	cardigannengine "github.com/Kcchouette/cardigann-go/engine"
	"github.com/Kcchouette/cardigann-go/httpclient"

	"github.com/Kcchouette/gowlarr/internal/cardigannadapter"
	"github.com/Kcchouette/gowlarr/internal/config"
	"github.com/Kcchouette/gowlarr/internal/flaresolverr"
	"github.com/Kcchouette/gowlarr/internal/search"
	"github.com/Kcchouette/gowlarr/internal/store"
)

// BuildConfiguredProviders construit un search.Provider Cardigann pour
// chaque indexeur activé, avec session authentifiée si la définition l'exige.
// Si cfg.FlareSolverrURL est défini, le transport FlareSolverr est utilisé
// pour contourner les protections Cloudflare.
func BuildConfiguredProviders(st *store.Store, cfg config.Config) ([]search.Provider, error) {
	configs, err := st.ListIndexerConfigs(true, nil)
	if err != nil {
		return nil, fmt.Errorf("listing indexer configs: %w", err)
	}

	var flareClient *flaresolverr.Client
	if cfg.FlareSolverrURL != "" {
		flareClient = flaresolverr.NewClient(cfg.FlareSolverrURL)
		slog.Debug("flaresolverr activé", "url", cfg.FlareSolverrURL)
	}

	providers := make([]search.Provider, 0, len(configs))
	for _, idxCfg := range configs {
		raw, _, err := st.GetDefinitionYAMLFallback(idxCfg.DefinitionID)
		if err != nil {
			slog.Warn("indexeur: definition non trouvée", "id", idxCfg.ID, "err", err)
			continue
		}
		def, err := definition.Parse([]byte(raw))
		if err != nil {
			slog.Warn("indexeur: parsing definition", "id", idxCfg.ID, "err", err)
			continue
		}
		if len(def.Links) == 0 {
			slog.Warn("indexeur: definition sans links[]", "id", idxCfg.ID)
			continue
		}

		client, err := httpclient.New(httpclient.Options{
			IndexerID: idxCfg.ID,
			Persister: store.NewCookiePersisterAdapter(st, nil),
			ProxyURL:  effectiveProxyURL(idxCfg.ProxyURL, cfg),
		})
		if err != nil {
			slog.Warn("indexeur: construction http client", "id", idxCfg.ID, "err", err)
			continue
		}

		// Injecter le transport FlareSolverr si configuré
		if flareClient != nil {
			transport := &flaresolverr.FlareSolverrTransport{
				Base:        client.HTTPClient().Transport,
				FlareClient: flareClient,
			}
			client.HTTPClient().Transport = transport
		}

		providers = append(providers, cardigannadapter.NewProvider(
			cardigannengine.NewAuthenticatedProvider(def, def.Links[0], idxCfg.Settings, client)))
	}
	return providers, nil
}

// BuildIndexerProvider construit un provider Cardigann pour un indexeur spécifique (test).
func BuildIndexerProvider(st *store.Store, appCfg config.Config, cfg store.IndexerConfig) (search.Provider, error) {
	raw, _, err := st.GetDefinitionYAMLFallback(cfg.DefinitionID)
	if err != nil {
		return nil, fmt.Errorf("loading definition: %w", err)
	}
	return buildIndexerProviderFromRaw(st, appCfg, cfg, raw)
}

func buildIndexerProviderFromRaw(st *store.Store, appCfg config.Config, cfg store.IndexerConfig, raw string) (search.Provider, error) {
	def, err := definition.Parse([]byte(raw))
	if err != nil {
		return nil, fmt.Errorf("parsing definition: %w", err)
	}
	if len(def.Links) == 0 {
		return nil, fmt.Errorf("definition %q has no links[]", cfg.DefinitionID)
	}

	client, err := httpclient.New(httpclient.Options{
		IndexerID: cfg.ID,
		Persister: store.NewCookiePersisterAdapter(st, nil),
		ProxyURL:  effectiveProxyURL(cfg.ProxyURL, appCfg),
	})
	if err != nil {
		return nil, fmt.Errorf("building http client: %w", err)
	}

	return cardigannadapter.NewProvider(
		cardigannengine.NewAuthenticatedProvider(def, def.Links[0], cfg.Settings, client)), nil
}
