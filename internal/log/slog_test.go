package log

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"testing"
)

func TestSetupLoggerConfiguresLevelAndFormat(t *testing.T) {
	old := slog.Default()
	t.Cleanup(func() { slog.SetDefault(old) })

	SetupLogger("warn", true)

	handler := slog.Default().Handler()
	if handler.Enabled(context.Background(), slog.LevelInfo) {
		t.Fatal("info level should be disabled for warn logger")
	}
	if !handler.Enabled(context.Background(), slog.LevelWarn) {
		t.Fatal("warn level should be enabled")
	}
	if got := fmt.Sprintf("%T", handler); !strings.Contains(got, "JSONHandler") {
		t.Fatalf("handler type = %q, want JSON handler", got)
	}
}

func TestSetupLoggerDefaultsToInfo(t *testing.T) {
	old := slog.Default()
	t.Cleanup(func() { slog.SetDefault(old) })

	SetupLogger("unexpected", false)

	handler := slog.Default().Handler()
	if !handler.Enabled(context.Background(), slog.LevelInfo) {
		t.Fatal("info level should be enabled by default")
	}
	if handler.Enabled(context.Background(), slog.LevelDebug) {
		t.Fatal("debug level should be disabled by default")
	}
}
