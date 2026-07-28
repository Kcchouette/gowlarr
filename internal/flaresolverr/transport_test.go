package flaresolverr

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

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
