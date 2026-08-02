package cli

import (
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/Kcchouette/gowlarr/internal/config"
)

func isolateCLIUserDirs(t *testing.T) {
	t.Helper()
	tmp := t.TempDir()
	switch runtime.GOOS {
	case "windows":
		t.Setenv("AppData", filepath.Join(tmp, "Roaming"))
		t.Setenv("LocalAppData", filepath.Join(tmp, "Local"))
	default:
		t.Setenv("XDG_CONFIG_HOME", filepath.Join(tmp, "config"))
		t.Setenv("XDG_CACHE_HOME", filepath.Join(tmp, "cache"))
		t.Setenv("HOME", tmp)
	}
}

func writeCLIConfig(t *testing.T) {
	t.Helper()
	cfg, err := config.Default()
	if err != nil {
		t.Fatalf("Default: %v", err)
	}
	if err := cfg.Save(); err != nil {
		t.Fatalf("Save: %v", err)
	}
}

func TestServeCmdRejectsPublicBindWithoutAPIKey(t *testing.T) {
	cmd := newServeCmd()
	cmd.SetArgs([]string{"--addr", "0.0.0.0:9696"})

	err := cmd.Execute()
	if err == nil || !strings.Contains(err.Error(), "--insecure-public") {
		t.Fatalf("expected insecure-public error, got %v", err)
	}
}

func TestSearchCmdRequiresQueryArg(t *testing.T) {
	cmd := newSearchCmd()
	cmd.SetArgs(nil)

	if err := cmd.Execute(); err == nil {
		t.Fatal("expected missing query error")
	}
}

func TestDownloadCmdRejectsInvalidResultID(t *testing.T) {
	cmd := newDownloadCmd()
	cmd.SetArgs([]string{"not-an-int"})

	err := cmd.Execute()
	if err == nil || !strings.Contains(err.Error(), "invalid result id") {
		t.Fatalf("expected invalid result id error, got %v", err)
	}
}

func TestIndexerAddCmdRejectsInvalidSetting(t *testing.T) {
	isolateCLIUserDirs(t)
	writeCLIConfig(t)

	cmd := newIndexerAddCmd()
	cmd.SetArgs([]string{"testtracker", "--setting", "broken"})

	err := cmd.Execute()
	if err == nil || !strings.Contains(err.Error(), "expected key=value") {
		t.Fatalf("expected invalid setting error, got %v", err)
	}
}

func TestIndexerAddCmdNoDefinitionFound(t *testing.T) {
	isolateCLIUserDirs(t)
	writeCLIConfig(t)

	cmd := newIndexerAddCmd()
	cmd.SetArgs([]string{"nonexistent-indexer"})

	err := cmd.Execute()
	if err == nil || !strings.Contains(err.Error(), "no definition found") {
		t.Fatalf("expected 'no definition found' error, got %v", err)
	}
	// err is guaranteed non-nil here (t.Fatalf above exits otherwise).
	if !strings.Contains(err.Error(), "defs sync") {
		t.Fatalf("expected 'defs sync' suggestion in error, got %v", err)
	}
}

func TestIndexerAddCmdAmbiguousQuery(t *testing.T) {
	isolateCLIUserDirs(t)
	writeCLIConfig(t)

	// Seed definitions into the store via the service layer
	st, _, err := openStore()
	if err != nil {
		t.Fatalf("openStore: %v", err)
	}
	defer st.Close()

	// Save two similar definitions
	yaml1 := `id: tracker-alpha
name: Alpha Tracker
type: public
links:
  - https://alpha.example.com
search:
  paths:
    - path: /search
      response:
        type: html
  rows:
    selector: tr
  fields:
    title:
      selector: a`
	yaml2 := `id: tracker-beta
name: Beta Tracker
type: public
links:
  - https://beta.example.com
search:
  paths:
    - path: /search
      response:
        type: html
  rows:
    selector: tr
  fields:
    title:
      selector: a`
	if err := st.SaveDefinition("tracker-alpha", "v11", "sha1", yaml1); err != nil {
		t.Fatalf("SaveDefinition: %v", err)
	}
	if err := st.SaveDefinition("tracker-beta", "v11", "sha2", yaml2); err != nil {
		t.Fatalf("SaveDefinition: %v", err)
	}
	st.Close()

	// Now try adding with a query that matches both domains partially
	cmd := newIndexerAddCmd()
	cmd.SetArgs([]string{"tracker"})

	err = cmd.Execute()
	if err == nil || !strings.Contains(err.Error(), "be more specific") {
		t.Fatalf("expected 'be more specific' error for ambiguous query, got %v", err)
	}
}

func TestDefsShowCmdRequiresDefinitionID(t *testing.T) {
	cmd := newDefsShowCmd()
	cmd.SetArgs(nil)

	if err := cmd.Execute(); err == nil {
		t.Fatal("expected missing definition id error")
	}
}

func TestConfigShowCmdLoadsDefaultConfig(t *testing.T) {
	isolateCLIUserDirs(t)

	cmd := newConfigShowCmd()
	cmd.SetArgs(nil)

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute: %v", err)
	}
}
