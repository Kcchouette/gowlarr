package cli

import (
	"fmt"
	"time"

	"github.com/Kcchouette/cardigann-go/definition"
	cardigannengine "github.com/Kcchouette/cardigann-go/engine"
	"github.com/Kcchouette/cardigann-go/httpclient"
	"github.com/spf13/cobra"

	"github.com/Kcchouette/gowlarr/internal/cardigannadapter"
	"github.com/Kcchouette/gowlarr/internal/newznab"
	"github.com/Kcchouette/gowlarr/internal/search"
	"github.com/Kcchouette/gowlarr/internal/search/providers/apibay"
	"github.com/Kcchouette/gowlarr/internal/store"
)

func newSearchCmd() *cobra.Command {
	var (
		newznabURL    string
		newznabAPIKey string
		newznabName   string
		jsonOutput    bool
		searchType    string
		categories    []int
		season        int
		episode       int
		imdbID        string
		tmdbID        string
	)

	cmd := &cobra.Command{
		Use:   "search <query>",
		Short: "Rechercher une release sur les indexeurs configurés (torrent + usenet)",
		Long: `Interroge en parallèle les indexeurs actifs (torrent et usenet) et affiche
une liste numérotée unifiée de résultats. Les résultats sont persistés (avec
expiration) pour permettre "gowlarr download <id>" dans une invocation
séparée du CLI.

L'indexeur torrent public "apibay" (The Pirate Bay) est toujours interrogé
par défaut au MVP. Un indexeur usenet Newznab générique peut être ajouté à
la volée via --newznab-url/--newznab-apikey (la gestion persistante
d'indexeurs multiples arrive en Slice E).`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			keywords := args[0]

			st, _, err := openStore()
			if err != nil {
				return err
			}
			defer st.Close()

			if err := st.PurgeExpiredResults(); err != nil {
				return err
			}

			providers := []search.Provider{apibay.New()}
			if newznabURL != "" && newznabAPIKey != "" {
				name := newznabName
				if name == "" {
					name = "newznab-generic"
				}
				providers = append(providers, newznab.New("newznab-generic", name, newznabURL, newznabAPIKey))
			}

			configured, err := buildConfiguredProviders(st)
			if err != nil {
				fmt.Printf("⚠ chargement des indexeurs configurés : %v\n", err)
			} else {
				providers = append(providers, configured...)
			}

			engine := search.NewEngine(providers)
			result := engine.Search(cmd.Context(), search.Query{
				Keywords:   keywords,
				Categories: categories,
				SearchType: searchType,
				Season:     season,
				Episode:    episode,
				IMDbID:     imdbID,
				TMDBID:     tmdbID,
			})

			for _, perr := range result.Errors {
				fmt.Printf("⚠ %s\n", perr.Error())
			}

			if len(result.Releases) == 0 {
				fmt.Println("Aucun résultat.")
				return nil
			}

			saved, err := st.SaveResults(result.Releases, 30*time.Minute)
			if err != nil {
				return fmt.Errorf("persisting search results: %w", err)
			}

			printResults(saved, jsonOutput)
			return nil
		},
	}

	cmd.Flags().StringVar(&newznabURL, "newznab-url", "", "URL de base d'un indexeur Newznab générique (ex: https://example.com)")
	cmd.Flags().StringVar(&newznabAPIKey, "newznab-apikey", "", "Clé API de l'indexeur Newznab générique")
	cmd.Flags().StringVar(&newznabName, "newznab-name", "", "Nom affiché de l'indexeur Newznab générique")
	cmd.Flags().BoolVar(&jsonOutput, "json", false, "Sortie au format JSON")
	cmd.Flags().StringVar(&searchType, "type", "search", "Type de recherche (search, tvsearch, movie, music, book)")
	cmd.Flags().IntSliceVar(&categories, "category", nil, "Filtrer par catégorie(s) Newznab (repeatable)")
	cmd.Flags().IntVar(&season, "season", 0, "Numéro de saison (pour tvsearch)")
	cmd.Flags().IntVar(&episode, "episode", 0, "Numéro d'épisode (pour tvsearch)")
	cmd.Flags().StringVar(&imdbID, "imdb-id", "", "ID IMDB (pour movie/tvsearch)")
	cmd.Flags().StringVar(&tmdbID, "tmdb-id", "", "ID TMDB (pour movie)")

	return cmd
}

// buildConfiguredProviders construit un search.Provider Cardigann pour
// chaque indexeur activé (`gowlarr indexer add`), avec session
// authentifiée si la définition l'exige (Slice C/E).
func buildConfiguredProviders(st *store.Store) ([]search.Provider, error) {
	configs, err := st.ListIndexerConfigs(true, nil)
	if err != nil {
		return nil, fmt.Errorf("listing indexer configs: %w", err)
	}

	providers := make([]search.Provider, 0, len(configs))
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

		providers = append(providers, cardigannadapter.NewProvider(cardigannengine.NewAuthenticatedProvider(def, def.Links[0], cfg.Settings, client)))
	}
	return providers, nil
}
