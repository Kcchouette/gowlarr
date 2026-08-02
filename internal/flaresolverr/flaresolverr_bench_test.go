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

// BenchmarkRoundTrip_BuffersBody measures the 503-path full-body buffering
// (readAllBounded up to 10 MiB) with a 1 MiB body. The response is closed
// WITHOUT being read by the caller, isolating the transport's own buffering.
func BenchmarkRoundTrip_BuffersBody(b *testing.B) {
	body := make([]byte, 1<<20)
	for i := range body {
		body[i] = 'x'
	}
	t := &FlareSolverrTransport{
		Base:          &fakeRoundTripper{status: http.StatusServiceUnavailable, body: body},
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
		resp.Body.Close()
	}
}

// BenchmarkRoundTrip_PassThrough_Non503 measures the non-503 path: the body
// must pass through untouched (no buffering), so the per-request allocation
// collapses versus the buffered path (BenchmarkRoundTrip_BuffersBody).
func BenchmarkRoundTrip_PassThrough_Non503(b *testing.B) {
	body := make([]byte, 1<<20)
	for i := range body {
		body[i] = 'x'
	}
	t := &FlareSolverrTransport{
		Base:          &trackingRoundTripper{status: http.StatusOK, body: body},
		SkipValidator: true,
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
		resp.Body.Close()
	}
}
