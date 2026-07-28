package search

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Kcchouette/gowlarr/internal/model"
)

// fakeProvider est un search.Provider de test entièrement en mémoire (pas de
// réseau), permettant de contrôler précisément latence/erreurs/résultats.
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
	// 3 providers avec 200ms de délai chacun : en série ça prendrait ~600ms,
	// en parallèle ça doit rester proche de 200ms.
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
