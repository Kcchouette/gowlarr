package download

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Kcchouette/gowlarr/internal/model"
)

func TestResolver_DirectTorrent(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("FAKE_TORRENT_BYTES"))
	}))
	defer server.Close()

	r := NewResolver()
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
	r := NewResolver()
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

	r := NewResolver()
	release := model.ReleaseInfo{Title: "Indirect.Release", DownloadLink: server.URL + "/details/1", Protocol: model.ProtocolTorrent}

	artifact, err := r.Resolve(context.Background(), release, DownloadSelectorStep{Selector: "a.dl", Attribute: "href"})
	if err != nil {
		t.Fatalf("Resolve with intermediate page: %v", err)
	}
	if string(artifact.Content) != "REAL_TORRENT_BYTES" {
		t.Errorf("unexpected content: %q", artifact.Content)
	}
}
