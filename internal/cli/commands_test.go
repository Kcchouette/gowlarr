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
