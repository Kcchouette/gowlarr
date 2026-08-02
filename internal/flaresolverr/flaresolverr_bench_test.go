package flaresolverr

import (
	"bytes"
	"io"
	"net/http"
	"testing"
)

// fakeRoundTripper returns a canned response without any I/O.
type fakeRoundTripper struct {
	status int
	body   []byte
}

func (f *fakeRoundTripper) RoundTrip(*http.Request) (*http.Response, error) {
	return &http.Response{
		StatusCode: f.status,
		Body:       io.NopCloser(bytes.NewReader(f.body)),
		Header:     make(http.Header),
	}, nil
}

// BenchmarkIsCloudflareChallenge_10MiB measures the marker scan over a 10 MiB
// body with a marker at the very end (worst case: several full scans).
func BenchmarkIsCloudflareChallenge_10MiB(b *testing.B) {
	body := make([]byte, 10<<20)
	for i := range body {
		body[i] = 'a'
	}
	copy(body[len(body)-len("Just a moment..."):], "Just a moment...")
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if !IsCloudflareChallenge(503, body) {
			b.Fatal("expected challenge detected")
		}
	}
}

// BenchmarkRoundTrip_BuffersBody measures the full-body buffering done on
// every response (readAllBounded up to 10 MiB) with a 1 MiB body, including
// the reconstruction of the response with the buffered bytes.
func BenchmarkRoundTrip_BuffersBody(b *testing.B) {
	body := make([]byte, 1<<20)
	for i := range body {
		body[i] = 'x'
	}
	t := &FlareSolverrTransport{
		Base:          &fakeRoundTripper{status: http.StatusOK, body: body},
		SkipValidator: true, // avoid URL validation in the hot loop
	}
	req, err := http.NewRequest(http.MethodGet, "https://example.invalid/", nil)
	if err != nil {
		b.Fatal(err)
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		resp, err := t.RoundTrip(req)
		if err != nil {
			b.Fatal(err)
		}
		got, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil {
			b.Fatal(err)
		}
		if len(got) != len(body) {
			b.Fatalf("expected %d bytes, got %d", len(body), len(got))
		}
	}
}
