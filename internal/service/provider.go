// Package service implements the business logic layer of Gowlarr, separating
// orchestration logic from CLI commands and HTTP server handlers.
package service

import (
	"fmt"
	"log/slog"
	"net/http"

	"github.com/Kcchouette/cardigann-go/definition"
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
	// Cloudflare-fronted hosts (e.g. a private tracker) still negotiate properly.
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

// clientOptions captures the transport-level differences between the code
// paths that build an indexer HTTP client. Each historical call site keeps
// exactly the behavior it always had — these knobs are NOT harmonized, because
// doing so would silently change observable behavior on paths the tests pin.
type clientOptions struct {
	// flareClient, when non-nil, wraps the transport with a FlareSolverr
	// transport to bypass Cloudflare challenges.
	flareClient *flaresolverr.Client
	// applyDNS forces DNS resolution of overridden hostnames to fixed IPs.
	applyDNS bool
	// maskUserAgent swaps the default Go User-Agent for a browser one, which
	// some WAF-protected sites require to avoid a 503.
	maskUserAgent bool
}

// newIndexerHTTPClient builds the HTTP client shared by every Cardigann code
// path: cookie persistence, effective proxy, then the optional DNS override,
// UA mask and FlareSolverr wrapper applied in that exact order.
func newIndexerHTTPClient(st *store.Store, appCfg config.Config, cfg store.IndexerConfig, opts clientOptions) (*httpclient.IndexerClient, error) {
	client, err := httpclient.New(httpclient.Options{
		IndexerID: cfg.ID,
		Persister: store.NewCookiePersisterAdapter(st, nil),
		ProxyURL:  effectiveProxyURL(cfg.ProxyURL, appCfg),
	})
	if err != nil {
		return nil, fmt.Errorf("building http client: %w", err)
	}

	// DNS overrides must be applied before the FlareSolverr transport wraps
	// the underlying transport (see applyDNSOverrides).
	if opts.applyDNS {
		applyDNSOverrides(client, appCfg.DNSOverrides)
	}

	if opts.maskUserAgent {
		client.HTTPClient().Transport = httpclient.WrapUserAgent(client.HTTPClient().Transport, browserUA)
	}

	if opts.flareClient != nil {
		client.HTTPClient().Transport = &flaresolverr.FlareSolverrTransport{
			Base:        client.HTTPClient().Transport,
			FlareClient: opts.flareClient,
		}
	}
	return client, nil
}

// buildCardigannProvider wraps a parsed definition and a client into a
// Cardigann search.Provider. Download chains (e.g. a "thanks" POST unlocking
// hidden DDL links) are skipped: they resolve only when the user explicitly
// requests the links (--links / download).
func buildCardigannProvider(cfg store.IndexerConfig, def definition.IndexerDefinition, client *httpclient.IndexerClient) search.Provider {
	inner := cardigannengine.NewAuthenticatedProvider(def, def.Links[0], cfg.Settings, client)
	inner.SkipSearchDownload()
	return cardigannadapter.NewProvider(inner)
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

		def, err := loadDefinition(st, idxCfg.DefinitionID, "")
		if err != nil {
			slog.Warn("indexer: loading definition", "id", idxCfg.ID, "err", err)
			continue
		}
		if len(def.Links) == 0 {
			slog.Warn("indexer: definition has no links", "id", idxCfg.ID)
			continue
		}

		client, err := newIndexerHTTPClient(st, cfg, idxCfg, clientOptions{
			flareClient:   flareClient,
			applyDNS:      true,
			maskUserAgent: true,
		})
		if err != nil {
			slog.Warn("indexer: building http client", "id", idxCfg.ID, "err", err)
			continue
		}

		providers = append(providers, buildCardigannProvider(idxCfg, def, client))
	}
	return providers, nil
}

// buildIndexerProvider builds a Cardigann provider for a specific indexer
// (testing). It returns errors instead of skipping, because the caller is an
// explicit single-indexer action where a failure must surface to the user.
func buildIndexerProvider(st *store.Store, appCfg config.Config, cfg store.IndexerConfig) (search.Provider, error) {
	// Native Newznab configs have no cached definition: build the native
	// client before any definition lookup.
	if cfg.DefinitionID == store.NewznabDefinitionID {
		return newznabProviderFromConfig(cfg, appCfg)
	}
	def, err := loadDefinition(st, cfg.DefinitionID, "")
	if err != nil {
		return nil, err
	}
	if len(def.Links) == 0 {
		return nil, fmt.Errorf("definition %q has no links[]", cfg.DefinitionID)
	}
	client, err := newIndexerHTTPClient(st, appCfg, cfg, clientOptions{
		applyDNS:      true,
		maskUserAgent: true,
	})
	if err != nil {
		return nil, err
	}
	return buildCardigannProvider(cfg, def, client), nil
}
