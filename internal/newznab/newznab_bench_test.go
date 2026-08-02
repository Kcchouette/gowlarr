package newznab

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Kcchouette/gowlarr/internal/search"
)

// benchRSS builds a Newznab RSS feed with n items, alternating pubDate
// formats (RFC1123Z and RFC3339) so parseRSSDate exercises its layout loop.
func benchRSS(n int) string {
	var sb strings.Builder
	sb.WriteString(`<?xml version="1.0" encoding="UTF-8"?>
<rss version="2.0" xmlns:newznab="http://www.newznab.com/DTD/2010/feeds/attributes/">
  <channel>`)
	for i := 0; i < n; i++ {
		pubDate := "Mon, 02 Jan 2006 15:04:05 +0000"
		if i%2 == 1 {
			pubDate = "2024-01-02T15:04:05Z"
		}
		fmt.Fprintf(&sb, `
    <item>
      <title>Some.Show.S%02dE02.1080p.WEB</title>
      <guid>https://example.invalid/details/abc%04d</guid>
      <link>https://example.invalid/getnzb/abc%04d.nzb</link>
      <pubDate>%s</pubDate>
      <newznab:attr name="size" value="1500000000" />
      <newznab:attr name="category" value="5040" />
    </item>`, i, i, i, pubDate)
	}
	sb.WriteString(`
  </channel>
</rss>`)
	return sb.String()
}

// BenchmarkSearch_RSS100 measures the full Newznab search path: request
// against an httptest server, XML decode, per-item linear attr() scans and
// parseRSSDate layout attempts.
func BenchmarkSearch_RSS100(b *testing.B) {
	feed := benchRSS(100)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/xml")
		_, _ = w.Write([]byte(feed))
	}))
	defer server.Close()

	client := New("bench-usenet", "BenchUsenet", server.URL, "key")
	ctx := context.Background()
	q := search.Query{Keywords: "some show"}

	// Warm-up: first request populates transport-level state.
	if rels, err := client.Search(ctx, q); err != nil {
		b.Fatal(err)
	} else if len(rels) != 100 {
		b.Fatalf("warm-up: expected 100 releases, got %d", len(rels))
	}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		rels, err := client.Search(ctx, q)
		if err != nil {
			b.Fatal(err)
		}
		if len(rels) != 100 {
			b.Fatalf("expected 100 releases, got %d", len(rels))
		}
	}
}

// BenchmarkParseRSSDate_RFC1123Z measures a single date parse that matches
// on the first of 4 layouts.
func BenchmarkParseRSSDate_RFC1123Z(b *testing.B) {
	raw := "Mon, 02 Jan 2006 15:04:05 +0000"
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if parseRSSDate(raw).IsZero() {
			b.Fatal("unexpected parse failure")
		}
	}
}
