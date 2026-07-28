package cli

import (
	"fmt"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"

	cardigannengine "github.com/Kcchouette/cardigann-go/engine"
	"github.com/Kcchouette/cardigann-go/definition"
	"github.com/Kcchouette/cardigann-go/httpclient"
	"github.com/Kcchouette/gowlarr/internal/download"
	"github.com/Kcchouette/gowlarr/internal/store"
)

func newDownloadCmd() *cobra.Command {
	var (
		outputPath string
		toStdout   bool
	)

	cmd := &cobra.Command{
		Use:   "download <result-id>",
		Short: "Télécharger/résoudre le fichier réel d'un résultat de recherche",
		Long: `Récupère automatiquement le bon fichier (.torrent, lien magnet, ou .nzb)
selon le protocole détecté du résultat sélectionné — vous n'avez pas à
préciser vous-même le protocole. L'ID provient d'un précédent "gowlarr search".`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			id, err := strconv.ParseInt(args[0], 10, 64)
			if err != nil {
				return fmt.Errorf("invalid result id %q: %w", args[0], err)
			}

			st, _, err := openStore()
			if err != nil {
				return err
			}
			defer st.Close()

			release, err := st.GetResult(id)
			if err != nil {
				return err
			}

			// Try to get an authenticated HTTP client for the indexer
			var httpClient interface{ Do(*http.Request) (*http.Response, error) } = &http.Client{Timeout: 30 * time.Second}
			var authHeaders map[string]string
			if release.IndexerID != "" {
				if cfg, err := st.GetIndexerConfig(release.IndexerID, nil); err == nil {
					if raw, err := st.GetDefinitionYAML(cfg.DefinitionID, "v11"); err == nil {
						if def, err := definition.Parse([]byte(raw)); err == nil {
							if client, err := httpclient.New(httpclient.Options{
								IndexerID: cfg.ID,
								Persister: store.NewCookiePersisterAdapter(st, nil),
								ProxyURL:  cfg.ProxyURL,
							}); err == nil {
								_ = cardigannengine.NewAuthenticatedProvider(def, def.Links[0], cfg.Settings, client)
								httpClient = client
							}
							// Extract auth headers from definition
							if def.Search.Headers != nil {
								authHeaders = make(map[string]string)
								for header, vals := range def.Search.Headers {
									if len(vals) > 0 {
										// Render the template with config values
										tmplStr := vals[0]
										for k, v := range cfg.Settings {
											tmplStr = strings.ReplaceAll(tmplStr, "{{ .Config."+k+" }}", v)
										}
										authHeaders[header] = tmplStr
									}
								}
							}
						}
					}
				}
			}

			resolver := download.NewResolverWithClient(httpClient, authHeaders)
			artifact, err := resolver.Resolve(cmd.Context(), release)
			if err != nil {
				return fmt.Errorf("resolving download for %q: %w", release.Title, err)
			}

			if toStdout || artifact.IsMagnet {
				if artifact.IsMagnet && !toStdout {
					fmt.Println(string(artifact.Content))
					return nil
				}
				_, err := os.Stdout.Write(artifact.Content)
				return err
			}

			path := outputPath
			if path == "" {
				path = artifact.Filename
			}
			if err := os.WriteFile(path, artifact.Content, 0o644); err != nil {
				return fmt.Errorf("writing file %s: %w", path, err)
			}
			fmt.Printf("Téléchargé : %s\n", path)
			return nil
		},
	}

	cmd.Flags().StringVarP(&outputPath, "output", "o", "", "Chemin du fichier de sortie (défaut: nom dérivé du titre)")
	cmd.Flags().BoolVar(&toStdout, "stdout", false, "Écrire le contenu sur la sortie standard")

	return cmd
}
