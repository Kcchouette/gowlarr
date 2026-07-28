package cli

import (
	"fmt"
	"strings"

	"github.com/Kcchouette/cardigann-go/definition"
	cardigannengine "github.com/Kcchouette/cardigann-go/engine"
	"github.com/Kcchouette/cardigann-go/httpclient"
	"github.com/spf13/cobra"

	"github.com/Kcchouette/gowlarr/internal/cardigannadapter"
	"github.com/Kcchouette/gowlarr/internal/search"
	"github.com/Kcchouette/gowlarr/internal/store"
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
		Long: `L'ID de définition doit avoir été préalablement synchronisé via
"gowlarr defs sync". Les identifiants/options (ex: --setting username=alice
--setting password=secret) sont stockés en clair dans la base SQLite locale
au MVP (le chiffrement au repos est un raffinement post-MVP documenté dans
le plan).`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			definitionID := args[0]
			instanceID := id
			if instanceID == "" {
				instanceID = definitionID
			}

			st, _, err := openStore()
			if err != nil {
				return err
			}
			defer st.Close()

			raw, err := st.GetDefinitionYAML(definitionID, version)
			if err != nil {
				return err
			}
			def, err := definition.Parse([]byte(raw))
			if err != nil {
				return fmt.Errorf("parsing definition: %w", err)
			}

			settingsMap, err := parseSettingFlags(settings)
			if err != nil {
				return err
			}

			protocol := "torrent"
			if def.IsUsenet() {
				protocol = "usenet"
			}

			cfg := store.IndexerConfig{
				ID:           instanceID,
				DefinitionID: definitionID,
				Protocol:     protocol,
				Enabled:      true,
				Settings:     settingsMap,
				ProxyURL:     proxyURL,
			}
			if err := st.SaveIndexerConfig(cfg, nil); err != nil {
				return err
			}
			fmt.Printf("Indexeur %q ajouté (définition %q, protocole %s).\n", instanceID, definitionID, protocol)
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
	cmd := &cobra.Command{
		Use:   "list",
		Short: "Lister les indexeurs configurés",
		RunE: func(cmd *cobra.Command, args []string) error {
			st, _, err := openStore()
			if err != nil {
				return err
			}
			defer st.Close()

			configs, err := st.ListIndexerConfigs(false, nil)
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
	return cmd
}

func newIndexerRemoveCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "remove <id>",
		Short: "Supprimer un indexeur configuré",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			st, _, err := openStore()
			if err != nil {
				return err
			}
			defer st.Close()
			if err := st.DeleteIndexerConfig(args[0]); err != nil {
				return err
			}
			fmt.Printf("Indexeur %q supprimé.\n", args[0])
			return nil
		},
	}
	return cmd
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
			st, _, err := openStore()
			if err != nil {
				return err
			}
			defer st.Close()
			if err := st.SetIndexerEnabled(args[0], enable); err != nil {
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
		Long: `Exécute une recherche minimale ("test") sur l'indexeur pour vérifier que
la connexion réseau, l'éventuel login, et le parsing des résultats
fonctionnent. Retourne un statut clair : OK / échec d'authentification /
erreur réseau.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			st, _, err := openStore()
			if err != nil {
				return err
			}
			defer st.Close()

			cfg, err := st.GetIndexerConfig(args[0], nil)
			if err != nil {
				return err
			}
			raw, err := st.GetDefinitionYAML(cfg.DefinitionID, version)
			if err != nil {
				return err
			}
			def, err := definition.Parse([]byte(raw))
			if err != nil {
				return fmt.Errorf("parsing definition: %w", err)
			}
			if len(def.Links) == 0 {
				return fmt.Errorf("definition %q has no links[] to test against", cfg.DefinitionID)
			}
			baseURL := def.Links[0]

			client, err := httpclient.New(httpclient.Options{
				IndexerID: cfg.ID,
				Persister: store.NewCookiePersisterAdapter(st, nil),
				ProxyURL:  cfg.ProxyURL,
			})
			if err != nil {
				return fmt.Errorf("building http client: %w", err)
			}

			provider := cardigannadapter.NewProvider(cardigannengine.NewAuthenticatedProvider(def, baseURL, cfg.Settings, client))

			_, err = provider.Search(cmd.Context(), search.Query{Keywords: "test"})
			if err != nil {
				if strings.Contains(err.Error(), "login:") {
					fmt.Printf("❌ %s : échec d'authentification (%v)\n", cfg.ID, err)
				} else {
					fmt.Printf("❌ %s : erreur réseau/parsing (%v)\n", cfg.ID, err)
				}
				return nil
			}
			fmt.Printf("✅ %s : OK (connexion + parsing fonctionnels)\n", cfg.ID)
			return nil
		},
	}
	cmd.Flags().StringVar(&version, "version", "v11", "Version du schéma de la définition")
	return cmd
}
