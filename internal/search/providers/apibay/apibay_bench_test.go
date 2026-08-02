package apibay

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/Kcchouette/gowlarr/internal/search"
)

// benchJSON builds an apibay-style JSON response with n items.
func benchJSON(n int) string {
	var sb strings.Builder
	sb.WriteByte('[')
	for i := 0; i < n; i++ {
		if i > 0 {
			sb.WriteByte(',')
		}
		// id/info_hash start at 1: "id":"0" and an all-zero info_hash are
		// sentinel values that the provider skips.
		fmt.Fprintf(&sb,
			`{"id":"%d","name":"Ubuntu 24.04 ISO %d","info_hash":"%040d",`+
				`"leechers":"10","seeders":"42","num_files":"3","size":"1048576",`+
				`"added":"1704150000","category":"205"}`, i+1, i, i+1)
	}
	sb.WriteByte(']')
	return sb.String()
}

// BenchmarkSearch_Items100 measures the apibay search path end-to-end:
// HTTP round-trip against an httptest server, JSON unmarshal, and per-item
// magnet construction (fmt.Sprintf + url.QueryEscape).
func BenchmarkSearch_Items100(b *testing.B) {
	body := benchJSON(100)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(body))
	}))
	defer server.Close()

	p := New()
	p.BaseURL = server.URL + "/q.php"
	ctx := context.Background()
	q := search.Query{Keywords: "ubuntu"}

	// Warm-up: first request populates transport-level state.
	if rels, err := p.Search(ctx, q); err != nil {
		b.Fatal(err)
	} else if len(rels) != 100 {
		b.Fatalf("warm-up: expected 100 releases, got %d", len(rels))
	}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		rels, err := p.Search(ctx, q)
		if err != nil {
			b.Fatal(err)
		}
		if len(rels) != 100 {
			b.Fatalf("expected 100 releases, got %d", len(rels))
		}
	}
}

// BenchmarkBuildMagnet_Inline measures the pure per-item magnet construction
// cost (the same expression as provider.go Search loop) without I/O.
func BenchmarkBuildMagnet_Inline(b *testing.B) {
	names := make([]string, 100)
	hashes := make([]string, 100)
	for i := range names {
		names[i] = fmt.Sprintf("Ubuntu 24.04 ISO %d", i)
		hashes[i] = fmt.Sprintf("%040d", i+1)
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		for j := 0; j < len(names); j++ {
			_ = fmt.Sprintf("magnet:?xt=urn:btih:%s&dn=%s", hashes[j], url.QueryEscape(names[j]))
		}
	}
}
