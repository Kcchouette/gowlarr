package service

import (
	"log/slog"

	"github.com/Kcchouette/gowlarr/internal/store"
)

// CapsSearchModes is the set of Torznab/Newznab search modes Gowlarr can
// advertise in a caps response.
var CapsSearchModes = []string{"search", "tv-search", "movie-search", "music-search", "book-search"}

// AvailableSearchModes aggregates the search modes supported by the enabled
// indexers: a mode is available if at least one enabled indexer supports it.
// Native Newznab indexers support all five modes; Cardigann definitions
// contribute their caps.modes, with the corpus's "audio-search" normalized to
// the caps XML's "music-search". Definitions that fail to load or parse are
// skipped rather than failing the whole aggregation.
func AvailableSearchModes(st *store.Store) map[string]bool {
	avail := make(map[string]bool, len(CapsSearchModes))
	configs, err := st.ListIndexerConfigs(true, nil)
	if err != nil {
		slog.Warn("caps: listing indexer configs", "err", err)
		return avail
	}
	for _, cfg := range configs {
		if cfg.DefinitionID == store.NewznabDefinitionID {
			// Native Newznab: all five modes (the persisted settings carry
			// no per-indexer caps information).
			for _, m := range CapsSearchModes {
				avail[m] = true
			}
			continue
		}
		def, err := loadDefinition(st, cfg.DefinitionID, "")
		if err != nil {
			continue
		}
		for mode := range def.Caps.Modes {
			switch mode {
			case "search":
				avail["search"] = true
			case "tv-search":
				avail["tv-search"] = true
			case "movie-search":
				avail["movie-search"] = true
			case "music-search", "audio-search": // corpus uses "audio-search" (e.g. some local definition)
				avail["music-search"] = true
			case "book-search":
				avail["book-search"] = true
			}
		}
	}
	return avail
}
