package search

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/Kcchouette/gowlarr/internal/model"
)

// fakeProvider is an in-memory test search.Provider (no network),
// allowing precise control over latency/errors/results.
type fakeProvider struct {
	id       string
	delay    time.Duration
	releases []model.ReleaseInfo
	err      error
}

func (f *fakeProvider) ID() string               { return f.id }
func (f *fakeProvider) Name() string             { return f.id }
func (f *fakeProvider) Protocol() model.Protocol { return model.ProtocolTorrent }
func (f *fakeProvider) Search(ctx context.Context, q Query) ([]model.ReleaseInfo, error) {
	select {
	case <-time.After(f.delay):
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	if f.err != nil {
		return nil, f.err
	}
	return f.releases, nil
}

func TestEngine_AggregatesAndSortsResults(t *testing.T) {
	t1 := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	t2 := time.Date(2024, 6, 1, 0, 0, 0, 0, time.UTC)

	p1 := &fakeProvider{id: "p1", releases: []model.ReleaseInfo{
		{Title: "Old", PublishDate: t1, Seeders: 100},
	}}
	p2 := &fakeProvider{id: "p2", releases: []model.ReleaseInfo{
		{Title: "New", PublishDate: t2, Seeders: 1},
	}}

	engine := NewEngine([]Provider{p1, p2})
	result := engine.Search(context.Background(), Query{Keywords: "x"})

	if len(result.Errors) != 0 {
		t.Fatalf("expected no errors, got %v", result.Errors)
	}
	if len(result.Releases) != 2 {
		t.Fatalf("expected 2 releases, got %d", len(result.Releases))
	}
	if result.Releases[0].Title != "New" {
		t.Errorf("expected 'New' (most recent) first, got %q", result.Releases[0].Title)
	}
}

func TestEngine_SortsBySeedersWhenSameDate(t *testing.T) {
	date := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	p := &fakeProvider{id: "p", releases: []model.ReleaseInfo{
		{Title: "LowSeeders", PublishDate: date, Seeders: 5},
		{Title: "HighSeeders", PublishDate: date, Seeders: 50},
	}}

	engine := NewEngine([]Provider{p})
	result := engine.Search(context.Background(), Query{Keywords: "x"})

	if result.Releases[0].Title != "HighSeeders" {
		t.Errorf("expected 'HighSeeders' first, got %q", result.Releases[0].Title)
	}
}

func TestEngine_PartialFailureIsVisiblePerProvider(t *testing.T) {
	good := &fakeProvider{id: "good", releases: []model.ReleaseInfo{{Title: "OK"}}}
	bad := &fakeProvider{id: "bad", err: errors.New("connection refused")}

	engine := NewEngine([]Provider{good, bad})
	result := engine.Search(context.Background(), Query{Keywords: "x"})

	if len(result.Releases) != 1 {
		t.Fatalf("expected 1 successful release despite partial failure, got %d", len(result.Releases))
	}
	if len(result.Errors) != 1 {
		t.Fatalf("expected 1 provider error, got %d", len(result.Errors))
	}
	if result.Errors[0].IndexerID != "bad" {
		t.Errorf("expected error attributed to 'bad', got %q", result.Errors[0].IndexerID)
	}
}

func TestEngine_RunsProvidersInParallel(t *testing.T) {
	// 3 providers with 200ms delay each: in series it would take ~600ms,
	// in parallel it should stay close to 200ms.
	providers := []Provider{
		&fakeProvider{id: "a", delay: 200 * time.Millisecond},
		&fakeProvider{id: "b", delay: 200 * time.Millisecond},
		&fakeProvider{id: "c", delay: 200 * time.Millisecond},
	}
	engine := NewEngine(providers)

	start := time.Now()
	engine.Search(context.Background(), Query{Keywords: "x"})
	elapsed := time.Since(start)

	if elapsed > 500*time.Millisecond {
		t.Errorf("expected parallel execution (~200ms), took %v", elapsed)
	}
}

func TestEngine_PerProviderTimeout(t *testing.T) {
	slow := &fakeProvider{id: "slow", delay: 500 * time.Millisecond}
	engine := NewEngine([]Provider{slow})
	engine.PerProviderTTL = 50 * time.Millisecond

	result := engine.Search(context.Background(), Query{Keywords: "x"})
	if len(result.Errors) != 1 {
		t.Fatalf("expected timeout error, got errors=%v releases=%v", result.Errors, result.Releases)
	}
}

// hangingProvider ignores its context and never returns, simulating a
// provider whose underlying HTTP stack is stuck.
type hangingProvider struct{ id string }

func (h *hangingProvider) ID() string               { return h.id }
func (h *hangingProvider) Name() string             { return h.id }
func (h *hangingProvider) Protocol() model.Protocol { return model.ProtocolTorrent }
func (h *hangingProvider) Search(context.Context, Query) ([]model.ReleaseInfo, error) {
	select {} // never returns, ignores ctx
}

func TestEngine_ReturnsOnDeadline_EvenIfProviderHangs(t *testing.T) {
	fast := &fakeProvider{id: "fast", releases: []model.ReleaseInfo{{Title: "OK"}}}
	engine := NewEngine([]Provider{fast, &hangingProvider{id: "hung"}})
	engine.PerProviderTTL = 100 * time.Millisecond // engine deadline = 200ms

	start := time.Now()
	result := engine.Search(context.Background(), Query{Keywords: "x"})
	elapsed := time.Since(start)

	if elapsed > time.Second {
		t.Fatalf("Search did not return on deadline (took %v)", elapsed)
	}
	if len(result.Releases) != 1 || result.Releases[0].Title != "OK" {
		t.Errorf("expected the fast provider's release, got %+v", result.Releases)
	}
	// The hung provider produced no error (it never returned); that is the
	// documented tradeoff of a bounded aggregation.
}

func TestEngine_CapsResultsPerProvider(t *testing.T) {
	releases := make([]model.ReleaseInfo, 0, 150)
	for i := 0; i < 150; i++ {
		releases = append(releases, model.ReleaseInfo{
			Title:   fmt.Sprintf("R%d", i),
			InfoHash: fmt.Sprintf("%040d", i),
		})
	}
	engine := NewEngine([]Provider{&fakeProvider{id: "p", releases: releases}})

	result := engine.Search(context.Background(), Query{Keywords: "x"})
	if len(result.Releases) != defaultMaxResultsPerProvider {
		t.Fatalf("expected %d capped releases, got %d", defaultMaxResultsPerProvider, len(result.Releases))
	}
}

func TestEngine_DedupsByInfoHash(t *testing.T) {
	dup := model.ReleaseInfo{Title: "Dup", InfoHash: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"}
	p1 := &fakeProvider{id: "p1", releases: []model.ReleaseInfo{dup}}
	p2 := &fakeProvider{id: "p2", releases: []model.ReleaseInfo{dup}}
	engine := NewEngine([]Provider{p1, p2})

	result := engine.Search(context.Background(), Query{Keywords: "x"})
	if len(result.Releases) != 1 {
		t.Fatalf("expected 1 release after infohash dedup, got %d", len(result.Releases))
	}
}

func TestEngine_DedupsByDownloadLink(t *testing.T) {
	dup := model.ReleaseInfo{Title: "Dup", DownloadLink: "https://example.invalid/dl/1.torrent"}
	p1 := &fakeProvider{id: "p1", releases: []model.ReleaseInfo{dup}}
	p2 := &fakeProvider{id: "p2", releases: []model.ReleaseInfo{dup}}
	engine := NewEngine([]Provider{p1, p2})

	result := engine.Search(context.Background(), Query{Keywords: "x"})
	if len(result.Releases) != 1 {
		t.Fatalf("expected 1 release after link dedup, got %d", len(result.Releases))
	}
}

func TestEngine_KeepsUnkeyableDuplicates(t *testing.T) {
	// Releases with neither InfoHash nor DownloadLink cannot be deduped and
	// must both be kept.
	rel := model.ReleaseInfo{Title: "NoKey"}
	p1 := &fakeProvider{id: "p1", releases: []model.ReleaseInfo{rel}}
	p2 := &fakeProvider{id: "p2", releases: []model.ReleaseInfo{rel}}
	engine := NewEngine([]Provider{p1, p2})

	result := engine.Search(context.Background(), Query{Keywords: "x"})
	if len(result.Releases) != 2 {
		t.Fatalf("expected both unkeyable releases kept, got %d", len(result.Releases))
	}
}
