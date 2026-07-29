package service

import (
	"fmt"

	"github.com/Kcchouette/gowlarr/internal/config"
	"github.com/Kcchouette/gowlarr/internal/store"
)

// DefinitionService manages Cardigann definitions.
type DefinitionService struct {
	store *store.Store
	cfg   config.Config
}

// NewDefinitionService creates a new DefinitionService.
func NewDefinitionService(st *store.Store, cfg config.Config) *DefinitionService {
	return &DefinitionService{store: st, cfg: cfg}
}

// List returns the list of cached definitions.
func (s *DefinitionService) List() ([]store.DefinitionMeta, error) {
	return s.store.ListDefinitions("")
}

// Show returns the raw YAML of a definition.
func (s *DefinitionService) Show(id string) (string, error) {
	raw, _, err := s.store.GetDefinitionYAMLFallback(id)
	if err != nil {
		return "", err
	}
	return raw, nil
}

// Sync synchronizes definitions from the remote source.
func (s *DefinitionService) Sync() error {
	return fmt.Errorf("sync not implemented in service layer — use cardigann-go directly")
}
