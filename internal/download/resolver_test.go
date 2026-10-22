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

func TestResolver_DisplayOnlyProtocols(t *testing.T) {
	// DDL and streaming results are display-only: Resolve must refuse without
	// issuing any HTTP request (the test client panics if dialed).
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("Resolve must not fetch for display-only protocols")
	}))
	defer server.Close()

	r := NewResolverWithClient(newResolverTestClient(server), nil)
	for _, tc := range []struct {
		name     string
		protocol model.Protocol
	}{
		{"ddl", model.ProtocolDDL},
		{"streaming", model.ProtocolStreaming},
	} {
		t.Run(tc.name, func(t *testing.T) {
			release := model.ReleaseInfo{Title: "X", DownloadLink: resolverTestBaseURL + "/dl", Protocol: tc.protocol}
			_, err := r.Resolve(context.Background(), release)
			if err == nil {
				t.Fatal("expected an error for a display-only protocol")
			}
			if !strings.Contains(err.Error(), "display-only") {
				t.Errorf("error should mention display-only, got %q", err)
			}
		})
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

func TestResolver_RejectsHTMLBody(t *testing.T) {
	// A 200 HTML page (login / challenge / intermediate) must never be saved
	// as a .torrent artifact.
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Write([]byte("<!DOCTYPE html><html><head><title>Login</title></head><body></body></html>"))
	}))
	defer server.Close()

	r := NewResolverWithClient(newResolverTestClient(server), nil)
	release := model.ReleaseInfo{Title: "Html.Release", DownloadLink: resolverTestBaseURL + "/dl", Protocol: model.ProtocolTorrent}

	_, err := r.Resolve(context.Background(), release)
	if err == nil {
		t.Fatal("expected an error for an HTML download body")
	}
	if !strings.Contains(err.Error(), "returned HTML") {
		t.Errorf("error should mention HTML payload, got %q", err)
	}
}

func TestResolver_RejectsHTMLBody_WrongContentType(t *testing.T) {
	// Mislabelled HTML (no text/html header) must still be rejected via body sniff.
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("<html><body>Cloudflare challenge</body></html>"))
	}))
	defer server.Close()

	r := NewResolverWithClient(newResolverTestClient(server), nil)
	release := model.ReleaseInfo{Title: "Mislabelled.Html", DownloadLink: resolverTestBaseURL + "/dl", Protocol: model.ProtocolTorrent}

	_, err := r.Resolve(context.Background(), release)
	if err == nil {
		t.Fatal("expected an error for an HTML body with a non-HTML Content-Type")
	}
	if !strings.Contains(err.Error(), "returned HTML") {
		t.Errorf("error should mention HTML payload, got %q", err)
	}
}

func TestResolver_RejectsHTMLHeader_CaseInsensitive(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "TEXT/HTML")
		w.Write([]byte("<!DOCTYPE html><html></html>"))
	}))
	defer server.Close()

	r := NewResolverWithClient(newResolverTestClient(server), nil)
	release := model.ReleaseInfo{Title: "Upper.CT", DownloadLink: resolverTestBaseURL + "/dl", Protocol: model.ProtocolTorrent}

	_, err := r.Resolve(context.Background(), release)
	if err == nil || !strings.Contains(err.Error(), "returned HTML") {
		t.Fatalf("expected HTML rejection for TEXT/HTML header, got %v", err)
	}
}

func TestResolver_RejectsHTMLHeader_EmptyBody(t *testing.T) {
	// Content-Type alone is enough when the body is empty or not sniffable.
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
	}))
	defer server.Close()

	r := NewResolverWithClient(newResolverTestClient(server), nil)
	release := model.ReleaseInfo{Title: "Empty.Html", DownloadLink: resolverTestBaseURL + "/dl", Protocol: model.ProtocolTorrent}

	_, err := r.Resolve(context.Background(), release)
	if err == nil || !strings.Contains(err.Error(), "returned HTML") {
		t.Fatalf("expected HTML rejection for text/html header with empty body, got %v", err)
	}
}

func TestResolver_AcceptsNzbXML(t *testing.T) {
	// Real NZB files are XML: they must not be rejected as HTML.
	const nzb = `<?xml version="1.0" encoding="utf-8"?><nzb xmlns="http://www.newzbin.com/DTD/2003/nzb"><files></files></nzb>`
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/x-nzb")
		w.Write([]byte(nzb))
	}))
	defer server.Close()

	r := NewResolverWithClient(newResolverTestClient(server), nil)
	release := model.ReleaseInfo{Title: "Xml.Nzb", DownloadLink: resolverTestBaseURL + "/dl.nzb", Protocol: model.ProtocolUsenet}

	artifact, err := r.Resolve(context.Background(), release)
	if err != nil {
		t.Fatalf("Resolve NZB: %v", err)
	}
	if artifact.Filename != "Xml.Nzb.nzb" {
		t.Errorf("unexpected filename: %q", artifact.Filename)
	}
	if string(artifact.Content) != nzb {
		t.Errorf("unexpected content: %q", artifact.Content)
	}
}

func TestResolver_AcceptsXHTMLWithXMLDecl(t *testing.T) {
	// XHTML with an XML declaration sniffs as text/xml, not text/html.
	const xhtml = `<?xml version="1.0"?><!DOCTYPE html PUBLIC "-//W3C//DTD XHTML 1.0 Strict//EN" "http://www.w3.org/TR/xhtml1/DTD/xhtml1-strict.dtd"><html xmlns="http://www.w3.org/1999/xhtml"></html>`
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(xhtml))
	}))
	defer server.Close()

	r := NewResolverWithClient(newResolverTestClient(server), nil)
	release := model.ReleaseInfo{Title: "Xhtml.Nzb", DownloadLink: resolverTestBaseURL + "/dl.nzb", Protocol: model.ProtocolUsenet}

	artifact, err := r.Resolve(context.Background(), release)
	if err != nil {
		t.Fatalf("Resolve XHTML-as-XML: %v", err)
	}
	if string(artifact.Content) != xhtml {
		t.Errorf("unexpected content: %q", artifact.Content)
	}
}

func TestResolver_AcceptsEmptyNonHTMLBody(t *testing.T) {
	// Empty bodies with a non-HTML Content-Type are intentionally accepted
	// (no schema validation); pins the deliberate non-rejection boundary.
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/octet-stream")
	}))
	defer server.Close()

	r := NewResolverWithClient(newResolverTestClient(server), nil)
	release := model.ReleaseInfo{Title: "Empty.Octet", DownloadLink: resolverTestBaseURL + "/dl", Protocol: model.ProtocolTorrent}

	if _, err := r.Resolve(context.Background(), release); err != nil {
		t.Fatalf("expected empty non-HTML body to be accepted, got %v", err)
	}
}

func TestLooksLikeHTML(t *testing.T) {
	for _, tc := range []struct {
		name        string
		contentType string
		body        string
		want        bool
	}{
		{"header text/html", "text/html; charset=utf-8", "whatever", true},
		{"header TEXT/HTML uppercase", "TEXT/HTML", "x", true},
		{"header with spaces", "  text/html", "x", true},
		{"body html5", "", "<!DOCTYPE html><html></html>", true},
		{"body html with leading whitespace", "", "\n\n  <html></html>", true},
		{"nzb xml", "application/x-nzb", `<?xml version="1.0"?><nzb></nzb>`, false},
		{"bencode-ish", "application/x-bittorrent", "d8:announce4:xxee", false},
		{"empty body empty ct", "", "", false},
		{"empty body octet-stream", "application/octet-stream", "", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := looksLikeHTML(tc.contentType, []byte(tc.body)); got != tc.want {
				t.Errorf("looksLikeHTML(%q, %q) = %v, want %v", tc.contentType, tc.body, got, tc.want)
			}
		})
	}
}
