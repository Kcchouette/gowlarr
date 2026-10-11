package service

import (
	"fmt"
	"sync"

	"github.com/Kcchouette/cardigann-go/definition"

	"github.com/Kcchouette/gowlarr/internal/store"
)

// parsedDefCache caches parsed Cardigann definitions, indexed by their raw
// YAML content (content-addressed). The same definition is parsed on the
// search, download, indexer and resolve paths; memoizing avoids re-parsing it
// within a process. The parsed IndexerDefinition is safe to share: it is
// treated as read-only after construction (the engine only mutates the
// provider's config, never the definition).
//
// Sizing assumption: the YAML strings come from the local definition cache,
// so the cardinality is bounded by the definitions corpus.
//
// Parse ERRORS are deliberately not cached: a definition fixed by a later
// `defs sync` must be re-parseable.
var parsedDefCache sync.Map // string -> definition.IndexerDefinition

// parseDefinition parses raw Cardigann YAML, reusing a previously parsed
// result when the same content was already seen.
func parseDefinition(raw string) (definition.IndexerDefinition, error) {
	if v, ok := parsedDefCache.Load(raw); ok {
		return v.(definition.IndexerDefinition), nil
	}
	def, err := definition.Parse([]byte(raw))
	if err != nil {
		return definition.IndexerDefinition{}, err
	}
	actual, _ := parsedDefCache.LoadOrStore(raw, def)
	return actual.(definition.IndexerDefinition), nil
}

// loadDefinition fetches a definition's raw YAML from the store and parses it,
// reusing the content-addressed parse cache. An empty version selects the
// fallback chain (latest stored, matching store.GetDefinitionYAMLFallback); a
// non-empty version pins that exact definition version.
func loadDefinition(st *store.Store, id, version string) (definition.IndexerDefinition, error) {
	var raw string
	var err error
	if version == "" {
		raw, _, err = st.GetDefinitionYAMLFallback(id)
	} else {
		raw, err = st.GetDefinitionYAML(id, version)
	}
	if err != nil {
		return definition.IndexerDefinition{}, fmt.Errorf("loading definition %q: %w", id, err)
	}
	def, err := parseDefinition(raw)
	if err != nil {
		return definition.IndexerDefinition{}, fmt.Errorf("parsing definition %q: %w", id, err)
	}
	return def, nil
}
