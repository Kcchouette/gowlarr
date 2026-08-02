package cli

import (
	"fmt"
	"log/slog"

	"github.com/spf13/cobra"

	"github.com/Kcchouette/gowlarr/internal/model"
	"github.com/Kcchouette/gowlarr/internal/service"
)

func newSearchCmd() *cobra.Command {
	var (
		newznabURL    string
		newznabAPIKey string
		newznabName   string
		jsonOutput    bool
		linksOutput   bool
		searchType    string
		categories    []int
		season        int
		episode       int
		imdbID        string
		tmdbID        string
		indexerID     string
		protocol      string
	)

	cmd := &cobra.Command{
		Use:   "search <query>",
		Short: "Search for a release on configured indexers (torrent + usenet + ddl)",
		Long: `Queries active indexers in parallel and displays a unified numbered
list of results. Results are persisted (with expiration) to allow
"gowlarr download <id>" in a separate CLI invocation.

DDL/streaming results are display-only: use --links to show the direct
links and file hosts (no automatic download).`,
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
				IndexerID:     indexerID,
				Protocol:      model.Protocol(protocol),
			})
			if err != nil {
				return err
			}

			for _, perr := range result.Errors {
				slog.Warn("indexer", "err", perr.Error())
			}

			if len(result.Releases) == 0 {
				fmt.Println("No results.")
				return nil
			}

			printResults(result.Releases, jsonOutput, linksOutput)
			return nil
		},
	}

	cmd.Flags().StringVar(&newznabURL, "newznab-url", "", "Base URL of a generic Newznab indexer")
	cmd.Flags().StringVar(&newznabAPIKey, "newznab-apikey", "", "API key of the generic Newznab indexer")
	cmd.Flags().StringVar(&newznabName, "newznab-name", "", "Display name of the generic Newznab indexer")
	cmd.Flags().BoolVar(&jsonOutput, "json", false, "Output in JSON format")
	cmd.Flags().BoolVar(&linksOutput, "links", false, "Show the direct download links and file hosts (DDL/streaming)")
	cmd.Flags().StringVar(&searchType, "type", "search", "Search type (search, tvsearch, movie, music, book)")
	cmd.Flags().IntSliceVar(&categories, "category", nil, "Filter by Newznab category ID(s) (repeatable)")
	cmd.Flags().IntVar(&season, "season", 0, "Season number (for tvsearch)")
	cmd.Flags().IntVar(&episode, "episode", 0, "Episode number (for tvsearch)")
	cmd.Flags().StringVar(&imdbID, "imdb-id", "", "IMDb ID (for movie/tvsearch)")
	cmd.Flags().StringVar(&tmdbID, "tmdb-id", "", "TMDB ID (for movie)")
	cmd.Flags().StringVar(&indexerID, "indexer", "", "Restrict the search to one configured indexer ID")
	cmd.Flags().StringVar(&protocol, "protocol", "", "Filter results by protocol (torrent, usenet, ddl, streaming)")

	return cmd
}
