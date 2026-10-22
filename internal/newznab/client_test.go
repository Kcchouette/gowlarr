package newznab

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Kcchouette/gowlarr/internal/model"
	"github.com/Kcchouette/gowlarr/internal/search"
)

// fixtureRSS is a completely fictional Newznab RSS feed for testing (not a
// real usenet indexer response), used to validate RSS 2.0 + newznab:attr
// parsing without network dependency.
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

	client := NewWithOverrides("test-usenet", "TestUsenet", server.URL, "mykey", nil)
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

// TestClient_Search_PrefersEnclosure pins the download-link precedence:
// the Newznab enclosure is the downloadable asset, while <link> may be a
// details page (feeds of some anime mirrors put their view page there).
func TestClient_Search_PrefersEnclosure(t *testing.T) {
	const body = `<?xml version="1.0" encoding="UTF-8"?>
<rss version="2.0"><channel>
  <item>
    <title>Example.Show.S01E01.1080p</title>
    <link>https://example.invalid/view/1</link>
    <enclosure url="https://example.invalid/nzb/1" type="application/x-nzb" length="12345" />
    <guid>https://example.invalid/view/1</guid>
  </item>
  <item>
    <title>Example.Show.S01E02.1080p</title>
    <link>https://example.invalid/getnzb/2.nzb</link>
    <guid>https://example.invalid/details/2</guid>
  </item>
</channel></rss>`
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/xml")
		w.Write([]byte(body))
	}))
	defer server.Close()

	client := NewWithOverrides("test-usenet", "TestUsenet", server.URL, "key", nil)
	releases, err := client.Search(context.Background(), search.Query{Keywords: "example"})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(releases) != 2 {
		t.Fatalf("expected 2 releases, got %d", len(releases))
	}
	if releases[0].DownloadLink != "https://example.invalid/nzb/1" {
		t.Errorf("enclosure not preferred: %q", releases[0].DownloadLink)
	}
	if releases[1].DownloadLink != "https://example.invalid/getnzb/2.nzb" {
		t.Errorf("expected link fallback without enclosure, got %q", releases[1].DownloadLink)
	}
}

func TestClient_Search_HTTPError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer server.Close()

	client := NewWithOverrides("test-usenet", "TestUsenet", server.URL, "badkey", nil)
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

	client := NewWithOverrides("test-usenet", "TestUsenet", server.URL, "key", nil)
	_, err := client.Search(context.Background(), search.Query{Keywords: "x", Categories: []int{2000, 2010}})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
}

func TestClient_Search_ErrorXML(t *testing.T) {
	// A bad API key is reported as HTTP 200 + <error> XML, not a 4xx: the
	// error must surface as such, not as a misleading RSS parse failure.
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/xml")
		w.Write([]byte(`<?xml version="1.0" encoding="UTF-8"?>
<error code="100" description="Invalid API Key"/>`))
	}))
	defer server.Close()

	client := NewWithOverrides("test-usenet", "TestUsenet", server.URL, "badkey", nil)
	_, err := client.Search(context.Background(), search.Query{Keywords: "x"})
	if err == nil {
		t.Fatal("expected error for <error> response")
	}
	if !strings.Contains(err.Error(), "Invalid API Key") {
		t.Errorf("expected description in error, got %q", err.Error())
	}
	if !strings.Contains(err.Error(), "code 100") {
		t.Errorf("expected code in error, got %q", err.Error())
	}
}

func TestNewWithTransport_HTTPProxy(t *testing.T) {
	c, err := NewWithTransport("id", "name", "http://example.invalid", "key", "http://proxy.example:8080", nil)
	if err != nil {
		t.Fatalf("NewWithTransport: %v", err)
	}
	tr, ok := c.HTTPClient.Transport.(*http.Transport)
	if !ok {
		t.Fatalf("expected *http.Transport, got %T", c.HTTPClient.Transport)
	}
	if tr.Proxy == nil {
		t.Fatal("expected Proxy to be set for an http proxy")
	}
	req, _ := http.NewRequest(http.MethodGet, "http://example.invalid/api", nil)
	u, err := tr.Proxy(req)
	if err != nil {
		t.Fatalf("Proxy: %v", err)
	}
	if u.String() != "http://proxy.example:8080" {
		t.Errorf("expected proxy http://proxy.example:8080, got %q", u.String())
	}
}

func TestNewWithTransport_SOCKS5(t *testing.T) {
	c, err := NewWithTransport("id", "name", "http://example.invalid", "key", "socks5://proxy.example:1080", nil)
	if err != nil {
		t.Fatalf("NewWithTransport: %v", err)
	}
	tr, ok := c.HTTPClient.Transport.(*http.Transport)
	if !ok {
		t.Fatalf("expected *http.Transport, got %T", c.HTTPClient.Transport)
	}
	if tr.Proxy != nil {
		t.Error("expected nil Proxy for socks5 (the dialer IS the proxy path)")
	}
	if tr.DialContext == nil {
		t.Error("expected a DialContext for socks5")
	}
}

func TestNewWithTransport_UnsupportedScheme(t *testing.T) {
	_, err := NewWithTransport("id", "name", "http://example.invalid", "key", "ftp://proxy.example", nil)
	if err == nil {
		t.Fatal("expected error for unsupported proxy scheme")
	}
	if !strings.Contains(err.Error(), "unsupported proxy scheme") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestNewWithTransport_DNSOverrides(t *testing.T) {
	c, err := NewWithTransport("id", "name", "http://example.invalid", "key", "", map[string]string{"example.invalid": "127.0.0.1"})
	if err != nil {
		t.Fatalf("NewWithTransport: %v", err)
	}
	tr, ok := c.HTTPClient.Transport.(*http.Transport)
	if !ok {
		t.Fatalf("expected *http.Transport, got %T", c.HTTPClient.Transport)
	}
	if tr.DialContext == nil {
		t.Error("expected DialContext with DNS overrides")
	}
	if !tr.ForceAttemptHTTP2 {
		t.Error("expected ForceAttemptHTTP2 when a custom DialContext is set")
	}

	// The shared transport must not be mutated: a client without overrides
	// keeps a nil DialContext.
	plain, _ := NewWithTransport("id2", "name", "http://example.invalid", "key", "", nil)
	plainTR, ok := plain.HTTPClient.Transport.(*http.Transport)
	if !ok {
		t.Fatalf("expected *http.Transport, got %T", plain.HTTPClient.Transport)
	}
	if plainTR.DialContext != nil {
		t.Error("shared transport was mutated: plain client has a DialContext")
	}
}

func TestNew_NoProxy_KeepsEnvProxyBehavior(t *testing.T) {
	c := NewWithOverrides("id", "name", "http://example.invalid", "key", nil)
	tr, ok := c.HTTPClient.Transport.(*http.Transport)
	if !ok {
		t.Fatalf("expected *http.Transport, got %T", c.HTTPClient.Transport)
	}
	if tr.Proxy == nil {
		t.Error("expected non-nil Proxy (env-var proxy preserved) without an explicit proxy")
	}
	if tr.DialContext != nil {
		t.Error("expected nil DialContext without DNS overrides")
	}
}

// TestNew_TransportsForceHTTP2 pins the fix for the shared-transport Clone
// footgun: Clone() inherits the source's post-auto-setup TLSClientConfig
// (ALPN h2) but not its TLSNextProto map, so without ForceAttemptHTTP2 the
// clone advertises h2 in ALPN while reading the response as HTTP/1.1 —
// every constructor must force it, whatever the proxy/override shape.
func TestNew_TransportsForceHTTP2(t *testing.T) {
	plain, err := NewWithTransport("id", "name", "http://example.invalid", "key", "", nil)
	if err != nil {
		t.Fatalf("NewWithTransport: %v", err)
	}
	proxied, err := NewWithTransport("id", "name", "http://example.invalid", "key", "http://proxy.example:8080", nil)
	if err != nil {
		t.Fatalf("NewWithTransport proxied: %v", err)
	}
	overrides := NewWithOverrides("id", "name", "http://example.invalid", "key", nil)

	for name, c := range map[string]*Client{"plain": plain, "proxied": proxied, "overrides": overrides} {
		tr, ok := c.HTTPClient.Transport.(*http.Transport)
		if !ok {
			t.Fatalf("%s: expected *http.Transport, got %T", name, c.HTTPClient.Transport)
		}
		if !tr.ForceAttemptHTTP2 {
			t.Errorf("%s: ForceAttemptHTTP2 = false, want true (h2/ALPN hybrid clone bug)", name)
		}
	}
}

func TestClient_Search_APIEndpoint(t *testing.T) {
	cases := []struct {
		name, baseURLSuffix, wantPath string
	}{
		{"bare origin", "", "/api"},
		{"trailing slash", "/", "/api"},
		{"ends with /api", "/api", "/api"}, // no double /api
		{"custom path", "/api/v1/newznab", "/api/v1/newznab"},
		{"custom path trailing slash", "/api/v1/newznab/", "/api/v1/newznab"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != tc.wantPath {
					t.Errorf("request path = %q, want %q", r.URL.Path, tc.wantPath)
				}
				if r.URL.Query().Get("t") != "search" {
					t.Errorf("expected t=search, got %q", r.URL.Query().Get("t"))
				}
				if r.URL.Query().Get("apikey") != "key" {
					t.Errorf("expected apikey=key, got %q", r.URL.Query().Get("apikey"))
				}
				w.Header().Set("Content-Type", "application/xml")
				w.Write([]byte(`<rss version="2.0"><channel></channel></rss>`))
			}))
			defer server.Close()

			client := NewWithOverrides("id", "name", server.URL+tc.baseURLSuffix, "key", nil)
			if _, err := client.Search(context.Background(), search.Query{Keywords: "x"}); err != nil {
				t.Fatalf("Search: %v", err)
			}
		})
	}
}
