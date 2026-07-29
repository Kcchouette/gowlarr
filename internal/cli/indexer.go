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
		Use:   "add <definition-id>",
		Short: "Add an indexer instance from a cached definition",
		Args:  cobra.ExactArgs(1),
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
			if err := svc.Add(args[0], id, version, proxyURL, settingsMap); err != nil {
				return err
			}

			instanceID := id
			if instanceID == "" {
				instanceID = args[0]
			}
			fmt.Printf("Indexer %q added (definition %q).\n", instanceID, args[0])
			return nil
		},
	}
	cmd.Flags().StringVar(&id, "id", "", "Instance ID (default: same as definition)")
	cmd.Flags().StringVar(&version, "version", "v11", "Definition schema version")
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
	cmd.Flags().StringVar(&version, "version", "v11", "Definition schema version")
	return cmd
}
