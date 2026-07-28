package apibay

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Kcchouette/gowlarr/internal/search"
)

func withFixtureServer(t *testing.T, body string) *httptest.Server {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(body))
	}))
	t.Cleanup(server.Close)

	original := baseURL
	baseURL = server.URL
	t.Cleanup(func() { baseURL = original })

	return server
}

func TestProvider_Search_ParsesResults(t *testing.T) {
	withFixtureServer(t, `[
		{"id":"1","name":"Ubuntu.24.04.ISO","info_hash":"AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA",
		 "leechers":"3","seeders":"42","num_files":"1","size":"1073741824","added":"1700000000","category":"300"}
	]`)

	p := New()
	releases, err := p.Search(context.Background(), search.Query{Keywords: "ubuntu"})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(releases) != 1 {
		t.Fatalf("expected 1 release, got %d", len(releases))
	}
	r := releases[0]
	if r.Title != "Ubuntu.24.04.ISO" {
		t.Errorf("unexpected title: %q", r.Title)
	}
	if r.Seeders != 42 {
		t.Errorf("expected 42 seeders, got %d", r.Seeders)
	}
	if r.Peers != 45 {
		t.Errorf("expected 45 peers (seeders+leechers), got %d", r.Peers)
	}
	if r.Size != 1073741824 {
		t.Errorf("unexpected size: %d", r.Size)
	}
	if r.DownloadLink == "" || r.DownloadLink[:7] != "magnet:" {
		t.Errorf("expected magnet link, got %q", r.DownloadLink)
	}
}

func TestProvider_Search_FiltersNoResultsSentinel(t *testing.T) {
	withFixtureServer(t, `[{"id":"0","name":"No results returned","info_hash":"0000000000000000000000000000000000000000"}]`)

	p := New()
	releases, err := p.Search(context.Background(), search.Query{Keywords: "zzznotfoundzzz"})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(releases) != 0 {
		t.Fatalf("expected 0 releases for sentinel response, got %d", len(releases))
	}
}

func TestProvider_Search_RejectsEmptyKeywords(t *testing.T) {
	p := New()
	_, err := p.Search(context.Background(), search.Query{Keywords: ""})
	if err == nil {
		t.Fatal("expected error for empty keywords")
	}
}

func TestProvider_Search_HTTPError(t *testing.T) {
	withFixtureServer(t, "")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))
	defer server.Close()
	baseURL = server.URL

	p := New()
	_, err := p.Search(context.Background(), search.Query{Keywords: "test"})
	if err == nil {
		t.Fatal("expected error on HTTP 403")
	}
}
