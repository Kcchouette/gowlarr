package service

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Kcchouette/gowlarr/internal/store"
)

func TestAddNewznab_PersistsAndRoundTrips(t *testing.T) {
	svc := NewIndexerService(openTestStore(t), testConfig())

	if err := svc.AddNewznab("nzbgeek", "https://api.nzbgeek.info/", "sekret", "socks5://127.0.0.1:1080"); err != nil {
		t.Fatalf("AddNewznab: %v", err)
	}

	configs, err := svc.List(false)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(configs) != 1 {
		t.Fatalf("expected 1 config, got %d", len(configs))
	}
	got := configs[0]
	if got.ID != "nzbgeek" {
		t.Errorf("ID = %q, want nzbgeek", got.ID)
	}
	if got.DefinitionID != store.NewznabDefinitionID {
		t.Errorf("DefinitionID = %q, want %q", got.DefinitionID, store.NewznabDefinitionID)
	}
	if got.Protocol != "usenet" {
		t.Errorf("Protocol = %q, want usenet", got.Protocol)
	}
	if !got.Enabled {
		t.Error("expected enabled")
	}
	if got.Settings["baseurl"] != "https://api.nzbgeek.info/" {
		t.Errorf("baseurl = %q", got.Settings["baseurl"])
	}
	if got.Settings["apikey"] != "sekret" {
		t.Errorf("apikey = %q", got.Settings["apikey"])
	}
	if got.ProxyURL != "socks5://127.0.0.1:1080" {
		t.Errorf("ProxyURL = %q", got.ProxyURL)
	}
}

func TestAddNewznab_RejectsDuplicateID(t *testing.T) {
	svc := NewIndexerService(openTestStore(t), testConfig())
	if err := svc.AddNewznab("nzbgeek", "https://api.nzbgeek.info/", "k", ""); err != nil {
		t.Fatalf("first AddNewznab: %v", err)
	}
	err := svc.AddNewznab("nzbgeek", "https://other.example/", "k2", "")
	if err == nil {
		t.Fatal("expected error for duplicate instance ID")
	}
	if !strings.Contains(err.Error(), "--id") {
		t.Errorf("expected error to suggest --id, got %v", err)
	}
}

func TestAddNewznab_InvalidBaseURL(t *testing.T) {
	svc := NewIndexerService(openTestStore(t), testConfig())
	for _, bad := range []string{"not a url", "ftp://x.example", "https://"} {
		if err := svc.AddNewznab("id", bad, "k", ""); err == nil {
			t.Errorf("expected error for base URL %q", bad)
		}
	}
}

// newznabTestServer returns an httptest server serving the given handler,
// plus the corresponding base URL.
func newznabTestServer(t *testing.T, handler http.HandlerFunc) string {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	return server.URL
}

func TestIndexerService_Test_Newznab_401(t *testing.T) {
	svc := NewIndexerService(openTestStore(t), testConfig())
	baseURL := newznabTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	})
	if err := svc.AddNewznab("nzb", baseURL, "badkey", ""); err != nil {
		t.Fatalf("AddNewznab: %v", err)
	}

	err := svc.Test(context.Background(), "nzb", "")
	if err == nil {
		t.Fatal("expected error on HTTP 401")
	}
	if !strings.Contains(err.Error(), "authentication failed") {
		t.Errorf("expected authentication failed, got %v", err)
	}
}

func TestIndexerService_Test_Newznab_BadKey(t *testing.T) {
	svc := NewIndexerService(openTestStore(t), testConfig())
	baseURL := newznabTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/xml")
		w.Write([]byte(`<?xml version="1.0" encoding="UTF-8"?>
<error code="100" description="Invalid API Key"/>`))
	})
	if err := svc.AddNewznab("nzb", baseURL, "badkey", ""); err != nil {
		t.Fatalf("AddNewznab: %v", err)
	}

	err := svc.Test(context.Background(), "nzb", "")
	if err == nil {
		t.Fatal("expected error for invalid API key")
	}
	if !strings.Contains(err.Error(), "authentication failed") {
		t.Errorf("expected authentication failed, got %v", err)
	}
}

func TestIndexerService_Test_Newznab_OK(t *testing.T) {
	svc := NewIndexerService(openTestStore(t), testConfig())
	baseURL := newznabTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/xml")
		w.Write([]byte(`<?xml version="1.0" encoding="UTF-8"?>
<rss version="2.0"><channel></channel></rss>`))
	})
	if err := svc.AddNewznab("nzb", baseURL, "key", ""); err != nil {
		t.Fatalf("AddNewznab: %v", err)
	}

	if err := svc.Test(context.Background(), "nzb", ""); err != nil {
		t.Fatalf("Test: %v", err)
	}
}
