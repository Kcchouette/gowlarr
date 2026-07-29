package service

import (
	"context"
	"fmt"
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
	def, err := definition.Parse([]byte(raw))
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
