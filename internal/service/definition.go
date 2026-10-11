package service

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/Kcchouette/cardigann-go/definition"

	"github.com/Kcchouette/gowlarr/internal/config"
	"github.com/Kcchouette/gowlarr/internal/store"
)

// DefinitionService manages Cardigann definitions: listing, inspection and
// ingestion of repo-local definitions.
//
// Interactive prompting is CLI-owned by design: asking a user for missing
// indexer settings is presentation, so it stays in the CLI layer rather than
// being lifted into the service.
type DefinitionService struct {
	store *store.Store
	cfg   config.Config
}

// NewDefinitionService creates a new DefinitionService.
func NewDefinitionService(st *store.Store, cfg config.Config) *DefinitionService {
	return &DefinitionService{store: st, cfg: cfg}
}

// List returns the cached definitions, optionally filtered by schema version
// (empty = all versions).
func (s *DefinitionService) List(version string) ([]store.DefinitionMeta, error) {
	return s.store.ListDefinitions(version)
}

// Show returns the raw YAML of a definition at the requested schema version.
// It always calls GetDefinitionYAML so `defs show` keeps its historical
// behavior: the CLI sets the "v11" default at the call-site, and an explicit
// empty --version= errors instead of silently falling back to another version.
func (s *DefinitionService) Show(id, version string) (string, error) {
	return s.store.GetDefinitionYAML(id, version)
}

// Load fetches and parses a definition at the requested schema version,
// reusing the content-addressed parse cache. An empty version selects the
// highest-priority cached version.
func (s *DefinitionService) Load(id, version string) (definition.IndexerDefinition, error) {
	return loadDefinition(s.store, id, version)
}

// IngestLocal upserts the repo-local definitions (definitions-local/*.yml)
// into the cache under version "local". It returns the number of ingested
// definitions. An empty dir is a no-op returning (0, nil).
func (s *DefinitionService) IngestLocal(dir string) (int, error) {
	if dir == "" {
		return 0, nil
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return 0, fmt.Errorf("reading local definitions dir: %w", err)
	}

	count := 0
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".yml") {
			continue
		}
		raw, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			return count, fmt.Errorf("reading local definition %s: %w", e.Name(), err)
		}
		def, err := definition.Parse(raw)
		if err != nil {
			return count, fmt.Errorf("parsing local definition %s: %w", e.Name(), err)
		}
		if err := s.store.SaveDefinition(def.ID, "local", "", string(raw)); err != nil {
			return count, err
		}
		count++
	}
	return count, nil
}
