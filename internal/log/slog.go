// Package log configure le logger structured (log/slog) pour Gowlarr.
// Les messages opérationnels (warnings, erreurs, debug) vont sur stderr via slog.
// Les résultats utilisateur (tableaux, messages de succès) restent en fmt.Printf sur stdout.
package log

import (
	"log/slog"
	"os"
)

// SetupLogger configure le logger global slog avec le niveau et le format donnés.
// level: "debug", "info", "warn", "error"
// jsonFormat: true pour du JSON (mode server), false pour du texte lisible (mode CLI).
func SetupLogger(level string, jsonFormat bool) {
	var lvl slog.Level
	switch level {
	case "debug":
		lvl = slog.LevelDebug
	case "warn":
		lvl = slog.LevelWarn
	case "error":
		lvl = slog.LevelError
	default:
		lvl = slog.LevelInfo
	}

	opts := &slog.HandlerOptions{
		Level: lvl,
	}

	var handler slog.Handler
	if jsonFormat {
		handler = slog.NewJSONHandler(os.Stderr, opts)
	} else {
		handler = slog.NewTextHandler(os.Stderr, opts)
	}

	slog.SetDefault(slog.New(handler))
}
