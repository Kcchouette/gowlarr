package server

import (
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Kcchouette/gowlarr/internal/model"
	"github.com/Kcchouette/gowlarr/internal/search"
	"github.com/Kcchouette/gowlarr/internal/store"
)

// fakeProvider implements search.Provider for tests.
type fakeProvider struct {
	id       string
	releases []model.ReleaseInfo
	err      error
}

func (f *fakeProvider) ID() string               { return f.id }
func (f *fakeProvider) Name() string             { return f.id }
func (f *fakeProvider) Protocol() model.Protocol { return model.ProtocolTorrent }
func (f *fakeProvider) Search(_ context.Context, _ search.Query) ([]model.ReleaseInfo, error) {
	return f.releases, f.err
}

func TestHandleCaps(t *testing.T) {
	engine := search.NewEngine(nil)
	srv := New(":0", "test-key", "", engine, nil)

	req := httptest.NewRequest(http.MethodGet, "/api?t=caps&apikey=test-key", nil)
	w := httptest.NewRecorder()

	srv.handleCaps(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	body := w.Body.String()
	if !strings.Contains(body, "<caps>") {
		t.Error("response should contain <caps> element")
	}
	if !strings.Contains(body, `<category id="2000" name="Movies"`) {
		t.Error("response should contain Movies category")
	}
}

func TestHandlePing(t *testing.T) {
	engine := search.NewEngine(nil)
	srv := New(":0", "test-key", "", engine, nil)

	req := httptest.NewRequest(http.MethodGet, "/api?t=serverping&apikey=test-key", nil)
	w := httptest.NewRecorder()

	srv.handlePing(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	body := w.Body.String()
	if !strings.Contains(body, "<title>Gowlarr</title>") {
		t.Error("response should contain Gowlarr title")
	}
}

func TestHandleSearch(t *testing.T) {
	provider := &fakeProvider{
		id: "test-provider",
		releases: []model.ReleaseInfo{
			{
				Title:        "Test Release",
				DownloadLink: "http://example.com/download.torrent",
				Size:         1024,
				Seeders:      10,
			},
		},
	}
	engine := search.NewEngine([]search.Provider{provider})
	srv := New(":0", "test-key", "", engine, nil)

	req := httptest.NewRequest(http.MethodGet, "/api?t=search&q=test&apikey=test-key", nil)
	w := httptest.NewRecorder()

	srv.handleSearch(w, req, "search")

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	body := w.Body.String()
	if !strings.Contains(body, "Test Release") {
		t.Error("response should contain Test Release")
	}
	if !strings.Contains(body, "http://example.com/download.torrent") {
		t.Error("response should contain download link")
	}
}

func TestHandleSearchWithCategories(t *testing.T) {
	provider := &fakeProvider{
		id:       "test-provider",
		releases: []model.ReleaseInfo{},
	}
	engine := search.NewEngine([]search.Provider{provider})
	srv := New(":0", "test-key", "", engine, nil)

	req := httptest.NewRequest(http.MethodGet, "/api?t=search&q=test&cat=2000,3000&apikey=test-key", nil)
	w := httptest.NewRecorder()

	srv.handleSearch(w, req, "search")

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
}

func TestHandleSearchUnsupportedType(t *testing.T) {
	engine := search.NewEngine(nil)
	srv := New(":0", "test-key", "", engine, nil)

	req := httptest.NewRequest(http.MethodGet, "/api?t=unsupported&apikey=test-key", nil)
	w := httptest.NewRecorder()

	srv.handleAPI(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestRequireAPIKey(t *testing.T) {
	engine := search.NewEngine(nil)
	srv := New(":0", "my-secret-key", "", engine, nil)

	// Use httptest.Server with the server's handler
	ts := httptest.NewServer(srv.srv.Handler)
	defer ts.Close()

	// Without key
	resp, err := http.Get(ts.URL + "/api?t=caps")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("expected 403 without API key, got %d", resp.StatusCode)
	}

	// With wrong key
	resp, err = http.Get(ts.URL + "/api?t=caps&apikey=wrong")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("expected 403 with wrong key, got %d", resp.StatusCode)
	}

	// With correct key
	resp, err = http.Get(ts.URL + "/api?t=caps&apikey=my-secret-key")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 with correct key, got %d", resp.StatusCode)
	}
}

func TestRequireAPIKeyEmpty(t *testing.T) {
	engine := search.NewEngine(nil)
	srv := New(":0", "", "", engine, nil)

	req := httptest.NewRequest(http.MethodGet, "/api?t=caps", nil)
	w := httptest.NewRecorder()
	srv.handleAPI(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 when no key configured, got %d", w.Code)
	}
}

func TestWriteTorznabResponse(t *testing.T) {
	releases := []model.ReleaseInfo{
		{
			Title:        "Test Release",
			DownloadLink: "http://example.com/download.torrent",
			Size:         1024,
			Seeders:      10,
			Peers:        20,
			InfoHash:     "abc123",
			Categories:   []int{2000, 3000},
		},
	}

	w := httptest.NewRecorder()
	err := WriteTorznabResponse(w, releases, "test-indexer")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	body := w.Body.String()
	if !strings.Contains(body, "Test Release") {
		t.Error("response should contain release title")
	}
	if !strings.Contains(body, "1024") {
		t.Error("response should contain size")
	}
	if !strings.Contains(body, "abc123") {
		t.Error("response should contain infohash")
	}
}

func TestWriteTorznabResponseEmpty(t *testing.T) {
	w := httptest.NewRecorder()
	err := WriteTorznabResponse(w, []model.ReleaseInfo{}, "test-indexer")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestWriteTorznabResponse_EnclosureByProtocol(t *testing.T) {
	releases := []model.ReleaseInfo{
		{Title: "Usenet Release", DownloadLink: "http://example.com/rel.nzb", Size: 100, Protocol: model.ProtocolUsenet},
		{Title: "Torrent Release", DownloadLink: "http://example.com/rel.torrent", Size: 200, Protocol: model.ProtocolTorrent},
	}

	w := httptest.NewRecorder()
	if err := WriteTorznabResponse(w, releases, "test-indexer"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	body := w.Body.String()
	if !strings.Contains(body, `type="application/x-nzb"`) {
		t.Error("usenet release should get an application/x-nzb enclosure")
	}
	if !strings.Contains(body, `type="application/x-bittorrent"`) {
		t.Error("torrent release should get an application/x-bittorrent enclosure")
	}
}

func openTestStore(t *testing.T) *store.Store {
	t.Helper()
	st, err := store.Open(filepath.Join(t.TempDir(), "server-test.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })
	return st
}

func TestHandleCaps_NilStore_Fallback(t *testing.T) {
	engine := search.NewEngine(nil)
	srv := New(":0", "test-key", "", engine, nil)

	req := httptest.NewRequest(http.MethodGet, "/api?t=caps&apikey=test-key", nil)
	w := httptest.NewRecorder()
	srv.handleCaps(w, req)

	body := w.Body.String()
	for _, mode := range capsModes {
		want := `<` + mode + ` available="yes" />`
		if !strings.Contains(body, want) {
			t.Errorf("expected %s with nil store, got:\n%s", want, body)
		}
	}
}

func TestHandleCaps_Dynamic_NativeNewznab(t *testing.T) {
	st := openTestStore(t)
	native := store.IndexerConfig{
		ID:           "nzbgeek",
		DefinitionID: store.NewznabDefinitionID,
		Protocol:     "usenet",
		Enabled:      true,
		Settings:     map[string]string{"baseurl": "https://api.example.invalid/", "apikey": "k"},
	}
	if err := st.SaveIndexerConfig(native, nil); err != nil {
		t.Fatalf("SaveIndexerConfig: %v", err)
	}

	engine := search.NewEngine(nil)
	srv := New(":0", "test-key", "", engine, st)
	req := httptest.NewRequest(http.MethodGet, "/api?t=caps&apikey=test-key", nil)
	w := httptest.NewRecorder()
	srv.handleCaps(w, req)

	body := w.Body.String()
	for _, mode := range capsModes {
		want := `<` + mode + ` available="yes" />`
		if !strings.Contains(body, want) {
			t.Errorf("expected %s for a native newznab config, got:\n%s", want, body)
		}
	}
}

func TestHandleCaps_Dynamic_Cardigann(t *testing.T) {
	st := openTestStore(t)
	// A Cardigann def with search, tv-search and audio-search modes: the
	// corpus's "audio-search" must be normalized to music-search.
	const fixtureYAML = `id: fixture
name: Fixture
type: public
links:
  - https://fixture.example/
caps:
  modes:
    search: [q]
    tv-search: [q, season, ep, imdbid, tmdbid]
    audio-search: [q]
search:
  paths:
    - path: search
      response:
        type: html
`
	if err := st.SaveDefinition("fixture", "local", "", fixtureYAML); err != nil {
		t.Fatalf("SaveDefinition: %v", err)
	}
	fixture := store.IndexerConfig{
		ID:           "fixture",
		DefinitionID: "fixture",
		Protocol:     "torrent",
		Enabled:      true,
		Settings:     map[string]string{},
	}
	if err := st.SaveIndexerConfig(fixture, nil); err != nil {
		t.Fatalf("SaveIndexerConfig: %v", err)
	}

	engine := search.NewEngine(nil)
	srv := New(":0", "test-key", "", engine, st)
	req := httptest.NewRequest(http.MethodGet, "/api?t=caps&apikey=test-key", nil)
	w := httptest.NewRecorder()
	srv.handleCaps(w, req)

	body := w.Body.String()
	cases := map[string]string{
		"search":       "yes",
		"tv-search":    "yes",
		"music-search": "yes", // normalized from audio-search
		"movie-search": "no",
		"book-search":  "no",
	}
	for mode, want := range cases {
		wantFragment := `<` + mode + ` available="` + want + `" />`
		if !strings.Contains(body, wantFragment) {
			t.Errorf("expected %s, got:\n%s", wantFragment, body)
		}
	}
}

func TestHandleCaps_Dynamic_NoIndexers(t *testing.T) {
	st := openTestStore(t) // no enabled indexers
	engine := search.NewEngine(nil)
	srv := New(":0", "test-key", "", engine, st)

	req := httptest.NewRequest(http.MethodGet, "/api?t=caps&apikey=test-key", nil)
	w := httptest.NewRecorder()
	srv.handleCaps(w, req)

	body := w.Body.String()
	for _, mode := range capsModes {
		want := `<` + mode + ` available="no" />`
		if !strings.Contains(body, want) {
			t.Errorf("expected %s without indexers, got:\n%s", want, body)
		}
	}
}

func TestHandleRoot(t *testing.T) {
	engine := search.NewEngine(nil)
	srv := New(":0", "", "", engine, nil)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	w := httptest.NewRecorder()
	srv.handleRoot(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), "Gowlarr") {
		t.Error("response should contain Gowlarr")
	}
}

func TestHandleRootNotFound(t *testing.T) {
	engine := search.NewEngine(nil)
	srv := New(":0", "", "", engine, nil)

	req := httptest.NewRequest(http.MethodGet, "/unknown", nil)
	w := httptest.NewRecorder()
	srv.handleRoot(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", w.Code)
	}
}

func TestHandleSearchMissingQuery(t *testing.T) {
	engine := search.NewEngine(nil)
	srv := New(":0", "test-key", "", engine, nil)

	req := httptest.NewRequest(http.MethodGet, "/api?t=search&apikey=test-key", nil)
	w := httptest.NewRecorder()

	srv.handleSearch(w, req, "search")

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for missing query, got %d", w.Code)
	}
}
