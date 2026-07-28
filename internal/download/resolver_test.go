package download

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Kcchouette/gowlarr/internal/model"
)

func TestResolver_DirectTorrent(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("FAKE_TORRENT_BYTES"))
	}))
	defer server.Close()

	r := NewResolverWithClient(&http.Client{Timeout: 30 * time.Second}, nil)
	release := model.ReleaseInfo{Title: "My.Release", DownloadLink: server.URL + "/dl", Protocol: model.ProtocolTorrent}

	artifact, err := r.Resolve(context.Background(), release)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if artifact.IsMagnet {
		t.Error("expected non-magnet artifact")
	}
	if string(artifact.Content) != "FAKE_TORRENT_BYTES" {
		t.Errorf("unexpected content: %q", artifact.Content)
	}
	if artifact.Filename != "My.Release.torrent" {
		t.Errorf("unexpected filename: %q", artifact.Filename)
	}
}

func TestResolver_Magnet(t *testing.T) {
	r := NewResolverWithClient(&http.Client{Timeout: 30 * time.Second}, nil)
	release := model.ReleaseInfo{Title: "Magnet.Release", DownloadLink: "magnet:?xt=urn:btih:abc", Protocol: model.ProtocolTorrent}

	artifact, err := r.Resolve(context.Background(), release)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if !artifact.IsMagnet {
		t.Error("expected magnet artifact")
	}
	if string(artifact.Content) != "magnet:?xt=urn:btih:abc" {
		t.Errorf("unexpected content: %q", artifact.Content)
	}
}

func TestResolver_IntermediatePage(t *testing.T) {
	var realURL string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/details/1":
			w.Write([]byte(`<html><body><a class="dl" href="` + realURL + `">Download</a></body></html>`))
		case "/real.torrent":
			w.Write([]byte("REAL_TORRENT_BYTES"))
		}
	}))
	defer server.Close()
	realURL = server.URL + "/real.torrent"

	r := NewResolverWithClient(&http.Client{Timeout: 30 * time.Second}, nil)
	release := model.ReleaseInfo{Title: "Indirect.Release", DownloadLink: server.URL + "/details/1", Protocol: model.ProtocolTorrent}

	artifact, err := r.Resolve(context.Background(), release, DownloadSelectorStep{Selector: "a.dl", Attribute: "href"})
	if err != nil {
		t.Fatalf("Resolve with intermediate page: %v", err)
	}
	if string(artifact.Content) != "REAL_TORRENT_BYTES" {
		t.Errorf("unexpected content: %q", artifact.Content)
	}
}

func TestResolver_WithAuthHeaders(t *testing.T) {
	var receivedAuth string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedAuth = r.Header.Get("Authorization")
		w.Write([]byte("AUTHENTICATED_TORRENT"))
	}))
	defer server.Close()

	authHeaders := map[string]string{
		"Authorization": "Bearer secret-token",
	}
	r := NewResolverWithClient(&http.Client{Timeout: 30 * time.Second}, authHeaders)
	release := model.ReleaseInfo{Title: "Auth.Release", DownloadLink: server.URL + "/dl", Protocol: model.ProtocolTorrent}

	artifact, err := r.Resolve(context.Background(), release)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if string(artifact.Content) != "AUTHENTICATED_TORRENT" {
		t.Errorf("unexpected content: %q", artifact.Content)
	}
	if receivedAuth != "Bearer secret-token" {
		t.Errorf("expected Authorization header 'Bearer secret-token', got %q", receivedAuth)
	}
}

func TestResolver_WithAuthHeaders_NilHeaders(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("NO_AUTH_TORRENT"))
	}))
	defer server.Close()

	r := NewResolverWithClient(&http.Client{Timeout: 30 * time.Second}, nil)
	release := model.ReleaseInfo{Title: "NoAuth.Release", DownloadLink: server.URL + "/dl", Protocol: model.ProtocolTorrent}

	artifact, err := r.Resolve(context.Background(), release)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if string(artifact.Content) != "NO_AUTH_TORRENT" {
		t.Errorf("unexpected content: %q", artifact.Content)
	}
}

func TestResolver_WithAuthHeaders_MultipleHeaders(t *testing.T) {
	var receivedHeaders http.Header
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedHeaders = r.Header
		w.Write([]byte("MULTI_HEADER_TORRENT"))
	}))
	defer server.Close()

	authHeaders := map[string]string{
		"Authorization": "Bearer token123",
		"X-API-Key":     "key456",
		"Custom":        "value",
	}
	r := NewResolverWithClient(&http.Client{Timeout: 30 * time.Second}, authHeaders)
	release := model.ReleaseInfo{Title: "Multi.Release", DownloadLink: server.URL + "/dl", Protocol: model.ProtocolTorrent}

	artifact, err := r.Resolve(context.Background(), release)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if string(artifact.Content) != "MULTI_HEADER_TORRENT" {
		t.Errorf("unexpected content: %q", artifact.Content)
	}
	if receivedHeaders.Get("Authorization") != "Bearer token123" {
		t.Errorf("expected Authorization header, got %q", receivedHeaders.Get("Authorization"))
	}
	if receivedHeaders.Get("X-API-Key") != "key456" {
		t.Errorf("expected X-API-Key header, got %q", receivedHeaders.Get("X-API-Key"))
	}
	if receivedHeaders.Get("Custom") != "value" {
		t.Errorf("expected Custom header, got %q", receivedHeaders.Get("Custom"))
	}
}

func TestResolver_WithAuthHeaders_Nzb(t *testing.T) {
	var receivedAuth string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedAuth = r.Header.Get("Authorization")
		w.Write([]byte("<nzb>content</nzb>"))
	}))
	defer server.Close()

	authHeaders := map[string]string{
		"Authorization": "Bearer usenet-token",
	}
	r := NewResolverWithClient(&http.Client{Timeout: 30 * time.Second}, authHeaders)
	release := model.ReleaseInfo{Title: "Usenet.Release", DownloadLink: server.URL + "/dl.nzb", Protocol: model.ProtocolUsenet}

	artifact, err := r.Resolve(context.Background(), release)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if artifact.Filename != "Usenet.Release.nzb" {
		t.Errorf("expected filename 'Usenet.Release.nzb', got %q", artifact.Filename)
	}
	if receivedAuth != "Bearer usenet-token" {
		t.Errorf("expected Authorization header, got %q", receivedAuth)
	}
}
