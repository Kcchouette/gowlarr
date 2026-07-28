package store

import (
	"testing"
)

func TestIndexerConfig_SaveAndGetRoundTrip(t *testing.T) {
	st := openTestStore(t)

	cfg := IndexerConfig{
		ID:           "1337x",
		DefinitionID: "1337x",
		Protocol:     "torrent",
		Enabled:      true,
		Settings:     map[string]string{"username": "testuser"},
		ProxyURL:     "socks5://127.0.0.1:1080",
	}

	if err := st.SaveIndexerConfig(cfg); err != nil {
		t.Fatalf("SaveIndexerConfig: %v", err)
	}

	got, err := st.GetIndexerConfig("1337x")
	if err != nil {
		t.Fatalf("GetIndexerConfig: %v", err)
	}
	if got.ID != cfg.ID {
		t.Errorf("ID = %q, want %q", got.ID, cfg.ID)
	}
	if got.DefinitionID != cfg.DefinitionID {
		t.Errorf("DefinitionID = %q, want %q", got.DefinitionID, cfg.DefinitionID)
	}
	if got.Protocol != cfg.Protocol {
		t.Errorf("Protocol = %q, want %q", got.Protocol, cfg.Protocol)
	}
	if !got.Enabled {
		t.Error("Enabled should be true")
	}
	if got.Settings["username"] != "testuser" {
		t.Errorf("Settings[username] = %q, want %q", got.Settings["username"], "testuser")
	}
	if got.ProxyURL != cfg.ProxyURL {
		t.Errorf("ProxyURL = %q, want %q", got.ProxyURL, cfg.ProxyURL)
	}
}

func TestIndexerConfig_Upsert(t *testing.T) {
	st := openTestStore(t)

	cfg := IndexerConfig{
		ID:           "1337x",
		DefinitionID: "1337x",
		Protocol:     "torrent",
		Enabled:      true,
		Settings:     map[string]string{"username": "old"},
	}
	if err := st.SaveIndexerConfig(cfg); err != nil {
		t.Fatalf("SaveIndexerConfig (initial): %v", err)
	}

	cfg.Settings = map[string]string{"username": "new"}
	cfg.Enabled = false
	if err := st.SaveIndexerConfig(cfg); err != nil {
		t.Fatalf("SaveIndexerConfig (update): %v", err)
	}

	got, err := st.GetIndexerConfig("1337x")
	if err != nil {
		t.Fatalf("GetIndexerConfig: %v", err)
	}
	if got.Settings["username"] != "new" {
		t.Errorf("Settings[username] = %q after upsert, want %q", got.Settings["username"], "new")
	}
	if got.Enabled {
		t.Error("Enabled should be false after upsert")
	}
}

func TestIndexerConfig_Get_NotFound(t *testing.T) {
	st := openTestStore(t)

	_, err := st.GetIndexerConfig("nonexistent")
	if err == nil {
		t.Fatal("expected error for nonexistent indexer config")
	}
}

func TestIndexerConfig_List(t *testing.T) {
	st := openTestStore(t)

	configs := []IndexerConfig{
		{ID: "aaa", DefinitionID: "aaa", Protocol: "torrent", Enabled: true, Settings: map[string]string{}},
		{ID: "bbb", DefinitionID: "bbb", Protocol: "usenet", Enabled: false, Settings: map[string]string{}},
		{ID: "ccc", DefinitionID: "ccc", Protocol: "torrent", Enabled: true, Settings: map[string]string{}},
	}
	for _, cfg := range configs {
		if err := st.SaveIndexerConfig(cfg); err != nil {
			t.Fatalf("SaveIndexerConfig(%s): %v", cfg.ID, err)
		}
	}

	// List all
	all, err := st.ListIndexerConfigs(false)
	if err != nil {
		t.Fatalf("ListIndexerConfigs(all): %v", err)
	}
	if len(all) != 3 {
		t.Fatalf("expected 3 configs, got %d", len(all))
	}

	// List enabled only
	enabled, err := st.ListIndexerConfigs(true)
	if err != nil {
		t.Fatalf("ListIndexerConfigs(enabled): %v", err)
	}
	if len(enabled) != 2 {
		t.Fatalf("expected 2 enabled configs, got %d", len(enabled))
	}
}

func TestIndexerConfig_Delete(t *testing.T) {
	st := openTestStore(t)

	cfg := IndexerConfig{ID: "to-delete", DefinitionID: "x", Protocol: "torrent", Enabled: true, Settings: map[string]string{}}
	if err := st.SaveIndexerConfig(cfg); err != nil {
		t.Fatalf("SaveIndexerConfig: %v", err)
	}

	if err := st.DeleteIndexerConfig("to-delete"); err != nil {
		t.Fatalf("DeleteIndexerConfig: %v", err)
	}

	_, err := st.GetIndexerConfig("to-delete")
	if err == nil {
		t.Fatal("expected error after deletion")
	}
}

func TestIndexerConfig_Delete_NotFound(t *testing.T) {
	st := openTestStore(t)

	err := st.DeleteIndexerConfig("nonexistent")
	if err == nil {
		t.Fatal("expected error when deleting nonexistent config")
	}
}

func TestIndexerConfig_SetEnabled(t *testing.T) {
	st := openTestStore(t)

	cfg := IndexerConfig{ID: "toggle", DefinitionID: "x", Protocol: "torrent", Enabled: true, Settings: map[string]string{}}
	if err := st.SaveIndexerConfig(cfg); err != nil {
		t.Fatalf("SaveIndexerConfig: %v", err)
	}

	if err := st.SetIndexerEnabled("toggle", false); err != nil {
		t.Fatalf("SetIndexerEnabled(false): %v", err)
	}
	got, _ := st.GetIndexerConfig("toggle")
	if got.Enabled {
		t.Error("expected Enabled=false after disable")
	}

	if err := st.SetIndexerEnabled("toggle", true); err != nil {
		t.Fatalf("SetIndexerEnabled(true): %v", err)
	}
	got, _ = st.GetIndexerConfig("toggle")
	if !got.Enabled {
		t.Error("expected Enabled=true after re-enable")
	}
}

func TestIndexerConfig_SetEnabled_NotFound(t *testing.T) {
	st := openTestStore(t)

	err := st.SetIndexerEnabled("nonexistent", true)
	if err == nil {
		t.Fatal("expected error when enabling nonexistent config")
	}
}
