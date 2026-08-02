package store

import (
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"github.com/Kcchouette/gowlarr/internal/model"
)

// benchOpen opens a fresh store on a temp file (migrations run automatically).
func benchOpen(b *testing.B) *Store {
	st, err := Open(filepath.Join(b.TempDir(), "bench.db"))
	if err != nil {
		b.Fatal(err)
	}
	b.Cleanup(func() { _ = st.Close() })
	return st
}

// benchReleases builds n ReleaseInfo records for persistence benchmarks.
func benchReleases(n int) []model.ReleaseInfo {
	results := make([]model.ReleaseInfo, 0, n)
	for i := 0; i < n; i++ {
		results = append(results, model.ReleaseInfo{
			IndexerID:    "bench",
			IndexerName:  "Bench",
			Title:        fmt.Sprintf("Release %d", i),
			DownloadLink: fmt.Sprintf("https://example.invalid/dl/%d.torrent", i),
			Size:         1048576,
			PublishDate:  time.Date(2024, 1, 1, 0, 0, i%60, 0, time.UTC),
			Seeders:      i,
			Categories:   []int{2000, 2010},
			Protocol:     model.ProtocolTorrent,
		})
	}
	return results
}

// BenchmarkSaveResults_500 measures the batch insert path (one transaction,
// one prepared statement, per-item json.Marshal + RFC3339 formatting).
// Rows are cleared inside StopTimer on every iteration: auto-increment IDs
// would otherwise accumulate and skew timing as the table grows.
func BenchmarkSaveResults_500(b *testing.B) {
	st := benchOpen(b)
	results := benchReleases(500)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		b.StopTimer()
		if _, err := st.db.Exec(`DELETE FROM search_results`); err != nil {
			b.Fatal(err)
		}
		b.StartTimer()
		saved, err := st.SaveResults(results, time.Hour)
		if err != nil {
			b.Fatal(err)
		}
		if len(saved) != 500 {
			b.Fatalf("expected 500 saved, got %d", len(saved))
		}
	}
}

// BenchmarkGetResult_1k measures reads (RFC3339 parse + json.Unmarshal per
// row). Rows are seeded once with a FUTURE expiry so GetResult exercises the
// full read path instead of returning early as expired.
func BenchmarkGetResult_1k(b *testing.B) {
	st := benchOpen(b)
	results := benchReleases(1000)
	if _, err := st.SaveResults(results, time.Hour); err != nil {
		b.Fatal(err)
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := st.GetResult(int64(i%1000 + 1)); err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkPurgeExpired_1k measures the expired-row DELETE. The operation is
// destructive, so expired rows are re-seeded inside StopTimer on every
// iteration (otherwise iterations after the first would measure an empty
// table).
func BenchmarkPurgeExpired_1k(b *testing.B) {
	st := benchOpen(b)
	expired := benchReleases(1000)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		b.StopTimer()
		if _, err := st.SaveResults(expired, -time.Hour); err != nil {
			b.Fatal(err)
		}
		b.StartTimer()
		if err := st.PurgeExpiredResults(); err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkGetDefinitionYAMLFallback_HitV11 measures the happy path of the
// versioned fallback: a v11 definition exists, so exactly one SELECT runs.
func BenchmarkGetDefinitionYAMLFallback_HitV11(b *testing.B) {
	st := benchOpen(b)
	if err := st.SaveDefinition("benchdef", "v11", "sha", benchDefinitionYAML()); err != nil {
		b.Fatal(err)
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		raw, _, err := st.GetDefinitionYAMLFallback("benchdef")
		if err != nil {
			b.Fatal(err)
		}
		if raw == "" {
			b.Fatal("empty raw yaml")
		}
	}
}

// BenchmarkGetDefinitionYAMLFallback_MissAll measures the worst case of the
// versioned fallback: no version exists, so all 11 versioned SELECTs run.
func BenchmarkGetDefinitionYAMLFallback_MissAll(b *testing.B) {
	st := benchOpen(b)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, _, err := st.GetDefinitionYAMLFallback("missingdef"); err == nil {
			b.Fatal("expected ErrDefinitionNotFound")
		}
	}
}

// BenchmarkCookiePersist_Roundtrip measures the SQLite-backed cookie
// persister (Store + Load of a ~40-cookie blob) via the adapter used by
// cardigann-go.
func BenchmarkCookiePersist_Roundtrip(b *testing.B) {
	st := benchOpen(b)
	adapter := NewCookiePersisterAdapter(st, nil)
	data := `[{"url":"https://a.invalid/","cookies":[{`
	for i := 0; i < 40; i++ {
		data += fmt.Sprintf(`"c%d":"v%d",`, i, i)
	}
	data += `}]}]`
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if err := adapter.Store("bench", data); err != nil {
			b.Fatal(err)
		}
		if got, err := adapter.Load("bench"); err != nil {
			b.Fatal(err)
		} else if got != data {
			b.Fatal("cookie roundtrip mismatch")
		}
	}
}

// benchDefinitionYAML is a minimal valid Cardigann definition used to seed
// the definition cache.
func benchDefinitionYAML() string {
	return `id: benchdef
name: BenchDef
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
`
}
