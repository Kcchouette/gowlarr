package cli

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"time"

	"github.com/Kcchouette/gowlarr/internal/model"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/lipgloss/table"
)

var (
	headerStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("6")). // cyan
			PaddingRight(1)

	cellStyle = lipgloss.NewStyle().
			PaddingRight(1)

	numberStyle = lipgloss.NewStyle().
			Align(lipgloss.Right).
			PaddingRight(1)
)

func printResults(results []model.ReleaseInfo, jsonOutput, linksOutput bool) {
	if jsonOutput {
		data, err := json.MarshalIndent(results, "", "  ")
		if err != nil {
			slog.Error("JSON encoding error", "err", err)
			return
		}
		fmt.Println(string(data))
		return
	}

	if !isatty() {
		printPlainTable(results, linksOutput)
		return
	}

	printStyledTable(results, linksOutput)
}

// resultsHeaders returns the column headers shared by the styled and plain
// result tables, with the trailing LINK column present only for `--links`.
func resultsHeaders(linksOutput bool) []string {
	headers := []string{"ID", "AGE", "PROTO", "SEEDS", "SIZE", "INDEXER", "TITLE", "HOSTS"}
	if linksOutput {
		headers = append(headers, "LINK")
	}
	return headers
}

// resultCells returns the display values for one release, in the column order
// shared by the styled and plain result tables.
func resultCells(r model.ReleaseInfo, linksOutput bool) []string {
	cells := []string{
		fmt.Sprintf("%d", r.ID),
		shortAge(r.PublishDate),
		string(r.Protocol),
		fmt.Sprintf("%d", r.Seeders),
		humanSize(r.Size),
		r.IndexerName,
		r.Title,
		strings.Join(r.Hosts, ","),
	}
	if linksOutput {
		cells = append(cells, displayLink(r))
	}
	return cells
}

func printStyledTable(results []model.ReleaseInfo, linksOutput bool) {
	headers := resultsHeaders(linksOutput)
	rows := make([][]string, 0, len(results))
	for _, r := range results {
		rows = append(rows, resultCells(r, linksOutput))
	}

	t := table.New().
		Headers(headers...).
		Rows(rows...).
		StyleFunc(func(row, col int) lipgloss.Style {
			if row == table.HeaderRow {
				return headerStyle
			}
			switch col {
			case 0, 3: // ID, SEEDS
				return numberStyle
			case 4: // SIZE
				return numberStyle
			default:
				return cellStyle
			}
		}).
		Border(lipgloss.RoundedBorder()).
		BorderColumn(true).
		BorderHeader(true)

	fmt.Println(t.Render())
	printResultsSummary(len(results))
}

// printResultsSummary prints the trailing result count and usage hint shared
// by the styled and plain result tables.
func printResultsSummary(n int) {
	fmt.Printf("\n%d result(s). Use `gowlarr download <ID>` to retrieve a file (DDL/streaming: use `--links`).\n", n)
}

func printPlainTable(results []model.ReleaseInfo, linksOutput bool) {
	if linksOutput {
		fmt.Printf("%-4s %-6s %-8s %-8s %-6s %-10s %-30s %-12s %s\n", "ID", "AGE", "PROTO", "SEEDS", "SIZE", "INDEXER", "TITLE", "HOSTS", "LINK")
	} else {
		fmt.Printf("%-4s %-6s %-8s %-8s %-6s %-10s %-30s %s\n", "ID", "AGE", "PROTO", "SEEDS", "SIZE", "INDEXER", "TITLE", "HOSTS")
	}
	for _, r := range results {
		cells := resultCells(r, linksOutput)
		if linksOutput {
			fmt.Printf("%-4s %-6s %-8s %-8s %-6s %-10s %-30s %-12s %s\n",
				cells[0], cells[1], cells[2], cells[3], cells[4], cells[5], cells[6], cells[7], cells[8])
		} else {
			fmt.Printf("%-4s %-6s %-8s %-8s %-6s %-10s %-30s %s\n",
				cells[0], cells[1], cells[2], cells[3], cells[4], cells[5], cells[6], cells[7])
		}
	}
	printResultsSummary(len(results))
}

// displayLink returns the link shown by `search --links`: the streaming URL
// when present, otherwise the resolved download link.
func displayLink(r model.ReleaseInfo) string {
	if r.Protocol == model.ProtocolStreaming && r.StreamURL != "" {
		return r.StreamURL
	}
	return r.DownloadLink
}

func shortAge(t time.Time) string {
	if t.IsZero() {
		return "-"
	}
	d := time.Since(t)
	switch {
	case d < time.Minute:
		return fmt.Sprintf("%ds", int(d.Seconds()))
	case d < time.Hour:
		return fmt.Sprintf("%dm", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh", int(d.Hours()))
	case d < 30*24*time.Hour:
		return fmt.Sprintf("%dd", int(d.Hours()/24))
	case d < 365*24*time.Hour:
		return fmt.Sprintf("%dmo", int(d.Hours()/(24*30)))
	default:
		return fmt.Sprintf("%dy", int(d.Hours()/(24*365)))
	}
}

func isatty() bool {
	f, err := os.Stdout.Stat()
	if err != nil {
		return false
	}
	return f.Mode()&os.ModeCharDevice != 0
}

func humanSize(bytes int64) string {
	const unit = 1024
	if bytes < unit {
		return fmt.Sprintf("%dB", bytes)
	}
	div, exp := int64(unit), 0
	for n := bytes / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f%ciB", float64(bytes)/float64(div), "KMGTPE"[exp])
}
