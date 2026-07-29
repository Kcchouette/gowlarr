// Package cli implements the cobra commands for Gowlarr.
package cli

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/Kcchouette/gowlarr/internal/config"
	golog "github.com/Kcchouette/gowlarr/internal/log"
	"github.com/Kcchouette/gowlarr/internal/store"
)

// Execute builds and runs the root cobra command.
func Execute() {
	root := newRootCmd()
	if err := root.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func newRootCmd() *cobra.Command {
	var verbose bool

	root := &cobra.Command{
		Use:   "gowlarr",
		Short: "Gowlarr — torrent/usenet indexer manager in Go",
		Long: `Gowlarr is a CLI tool for searching and resolving torrent/usenet links
on user-configured indexers.

Not affiliated with Prowlarr/Servarr. See README for the full legal disclaimer.
You are solely responsible for how you use this tool.`,
		PersistentPreRun: func(cmd *cobra.Command, args []string) {
			level := "info"
			if verbose {
				level = "debug"
			}
			golog.SetupLogger(level, false)
		},
	}

	root.PersistentFlags().BoolVarP(&verbose, "verbose", "v", false, "Enable verbose debug logging")

	root.AddCommand(newConfigCmd())
	root.AddCommand(newDefsCmd())
	root.AddCommand(newIndexerCmd())
	root.AddCommand(newSearchCmd())
	root.AddCommand(newDownloadCmd())
	root.AddCommand(newServeCmd())

	return root
}

// openStore loads config and opens the associated SQLite database — shared
// utility for all subcommands that need persistence.
func openStore() (*store.Store, config.Config, error) {
	cfg, err := config.Load()
	if err != nil {
		return nil, config.Config{}, fmt.Errorf("loading config: %w", err)
	}
	st, err := store.Open(cfg.DatabasePath)
	if err != nil {
		return nil, config.Config{}, fmt.Errorf("opening store: %w", err)
	}
	return st, cfg, nil
}
