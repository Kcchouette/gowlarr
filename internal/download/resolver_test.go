package download

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Kcchouette/gowlarr/internal/model"
)

const resolverTestBaseURL = "http://1.1.1.1"

func newResolverTestClient(server *httptest.Server) *http.Client {
	return &http.Client{
		Timeout: 30 * time.Second,
		Transport: &http.Transport{
			DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
				var dialer net.Dialer
				return dialer.DialContext(ctx, network, server.Listener.Addr().String())
			},
		},
	}
}

func TestResolver_DirectTorrent(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("FAKE_TORRENT_BYTES"))
	}))
	defer server.Close()

	r := NewResolverWithClient(newResolverTestClient(server), nil)
	release := model.ReleaseInfo{Title: "My.Release", DownloadLink: resolverTestBaseURL + "/dl", Protocol: model.ProtocolTorrent}

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

func TestResolver_WithAuthHeaders(t *testing.T) {
	var receivedAuth string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedAuth = r.Header.Get("Authorization")
		w.Write([]byte("AUTHENTICATED_TORRENT"))
	}))
	defer server.Close()

	authHeaders := map[string]string{
		"Authorization": "******",
	}
	r := NewResolverWithClient(newResolverTestClient(server), authHeaders)
	release := model.ReleaseInfo{Title: "Auth.Release", DownloadLink: resolverTestBaseURL + "/dl", Protocol: model.ProtocolTorrent}

	artifact, err := r.Resolve(context.Background(), release)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if string(artifact.Content) != "AUTHENTICATED_TORRENT" {
		t.Errorf("unexpected content: %q", artifact.Content)
	}
	if receivedAuth != "******" {
		t.Errorf("expected Authorization header '******', got %q", receivedAuth)
	}
}

func TestResolver_WithAuthHeaders_NilHeaders(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("NO_AUTH_TORRENT"))
	}))
	defer server.Close()

	r := NewResolverWithClient(newResolverTestClient(server), nil)
	release := model.ReleaseInfo{Title: "NoAuth.Release", DownloadLink: resolverTestBaseURL + "/dl", Protocol: model.ProtocolTorrent}

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
		"Authorization": "******",
		"X-API-Key":     "key456",
		"Custom":        "value",
	}
	r := NewResolverWithClient(newResolverTestClient(server), authHeaders)
	release := model.ReleaseInfo{Title: "Multi.Release", DownloadLink: resolverTestBaseURL + "/dl", Protocol: model.ProtocolTorrent}

	artifact, err := r.Resolve(context.Background(), release)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if string(artifact.Content) != "MULTI_HEADER_TORRENT" {
		t.Errorf("unexpected content: %q", artifact.Content)
	}
	if receivedHeaders.Get("Authorization") != "******" {
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
		"Authorization": "******",
	}
	r := NewResolverWithClient(newResolverTestClient(server), authHeaders)
	release := model.ReleaseInfo{Title: "Usenet.Release", DownloadLink: resolverTestBaseURL + "/dl.nzb", Protocol: model.ProtocolUsenet}

	artifact, err := r.Resolve(context.Background(), release)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if artifact.Filename != "Usenet.Release.nzb" {
		t.Errorf("expected filename 'Usenet.Release.nzb', got %q", artifact.Filename)
	}
	if receivedAuth != "******" {
		t.Errorf("expected Authorization header '******', got %q", receivedAuth)
	}
}

func TestResolver_PrivateIPBlocked(t *testing.T) {
	r := NewResolverWithClient(&http.Client{Timeout: 30 * time.Second}, nil)
	release := model.ReleaseInfo{
		Title:        "Blocked.Release",
		DownloadLink: "http://192.168.1.1/dl",
		Protocol:     model.ProtocolTorrent,
	}

	_, err := r.Resolve(context.Background(), release)
	if err == nil {
		t.Fatal("expected error for private IP")
	}
	if !strings.Contains(err.Error(), "private/internal IP addresses are not allowed") {
		t.Fatalf("unexpected error: %v", err)
	}
}
