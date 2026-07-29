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
		Short: "Start the Torznab/Newznab HTTP server",
		Long: `Starts an HTTP server exposing a Torznab/Newznab-compatible API
for integration with Sonarr, Radarr, or any other compatible client.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			st, cfg, err := openStore()
			if err != nil {
				return err
			}
			defer st.Close()

			if apiKey == "" && !insecurePublic && !isLoopbackAddr(addr) {
				return fmt.Errorf(
					"--addr %q is not restricted to localhost and --apikey is empty: "+
						"configured indexers would be exposed without authentication on the network. "+
						"Provide --apikey, use a loopback address (127.0.0.1:PORT), "+
						"or pass --insecure-public to accept this risk explicitly", addr)
			}

			providers := []search.Provider{apibay.New()}

			configured, err := service.BuildConfiguredProviders(st, cfg)
			if err != nil {
				slog.Warn("loading configured indexers", "err", err)
			} else {
				providers = append(providers, configured...)
			}

			engine := search.NewEngine(providers)
			srv := server.New(addr, apiKey, corsOrigin, engine, st)

			ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
			defer stop()

			go func() {
				<-ctx.Done()
				slog.Info("shutting down server...")
				shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
				defer cancel()
				if err := srv.Shutdown(shutdownCtx); err != nil {
					slog.Error("error during shutdown", "err", err)
				}
			}()

			return srv.ListenAndServe()
		},
	}

	cmd.Flags().StringVar(&addr, "addr", ":9696", "Listen address")
	cmd.Flags().StringVar(&apiKey, "apikey", "", "API key for authentication")
	cmd.Flags().StringVar(&corsOrigin, "cors-origin", "", "Allowed CORS origin (disabled by default)")
	cmd.Flags().BoolVar(&insecurePublic, "insecure-public", false,
		"Allow non-loopback listen without API key (not recommended)")

	return cmd
}

// isLoopbackAddr reports whether addr (host:port or :port) is restricted to
// the loopback interface (127.0.0.1, ::1, localhost). An empty host
// (e.g. ":9696") listens on all interfaces and is not considered loopback.
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
