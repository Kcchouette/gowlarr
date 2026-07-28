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

// IndexerService gère les opérations CRUD sur les indexeurs.
type IndexerService struct {
	store *store.Store
	cfg   config.Config
}

// NewIndexerService crée un nouveau IndexerService.
func NewIndexerService(st *store.Store, cfg config.Config) *IndexerService {
	return &IndexerService{store: st, cfg: cfg}
}

// Add ajoute une instance d'indexeur à partir d'une définition.
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

// List retourne la liste des indexeurs configurés.
func (s *IndexerService) List(onlyEnabled bool) ([]store.IndexerConfig, error) {
	return s.store.ListIndexerConfigs(onlyEnabled, nil)
}

// Remove supprime un indexeur configuré.
func (s *IndexerService) Remove(id string) error {
	return s.store.DeleteIndexerConfig(id)
}

// Enable active ou désactive un indexeur.
func (s *IndexerService) Enable(id string, enabled bool) error {
	return s.store.SetIndexerEnabled(id, enabled)
}

// Test teste la connectivité/authentification d'un indexeur.
func (s *IndexerService) Test(ctx context.Context, id, version string) error {
	cfg, err := s.store.GetIndexerConfig(id, nil)
	if err != nil {
		return err
	}

	provider, err := BuildIndexerProvider(s.store, cfg)
	if err != nil {
		return err
	}

	_, err = provider.Search(ctx, search.Query{Keywords: "test"})
	if err != nil {
		if strings.Contains(err.Error(), "login:") {
			return fmt.Errorf("échec d'authentification: %w", err)
		}
		return fmt.Errorf("erreur réseau/parsing: %w", err)
	}
	return nil
}
