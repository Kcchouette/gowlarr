package newznab

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Kcchouette/gowlarr/internal/model"
	"github.com/Kcchouette/gowlarr/internal/search"
)

// fixtureRSS est un flux RSS Newznab entièrement inventé pour le test (pas
// une réponse réelle d'un indexeur usenet), afin de valider le parsing
// RSS 2.0 + newznab:attr sans dépendance réseau.
const fixtureRSS = `<?xml version="1.0" encoding="UTF-8"?>
<rss version="2.0" xmlns:newznab="http://www.newznab.com/DTD/2010/feeds/attributes/">
  <channel>
    <item>
      <title>Some.Show.S01E02.1080p.WEB</title>
      <guid>https://example.invalid/details/abc123</guid>
      <link>https://example.invalid/getnzb/abc123.nzb</link>
      <pubDate>Mon, 02 Jan 2006 15:04:05 +0000</pubDate>
      <newznab:attr name="size" value="1500000000" />
      <newznab:attr name="category" value="5040" />
    </item>
    <item>
      <title>Another.Release</title>
      <guid>https://example.invalid/details/def456</guid>
      <pubDate>Mon, 02 Jan 2006 16:00:00 +0000</pubDate>
      <newznab:attr name="size" value="700000000" />
    </item>
  </channel>
</rss>`

func TestClient_Search_ParsesRSSAndAttrs(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("t") != "search" {
			t.Errorf("expected t=search, got %q", r.URL.Query().Get("t"))
		}
		if r.URL.Query().Get("apikey") != "mykey" {
			t.Errorf("expected apikey=mykey, got %q", r.URL.Query().Get("apikey"))
		}
		w.Header().Set("Content-Type", "application/xml")
		w.Write([]byte(fixtureRSS))
	}))
	defer server.Close()

	client := New("test-usenet", "TestUsenet", server.URL, "mykey")
	releases, err := client.Search(context.Background(), search.Query{Keywords: "some show"})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(releases) != 2 {
		t.Fatalf("expected 2 releases, got %d", len(releases))
	}

	first := releases[0]
	if first.Title != "Some.Show.S01E02.1080p.WEB" {
		t.Errorf("unexpected title: %q", first.Title)
	}
	if first.DownloadLink != "https://example.invalid/getnzb/abc123.nzb" {
		t.Errorf("unexpected download link: %q", first.DownloadLink)
	}
	if first.Size != 1500000000 {
		t.Errorf("unexpected size: %d", first.Size)
	}
	if len(first.Categories) != 1 || first.Categories[0] != 5040 {
		t.Errorf("unexpected categories: %v", first.Categories)
	}
	if first.Protocol != model.ProtocolUsenet {
		t.Errorf("expected usenet protocol, got %q", first.Protocol)
	}
	if first.PublishDate.IsZero() {
		t.Error("expected non-zero publish date")
	}

	second := releases[1]
	if second.DownloadLink != second.Details {
		t.Errorf("expected download link to fall back to guid when link is empty, got %q vs %q", second.DownloadLink, second.Details)
	}
}

func TestClient_Search_HTTPError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer server.Close()

	client := New("test-usenet", "TestUsenet", server.URL, "badkey")
	_, err := client.Search(context.Background(), search.Query{Keywords: "x"})
	if err == nil {
		t.Fatal("expected error on HTTP 401")
	}
}

func TestClient_Search_CategoriesInRequest(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.URL.Query().Get("cat"); got != "2000,2010" {
			t.Errorf("expected cat=2000,2010, got %q", got)
		}
		w.Write([]byte(`<rss version="2.0"><channel></channel></rss>`))
	}))
	defer server.Close()

	client := New("test-usenet", "TestUsenet", server.URL, "key")
	_, err := client.Search(context.Background(), search.Query{Keywords: "x", Categories: []int{2000, 2010}})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
}
