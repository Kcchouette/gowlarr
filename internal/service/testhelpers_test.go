package service

import (
	"bytes"
	"log/slog"
	"net/http"
	"path/filepath"
	"testing"

	"github.com/Kcchouette/gowlarr/internal/config"
	"github.com/Kcchouette/gowlarr/internal/store"
)

func openTestStore(t *testing.T) *store.Store {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "service-test.db")
	st, err := store.Open(dbPath)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })
	return st
}

func captureLogs(t *testing.T) *bytes.Buffer {
	t.Helper()
	var buf bytes.Buffer
	old := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug})))
	t.Cleanup(func() { slog.SetDefault(old) })
	return &buf
}

func testConfig() config.Config {
	return config.Config{LogLevel: "debug"}
}

func trackerDefinitionYAML(baseURL string) string {
	return `id: testtracker
name: TestTracker
type: public
links:
  - ` + baseURL + `
search:
  headers:
    Authorization:
      - "Bearer {{ .Config.token }}"
  paths:
    - path: /search
      response:
        type: html
  rows:
    selector: table.results tr
  fields:
    title:
      selector: a.title
    details:
      selector: a.title
      attribute: href
    download:
      selector: a.dl
      attribute: href
    size:
      selector: td.size
    seeders:
      selector: td.seeders
`
}

func brokenTrackerDefinitionYAML() string {
	return `id: testtracker
name: BrokenTracker
type: public
search:
  paths:
    - path: /search
      response:
        type: html
  rows:
    selector: table.results tr
  fields:
    title:
      selector: a.title
`
}

func newznabRSSFixture() string {
	return `<?xml version="1.0" encoding="UTF-8"?>
<rss version="2.0">
  <channel>
    <item>
      <title>Ubuntu NZB</title>
      <link>https://example.invalid/download.nzb</link>
      <guid>https://example.invalid/details/1</guid>
      <pubDate>Mon, 02 Jan 2006 15:04:05 -0700</pubDate>
      <attr name="size" value="1234"></attr>
      <attr name="category" value="2000"></attr>
    </item>
  </channel>
</rss>`
}

type httpClientProvider interface {
	HTTPClient() *http.Client
}

func clientProxyURL(t *testing.T, doer interface {
	Do(*http.Request) (*http.Response, error)
}) string {
	t.Helper()
	provider, ok := doer.(httpClientProvider)
	if !ok {
		t.Fatalf("client does not expose HTTPClient")
	}
	transport, ok := provider.HTTPClient().Transport.(*http.Transport)
	if !ok {
		t.Fatalf("transport type = %T, want *http.Transport", provider.HTTPClient().Transport)
	}
	if transport.Proxy == nil {
		return ""
	}
	req, err := http.NewRequest(http.MethodGet, "https://example.invalid", nil)
	if err != nil {
		t.Fatalf("NewRequest: %v", err)
	}
	proxyURL, err := transport.Proxy(req)
	if err != nil {
		t.Fatalf("Proxy: %v", err)
	}
	if proxyURL == nil {
		return ""
	}
	return proxyURL.String()
}
