// Package config gère la configuration explicite de Gowlarr (pas de viper,
// cf. décision suite à revue indépendante : config chargée/validée à la main
// pour rester simple à tester).
package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// Config représente la configuration persistée de Gowlarr.
type Config struct {
	// DatabasePath est le chemin vers le fichier SQLite.
	DatabasePath string `json:"database_path"`
	// DefsCacheDir est le dossier de cache des définitions Cardigann téléchargées.
	DefsCacheDir string `json:"defs_cache_dir"`
	// LogLevel est le niveau de log (debug, info, warn, error).
	LogLevel string `json:"log_level"`
	// HTTPProxy est un proxy HTTP par défaut (optionnel, peut être surchargé par indexeur).
	HTTPProxy string `json:"http_proxy,omitempty"`
	// FlareSolverrURL est l'URL du service FlareSolverr (optionnel, post-MVP).
	FlareSolverrURL string `json:"flaresolverr_url,omitempty"`
}

// Dir retourne le dossier de configuration de Gowlarr, en utilisant les
// conventions du système d'exploitation (os.UserConfigDir), pas un chemin
// codé en dur — important pour la portabilité Windows.
func Dir() (string, error) {
	base, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("resolving user config dir: %w", err)
	}
	return filepath.Join(base, "gowlarr"), nil
}

// CacheDir retourne le dossier de cache de Gowlarr (os.UserCacheDir).
func CacheDir() (string, error) {
	base, err := os.UserCacheDir()
	if err != nil {
		return "", fmt.Errorf("resolving user cache dir: %w", err)
	}
	return filepath.Join(base, "gowlarr"), nil
}

// Path retourne le chemin complet du fichier config.json.
func Path() (string, error) {
	dir, err := Dir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "config.json"), nil
}

// Default construit une configuration par défaut avec des chemins résolus
// dynamiquement (jamais codés en dur).
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

// Load lit la configuration depuis le disque. Si le fichier n'existe pas,
// retourne une configuration par défaut sans erreur.
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

// Save écrit la configuration sur le disque, en créant le dossier parent si besoin.
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
