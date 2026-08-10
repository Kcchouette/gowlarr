// Package service implements the business logic layer of Gowlarr, separating
// orchestration logic from CLI commands and HTTP server handlers.
package service

import (
	"fmt"
	"log/slog"
	"net/http"

	cardigannengine "github.com/Kcchouette/cardigann-go/engine"
	"github.com/Kcchouette/cardigann-go/httpclient"

	"github.com/Kcchouette/gowlarr/internal/cardigannadapter"
	"github.com/Kcchouette/gowlarr/internal/config"
	"github.com/Kcchouette/gowlarr/internal/flaresolverr"
	"github.com/Kcchouette/gowlarr/internal/netutil"
	"github.com/Kcchouette/gowlarr/internal/newznab"
	"github.com/Kcchouette/gowlarr/internal/search"
	"github.com/Kcchouette/gowlarr/internal/store"
)

// browserUA masks gowlarr as a regular browser: some WAF-protected sites
// return 503 to Go's default User-Agent.
const browserUA = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/126.0.0.0 Safari/537.36"

// applyDNSOverrides clones the client's transport with a DialContext that
// forces DNS resolution of the overridden hostnames to fixed IPs (equivalent
// to curl's --resolve). It only acts when overrides are non-empty and the
// transport is a plain *http.Transport; otherwise it leaves the client
// untouched (FlareSolverr wraps the transport afterwards when configured).
func applyDNSOverrides(client *httpclient.IndexerClient, overrides map[string]string) {
	if len(overrides) == 0 {
		return
	}
	tr, ok := client.HTTPClient().Transport.(*http.Transport)
	if !ok {
		return
	}
	cloned := tr.Clone()
	cloned.DialContext = netutil.OverrideDialContext(cloned.DialContext, overrides)
	// A custom DialContext disables HTTP/2 by default; re-enable it so
	// Cloudflare-fronted hosts (e.g. hydra) still negotiate properly.
	cloned.ForceAttemptHTTP2 = true
	client.HTTPClient().Transport = cloned
}

// newznabProviderFromConfig builds a native Newznab provider from a
// persisted native config (DefinitionID == store.NewznabDefinitionID).
func newznabProviderFromConfig(cfg store.IndexerConfig, appCfg config.Config) (search.Provider, error) {
	baseURL := cfg.Settings["baseurl"]
	if baseURL == "" {
		return nil, fmt.Errorf("newznab indexer %s: missing baseurl setting", cfg.ID)
	}
	apiKey := cfg.Settings["apikey"]
	if apiKey == "" {
		return nil, fmt.Errorf("newznab indexer %s: missing apikey setting", cfg.ID)
	}
	return newznab.NewWithTransport(cfg.ID, cfg.ID, baseURL, apiKey, effectiveProxyURL(cfg.ProxyURL, appCfg), appCfg.DNSOverrides)
}

// BuildConfiguredProviders builds a Cardigann search.Provider for each
// enabled indexer, with authenticated session if the definition requires it.
// If cfg.FlareSolverrURL is set, the FlareSolverr transport is used to
// bypass Cloudflare protections.
func BuildConfiguredProviders(st *store.Store, cfg config.Config) ([]search.Provider, error) {
	configs, err := st.ListIndexerConfigs(true, nil)
	if err != nil {
		return nil, fmt.Errorf("listing indexer configs: %w", err)
	}

	var flareClient *flaresolverr.Client
	if cfg.FlareSolverrURL != "" {
		flareClient = flaresolverr.NewClient(cfg.FlareSolverrURL)
		slog.Debug("flaresolverr enabled", "url", cfg.FlareSolverrURL)
	}

	providers := make([]search.Provider, 0, len(configs))
	for _, idxCfg := range configs {
		// Native Newznab configs have no cached definition: build the native
		// client before any definition lookup.
		if idxCfg.DefinitionID == store.NewznabDefinitionID {
			p, err := newznabProviderFromConfig(idxCfg, cfg)
			if err != nil {
				slog.Warn("indexer: building native newznab provider", "id", idxCfg.ID, "err", err)
				continue
			}
			providers = append(providers, p)
			continue
		}

		raw, _, err := st.GetDefinitionYAMLFallback(idxCfg.DefinitionID)
		if err != nil {
			slog.Warn("indexer: definition not found", "id", idxCfg.ID, "err", err)
			continue
		}
		def, err := parseDefinition(raw)
		if err != nil {
			slog.Warn("indexer: parsing definition", "id", idxCfg.ID, "err", err)
			continue
		}
		if len(def.Links) == 0 {
			slog.Warn("indexer: definition has no links", "id", idxCfg.ID)
			continue
		}

		client, err := httpclient.New(httpclient.Options{
			IndexerID: idxCfg.ID,
			Persister: store.NewCookiePersisterAdapter(st, nil),
			ProxyURL:  effectiveProxyURL(idxCfg.ProxyURL, cfg),
		})
		if err != nil {
			slog.Warn("indexer: building http client", "id", idxCfg.ID, "err", err)
			continue
		}

		// Force DNS resolution of overridden hostnames before the
		// FlareSolverr transport wraps the underlying transport.
		applyDNSOverrides(client, cfg.DNSOverrides)

		// Mask as a regular browser: some WAF-protected sites reject Go's
		// default User-Agent with a 503.
		client.HTTPClient().Transport = httpclient.WrapUserAgent(client.HTTPClient().Transport, browserUA)

		// Inject FlareSolverr transport if configured
		if flareClient != nil {
			transport := &flaresolverr.FlareSolverrTransport{
				Base:        client.HTTPClient().Transport,
				FlareClient: flareClient,
			}
			client.HTTPClient().Transport = transport
		}

		// Download chains (e.g. a "thanks" POST that unlocks hidden DDL
		// links) must not run during search: they resolve only when the user
		// explicitly requests the links (--links / download).
		inner := cardigannengine.NewAuthenticatedProvider(def, def.Links[0], idxCfg.Settings, client)
		inner.SkipSearchDownload()
		providers = append(providers, cardigannadapter.NewProvider(inner))
	}
	return providers, nil
}

// BuildIndexerProvider builds a Cardigann provider for a specific indexer (testing).
func BuildIndexerProvider(st *store.Store, appCfg config.Config, cfg store.IndexerConfig) (search.Provider, error) {
	// Native Newznab configs have no cached definition: build the native
	// client before any definition lookup.
	if cfg.DefinitionID == store.NewznabDefinitionID {
		return newznabProviderFromConfig(cfg, appCfg)
	}
	raw, _, err := st.GetDefinitionYAMLFallback(cfg.DefinitionID)
	if err != nil {
		return nil, fmt.Errorf("loading definition: %w", err)
	}
	return buildIndexerProviderFromRaw(st, appCfg, cfg, raw)
}

func buildIndexerProviderFromRaw(st *store.Store, appCfg config.Config, cfg store.IndexerConfig, raw string) (search.Provider, error) {
	// Defensive: any caller of this raw variant is also protected against a
	// native config (raw is ignored for the sentinel).
	if cfg.DefinitionID == store.NewznabDefinitionID {
		return newznabProviderFromConfig(cfg, appCfg)
	}
	def, err := parseDefinition(raw)
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

	applyDNSOverrides(client, appCfg.DNSOverrides)

	// Mask as a regular browser (same rationale as BuildConfiguredProviders).
	client.HTTPClient().Transport = httpclient.WrapUserAgent(client.HTTPClient().Transport, browserUA)

	inner := cardigannengine.NewAuthenticatedProvider(def, def.Links[0], cfg.Settings, client)
	inner.SkipSearchDownload()
	return cardigannadapter.NewProvider(inner), nil
}
