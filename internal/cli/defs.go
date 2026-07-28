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
		Short: "Gérer le cache local des définitions Cardigann (Prowlarr/Indexers)",
		Long: `Synchronise et consulte les définitions Cardigann. Les fichiers YAML ne
sont JAMAIS redistribués avec Gowlarr : ils sont téléchargés à la demande
depuis le dépôt GitHub Prowlarr/Indexers (licence non spécifiée par
l'auteur amont) et mis en cache localement uniquement pour votre usage.`,
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
		Short: "Télécharger/mettre à jour les définitions Cardigann depuis GitHub",
		RunE: func(cmd *cobra.Command, args []string) error {
			st, _, err := openStore()
			if err != nil {
				return err
			}
			defer st.Close()

			fetcher := defs.NewFetcher()
			raws, err := fetcher.FetchVersion(cmd.Context(), version)
			if err != nil {
				return fmt.Errorf("synchronisation des définitions %s: %w", version, err)
			}

			for _, raw := range raws {
				if err := st.SaveDefinition(raw.ID, version, raw.SHA, raw.YAML); err != nil {
					return err
				}
			}
			fmt.Printf("%d définition(s) %s synchronisée(s).\n", len(raws), version)
			return nil
		},
	}
	cmd.Flags().StringVar(&version, "version", "v11", "Version du schéma Cardigann à synchroniser")
	return cmd
}

func newDefsListCmd() *cobra.Command {
	var version string
	cmd := &cobra.Command{
		Use:   "list",
		Short: "Lister les définitions Cardigann en cache local",
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
				fmt.Println("Aucune définition en cache. Lancez `gowlarr defs sync` d'abord.")
				return nil
			}
			for _, m := range metas {
				fmt.Printf("%-30s %-6s %s\n", m.ID, m.Version, m.DownloadedAt.Format("2006-01-02 15:04"))
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&version, "version", "v11", "Filtrer par version (vide = toutes)")
	return cmd
}

func newDefsShowCmd() *cobra.Command {
	var version string
	cmd := &cobra.Command{
		Use:   "show <definition-id>",
		Short: "Afficher une définition Cardigann en cache (résumé)",
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
	cmd.Flags().StringVar(&version, "version", "v11", "Version du schéma")
	return cmd
}

func methodOrNone(method string) string {
	if method == "" {
		return "(aucun)"
	}
	return method
}
