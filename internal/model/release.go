// Package model contains the types shared across all of Gowlarr (normalized
// search results, protocols, etc.), independent of any concrete indexer
// or parsing engine.
package model

import "time"

// Protocol distinguishes the distribution protocol of a release.
type Protocol string

const (
	ProtocolTorrent Protocol = "torrent"
	ProtocolUsenet  Protocol = "usenet"
)

// ReleaseInfo is the normalized result of a search, regardless of the
// originating indexer or engine (Cardigann torrent, Cardigann usenet,
// or native generic Newznab).
type ReleaseInfo struct {
	// ID is a stable identifier, unique within the current search session,
	// used by the `download <id>` command.
	ID int64

	Title        string
	Details      string // URL of the details page, if available.
	DownloadLink string // Direct link (magnet, .torrent, .nzb) or link to resolve.
	InfoHash     string

	Size        int64 // in bytes, 0 if unknown.
	PublishDate time.Time

	Seeders int
	Peers   int
	Grabs   int

	Categories []int

	Protocol Protocol

	IndexerID   string
	IndexerName string
}
