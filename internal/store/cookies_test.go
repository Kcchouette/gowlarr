package store

import (
	"path/filepath"
	"testing"

	"github.com/Kcchouette/gowlarr/internal/crypt"
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

	// No cookies stored yet: LoadCookies must return "" without error.
	got, err := st.LoadCookies("my-indexer", nil)
	if err != nil {
		t.Fatalf("LoadCookies (empty): %v", err)
	}
	if got != "" {
		t.Fatalf("expected empty string for unknown indexer, got %q", got)
	}

	if err := st.SaveCookies("my-indexer", `[{"url":"https://example.invalid/","cookies":[]}]`, nil); err != nil {
		t.Fatalf("SaveCookies: %v", err)
	}

	got, err = st.LoadCookies("my-indexer", nil)
	if err != nil {
		t.Fatalf("LoadCookies: %v", err)
	}
	if got != `[{"url":"https://example.invalid/","cookies":[]}]` {
		t.Fatalf("unexpected loaded cookies: %q", got)
	}

	// Upsert: a second save must replace, not duplicate.
	if err := st.SaveCookies("my-indexer", `[]`, nil); err != nil {
		t.Fatalf("SaveCookies (update): %v", err)
	}
	got, err = st.LoadCookies("my-indexer", nil)
	if err != nil {
		t.Fatalf("LoadCookies (after update): %v", err)
	}
	if got != "[]" {
		t.Fatalf("expected updated cookies '[]', got %q", got)
	}
}

func TestCookies_EncryptedRoundTrip(t *testing.T) {
	st := openTestStore(t)

	salt, _ := crypt.GenerateSalt()
	key, _ := crypt.DeriveKey("test-passphrase", salt)

	cookieData := `[{"url":"https://example.invalid/","cookies":[{"name":"session","value":"abc123"}]}]`

	if err := st.SaveCookies("encrypted-indexer", cookieData, key); err != nil {
		t.Fatalf("SaveCookies encrypted: %v", err)
	}

	got, err := st.LoadCookies("encrypted-indexer", key)
	if err != nil {
		t.Fatalf("LoadCookies encrypted: %v", err)
	}
	if got != cookieData {
		t.Errorf("decrypted cookies = %q, want %q", got, cookieData)
	}
}

func TestCookies_EncryptedWrongKey(t *testing.T) {
	st := openTestStore(t)

	salt1, _ := crypt.GenerateSalt()
	key1, _ := crypt.DeriveKey("passphrase-one", salt1)
	salt2, _ := crypt.GenerateSalt()
	key2, _ := crypt.DeriveKey("passphrase-two", salt2)

	if err := st.SaveCookies("idx", "data", key1); err != nil {
		t.Fatalf("SaveCookies: %v", err)
	}

	_, err := st.LoadCookies("idx", key2)
	if err == nil {
		t.Fatal("expected error decrypting with wrong key")
	}
}

func TestCookies_PlaintextFallback(t *testing.T) {
	st := openTestStore(t)

	// Save plaintext
	if err := st.SaveCookies("plain-idx", "plain-data", nil); err != nil {
		t.Fatalf("SaveCookies: %v", err)
	}

	// Load with key (should fall back to plaintext)
	salt, _ := crypt.GenerateSalt()
	key, _ := crypt.DeriveKey("test", salt)

	got, err := st.LoadCookies("plain-idx", key)
	if err != nil {
		t.Fatalf("LoadCookies: %v", err)
	}
	if got != "plain-data" {
		t.Errorf("got %q, want %q", got, "plain-data")
	}
}
