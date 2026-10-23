package service

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Kcchouette/gowlarr/internal/config"
	"github.com/Kcchouette/gowlarr/internal/model"
)

func TestDownload_BuildClient_Newznab(t *testing.T) {
	st := openTestStore(t)
	cfg := config.Config{LogLevel: "debug", HTTPProxy: "http://global.proxy:8080"}
	svc := NewIndexerService(st, cfg)
	if err := svc.AddNewznab("nzb", "https://api.example.invalid/", "key", ""); err != nil {
		t.Fatalf("AddNewznab: %v", err)
	}

	ds := NewDownloadService(st, cfg)
	client, authHeaders := ds.buildClient("nzb")
	if client == nil {
		t.Fatal("expected a non-nil client for a native newznab config")
	}
	if authHeaders != nil {
		t.Errorf("expected nil auth headers for native newznab, got %v", authHeaders)
	}
	httpClient, ok := client.(*http.Client)
	if !ok {
		t.Fatalf("expected *http.Client, got %T", client)
	}
	tr, ok := httpClient.Transport.(*http.Transport)
	if !ok {
		t.Fatalf("expected *http.Transport, got %T", httpClient.Transport)
	}
	// The effective (global) proxy must be honored on the NZB fetch path.
	req, _ := http.NewRequest(http.MethodGet, "https://api.example.invalid/getnzb", nil)
	u, err := tr.Proxy(req)
	if err != nil {
		t.Fatalf("Proxy: %v", err)
	}
	if u == nil || u.String() != "http://global.proxy:8080" {
		t.Errorf("expected global proxy http://global.proxy:8080, got %v", u)
	}
}

// magnetFicheYAML is the anonymized Cardigann shape of a public torrent
// index whose magnet lives only on the /detail/{id} fiche page.
func magnetFicheYAML(baseURL string) string {
	return fmt.Sprintf(`id: magnetfiche
name: Magnet Fiche
type: public
links:
  - %s
search:
  paths:
    - path: /search
  rows:
    selector: tr
  fields:
    title:
      text: Demo
    download:
      text: /detail/1
download:
  before:
    path: "{{ .DownloadUri }}"
  selectors:
    - selector: 'a[href^="magnet:"]'
      attribute: href
      usebeforeresponse: true
`, baseURL)
}

func TestDownloadService_Resolve_MagnetFromFiche(t *testing.T) {
	const magnet = "magnet:?xt=urn:btih:a0b1c2d3e4f5a0b1c2d3e4f5a0b1c2d3e4f5a0b1&tr=udp://t.example/announce"
	var ficheGets int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/detail/1" {
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
		ficheGets++
		w.Header().Set("Content-Type", "text/html")
		_, _ = fmt.Fprintf(w, `<html><body><a href="%s">Magnet</a></body></html>`, magnet)
	}))
	defer server.Close()

	st := openTestStore(t)
	if err := st.SaveDefinition("magnetfiche", "local", "", magnetFicheYAML(server.URL)); err != nil {
		t.Fatalf("SaveDefinition: %v", err)
	}
	idx := NewIndexerService(st, testConfig())
	if err := idx.Add("magnetfiche", "magnetfiche", "local", "", nil); err != nil {
		t.Fatalf("Add indexer: %v", err)
	}

	saved, err := st.SaveResults([]model.ReleaseInfo{{
		Title:        "Demo",
		DownloadLink: server.URL + "/detail/1",
		Protocol:     model.ProtocolTorrent,
		IndexerID:    "magnetfiche",
	}}, time.Minute)
	if err != nil {
		t.Fatalf("SaveResults: %v", err)
	}

	ds := NewDownloadService(st, testConfig())
	artifact, err := ds.Resolve(context.Background(), saved[0].ID)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if !artifact.IsMagnet {
		t.Fatalf("IsMagnet = false, content %q", artifact.Content)
	}
	if !strings.HasPrefix(string(artifact.Content), "magnet:?xt=urn:btih:a0b1c2d3") {
		t.Errorf("Content = %q, want magnet", artifact.Content)
	}
	if ficheGets != 1 {
		t.Errorf("expected 1 fiche GET, got %d", ficheGets)
	}
}

func TestDownloadService_Resolve_AlreadyMagnet_SkipsChain(t *testing.T) {
	const magnet = "magnet:?xt=urn:btih:b1c2d3e4f5a0b1c2d3e4f5a0b1c2d3e4f5a0b1c2"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatalf("unexpected HTTP request to %s", r.URL.Path)
	}))
	defer server.Close()

	st := openTestStore(t)
	if err := st.SaveDefinition("magnetfiche", "local", "", magnetFicheYAML(server.URL)); err != nil {
		t.Fatalf("SaveDefinition: %v", err)
	}
	idx := NewIndexerService(st, testConfig())
	if err := idx.Add("magnetfiche", "magnetfiche", "local", "", nil); err != nil {
		t.Fatalf("Add indexer: %v", err)
	}

	saved, err := st.SaveResults([]model.ReleaseInfo{{
		Title:        "Demo",
		DownloadLink: magnet,
		Protocol:     model.ProtocolTorrent,
		IndexerID:    "magnetfiche",
	}}, time.Minute)
	if err != nil {
		t.Fatalf("SaveResults: %v", err)
	}

	ds := NewDownloadService(st, testConfig())
	artifact, err := ds.Resolve(context.Background(), saved[0].ID)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if !artifact.IsMagnet || string(artifact.Content) != magnet {
		t.Errorf("artifact = %+v, want already-resolved magnet", artifact)
	}
}

func TestDownloadService_Resolve_ChainFailure_Errors(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		_, _ = fmt.Fprint(w, `<html><body><p>no magnet</p></body></html>`)
	}))
	defer server.Close()

	st := openTestStore(t)
	if err := st.SaveDefinition("magnetfiche", "local", "", magnetFicheYAML(server.URL)); err != nil {
		t.Fatalf("SaveDefinition: %v", err)
	}
	idx := NewIndexerService(st, testConfig())
	if err := idx.Add("magnetfiche", "magnetfiche", "local", "", nil); err != nil {
		t.Fatalf("Add indexer: %v", err)
	}

	saved, err := st.SaveResults([]model.ReleaseInfo{{
		Title:        "Demo",
		DownloadLink: server.URL + "/detail/1",
		Protocol:     model.ProtocolTorrent,
		IndexerID:    "magnetfiche",
	}}, time.Minute)
	if err != nil {
		t.Fatalf("SaveResults: %v", err)
	}

	ds := NewDownloadService(st, testConfig())
	_, err = ds.Resolve(context.Background(), saved[0].ID)
	if err == nil {
		t.Fatal("expected error when magnet chain fails")
	}
	if !strings.Contains(err.Error(), "chain") && !strings.Contains(err.Error(), "matched nothing") {
		t.Errorf("error = %v, want chain failure", err)
	}
}

func TestDownloadService_resolveChain_NoDownloadBlock_KeepsStoredLink(t *testing.T) {
	// No download block: ResolveDownloadLink is a no-op; the stored direct
	// link must pass through unchanged (no chain HTTP).
	yaml := `id: directtorrent
name: Direct Torrent
type: public
links:
  - https://direct.example.invalid/
search:
  paths:
    - path: /search
  rows:
    selector: tr
  fields:
    title:
      text: Demo
    download:
      text: /file.torrent
`
	st := openTestStore(t)
	if err := st.SaveDefinition("directtorrent", "local", "", yaml); err != nil {
		t.Fatalf("SaveDefinition: %v", err)
	}
	idx := NewIndexerService(st, testConfig())
	if err := idx.Add("directtorrent", "directtorrent", "local", "", nil); err != nil {
		t.Fatalf("Add indexer: %v", err)
	}

	ds := NewDownloadService(st, testConfig())
	in := model.ReleaseInfo{
		Title:        "Demo",
		DownloadLink: "https://direct.example.invalid/file.torrent",
		Protocol:     model.ProtocolTorrent,
		IndexerID:    "directtorrent",
	}
	out, err := ds.resolveChain(context.Background(), in)
	if err != nil {
		t.Fatalf("resolveChain: %v", err)
	}
	if out.DownloadLink != in.DownloadLink {
		t.Errorf("DownloadLink = %q, want stored %q", out.DownloadLink, in.DownloadLink)
	}
}

func TestDownloadService_Resolve_DDL_SkipsChain(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatalf("DDL download must not hit the network, got %s", r.URL)
	}))
	defer server.Close()

	st := openTestStore(t)
	saved, err := st.SaveResults([]model.ReleaseInfo{{
		Title:        "DDL",
		DownloadLink: server.URL + "/hoster",
		Protocol:     model.ProtocolDDL,
		IndexerID:    "somesite",
	}}, time.Minute)
	if err != nil {
		t.Fatalf("SaveResults: %v", err)
	}

	ds := NewDownloadService(st, testConfig())
	_, err = ds.Resolve(context.Background(), saved[0].ID)
	if err == nil || !strings.Contains(err.Error(), "display-only") {
		t.Errorf("error = %v, want display-only", err)
	}
}
