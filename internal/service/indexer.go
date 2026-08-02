package service

import (
	"context"
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

	raw, err := s.store.GetDefinitionYAML(defID, version)
	if err != nil {
		return err
	}
	def, err := parseDefinition(raw)
	if err != nil {
		return fmt.Errorf("parsing definition: %w", err)
	}

	protocol := "torrent"
	if def.IsUsenet() {
		protocol = "usenet"
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

	var raw string
	if version != "" {
		raw, err = s.store.GetDefinitionYAML(cfg.DefinitionID, version)
	} else {
		raw, _, err = s.store.GetDefinitionYAMLFallback(cfg.DefinitionID)
	}
	if err != nil {
		return fmt.Errorf("loading definition: %w", err)
	}

	provider, err := buildIndexerProviderFromRaw(s.store, s.cfg, cfg, raw)
	if err != nil {
		return err
	}

	_, err = provider.Search(ctx, search.Query{Keywords: "test"})
	if err != nil {
		if strings.Contains(err.Error(), "login:") {
			return fmt.Errorf("authentication failed: %w", err)
		}
		return fmt.Errorf("network/parsing error: %w", err)
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
	var results []ResolvedDefinition

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
			results = append(results, ResolvedDefinition{
				ID:      def.ID,
				Name:    def.Name,
				Domain:  domain,
				Version: d.Version,
				Score:   score,
			})
		}
	}

	sort.Slice(results, func(i, j int) bool {
		return results[i].Score > results[j].Score
	})

	return results, nil
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
