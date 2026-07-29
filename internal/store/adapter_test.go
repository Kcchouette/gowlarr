package store

import (
	"testing"

	"github.com/Kcchouette/gowlarr/internal/crypt"
)

func TestCookiePersisterAdapter_RoundTrip(t *testing.T) {
	st := openTestStore(t)
	adapter := NewCookiePersisterAdapter(st, nil)

	got, err := adapter.Load("adapter-plain")
	if err != nil {
		t.Fatalf("Load empty: %v", err)
	}
	if got != "" {
		t.Fatalf("expected empty cookies, got %q", got)
	}

	if err := adapter.Store("adapter-plain", `[{"name":"plain"}]`); err != nil {
		t.Fatalf("Store: %v", err)
	}

	got, err = adapter.Load("adapter-plain")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got != `[{"name":"plain"}]` {
		t.Fatalf("Load = %q, want %q", got, `[{"name":"plain"}]`)
	}
}

func TestCookiePersisterAdapter_EncryptedRoundTrip(t *testing.T) {
	st := openTestStore(t)

	salt, _ := crypt.GenerateSalt()
	key, _ := crypt.DeriveKey("adapter-passphrase", salt)
	adapter := NewCookiePersisterAdapter(st, key)

	if err := adapter.Store("adapter-encrypted", `[{"name":"secret"}]`); err != nil {
		t.Fatalf("Store: %v", err)
	}

	got, err := adapter.Load("adapter-encrypted")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got != `[{"name":"secret"}]` {
		t.Fatalf("Load = %q, want %q", got, `[{"name":"secret"}]`)
	}
}
