package config

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

// isolateUserDirs redirects os.UserConfigDir()/os.UserCacheDir() to a
// temporary directory for testing, so it never touches the real user's
// configuration.
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
	cfg.DNSServers = []string{"1.1.1.1", "1.0.0.1"}

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
	if len(loaded.DNSServers) != 2 || loaded.DNSServers[0] != "1.1.1.1" || loaded.DNSServers[1] != "1.0.0.1" {
		t.Errorf("expected dns_servers to survive round trip, got %v", loaded.DNSServers)
	}
}

func TestLoad_IgnoresRemovedDNSOverrides(t *testing.T) {
	isolateUserDirs(t)

	path, err := Path()
	if err != nil {
		t.Fatalf("Path: %v", err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	legacy := `{"log_level": "warn", "dns_overrides": {"tracker.example": "1.2.3.4"}}`
	if err := os.WriteFile(path, []byte(legacy), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	loaded, err := Load()
	if err != nil {
		t.Fatalf("Load with legacy dns_overrides: %v", err)
	}
	if loaded.LogLevel != "warn" {
		t.Errorf("expected log level 'warn', got %q", loaded.LogLevel)
	}
	if loaded.DNSServers != nil {
		t.Errorf("expected nil DNSServers from legacy config, got %v", loaded.DNSServers)
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
