package cli

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	cardigannengine "github.com/Kcchouette/cardigann-go/engine"
	"github.com/Kcchouette/cardigann-go/definition"
	"github.com/Kcchouette/cardigann-go/httpclient"
	"github.com/spf13/cobra"

	"github.com/Kcchouette/gowlarr/internal/cardigannadapter"
	"github.com/Kcchouette/gowlarr/internal/search"
	"github.com/Kcchouette/gowlarr/internal/search/providers/apibay"
	"github.com/Kcchouette/gowlarr/internal/server"
	"github.com/Kcchouette/gowlarr/internal/store"
)

func newServeCmd() *cobra.Command {
	var (
		addr   string
		apiKey string
	)

	cmd := &cobra.Command{
		Use:   "serve",
		Short: "Démarrer le serveur HTTP Torznab/Newznab",
		Long: `Démarre un serveur HTTP exposant une API compatible Torznab/Newznab
pour une intégration avec Sonarr, Radarr, ou tout autre client compatible.

Exemples :
  gowlarr serve --apikey mon-cle-api
  gowlarr serve --addr :8080 --apikey mon-cle-api`,
		RunE: func(cmd *cobra.Command, args []string) error {
			st, _, err := openStore()
			if err != nil {
				return err
			}
			defer st.Close()

			// Build providers
			providers := []search.Provider{apibay.New()}

			configs, err := st.ListIndexerConfigs(true, nil)
			if err != nil {
				return fmt.Errorf("listing indexer configs: %w", err)
			}

			for _, cfg := range configs {
				raw, err := st.GetDefinitionYAML(cfg.DefinitionID, "v11")
				if err != nil {
					fmt.Printf("⚠ indexeur %s: %v\n", cfg.ID, err)
					continue
				}
				def, err := definition.Parse([]byte(raw))
				if err != nil {
					fmt.Printf("⚠ indexeur %s: parsing definition: %v\n", cfg.ID, err)
					continue
				}
				if len(def.Links) == 0 {
					fmt.Printf("⚠ indexeur %s: definition has no links[]\n", cfg.ID)
					continue
				}

				client, err := httpclient.New(httpclient.Options{
					IndexerID: cfg.ID,
					Persister: store.NewCookiePersisterAdapter(st, nil),
					ProxyURL:  cfg.ProxyURL,
				})
				if err != nil {
					fmt.Printf("⚠ indexeur %s: building http client: %v\n", cfg.ID, err)
					continue
				}

				providers = append(providers, cardigannadapter.NewProvider(
					cardigannengine.NewAuthenticatedProvider(def, def.Links[0], cfg.Settings, client)))
			}

			engine := search.NewEngine(providers)
			srv := server.New(addr, apiKey, engine, st)

			// Graceful shutdown
			ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
			defer stop()

			go func() {
				<-ctx.Done()
				fmt.Println("\nShutting down server...")
				shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
				defer cancel()
				srv.Shutdown(shutdownCtx)
			}()

			return srv.ListenAndServe()
		},
	}

	cmd.Flags().StringVar(&addr, "addr", ":9696", "Adresse d'écoute")
	cmd.Flags().StringVar(&apiKey, "apikey", "", "Clé API pour authentification")

	return cmd
}
