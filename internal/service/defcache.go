package service

import (
	"sync"

	"github.com/Kcchouette/cardigann-go/definition"
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
