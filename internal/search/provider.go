// Package search définit le contrat commun de recherche, implémenté aussi
// bien par les providers Cardigann (torrent/usenet) que par le client
// Newznab générique natif — le moteur de recherche/CLI ne doit jamais
// connaître les détails d'implémentation d'un provider concret.
package search

import (
	"context"

	"github.com/Kcchouette/gowlarr/internal/model"
)

// Query représente une requête de recherche unifiée, indépendante du provider.
type Query struct {
	Keywords   string
	Categories []int
	SearchType string // "search" | "tvsearch" | "movie" | "music" | "book"
	Season     int    // pour tvsearch
	Episode    int    // pour tvsearch
	IMDbID     string // pour movie/tvsearch
	TMDBID     string // pour movie
}

// Provider est le contrat que doit implémenter tout moyen de recherche
// d'indexeur (Cardigann torrent, Cardigann usenet, Newznab générique natif).
type Provider interface {
	// ID retourne l'identifiant stable de l'indexeur (ex: "1337x", "nzbgeek").
	ID() string
	// Name retourne le nom lisible de l'indexeur.
	Name() string
	// Protocol retourne le protocole de distribution de cet indexeur.
	Protocol() model.Protocol
	// Search exécute la recherche et retourne des résultats normalisés.
	Search(ctx context.Context, q Query) ([]model.ReleaseInfo, error)
}
