package cli

import (
	"fmt"
	"log/slog"

	"github.com/spf13/cobra"

	"github.com/Kcchouette/gowlarr/internal/service"
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
séparée du CLI.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			st, cfg, err := openStore()
			if err != nil {
				return err
			}
			defer st.Close()

			svc := service.NewSearchService(st, cfg)
			result, err := svc.Search(cmd.Context(), service.SearchParams{
				Keywords:      args[0],
				Categories:    categories,
				SearchType:    searchType,
				Season:        season,
				Episode:       episode,
				IMDbID:        imdbID,
				TMDBID:        tmdbID,
				NewznabURL:    newznabURL,
				NewznabAPIKey: newznabAPIKey,
				NewznabName:   newznabName,
			})
			if err != nil {
				return err
			}

			for _, perr := range result.Errors {
				slog.Warn("indexeur", "err", perr.Error())
			}

			if len(result.Releases) == 0 {
				fmt.Println("Aucun résultat.")
				return nil
			}

			printResults(result.Releases, jsonOutput)
			return nil
		},
	}

	cmd.Flags().StringVar(&newznabURL, "newznab-url", "", "URL de base d'un indexeur Newznab générique")
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
