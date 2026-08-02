package store

import (
	"testing"
	"time"

	"github.com/Kcchouette/gowlarr/internal/model"
)

func TestResults_SaveAndGetRoundTrip(t *testing.T) {
	st := openTestStore(t)

	results := []model.ReleaseInfo{
		{
			Title:        "Test Torrent",
			Details:      "https://example.invalid/details",
			DownloadLink: "magnet:?xt=urn:btih:abc123",
			InfoHash:     "abc123",
			Size:         1024 * 1024 * 500,
			PublishDate:  time.Now().UTC().Add(-1 * time.Hour),
			Seeders:      100,
			Peers:        50,
			Grabs:        10,
			Categories:   []int{2000, 2010},
			Protocol:     model.ProtocolTorrent,
			IndexerID:    "1337x",
			IndexerName:  "1337x",
		},
	}

	saved, err := st.SaveResults(results, 30*time.Minute)
	if err != nil {
		t.Fatalf("SaveResults: %v", err)
	}
	if len(saved) != 1 {
		t.Fatalf("expected 1 saved result, got %d", len(saved))
	}
	if saved[0].ID == 0 {
		t.Fatal("expected non-zero ID after save")
	}

	got, err := st.GetResult(saved[0].ID)
	if err != nil {
		t.Fatalf("GetResult: %v", err)
	}
	if got.Title != "Test Torrent" {
		t.Errorf("Title = %q, want %q", got.Title, "Test Torrent")
	}
	if got.DownloadLink != "magnet:?xt=urn:btih:abc123" {
		t.Errorf("DownloadLink = %q, want magnet link", got.DownloadLink)
	}
	if got.Size != 1024*1024*500 {
		t.Errorf("Size = %d, want %d", got.Size, 1024*1024*500)
	}
	if got.Seeders != 100 {
		t.Errorf("Seeders = %d, want 100", got.Seeders)
	}
	if got.Protocol != model.ProtocolTorrent {
		t.Errorf("Protocol = %q, want %q", got.Protocol, model.ProtocolTorrent)
	}
	if len(got.Categories) != 2 || got.Categories[0] != 2000 || got.Categories[1] != 2010 {
		t.Errorf("Categories = %v, want [2000 2010]", got.Categories)
	}
}

func TestResults_SaveAndGetDDLFields(t *testing.T) {
	st := openTestStore(t)

	results := []model.ReleaseInfo{
		{
			Title:        "Bleach 01",
			DownloadLink: "https://www.uptobox.com/abc123",
			Protocol:     model.ProtocolDDL,
			IndexerID:    "japanfan",
			IndexerName:  "JapanFan",
			Hosts:        []string{"uptobox.com"},
			Unlocked:     true,
		},
		{
			Title:        "Stream Show",
			DownloadLink: "https://example.invalid/ep/1",
			StreamURL:    "https://cdn.example.invalid/stream.mp4",
			Protocol:     model.ProtocolStreaming,
			IndexerID:    "streamidx",
			IndexerName:  "StreamIdx",
		},
	}

	saved, err := st.SaveResults(results, 30*time.Minute)
	if err != nil {
		t.Fatalf("SaveResults: %v", err)
	}
	if len(saved) != 2 {
		t.Fatalf("expected 2 saved results, got %d", len(saved))
	}

	ddl, err := st.GetResult(saved[0].ID)
	if err != nil {
		t.Fatalf("GetResult(ddl): %v", err)
	}
	if ddl.Protocol != model.ProtocolDDL {
		t.Errorf("Protocol = %q, want ddl", ddl.Protocol)
	}
	if len(ddl.Hosts) != 1 || ddl.Hosts[0] != "uptobox.com" {
		t.Errorf("Hosts = %v, want [uptobox.com]", ddl.Hosts)
	}
	if !ddl.Unlocked {
		t.Error("Unlocked = false, want true")
	}

	stream, err := st.GetResult(saved[1].ID)
	if err != nil {
		t.Fatalf("GetResult(streaming): %v", err)
	}
	if stream.Protocol != model.ProtocolStreaming {
		t.Errorf("Protocol = %q, want streaming", stream.Protocol)
	}
	if stream.StreamURL != "https://cdn.example.invalid/stream.mp4" {
		t.Errorf("StreamURL = %q, want the stream URL", stream.StreamURL)
	}
	if stream.Unlocked {
		t.Error("Unlocked = true, want false for a plain streaming row")
	}
}

func TestResults_GetResultLegacyNullHosts(t *testing.T) {
	// Rows inserted before migration 0004 have a NULL hosts column:
	// GetResult must not fail and must yield an empty Hosts slice.
	st := openTestStore(t)

	_, err := st.db.Exec(`INSERT INTO search_results
		(indexer_id, indexer_name, title, details_url, download_link, info_hash,
		 size_bytes, publish_date, seeders, peers, grabs, categories, protocol,
		 created_at, expires_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		"legacy", "Legacy", "Old Row", "https://example.invalid/details", "https://example.invalid/dl",
		"hash123", 1024, time.Now().UTC().Format(time.RFC3339), 1, 0, 0, "[]", "torrent",
		time.Now().UTC().Format(time.RFC3339), time.Now().UTC().Add(time.Hour).Format(time.RFC3339))
	if err != nil {
		t.Fatalf("inserting legacy row: %v", err)
	}

	var id int64
	if err := st.db.QueryRow(`SELECT id FROM search_results WHERE title = ?`, "Old Row").Scan(&id); err != nil {
		t.Fatalf("reading legacy row id: %v", err)
	}

	got, err := st.GetResult(id)
	if err != nil {
		t.Fatalf("GetResult(legacy): %v", err)
	}
	if len(got.Hosts) != 0 {
		t.Errorf("Hosts = %v, want empty for legacy row", got.Hosts)
	}
	if !got.Unlocked {
		t.Error("Unlocked = false, want default true for legacy row")
	}
}

func TestResults_Get_NotFound(t *testing.T) {
	st := openTestStore(t)

	_, err := st.GetResult(999)
	if err == nil {
		t.Fatal("expected error for nonexistent result")
	}
}

func TestResults_Get_Expired(t *testing.T) {
	st := openTestStore(t)

	results := []model.ReleaseInfo{
		{
			Title:        "Expired",
			DownloadLink: "magnet:?xt=urn:btih:expired",
			Size:         100,
			PublishDate:  time.Now().UTC(),
			Protocol:     model.ProtocolTorrent,
			IndexerID:    "test",
			IndexerName:  "test",
		},
	}

	// Save with a very short TTL (1 nanosecond) so it expires immediately
	saved, err := st.SaveResults(results, 1*time.Nanosecond)
	if err != nil {
		t.Fatalf("SaveResults: %v", err)
	}

	// Wait briefly to ensure expiry
	time.Sleep(10 * time.Millisecond)

	_, err = st.GetResult(saved[0].ID)
	if err == nil {
		t.Fatal("expected error for expired result")
	}
}

func TestResults_PurgeExpired(t *testing.T) {
	st := openTestStore(t)

	// Save one result with short TTL
	short := []model.ReleaseInfo{
		{Title: "Short-lived", DownloadLink: "magnet:?x=1", Size: 1, PublishDate: time.Now().UTC(), Protocol: model.ProtocolTorrent, IndexerID: "a", IndexerName: "a"},
	}
	savedShort, _ := st.SaveResults(short, 1*time.Nanosecond)

	// Save one result with long TTL
	long := []model.ReleaseInfo{
		{Title: "Long-lived", DownloadLink: "magnet:?x=2", Size: 2, PublishDate: time.Now().UTC(), Protocol: model.ProtocolTorrent, IndexerID: "b", IndexerName: "b"},
	}
	savedLong, _ := st.SaveResults(long, 1*time.Hour)

	time.Sleep(10 * time.Millisecond)

	if err := st.PurgeExpiredResults(); err != nil {
		t.Fatalf("PurgeExpiredResults: %v", err)
	}

	// Short-lived should be gone
	_, err := st.GetResult(savedShort[0].ID)
	if err == nil {
		t.Error("expected short-lived result to be purged")
	}

	// Long-lived should still exist
	_, err = st.GetResult(savedLong[0].ID)
	if err != nil {
		t.Errorf("long-lived result should still exist: %v", err)
	}
}

func TestResults_MultipleResults(t *testing.T) {
	st := openTestStore(t)

	results := []model.ReleaseInfo{
		{Title: "First", DownloadLink: "m1", Size: 1, PublishDate: time.Now().UTC(), Protocol: model.ProtocolTorrent, IndexerID: "a", IndexerName: "a"},
		{Title: "Second", DownloadLink: "m2", Size: 2, PublishDate: time.Now().UTC(), Protocol: model.ProtocolUsenet, IndexerID: "b", IndexerName: "b"},
		{Title: "Third", DownloadLink: "m3", Size: 3, PublishDate: time.Now().UTC(), Protocol: model.ProtocolTorrent, IndexerID: "c", IndexerName: "c"},
	}

	saved, err := st.SaveResults(results, 30*time.Minute)
	if err != nil {
		t.Fatalf("SaveResults: %v", err)
	}
	if len(saved) != 3 {
		t.Fatalf("expected 3 saved, got %d", len(saved))
	}

	// Each should have a unique ID
	ids := make(map[int64]bool)
	for _, r := range saved {
		if ids[r.ID] {
			t.Errorf("duplicate ID %d", r.ID)
		}
		ids[r.ID] = true
	}
}
