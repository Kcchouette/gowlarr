package cli

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/Kcchouette/gowlarr/internal/service"
)

func newIndexerCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "indexer",
		Short: "Gérer les indexeurs configurés (instances persistantes d'une définition)",
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
		Short: "Ajouter une instance d'indexeur à partir d'une définition en cache",
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
			fmt.Printf("Indexeur %q ajouté (définition %q).\n", instanceID, args[0])
			return nil
		},
	}
	cmd.Flags().StringVar(&id, "id", "", "Identifiant de l'instance (défaut: identique à la définition)")
	cmd.Flags().StringVar(&version, "version", "v11", "Version du schéma de la définition")
	cmd.Flags().StringVar(&proxyURL, "proxy", "", "URL de proxy dédié (http://... ou socks5://...)")
	cmd.Flags().StringArrayVar(&settings, "setting", nil, "Paramètre au format key=value (répétable)")
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
		Short: "Lister les indexeurs configurés",
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
				fmt.Println("Aucun indexeur configuré. Utilisez `gowlarr indexer add <definition-id>`.")
				return nil
			}
			fmt.Printf("%-20s %-20s %-8s %s\n", "ID", "DEFINITION", "PROTO", "ACTIF")
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
		Short: "Supprimer un indexeur configuré",
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
			fmt.Printf("Indexeur %q supprimé.\n", args[0])
			return nil
		},
	}
}

func newIndexerEnableCmd(enable bool) *cobra.Command {
	use, short := "enable <id>", "Activer un indexeur configuré"
	if !enable {
		use, short = "disable <id>", "Désactiver un indexeur configuré"
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
			fmt.Printf("Indexeur %q mis à jour (actif=%v).\n", args[0], enable)
			return nil
		},
	}
}

func newIndexerTestCmd() *cobra.Command {
	var version string
	cmd := &cobra.Command{
		Use:   "test <id>",
		Short: "Tester la connectivité/authentification d'un indexeur configuré",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			st, cfg, err := openStore()
			if err != nil {
				return err
			}
			defer st.Close()

			svc := service.NewIndexerService(st, cfg)
			if err := svc.Test(cmd.Context(), args[0], version); err != nil {
				fmt.Printf("❌ %s : %v\n", args[0], err)
				return nil
			}
			fmt.Printf("✅ %s : OK (connexion + parsing fonctionnels)\n", args[0])
			return nil
		},
	}
	cmd.Flags().StringVar(&version, "version", "v11", "Version du schéma de la définition")
	return cmd
}
