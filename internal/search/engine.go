package search

import (
	"context"
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/Kcchouette/gowlarr/internal/model"
)

// Engine orchestre l'exécution en parallèle de plusieurs Provider et agrège
// les résultats. Chaque indexeur est borné par un timeout individuel pour
// qu'un indexeur lent ou en panne ne bloque pas les autres (retour d'erreur
// partielle visible par source, cf. plan Slice D).
type Engine struct {
	Providers      []Provider
	PerProviderTTL time.Duration // timeout appliqué à chaque provider.
}

// NewEngine construit un moteur de recherche avec un timeout par défaut de 20s
// par indexeur si aucun n'est spécifié.
func NewEngine(providers []Provider) *Engine {
	return &Engine{Providers: providers, PerProviderTTL: 20 * time.Second}
}

// ProviderError associe une erreur à l'indexeur qui l'a produite, pour que
// l'utilisateur voie précisément quel indexeur a échoué sans faire échouer
// toute la recherche.
type ProviderError struct {
	IndexerID string
	Err       error
}

func (e *ProviderError) Error() string {
	return fmt.Sprintf("indexer %s: %v", e.IndexerID, e.Err)
}

// Result est le résultat agrégé d'une recherche multi-indexeurs.
type Result struct {
	Releases []model.ReleaseInfo
	Errors   []*ProviderError
}

// Search interroge tous les providers en parallèle et agrège les résultats,
// triés par date de publication décroissante puis par seeders décroissants.
func (e *Engine) Search(ctx context.Context, q Query) Result {
	var (
		mu      sync.Mutex
		wg      sync.WaitGroup
		result  Result
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
