package service

import (
	"fmt"
	"path/filepath"
	"testing"

	"github.com/Kcchouette/gowlarr/internal/config"
	"github.com/Kcchouette/gowlarr/internal/store"
)

// benchIndexerDefYAML is a minimal valid Cardigann definition (with links,
// required by BuildConfiguredProviders) for a given indexer id.
func benchIndexerDefYAML(id string) string {
	return fmt.Sprintf(`id: %s
name: Bench %s
links:
  - https://example.invalid/
search:
  paths:
    - path: /search
      response:
        type: html
  rows:
    selector: table.results tr
  fields:
    title:
      selector: a.title
`, id, id)
}

// BenchmarkBuildConfiguredProviders_10 measures the full per-search provider
// stack rebuild on the CLI path: ListIndexerConfigs + per-indexer
// GetDefinitionYAMLFallback (v11 hit, 1 SELECT each) + definition.Parse +
// httpclient.New (incl. cookie-jar SQLite load). 10 configured indexers.
func BenchmarkBuildConfiguredProviders_10(b *testing.B) {
	st, err := store.Open(filepath.Join(b.TempDir(), "bench.db"))
	if err != nil {
		b.Fatal(err)
	}
	b.Cleanup(func() { _ = st.Close() })

	for i := 0; i < 10; i++ {
		id := fmt.Sprintf("bench%d", i)
		if err := st.SaveDefinition(id, "v11", "sha", benchIndexerDefYAML(id)); err != nil {
			b.Fatal(err)
		}
		if err := st.SaveIndexerConfig(store.IndexerConfig{
			ID:           "cfg" + fmt.Sprint(i),
			DefinitionID: id,
			Protocol:     "torrent",
			Enabled:      true,
			Settings:     map[string]string{},
		}, nil); err != nil {
			b.Fatal(err)
		}
	}

	cfg := config.Config{}

	// Warm-up: first call populates any one-shot state.
	if providers, err := BuildConfiguredProviders(st, cfg); err != nil {
		b.Fatal(err)
	} else if len(providers) != 10 {
		b.Fatalf("warm-up: expected 10 providers, got %d", len(providers))
	}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		providers, err := BuildConfiguredProviders(st, cfg)
		if err != nil {
			b.Fatal(err)
		}
		if len(providers) != 10 {
			b.Fatalf("expected 10 providers, got %d", len(providers))
		}
	}
}
