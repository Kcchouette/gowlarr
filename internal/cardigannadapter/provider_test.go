package cardigannadapter

import (
	"testing"
	"time"

	cardigannrelease "github.com/Kcchouette/cardigann-go/release"
	"github.com/Kcchouette/gowlarr/internal/model"
)

func TestToReleaseInfo_Torrent(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	input := cardigannrelease.Release{
		Title:        "Ubuntu 24.04 LTS",
		Details:      "https://example.invalid/details",
		DownloadLink: "magnet:?xt=urn:btih:abc123",
		InfoHash:     "abc123",
		Size:         1024 * 1024 * 4000,
		PublishDate:  now,
		Seeders:      200,
		Peers:        50,
		Grabs:        1000,
		Categories:   []int{2000, 2010},
		Protocol:     cardigannrelease.ProtocolTorrent,
		IndexerID:    "1337x",
		IndexerName:  "1337x",
	}

	got := ToReleaseInfo(input)

	if got.Title != input.Title {
		t.Errorf("Title = %q, want %q", got.Title, input.Title)
	}
	if got.Details != input.Details {
		t.Errorf("Details = %q, want %q", got.Details, input.Details)
	}
	if got.DownloadLink != input.DownloadLink {
		t.Errorf("DownloadLink = %q, want %q", got.DownloadLink, input.DownloadLink)
	}
	if got.InfoHash != input.InfoHash {
		t.Errorf("InfoHash = %q, want %q", got.InfoHash, input.InfoHash)
	}
	if got.Size != input.Size {
		t.Errorf("Size = %d, want %d", got.Size, input.Size)
	}
	if got.Seeders != input.Seeders {
		t.Errorf("Seeders = %d, want %d", got.Seeders, input.Seeders)
	}
	if got.Peers != input.Peers {
		t.Errorf("Peers = %d, want %d", got.Peers, input.Peers)
	}
	if got.Grabs != input.Grabs {
		t.Errorf("Grabs = %d, want %d", got.Grabs, input.Grabs)
	}
	if got.Protocol != model.ProtocolTorrent {
		t.Errorf("Protocol = %q, want %q", got.Protocol, model.ProtocolTorrent)
	}
	if got.IndexerID != input.IndexerID {
		t.Errorf("IndexerID = %q, want %q", got.IndexerID, input.IndexerID)
	}
	if got.IndexerName != input.IndexerName {
		t.Errorf("IndexerName = %q, want %q", got.IndexerName, input.IndexerName)
	}
}

func TestToReleaseInfo_Usenet(t *testing.T) {
	input := cardigannrelease.Release{
		Title:        "NZB Test",
		DownloadLink: "https://example.invalid/test.nzb",
		Protocol:     cardigannrelease.ProtocolUsenet,
		IndexerID:    "nzbindex",
		IndexerName:  "NZBIndex",
	}

	got := ToReleaseInfo(input)

	if got.Protocol != model.ProtocolUsenet {
		t.Errorf("Protocol = %q, want %q", got.Protocol, model.ProtocolUsenet)
	}
}

func TestToReleaseInfos(t *testing.T) {
	inputs := []cardigannrelease.Release{
		{Title: "First", Protocol: cardigannrelease.ProtocolTorrent, IndexerID: "a", IndexerName: "A"},
		{Title: "Second", Protocol: cardigannrelease.ProtocolUsenet, IndexerID: "b", IndexerName: "B"},
		{Title: "Third", Protocol: cardigannrelease.ProtocolTorrent, IndexerID: "c", IndexerName: "C"},
	}

	got := ToReleaseInfos(inputs)

	if len(got) != 3 {
		t.Fatalf("expected 3 results, got %d", len(got))
	}
	if got[0].Title != "First" {
		t.Errorf("got[0].Title = %q, want %q", got[0].Title, "First")
	}
	if got[1].Protocol != model.ProtocolUsenet {
		t.Errorf("got[1].Protocol = %q, want %q", got[1].Protocol, model.ProtocolUsenet)
	}
}

func TestToReleaseInfos_Empty(t *testing.T) {
	got := ToReleaseInfos(nil)
	if len(got) != 0 {
		t.Errorf("expected empty slice, got %d elements", len(got))
	}
}

func TestToReleaseInfo_CategoriesCopied(t *testing.T) {
	input := cardigannrelease.Release{
		Categories: []int{1000, 2000, 3000},
		Protocol:   cardigannrelease.ProtocolTorrent,
		IndexerID:  "test",
	}

	got := ToReleaseInfo(input)

	// Verify categories are copied (not shared reference)
	if len(got.Categories) != 3 {
		t.Fatalf("expected 3 categories, got %d", len(got.Categories))
	}
	got.Categories[0] = 9999
	if input.Categories[0] == 9999 {
		t.Error("modifying output categories should not affect input")
	}
}
