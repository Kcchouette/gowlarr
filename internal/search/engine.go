package search

import (
	"context"
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/Kcchouette/gowlarr/internal/model"
)

// Engine orchestrates parallel execution of multiple Providers and aggregates
// results. Each indexer is bounded by an individual timeout so that a slow
// or failing indexer does not block the others (partial error return visible
// per source).
type Engine struct {
	Providers      []Provider
	PerProviderTTL time.Duration // timeout applied to each provider.
}

// NewEngine builds a search engine with a default 20s timeout per indexer
// if none is specified.
func NewEngine(providers []Provider) *Engine {
	return &Engine{Providers: providers, PerProviderTTL: 20 * time.Second}
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

// Search queries all providers in parallel and aggregates results,
// sorted by publication date descending then by seeders descending.
func (e *Engine) Search(ctx context.Context, q Query) Result {
	engineTimeout := e.PerProviderTTL * 2
	ctx, cancel := context.WithTimeout(ctx, engineTimeout)
	defer cancel()

	// This "TTL * 2" margin is only a cooperative cancellation grace
	// period for providers that respect their context. It is not a
	// hard deadline guarantee: if a provider ignores its context, its
	// goroutine can still block wg.Wait() indefinitely.

	var (
		mu     sync.Mutex
		wg     sync.WaitGroup
		result Result
	)

	for _, p := range e.Providers {
		wg.Add(1)
		go func(p Provider) {
			defer wg.Done()

			pctx, cancel := context.WithTimeout(ctx, e.PerProviderTTL)
			defer cancel()

			releases, err := p.Search(pctx, q)

			mu.Lock()
			defer mu.Unlock()
			if err != nil {
				result.Errors = append(result.Errors, &ProviderError{IndexerID: p.ID(), Err: err})
				return
			}
			result.Releases = append(result.Releases, releases...)
		}(p)
	}

	wg.Wait()

	sort.Slice(result.Releases, func(i, j int) bool {
		if !result.Releases[i].PublishDate.Equal(result.Releases[j].PublishDate) {
			return result.Releases[i].PublishDate.After(result.Releases[j].PublishDate)
		}
		return result.Releases[i].Seeders > result.Releases[j].Seeders
	})

	return result
}
