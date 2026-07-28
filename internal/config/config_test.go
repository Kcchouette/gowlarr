package config

import (
	"path/filepath"
	"runtime"
	"testing"
)

// isolateUserDirs redirige os.UserConfigDir()/os.UserCacheDir() vers un
// dossier temporaire pour le test, afin de ne jamais toucher la vraie
// configuration de l'utilisateur qui exécute les tests.
func isolateUserDirs(t *testing.T) {
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

func TestDefault_ResolvesPaths(t *testing.T) {
	isolateUserDirs(t)

	cfg, err := Default()
	if err != nil {
		t.Fatalf("Default: %v", err)
	}
	if cfg.LogLevel != "info" {
		t.Errorf("expected default log level 'info', got %q", cfg.LogLevel)
	}
	if cfg.DatabasePath == "" || filepath.Base(cfg.DatabasePath) != "gowlarr.db" {
		t.Errorf("unexpected database path: %q", cfg.DatabasePath)
	}
	if cfg.DefsCacheDir == "" {
		t.Error("expected non-empty defs cache dir")
	}
}

func TestSaveAndLoad_RoundTrip(t *testing.T) {
	isolateUserDirs(t)

	cfg, err := Default()
	if err != nil {
		t.Fatalf("Default: %v", err)
	}
	cfg.LogLevel = "debug"
	cfg.HTTPProxy = "http://proxy.invalid:8080"

	if err := cfg.Save(); err != nil {
		t.Fatalf("Save: %v", err)
	}

	loaded, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if loaded.LogLevel != "debug" {
		t.Errorf("expected log level 'debug' after round trip, got %q", loaded.LogLevel)
	}
	if loaded.HTTPProxy != "http://proxy.invalid:8080" {
		t.Errorf("expected proxy to survive round trip, got %q", loaded.HTTPProxy)
	}
	if loaded.DatabasePath != cfg.DatabasePath {
		t.Errorf("expected database path to survive round trip: %q vs %q", loaded.DatabasePath, cfg.DatabasePath)
	}
}

func TestLoad_ReturnsDefaultWhenFileMissing(t *testing.T) {
	isolateUserDirs(t)

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.LogLevel != "info" {
		t.Errorf("expected default config when file missing, got log level %q", cfg.LogLevel)
	}
}

func TestPath_CreatesUnderGowlarrDir(t *testing.T) {
	isolateUserDirs(t)

	p, err := Path()
	if err != nil {
		t.Fatalf("Path: %v", err)
	}
	if filepath.Base(filepath.Dir(p)) != "gowlarr" {
		t.Errorf("expected config path under a 'gowlarr' dir, got %q", p)
	}
}
