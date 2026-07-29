package cli

import (
	"fmt"

	"github.com/Kcchouette/cardigann-go/definition"
	"github.com/Kcchouette/cardigann-go/defs"
	"github.com/spf13/cobra"
)

func newDefsCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "defs",
		Short: "Manage local Cardigann definition cache (Prowlarr/Indexers)",
		Long: `Synchronize and browse Cardigann definitions. YAML files are NEVER
redistributed with Gowlarr: they are fetched on demand from the
Prowlarr/Indexers GitHub repo and cached locally for your use only.`,
	}
	cmd.AddCommand(newDefsSyncCmd())
	cmd.AddCommand(newDefsListCmd())
	cmd.AddCommand(newDefsShowCmd())
	return cmd
}

func newDefsSyncCmd() *cobra.Command {
	var version string
	cmd := &cobra.Command{
		Use:   "sync",
		Short: "Download/update Cardigann definitions from GitHub",
		RunE: func(cmd *cobra.Command, args []string) error {
			st, _, err := openStore()
			if err != nil {
				return err
			}
			defer st.Close()

			fetcher := defs.NewFetcher()
			raws, err := fetcher.FetchVersion(cmd.Context(), version)
			if err != nil {
				return fmt.Errorf("synchronizing definitions %s: %w", version, err)
			}

			for _, raw := range raws {
				if err := st.SaveDefinition(raw.ID, version, raw.SHA, raw.YAML); err != nil {
					return err
				}
			}
			fmt.Printf("%d definition(s) %s synchronized.\n", len(raws), version)
			return nil
		},
	}
	cmd.Flags().StringVar(&version, "version", "v11", "Cardigann schema version to synchronize")
	return cmd
}

func newDefsListCmd() *cobra.Command {
	var version string
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List Cardigann definitions in local cache",
		RunE: func(cmd *cobra.Command, args []string) error {
			st, _, err := openStore()
			if err != nil {
				return err
			}
			defer st.Close()

			metas, err := st.ListDefinitions(version)
			if err != nil {
				return err
			}
			if len(metas) == 0 {
				fmt.Println("No definitions in cache. Run `gowlarr defs sync` first.")
				return nil
			}
			for _, m := range metas {
				fmt.Printf("%-30s %-6s %s\n", m.ID, m.Version, m.DownloadedAt.Format("2006-01-02 15:04"))
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&version, "version", "v11", "Filter by version (empty = all)")
	return cmd
}

func newDefsShowCmd() *cobra.Command {
	var version string
	cmd := &cobra.Command{
		Use:   "show <definition-id>",
		Short: "Show a Cardigann definition in cache (summary)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			st, _, err := openStore()
			if err != nil {
				return err
			}
			defer st.Close()

			raw, err := st.GetDefinitionYAML(args[0], version)
			if err != nil {
				return err
			}
			def, err := definition.Parse([]byte(raw))
			if err != nil {
				return fmt.Errorf("parsing cached definition: %w", err)
			}
			fmt.Printf("id:          %s\n", def.ID)
			fmt.Printf("name:        %s\n", def.Name)
			fmt.Printf("type:        %s\n", def.Type)
			fmt.Printf("login:       %s\n", methodOrNone(def.Login.Method))
			fmt.Printf("links:       %v\n", def.Links)
			return nil
		},
	}
	cmd.Flags().StringVar(&version, "version", "v11", "Schema version")
	return cmd
}

func methodOrNone(method string) string {
	if method == "" {
		return "(none)"
	}
	return method
}
