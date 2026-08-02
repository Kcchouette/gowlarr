// Package config manages Gowlarr's explicit configuration (no viper,
// per decision following independent review: config loaded/validated
// manually to keep testing simple).
package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// Config represents Gowlarr's persisted configuration.
type Config struct {
	// DatabasePath is the path to the SQLite file.
	DatabasePath string `json:"database_path"`
	// DefsCacheDir is the cache directory for downloaded Cardigann definitions.
	DefsCacheDir string `json:"defs_cache_dir"`
	// LogLevel is the log level (debug, info, warn, error).
	LogLevel string `json:"log_level"`
	// HTTPProxy is a default HTTP proxy (optional, can be overridden per indexer).
	HTTPProxy string `json:"http_proxy,omitempty"`
	// FlareSolverrURL is the FlareSolverr service URL (optional, post-MVP).
	FlareSolverrURL string `json:"flaresolverr_url,omitempty"`
	// MaxResultsPerIndexer caps the number of results a single indexer can
	// contribute to a search (0 = default of 10).
	MaxResultsPerIndexer int `json:"max_results_per_indexer,omitempty"`
}

// Dir returns Gowlarr's config directory, using OS conventions
// (os.UserConfigDir), not a hardcoded path — important for Windows portability.
func Dir() (string, error) {
	base, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("resolving user config dir: %w", err)
	}
	return filepath.Join(base, "gowlarr"), nil
}

// CacheDir returns Gowlarr's cache directory (os.UserCacheDir).
func CacheDir() (string, error) {
	base, err := os.UserCacheDir()
	if err != nil {
		return "", fmt.Errorf("resolving user cache dir: %w", err)
	}
	return filepath.Join(base, "gowlarr"), nil
}

// Path returns the full path to the config.json file.
func Path() (string, error) {
	dir, err := Dir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "config.json"), nil
}

// Default builds a default configuration with dynamically resolved paths
// (never hardcoded).
func Default() (Config, error) {
	dir, err := Dir()
	if err != nil {
		return Config{}, err
	}
	cache, err := CacheDir()
	if err != nil {
		return Config{}, err
	}
	return Config{
		DatabasePath: filepath.Join(dir, "gowlarr.db"),
		DefsCacheDir: filepath.Join(cache, "defs"),
		LogLevel:     "info",
	}, nil
}

// Load reads the configuration from disk. If the file does not exist,
// returns a default configuration without error.
func Load() (Config, error) {
	path, err := Path()
	if err != nil {
		return Config{}, err
	}
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return Default()
	}
	if err != nil {
		return Config{}, fmt.Errorf("reading config file %s: %w", path, err)
	}
	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return Config{}, fmt.Errorf("parsing config file %s: %w", path, err)
	}
	return cfg, nil
}

// Save writes the configuration to disk, creating the parent directory if needed.
func (c Config) Save() error {
	path, err := Path()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("creating config dir: %w", err)
	}
	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling config: %w", err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		return fmt.Errorf("writing config file %s: %w", path, err)
	}
	return nil
}
