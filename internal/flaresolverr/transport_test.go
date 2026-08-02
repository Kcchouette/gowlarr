package flaresolverr

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// countingReadCloser wraps a reader and counts Read calls, so tests can
// assert whether RoundTrip buffered the body or passed it through untouched.
type countingReadCloser struct {
	r         io.Reader
	readCalls int
}

func (c *countingReadCloser) Read(p []byte) (int, error) {
	c.readCalls++
	return c.r.Read(p)
}

func (c *countingReadCloser) Close() error { return nil }

// trackingRoundTripper returns a canned response whose body counts reads.
type trackingRoundTripper struct {
	status int
	body   []byte
}

func (f *trackingRoundTripper) RoundTrip(*http.Request) (*http.Response, error) {
	return &http.Response{
		StatusCode: f.status,
		Body:       &countingReadCloser{r: bytes.NewReader(f.body)},
		Header:     make(http.Header),
	}, nil
}

// TestTransport_Non503PassThrough verifies that non-503 responses are NOT
// buffered: the challenge scan can only trigger on 503, so reading the body
// of a regular response is pure waste (up to 10 MiB per request).
func TestTransport_Non503PassThrough(t *testing.T) {
	base := &trackingRoundTripper{status: 200, body: bytes.Repeat([]byte("x"), 1<<20)}
	transport := &FlareSolverrTransport{Base: base, SkipValidator: true}

	req, err := http.NewRequest(http.MethodGet, "https://example.invalid/", nil)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := transport.RoundTrip(req)
	if err != nil {
		t.Fatalf("RoundTrip: %v", err)
	}
	defer resp.Body.Close()

	rc := resp.Body.(*countingReadCloser)
	if rc.readCalls != 0 {
		t.Fatalf("expected the 200 body to pass through untouched (0 reads), got %d reads", rc.readCalls)
	}
}

func TestTransport_NoFallback(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
		w.Write([]byte("normal response"))
	}))
	defer server.Close()

	transport := &FlareSolverrTransport{
		Base:          http.DefaultTransport,
		SkipValidator: true,
	}

	req, _ := http.NewRequest("GET", server.URL, nil)
	resp, err := transport.RoundTrip(req)
	if err != nil {
		t.Fatalf("RoundTrip: %v", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if string(body) != "normal response" {
		t.Errorf("body = %q, want %q", string(body), "normal response")
	}
}

func TestTransport_Fallback(t *testing.T) {
	// Simulate Cloudflare challenge
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(503)
		w.Write([]byte("Just a moment... Verifying you are human"))
	}))
	defer server.Close()

	// FlareSolverr mock
	flareServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
		w.Write([]byte(`{"status":"ok","solution":{"url":"https://example.com","status":200,"response":"resolved page"}}`))
	}))
	defer flareServer.Close()

	flareClient := NewClient(flareServer.URL)
	transport := &FlareSolverrTransport{
		Base:          http.DefaultTransport,
		FlareClient:   flareClient,
		SkipValidator: true,
	}

	req, _ := http.NewRequest("GET", server.URL, nil)
	resp, err := transport.RoundTrip(req)
	if err != nil {
		t.Fatalf("RoundTrip: %v", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(body), "resolved page") {
		t.Errorf("expected resolved page, got %q", string(body))
	}
}

func TestTransport_FlareDown(t *testing.T) {
	// Simulate Cloudflare challenge
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(503)
		w.Write([]byte("Just a moment..."))
	}))
	defer server.Close()

	transport := &FlareSolverrTransport{
		Base:          http.DefaultTransport,
		FlareClient:   NewClient("http://localhost:19999"),
		SkipValidator: true,
	}

	req, _ := http.NewRequest("GET", server.URL, nil)
	resp, err := transport.RoundTrip(req)
	if err != nil {
		t.Fatalf("RoundTrip: %v", err)
	}
	defer resp.Body.Close()

	// Should return original 503 response
	if resp.StatusCode != 503 {
		t.Errorf("status = %d, want 503", resp.StatusCode)
	}
}

func TestTransport_PrivateIPBlocked(t *testing.T) {
	transport := &FlareSolverrTransport{
		Base: http.DefaultTransport,
	}

	req, _ := http.NewRequest("GET", "http://192.168.1.1", nil)
	_, err := transport.RoundTrip(req)
	if err == nil {
		t.Fatal("expected error for private IP")
	}
}
