package cli

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/spf13/cobra"

	"github.com/Kcchouette/gowlarr/internal/search"
	"github.com/Kcchouette/gowlarr/internal/search/providers/apibay"
	"github.com/Kcchouette/gowlarr/internal/server"
	"github.com/Kcchouette/gowlarr/internal/service"
)

func newServeCmd() *cobra.Command {
	var (
		addr           string
		apiKey         string
		corsOrigin     string
		insecurePublic bool
	)

	cmd := &cobra.Command{
		Use:   "serve",
		Short: "Démarrer le serveur HTTP Torznab/Newznab",
		Long: `Démarre un serveur HTTP exposant une API compatible Torznab/Newznab
pour une intégration avec Sonarr, Radarr, ou tout autre client compatible.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			st, cfg, err := openStore()
			if err != nil {
				return err
			}
			defer st.Close()

			if apiKey == "" && !insecurePublic && !isLoopbackAddr(addr) {
				return fmt.Errorf(
					"--addr %q n'est pas restreint à localhost et --apikey est vide : "+
						"les indexeurs configurés seraient exposés sans authentification sur le réseau. "+
						"Fournissez --apikey, utilisez une adresse loopback (127.0.0.1:PORT), "+
						"ou passez --insecure-public pour accepter ce risque explicitement", addr)
			}

			providers := []search.Provider{apibay.New()}

			configured, err := service.BuildConfiguredProviders(st, cfg)
			if err != nil {
				slog.Warn("chargement des indexeurs configurés", "err", err)
			} else {
				providers = append(providers, configured...)
			}

			engine := search.NewEngine(providers)
			srv := server.New(addr, apiKey, corsOrigin, engine, st)

			ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
			defer stop()

			go func() {
				<-ctx.Done()
				slog.Info("arrêt du serveur...")
				shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
				defer cancel()
				if err := srv.Shutdown(shutdownCtx); err != nil {
					slog.Error("erreur lors de l'arrêt", "err", err)
				}
			}()

			return srv.ListenAndServe()
		},
	}

	cmd.Flags().StringVar(&addr, "addr", ":9696", "Adresse d'écoute")
	cmd.Flags().StringVar(&apiKey, "apikey", "", "Clé API pour authentification")
	cmd.Flags().StringVar(&corsOrigin, "cors-origin", "", "Origine CORS autorisée (désactivé par défaut)")
	cmd.Flags().BoolVar(&insecurePublic, "insecure-public", false,
		"Autoriser une écoute non-loopback sans clé API (déconseillé)")

	return cmd
}

// isLoopbackAddr indique si addr (host:port ou :port) est restreinte à
// l'interface loopback (127.0.0.1, ::1, localhost). Une adresse vide de
// host (ex: ":9696") écoute sur toutes les interfaces et n'est donc pas
// considérée comme loopback.
func isLoopbackAddr(addr string) bool {
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		host = addr
	}
	if host == "" {
		return false
	}
	if strings.EqualFold(host, "localhost") {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}
