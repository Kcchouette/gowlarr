package search

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/Kcchouette/gowlarr/internal/model"
)

// BenchmarkEngineSearch_N10 measures the full engine aggregation path:
// goroutine fan-out per provider, mutex-protected append, wg.Wait, and the
// final sort. 10 providers x 20 releases, zero latency (fakeProvider).
func BenchmarkEngineSearch_N10(b *testing.B) {
	providers := make([]Provider, 0, 10)
	for i := 0; i < 10; i++ {
		releases := make([]model.ReleaseInfo, 0, 20)
		for j := 0; j < 20; j++ {
			releases = append(releases, model.ReleaseInfo{
				Title:       fmt.Sprintf("Release %d-%d", i, j),
				PublishDate: time.Date(2024, 1, 1, 0, 0, j, 0, time.UTC),
				Seeders:     j,
			})
		}
		providers = append(providers, &fakeProvider{id: fmt.Sprintf("p%d", i), releases: releases})
	}
	engine := NewEngine(providers)
	engine.MaxResultsPerProvider = 20 // keep the 200-release semantics (10x20); the bench measures aggregation, not the cap
	ctx := context.Background()
	q := Query{Keywords: "ubuntu"}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		res := engine.Search(ctx, q)
		if len(res.Releases) != 200 {
			b.Fatalf("expected 200 releases, got %d", len(res.Releases))
		}
		if len(res.Errors) != 0 {
			b.Fatalf("expected no errors, got %v", res.Errors)
		}
	}
}
