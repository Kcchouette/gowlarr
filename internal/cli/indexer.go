package cli

import (
	"fmt"
	"log/slog"
	"strings"

	"github.com/spf13/cobra"

	"github.com/Kcchouette/gowlarr/internal/service"
)

func newIndexerCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "indexer",
		Short: "Manage configured indexers (persistent instances of a definition)",
	}
	cmd.AddCommand(newIndexerAddCmd())
	cmd.AddCommand(newIndexerListCmd())
	cmd.AddCommand(newIndexerRemoveCmd())
	cmd.AddCommand(newIndexerTestCmd())
	cmd.AddCommand(newIndexerEnableCmd(true))
	cmd.AddCommand(newIndexerEnableCmd(false))
	return cmd
}

func newIndexerAddCmd() *cobra.Command {
	var (
		id       string
		version  string
		proxyURL string
		settings []string
	)
	cmd := &cobra.Command{
		Use:   "add <query>",
		Short: "Add an indexer instance (by definition ID, domain, or name)",
		Long: `Add an indexer instance from a cached definition.
The query can be a definition ID (e.g. "1337x"), a domain (e.g. "abn.lol"),
or a partial name (e.g. "abnormal"). The system resolves it automatically.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			st, cfg, err := openStore()
			if err != nil {
				return err
			}
			defer st.Close()

			settingsMap, err := parseSettingFlags(settings)
			if err != nil {
				return err
			}

			svc := service.NewIndexerService(st, cfg)

			// Resolve the query to a definition ID.
			resolved, err := svc.ResolveDefinition(args[0], version)
			if err != nil {
				return err
			}
			if len(resolved) == 0 {
				return fmt.Errorf("no definition found for %q; run `gowlarr defs sync` and retry", args[0])
			}

			best := resolved[0]
			if best.Score < 60 {
				fmt.Printf("Multiple matches for %q:\n", args[0])
				limit := len(resolved)
				if limit > 5 {
					limit = 5
				}
				for _, r := range resolved[:limit] {
					domain := r.Domain
					if domain == "" {
						domain = "-"
					}
					fmt.Printf("  %-25s %-20s %s\n", r.ID, r.Name, domain)
				}
				return fmt.Errorf("be more specific: use the exact definition ID or a more precise domain")
			}

			defID := best.ID
			instanceID := id
			if instanceID == "" {
				if best.Domain != "" {
					instanceID = best.Domain
				} else {
					instanceID = defID
				}
			}
			fmt.Printf("Resolved %q → definition %q (%s)\n", args[0], defID, best.Name)

			if err := svc.Add(defID, instanceID, version, proxyURL, settingsMap); err != nil {
				return err
			}

			fmt.Printf("Indexer %q added (definition %q).\n", instanceID, defID)
			return nil
		},
	}
	cmd.Flags().StringVar(&id, "id", "", "Instance ID (default: same as definition)")
	cmd.Flags().StringVar(&version, "version", "", "Definition schema version (default: local then v11)")
	cmd.Flags().StringVar(&proxyURL, "proxy", "", "Dedicated proxy URL (http://... or socks5://...)")
	cmd.Flags().StringArrayVar(&settings, "setting", nil, "Setting in key=value format (repeatable)")
	return cmd
}

func parseSettingFlags(settings []string) (map[string]string, error) {
	out := make(map[string]string, len(settings))
	for _, s := range settings {
		parts := strings.SplitN(s, "=", 2)
		if len(parts) != 2 {
			return nil, fmt.Errorf("invalid --setting %q (expected key=value)", s)
		}
		out[parts[0]] = parts[1]
	}
	return out, nil
}

func newIndexerListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List configured indexers",
		RunE: func(cmd *cobra.Command, args []string) error {
			st, cfg, err := openStore()
			if err != nil {
				return err
			}
			defer st.Close()

			svc := service.NewIndexerService(st, cfg)
			configs, err := svc.List(false)
			if err != nil {
				return err
			}
			if len(configs) == 0 {
				fmt.Println("No indexers configured. Use `gowlarr indexer add <definition-id>`.")
				return nil
			}
			fmt.Printf("%-20s %-20s %-8s %s\n", "ID", "DEFINITION", "PROTO", "ENABLED")
			for _, c := range configs {
				fmt.Printf("%-20s %-20s %-8s %v\n", c.ID, c.DefinitionID, c.Protocol, c.Enabled)
			}
			return nil
		},
	}
}

func newIndexerRemoveCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "remove <id>",
		Short: "Remove a configured indexer",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			st, cfg, err := openStore()
			if err != nil {
				return err
			}
			defer st.Close()

			svc := service.NewIndexerService(st, cfg)
			if err := svc.Remove(args[0]); err != nil {
				return err
			}
			fmt.Printf("Indexer %q removed.\n", args[0])
			return nil
		},
	}
}

func newIndexerEnableCmd(enable bool) *cobra.Command {
	use, short := "enable <id>", "Enable a configured indexer"
	if !enable {
		use, short = "disable <id>", "Disable a configured indexer"
	}
	return &cobra.Command{
		Use:   use,
		Short: short,
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			st, cfg, err := openStore()
			if err != nil {
				return err
			}
			defer st.Close()

			svc := service.NewIndexerService(st, cfg)
			if err := svc.Enable(args[0], enable); err != nil {
				return err
			}
			fmt.Printf("Indexer %q updated (enabled=%v).\n", args[0], enable)
			return nil
		},
	}
}

func newIndexerTestCmd() *cobra.Command {
	var version string
	cmd := &cobra.Command{
		Use:   "test <id>",
		Short: "Test connectivity/authentication of a configured indexer",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			st, cfg, err := openStore()
			if err != nil {
				return err
			}
			defer st.Close()

			svc := service.NewIndexerService(st, cfg)
			if err := svc.Test(cmd.Context(), args[0], version); err != nil {
				slog.Error("indexer test failed", "id", args[0], "err", err)
				return err
			}
			fmt.Printf("✅ %s: OK (connection + parsing functional)\n", args[0])
			return nil
		},
	}
	cmd.Flags().StringVar(&version, "version", "", "Definition schema version (default: local then v11)")
	return cmd
}
