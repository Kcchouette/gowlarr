package server

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Kcchouette/gowlarr/internal/model"
	"github.com/Kcchouette/gowlarr/internal/search"
)

// fakeProvider implémente search.Provider pour les tests.
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
	srv := New(":0", "test-key", engine, nil)

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
	srv := New(":0", "test-key", engine, nil)

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
	srv := New(":0", "test-key", engine, nil)

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
	srv := New(":0", "test-key", engine, nil)

	req := httptest.NewRequest(http.MethodGet, "/api?t=search&q=test&cat=2000,3000&apikey=test-key", nil)
	w := httptest.NewRecorder()

	srv.handleSearch(w, req, "search")

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
}

func TestHandleSearchUnsupportedType(t *testing.T) {
	engine := search.NewEngine(nil)
	srv := New(":0", "test-key", engine, nil)

	req := httptest.NewRequest(http.MethodGet, "/api?t=unsupported&apikey=test-key", nil)
	w := httptest.NewRecorder()

	srv.handleAPI(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestRequireAPIKey(t *testing.T) {
	engine := search.NewEngine(nil)
	srv := New(":0", "my-secret-key", engine, nil)

	// Use httptest.Server with the server's handler
	ts := httptest.NewServer(srv.srv.Handler)
	defer ts.Close()

	// Sans clé
	resp, err := http.Get(ts.URL + "/api?t=caps")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("expected 403 without API key, got %d", resp.StatusCode)
	}

	// Avec mauvaise clé
	resp, err = http.Get(ts.URL + "/api?t=caps&apikey=wrong")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("expected 403 with wrong key, got %d", resp.StatusCode)
	}

	// Avec bonne clé
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
	srv := New(":0", "", engine, nil)

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

func TestHandleRoot(t *testing.T) {
	engine := search.NewEngine(nil)
	srv := New(":0", "", engine, nil)

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
	srv := New(":0", "", engine, nil)

	req := httptest.NewRequest(http.MethodGet, "/unknown", nil)
	w := httptest.NewRecorder()
	srv.handleRoot(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", w.Code)
	}
}
