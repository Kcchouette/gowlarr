package service

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"sort"
	"strings"

	"github.com/Kcchouette/cardigann-go/definition"

	"github.com/Kcchouette/gowlarr/internal/config"
	"github.com/Kcchouette/gowlarr/internal/search"
	"github.com/Kcchouette/gowlarr/internal/store"
)

// IndexerService manages CRUD operations on indexers.
type IndexerService struct {
	store *store.Store
	cfg   config.Config
}

// NewIndexerService creates a new IndexerService.
func NewIndexerService(st *store.Store, cfg config.Config) *IndexerService {
	return &IndexerService{store: st, cfg: cfg}
}

// Add adds an indexer instance from a definition.
func (s *IndexerService) Add(defID, instanceID, version, proxyURL string, settings map[string]string) error {
	if instanceID == "" {
		instanceID = defID
	}

	def, err := loadDefinition(s.store, defID, version)
	if err != nil {
		return err
	}

	protocol := "torrent"
	switch def.Type {
	case "usenet":
		protocol = "usenet"
	case "ddl":
		protocol = "ddl"
	case "streaming":
		protocol = "streaming"
	default:
		if def.IsUsenet() {
			protocol = "usenet"
		}
	}

	cfg := store.IndexerConfig{
		ID:           instanceID,
		DefinitionID: defID,
		Protocol:     protocol,
		Enabled:      true,
		Settings:     settings,
		ProxyURL:     proxyURL,
	}
	if err := s.store.SaveIndexerConfig(cfg, nil); err != nil {
		return err
	}
	return nil
}

// AddNewznab persists a native Newznab indexer (base URL, optional API key),
// the entry point behind `gowlarr indexer add newznab|nzb|usenet`. The config
// uses the reserved DefinitionID store.NewznabDefinitionID and is served by
// the native newznab client.
func (s *IndexerService) AddNewznab(instanceID, baseURL, apiKey, proxyURL string) error {
	u, err := url.Parse(baseURL)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return fmt.Errorf("invalid newznab base URL %q: expected http(s)://host[/path]", baseURL)
	}

	// SaveIndexerConfig upserts silently on id: reject a duplicate instance
	// ID instead of silently overwriting an existing indexer.
	if _, err := s.store.GetIndexerConfig(instanceID, nil); err == nil {
		return fmt.Errorf("indexer %q already exists: pass --id to choose another instance ID", instanceID)
	} else if !errors.Is(err, store.ErrIndexerConfigNotFound) {
		return err
	}

	cfg := store.IndexerConfig{
		ID:           instanceID,
		DefinitionID: store.NewznabDefinitionID,
		Protocol:     "usenet",
		Enabled:      true,
		Settings:     map[string]string{"baseurl": baseURL, "apikey": apiKey},
		ProxyURL:     proxyURL,
	}
	return s.store.SaveIndexerConfig(cfg, nil)
}

// List returns the list of configured indexers.
func (s *IndexerService) List(onlyEnabled bool) ([]store.IndexerConfig, error) {
	return s.store.ListIndexerConfigs(onlyEnabled, nil)
}

// Remove deletes a configured indexer.
func (s *IndexerService) Remove(id string) error {
	return s.store.DeleteIndexerConfig(id)
}

// Enable enables or disables an indexer.
func (s *IndexerService) Enable(id string, enabled bool) error {
	return s.store.SetIndexerEnabled(id, enabled)
}

// Test tests the connectivity/authentication of an indexer.
func (s *IndexerService) Test(ctx context.Context, id, version string) error {
	cfg, err := s.store.GetIndexerConfig(id, nil)
	if err != nil {
		return err
	}

	// Native Newznab indexers have no cached definition: the provider comes
	// straight from the persisted settings, without any definition lookup.
	if cfg.DefinitionID == store.NewznabDefinitionID {
		provider, err := newznabProviderFromConfig(cfg, s.cfg)
		if err != nil {
			return err
		}
		_, err = provider.Search(ctx, search.Query{Keywords: "test"})
		if err != nil {
			// The newznab client emits "returned HTTP 401" (uppercase) and,
			// after the <error> parsing, "Invalid API Key (code 100)".
			return classifySearchError(err, "http 401", "http 403", "invalid api")
		}
		return nil
	}

	// loadDefinition resolves the requested version (or the highest-priority
	// fallback when version is empty) and parses it via the shared cache.
	def, err := loadDefinition(s.store, cfg.DefinitionID, version)
	if err != nil {
		return err
	}
	if len(def.Links) == 0 {
		// Same guard as the batch/test provider builders: a malformed
		// definition must fail with an error, not panic on def.Links[0].
		return fmt.Errorf("definition %q has no links[]", cfg.DefinitionID)
	}

	client, err := newIndexerHTTPClient(s.store, s.cfg, cfg, clientOptions{
		maskUserAgent: true,
	})
	if err != nil {
		return err
	}

	provider := buildCardigannProvider(cfg, def, client)

	_, err = provider.Search(ctx, search.Query{Keywords: "test"})
	if err != nil {
		return classifySearchError(err, "login:")
	}
	return nil
}

// ResolvedDefinition is a Cardigann definition matched by fuzzy query.
type ResolvedDefinition struct {
	ID      string
	Name    string
	Domain  string
	Version string
	Score   int
}

// ResolveDefinition searches cached definitions by domain, name, or ID.
// Results are sorted by relevance score descending.
func (s *IndexerService) ResolveDefinition(query string, version string) ([]ResolvedDefinition, error) {
	defs, err := s.store.ListDefinitionsWithYAML(version)
	if err != nil {
		return nil, err
	}

	query = strings.ToLower(strings.TrimSpace(query))
	// Deduplicated by definition ID: when several versions match, only the
	// highest-priority one ("local" > v11 > v10 > ...) is kept.
	byID := make(map[string]ResolvedDefinition)

	for _, d := range defs {
		def, err := parseDefinition(d.YAML)
		if err != nil {
			continue
		}

		score := scoreMatch(query, def)
		if score > 0 {
			domain := ""
			if len(def.Links) > 0 {
				if u, err := url.Parse(def.Links[0]); err == nil {
					domain = u.Hostname()
				}
			}
			existing, ok := byID[def.ID]
			if !ok || versionPriority(d.Version) < versionPriority(existing.Version) {
				byID[def.ID] = ResolvedDefinition{
					ID:      def.ID,
					Name:    def.Name,
					Domain:  domain,
					Version: d.Version,
					Score:   score,
				}
			}
		}
	}

	results := make([]ResolvedDefinition, 0, len(byID))
	for _, r := range byID {
		results = append(results, r)
	}

	sort.Slice(results, func(i, j int) bool {
		return results[i].Score > results[j].Score
	})

	return results, nil
}

// versionPriority returns the rank of a definition version in
// versionsByPriority (lower = higher priority; unknown versions rank last).
func versionPriority(version string) int {
	for i, v := range store.VersionsByPriority() {
		if v == version {
			return i
		}
	}
	return len(store.VersionsByPriority())
}

// scoreMatch returns a relevance score (0-100) for a query against a definition.
func scoreMatch(query string, def definition.IndexerDefinition) int {
	query = strings.ToLower(strings.TrimSpace(query))

	// 1. Exact domain match → 100
	for _, link := range def.Links {
		if u, err := url.Parse(link); err == nil {
			if strings.ToLower(u.Hostname()) == query {
				return 100
			}
		}
	}

	// 2. Domain contains query → 80
	for _, link := range def.Links {
		if u, err := url.Parse(link); err == nil {
			if strings.Contains(strings.ToLower(u.Hostname()), query) {
				return 80
			}
		}
	}

	// 3. Definition ID exact match → 60
	idLower := strings.ToLower(def.ID)
	if idLower == query {
		return 60
	}

	// 4. Definition ID contains query → 50
	if strings.Contains(idLower, query) {
		return 50
	}

	// 5. Name contains query → 40
	nameLower := strings.ToLower(def.Name)
	if strings.Contains(nameLower, query) {
		return 40
	}

	// 6. Token overlap → 20
	queryTokens := strings.Fields(query)
	defTokens := strings.Fields(nameLower)
	for _, qt := range queryTokens {
		for _, dt := range defTokens {
			if strings.Contains(dt, qt) || strings.Contains(qt, dt) {
				return 20
			}
		}
	}

	return 0
}
