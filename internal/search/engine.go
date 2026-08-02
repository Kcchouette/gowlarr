package search

import (
	"context"
	"fmt"
	"sort"
	"time"

	"github.com/Kcchouette/gowlarr/internal/model"
)

// defaultMaxResultsPerProvider caps how many releases a single provider can
// contribute to a search when no explicit limit is configured.
const defaultMaxResultsPerProvider = 10

// Engine orchestrates parallel execution of multiple Providers and aggregates
// results. Each indexer is bounded by an individual timeout so that a slow
// or failing indexer does not block the others (partial error return visible
// per source).
type Engine struct {
	Providers []Provider
	// PerProviderTTL is the timeout applied to each provider (default 20s).
	PerProviderTTL time.Duration
	// MaxResultsPerProvider caps the number of releases a single provider can
	// contribute to a search (0 = defaultMaxResultsPerProvider). Use a high
	// value to effectively disable the cap.
	MaxResultsPerProvider int
}

// NewEngine builds a search engine with a default 20s timeout per indexer
// and a default 10-results-per-indexer cap.
func NewEngine(providers []Provider) *Engine {
	return &Engine{Providers: providers, PerProviderTTL: 20 * time.Second, MaxResultsPerProvider: defaultMaxResultsPerProvider}
}

// ProviderError associates an error with the indexer that produced it, so
// the user can see exactly which indexer failed without failing the
// entire search.
type ProviderError struct {
	IndexerID string
	Err       error
}

func (e *ProviderError) Error() string {
	return fmt.Sprintf("indexer %s: %v", e.IndexerID, e.Err)
}

// Result is the aggregated result of a multi-indexer search.
type Result struct {
	Releases []model.ReleaseInfo
	Errors   []*ProviderError
}

// outcome carries one provider's contribution from its goroutine to the
// collecting loop.
type outcome struct {
	idx      int
	releases []model.ReleaseInfo
	err      error
}

// Search queries all providers in parallel and aggregates results, sorted by
// publication date descending then by seeders descending.
//
// Search always returns before the engine deadline (PerProviderTTL * 2), even
// if a provider ignores its context: results are collected from a buffered
// channel with a select on ctx.Done, so a hung provider goroutine can never
// block the caller (it may keep running in the background, but Search has
// already returned). Duplicates (same InfoHash, else same DownloadLink) are
// removed across providers.
func (e *Engine) Search(ctx context.Context, q Query) Result {
	engineTimeout := e.PerProviderTTL * 2
	ctx, cancel := context.WithTimeout(ctx, engineTimeout)
	defer cancel()

	maxPerProvider := e.MaxResultsPerProvider
	if maxPerProvider <= 0 {
		maxPerProvider = defaultMaxResultsPerProvider
	}

	// Buffered with one slot per provider: senders never block, even when
	// Search bails out on the deadline before consuming everything.
	outcomes := make(chan outcome, len(e.Providers))
	for i, p := range e.Providers {
		go func(i int, p Provider) {
			pctx, cancel := context.WithTimeout(ctx, e.PerProviderTTL)
			defer cancel()

			releases, err := p.Search(pctx, q)
			if len(releases) > maxPerProvider {
				releases = releases[:maxPerProvider]
			}
			outcomes <- outcome{idx: i, releases: releases, err: err}
		}(i, p)
	}

	var result Result
	completed := 0
	for completed < len(e.Providers) {
		select {
		case o := <-outcomes:
			completed++
			if o.err != nil {
				result.Errors = append(result.Errors, &ProviderError{IndexerID: e.Providers[o.idx].ID(), Err: o.err})
				continue
			}
			result.Releases = append(result.Releases, o.releases...)
		case <-ctx.Done():
			// Deadline reached: return what we have. The buffered channel
			// lets the remaining goroutines finish their send without
			// blocking, and their results are simply not collected.
			completed = len(e.Providers)
		}
	}

	result.Releases = dedupReleases(result.Releases)

	sort.Slice(result.Releases, func(i, j int) bool {
		if !result.Releases[i].PublishDate.Equal(result.Releases[j].PublishDate) {
			return result.Releases[i].PublishDate.After(result.Releases[j].PublishDate)
		}
		return result.Releases[i].Seeders > result.Releases[j].Seeders
	})

	return result
}

// dedupReleases removes duplicates across providers: a release is a duplicate
// if its InfoHash (or, when empty, its DownloadLink) was already seen.
// Releases with neither key are always kept.
func dedupReleases(releases []model.ReleaseInfo) []model.ReleaseInfo {
	seen := make(map[string]struct{}, len(releases))
	deduped := releases[:0]
	for _, r := range releases {
		key := r.InfoHash
		if key == "" {
			key = r.DownloadLink
		}
		if key != "" {
			if _, ok := seen[key]; ok {
				continue
			}
			seen[key] = struct{}{}
		}
		deduped = append(deduped, r)
	}
	return deduped
}
