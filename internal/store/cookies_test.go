package store

import (
	"path/filepath"
	"testing"
)

func openTestStore(t *testing.T) *Store {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "test.db")
	st, err := Open(dbPath)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })
	return st
}

func TestCookies_SaveAndLoadRoundTrip(t *testing.T) {
	st := openTestStore(t)

	// Aucun cookie encore stocké : LoadCookies doit renvoyer "" sans erreur.
	got, err := st.LoadCookies("my-indexer")
	if err != nil {
		t.Fatalf("LoadCookies (empty): %v", err)
	}
	if got != "" {
		t.Fatalf("expected empty string for unknown indexer, got %q", got)
	}

	if err := st.SaveCookies("my-indexer", `[{"url":"https://example.invalid/","cookies":[]}]`); err != nil {
		t.Fatalf("SaveCookies: %v", err)
	}

	got, err = st.LoadCookies("my-indexer")
	if err != nil {
		t.Fatalf("LoadCookies: %v", err)
	}
	if got != `[{"url":"https://example.invalid/","cookies":[]}]` {
		t.Fatalf("unexpected loaded cookies: %q", got)
	}

	// Upsert : une deuxième sauvegarde doit remplacer, pas dupliquer.
	if err := st.SaveCookies("my-indexer", `[]`); err != nil {
		t.Fatalf("SaveCookies (update): %v", err)
	}
	got, err = st.LoadCookies("my-indexer")
	if err != nil {
		t.Fatalf("LoadCookies (after update): %v", err)
	}
	if got != "[]" {
		t.Fatalf("expected updated cookies '[]', got %q", got)
	}
}
