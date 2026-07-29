package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/Kcchouette/gowlarr/internal/config"
)

func newConfigCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "config",
		Short: "Manage Gowlarr configuration",
	}
	cmd.AddCommand(newConfigInitCmd())
	cmd.AddCommand(newConfigShowCmd())
	return cmd
}

func newConfigInitCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "init",
		Short: "Create default configuration and database",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Default()
			if err != nil {
				return fmt.Errorf("building default config: %w", err)
			}
			if err := cfg.Save(); err != nil {
				return fmt.Errorf("saving config: %w", err)
			}

			st, _, err := openStore()
			if err != nil {
				return err
			}
			defer st.Close()

			path, _ := config.Path()
			fmt.Printf("Configuration initialized: %s\n", path)
			fmt.Printf("Database: %s\n", cfg.DatabasePath)
			return nil
		},
	}
}

func newConfigShowCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "show",
		Short: "Show active configuration",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load()
			if err != nil {
				return err
			}
			fmt.Printf("database_path:     %s\n", cfg.DatabasePath)
			fmt.Printf("defs_cache_dir:    %s\n", cfg.DefsCacheDir)
			fmt.Printf("log_level:         %s\n", cfg.LogLevel)
			if cfg.HTTPProxy != "" {
				fmt.Printf("http_proxy:        %s\n", cfg.HTTPProxy)
			}
			return nil
		},
	}
}
