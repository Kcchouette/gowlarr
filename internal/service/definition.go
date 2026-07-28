package service

import (
	"fmt"

	"github.com/Kcchouette/gowlarr/internal/config"
	"github.com/Kcchouette/gowlarr/internal/store"
)

// DefinitionService gère les définitions Cardigann.
type DefinitionService struct {
	store *store.Store
	cfg   config.Config
}

// NewDefinitionService crée un nouveau DefinitionService.
func NewDefinitionService(st *store.Store, cfg config.Config) *DefinitionService {
	return &DefinitionService{store: st, cfg: cfg}
}

// List retourne la liste des définitions en cache.
func (s *DefinitionService) List() ([]store.DefinitionMeta, error) {
	return s.store.ListDefinitions("")
}

// Show retourne le YAML brut d'une définition.
func (s *DefinitionService) Show(id string) (string, error) {
	raw, _, err := s.store.GetDefinitionYAMLFallback(id)
	if err != nil {
		return "", err
	}
	return raw, nil
}

// Sync synchronise les définitions depuis la source distante.
func (s *DefinitionService) Sync() error {
	return fmt.Errorf("sync not implemented in service layer — use cardigann-go directly")
}
