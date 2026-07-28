// Package cli implémente les commandes cobra de Gowlarr (Slice A/D du MVP).
package cli

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/Kcchouette/gowlarr/internal/config"
	golog "github.com/Kcchouette/gowlarr/internal/log"
	"github.com/Kcchouette/gowlarr/internal/store"
)

// Execute construit et exécute la commande racine cobra.
func Execute() {
	root := newRootCmd()
	if err := root.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, "erreur:", err)
		os.Exit(1)
	}
}

func newRootCmd() *cobra.Command {
	var verbose bool

	root := &cobra.Command{
		Use:   "gowlarr",
		Short: "Gowlarr — gestionnaire d'indexeurs torrent/usenet en Go (MVP)",
		Long: `Gowlarr est un outil CLI de recherche et de résolution de liens
torrent/usenet sur des indexeurs configurés par l'utilisateur.

Non-affilié à Prowlarr/Servarr. Voir le README pour le disclaimer légal complet.
Vous êtes seul responsable de l'usage que vous faites de cet outil.`,
		PersistentPreRun: func(cmd *cobra.Command, args []string) {
			level := "info"
			if verbose {
				level = "debug"
			}
			golog.SetupLogger(level, false)
		},
	}

	root.PersistentFlags().BoolVarP(&verbose, "verbose", "v", false, "Activer les messages de debug détaillés")

	root.AddCommand(newConfigCmd())
	root.AddCommand(newDefsCmd())
	root.AddCommand(newIndexerCmd())
	root.AddCommand(newSearchCmd())
	root.AddCommand(newDownloadCmd())
	root.AddCommand(newServeCmd())

	return root
}

// openStore charge la config et ouvre la base SQLite associée — utilitaire
// commun à toutes les sous-commandes qui ont besoin de persistance.
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
