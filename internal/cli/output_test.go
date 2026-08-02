package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/Kcchouette/gowlarr/internal/model"
)

func TestHumanSize(t *testing.T) {
	cases := []struct {
		bytes int64
		want  string
	}{
		{0, "0B"},
		{500, "500B"},
		{1023, "1023B"},
		{1024, "1.0KiB"},
		{1048576, "1.0MiB"},
		{1073741824, "1.0GiB"},
		{1099511627776, "1.0TiB"},
	}
	for _, tc := range cases {
		got := humanSize(tc.bytes)
		if got != tc.want {
			t.Errorf("humanSize(%d) = %q, want %q", tc.bytes, got, tc.want)
		}
	}
}

func TestShortAge(t *testing.T) {
	now := time.Now()
	cases := []struct {
		name string
		t    time.Time
		want string
	}{
		{"zero time", time.Time{}, "-"},
		{"1 second ago", now.Add(-1 * time.Second), "1s"},
		{"30 seconds ago", now.Add(-30 * time.Second), "30s"},
		{"59 seconds ago", now.Add(-59 * time.Second), "59s"},
		{"1 minute ago", now.Add(-1 * time.Minute), "1m"},
		{"5 minutes ago", now.Add(-5 * time.Minute), "5m"},
		{"59 minutes ago", now.Add(-59 * time.Minute), "59m"},
		{"1 hour ago", now.Add(-1 * time.Hour), "1h"},
		{"3 hours ago", now.Add(-3 * time.Hour), "3h"},
		{"23 hours ago", now.Add(-23 * time.Hour), "23h"},
		{"1 day ago", now.Add(-24 * time.Hour), "1d"},
		{"7 days ago", now.Add(-7 * 24 * time.Hour), "7d"},
		{"29 days ago", now.Add(-29 * 24 * time.Hour), "29d"},
		{"30 days ago", now.Add(-30 * 24 * time.Hour), "1mo"},
		{"2 months ago", now.Add(-60 * 24 * time.Hour), "2mo"},
		{"11 months ago", now.Add(-330 * 24 * time.Hour), "11mo"},
		{"1 year ago", now.Add(-365 * 24 * time.Hour), "1y"},
		{"2 years ago", now.Add(-2 * 365 * 24 * time.Hour), "2y"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := shortAge(tc.t)
			if got != tc.want {
				t.Errorf("shortAge(%v) = %q, want %q", tc.t, got, tc.want)
			}
		})
	}
}

func TestPrintPlainTable(t *testing.T) {
	results := []model.ReleaseInfo{
		{
			ID:          1,
			Title:       "Ubuntu 24.04 LTS",
			Protocol:    model.ProtocolTorrent,
			Size:        1234567890,
			PublishDate: time.Now().Add(-2 * time.Hour),
			Seeders:     42,
			IndexerName: "1337x",
			IndexerID:   "1337x",
		},
		{
			ID:          2,
			Title:       "Fedora 40 Server",
			Protocol:    model.ProtocolUsenet,
			Size:        987654321,
			PublishDate: time.Time{},
			Seeders:     0,
			IndexerName: "nzbgeek",
			IndexerID:   "nzbgeek",
		},
	}

	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	printPlainTable(results)

	w.Close()
	os.Stdout = old

	var buf bytes.Buffer
	buf.ReadFrom(r)
	output := buf.String()

	// Header present
	if !strings.Contains(output, "ID") || !strings.Contains(output, "AGE") || !strings.Contains(output, "TITLE") {
		t.Errorf("missing header columns in output:\n%s", output)
	}
	// Rows present
	if !strings.Contains(output, "Ubuntu 24.04 LTS") {
		t.Errorf("missing first result title in output:\n%s", output)
	}
	if !strings.Contains(output, "nzbgeek") {
		t.Errorf("missing second result indexer in output:\n%s", output)
	}
	// AGE column: relative for first, "-" for second (zero time)
	if !strings.Contains(output, "2h") {
		t.Errorf("missing relative age in output:\n%s", output)
	}
	// Summary line
	if !strings.Contains(output, "2 result(s)") {
		t.Errorf("missing summary line in output:\n%s", output)
	}
	// No ANSI escape codes
	if strings.Contains(output, "\x1b[") {
		t.Errorf("plain table contains ANSI escape codes:\n%q", output)
	}
}

func TestPrintResultsJSON(t *testing.T) {
	results := []model.ReleaseInfo{
		{
			ID:          1,
			Title:       "Test Release",
			Protocol:    model.ProtocolTorrent,
			Size:        1024,
			PublishDate: time.Date(2025, 1, 15, 10, 30, 0, 0, time.UTC),
			Seeders:     10,
			IndexerName: "test-indexer",
			IndexerID:   "test",
		},
	}

	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	printResults(results, true)

	w.Close()
	os.Stdout = old

	var buf bytes.Buffer
	buf.ReadFrom(r)
	output := buf.String()

	// Should be valid JSON
	var parsed []model.ReleaseInfo
	if err := json.Unmarshal([]byte(output), &parsed); err != nil {
		t.Fatalf("JSON output is not valid: %v\nOutput:\n%s", err, output)
	}
	if len(parsed) != 1 {
		t.Fatalf("expected 1 result in JSON, got %d", len(parsed))
	}
	if parsed[0].Title != "Test Release" {
		t.Errorf("JSON title = %q, want %q", parsed[0].Title, "Test Release")
	}
	// No ANSI codes in JSON
	if strings.Contains(output, "\x1b[") {
		t.Errorf("JSON output contains ANSI escape codes")
	}
}

func TestPrintResultsEmpty(t *testing.T) {
	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	printResults(nil, false)

	w.Close()
	os.Stdout = old

	var buf bytes.Buffer
	buf.ReadFrom(r)
	output := buf.String()

	if !strings.Contains(output, "0 result(s)") {
		t.Errorf("expected '0 result(s)' in empty output:\n%s", output)
	}
}
