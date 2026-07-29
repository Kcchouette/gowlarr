package cli

import (
	"encoding/json"
	"fmt"
	"log/slog"

	"github.com/Kcchouette/gowlarr/internal/model"
)

func printResults(results []model.ReleaseInfo, jsonOutput bool) {
	if jsonOutput {
		data, err := json.MarshalIndent(results, "", "  ")
		if err != nil {
			slog.Error("JSON encoding error", "err", err)
			return
		}
		fmt.Println(string(data))
		return
	}

	fmt.Printf("%-4s %-8s %-8s %-6s %-10s %s\n", "ID", "PROTO", "SEEDS", "SIZE", "INDEXER", "TITLE")
	for _, r := range results {
		fmt.Printf("%-4d %-8s %-8d %-10s %-10s %s\n",
			r.ID, r.Protocol, r.Seeders, humanSize(r.Size), r.IndexerName, r.Title)
	}
	fmt.Printf("\n%d result(s). Use `gowlarr download <ID>` to retrieve a file.\n", len(results))
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
