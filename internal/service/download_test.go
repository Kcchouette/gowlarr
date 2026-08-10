package service

import (
	"net/http"
	"testing"

	"github.com/Kcchouette/gowlarr/internal/config"
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
