// Package model contient les types partagés par tout Gowlarr (résultats de
// recherche normalisés, protocoles, etc.), indépendants de tout indexeur
// concret ou moteur de parsing.
package model

import "time"

// Protocol distingue le protocole de distribution d'une release.
type Protocol string

const (
	ProtocolTorrent Protocol = "torrent"
	ProtocolUsenet  Protocol = "usenet"
)

// ReleaseInfo est le résultat normalisé d'une recherche, quel que soit
// l'indexeur ou le moteur d'origine (Cardigann torrent, Cardigann usenet,
// ou Newznab générique natif).
type ReleaseInfo struct {
	// ID est un identifiant stable, unique dans la session de recherche
	// courante, utilisé par la commande `download <id>`.
	ID int64

	Title        string
	Details      string // URL de la page de détails, si disponible.
	DownloadLink string // Lien direct (magnet, .torrent, .nzb) ou lien à résoudre.
	InfoHash     string

	Size       int64 // en octets, 0 si inconnu.
	PublishDate time.Time

	Seeders  int
	Peers    int
	Grabs    int

	Categories []int

	Protocol Protocol

	IndexerID   string
	IndexerName string
}
