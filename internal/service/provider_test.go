package service

import (
	"fmt"
	"testing"

	"github.com/Kcchouette/gowlarr/internal/model"
	"github.com/Kcchouette/gowlarr/internal/store"
)

// saveCardigannConfig persists a minimal Cardigann-backed indexer config:
// a definition row + an enabled config referencing it.
func saveCardigannConfig(t *testing.T, st *store.Store, defID string) {
	t.Helper()
	yaml := fmt.Sprintf(`id: %s
name: Fixture
type: public
links:
  - https://fixture.example/
search:
  paths:
    - path: search
      response:
        type: html
`, defID)
	if err := st.SaveDefinition(defID, "local", "", yaml); err != nil {
		t.Fatalf("SaveDefinition: %v", err)
	}
	cfg := store.IndexerConfig{
		ID:           defID,
		DefinitionID: defID,
		Protocol:     "torrent",
		Enabled:      true,
		Settings:     map[string]string{},
	}
	if err := st.SaveIndexerConfig(cfg, nil); err != nil {
		t.Fatalf("SaveIndexerConfig: %v", err)
	}
}

func TestBuildConfiguredProviders_IncludesNativeNewznab(t *testing.T) {
	st := openTestStore(t)
	svc := NewIndexerService(st, testConfig())
	if err := svc.AddNewznab("nzbgeek", "https://api.nzbgeek.info/", "key", ""); err != nil {
		t.Fatalf("AddNewznab: %v", err)
	}
	saveCardigannConfig(t, st, "fixture")

	providers, err := BuildConfiguredProviders(st, testConfig())
	if err != nil {
		t.Fatalf("BuildConfiguredProviders: %v", err)
	}
	if len(providers) != 2 {
		t.Fatalf("expected 2 providers, got %d", len(providers))
	}

	var nativeFound bool
	for _, p := range providers {
		if p.ID() == "nzbgeek" {
			nativeFound = true
			if p.Protocol() != model.ProtocolUsenet {
				t.Errorf("expected usenet protocol for native provider, got %q", p.Protocol())
			}
		}
	}
	if !nativeFound {
		t.Error("expected a native Newznab provider for config nzbgeek")
	}
}

func TestBuildConfiguredProviders_NewznabMissingSettings_Skips(t *testing.T) {
	st := openTestStore(t)
	// A native config with empty settings (e.g. hand-edited DB) must be
	// skipped with a warning, not fail the whole build.
	cfg := store.IndexerConfig{
		ID:           "broken",
		DefinitionID: store.NewznabDefinitionID,
		Protocol:     "usenet",
		Enabled:      true,
		Settings:     map[string]string{},
	}
	if err := st.SaveIndexerConfig(cfg, nil); err != nil {
		t.Fatalf("SaveIndexerConfig: %v", err)
	}

	providers, err := BuildConfiguredProviders(st, testConfig())
	if err != nil {
		t.Fatalf("BuildConfiguredProviders: %v", err)
	}
	if len(providers) != 0 {
		t.Errorf("expected 0 providers (broken config skipped), got %d", len(providers))
	}
}

func TestBuildIndexerProvider_Newznab(t *testing.T) {
	st := openTestStore(t) // NO definitions in this store
	svc := NewIndexerService(st, testConfig())
	if err := svc.AddNewznab("nzb", "https://api.example.invalid/", "key", ""); err != nil {
		t.Fatalf("AddNewznab: %v", err)
	}
	cfg, err := st.GetIndexerConfig("nzb", nil)
	if err != nil {
		t.Fatalf("GetIndexerConfig: %v", err)
	}

	// The definition lookup must be short-circuited: the store has no
	// definitions at all, yet the native provider builds fine.
	provider, err := BuildIndexerProvider(st, testConfig(), cfg)
	if err != nil {
		t.Fatalf("BuildIndexerProvider: %v", err)
	}
	if provider.ID() != "nzb" {
		t.Errorf("provider ID = %q, want nzb", provider.ID())
	}
	if provider.Protocol() != model.ProtocolUsenet {
		t.Errorf("provider protocol = %q, want usenet", provider.Protocol())
	}
}
