// Package log configures the structured logger (log/slog) for Gowlarr.
// Operational messages (warnings, errors, debug) go to stderr via slog.
// User results (tables, success messages) stay on fmt.Printf to stdout.
package log

import (
	"log/slog"
	"os"
)

// SetupLogger configures the global slog logger with the given level and format.
// level: "debug", "info", "warn", "error"
// jsonFormat: true for JSON (server mode), false for human-readable text (CLI mode).
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
