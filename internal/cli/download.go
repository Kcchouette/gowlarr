package cli

import (
	"fmt"
	"os"
	"strconv"

	"github.com/spf13/cobra"

	"github.com/Kcchouette/gowlarr/internal/service"
)

func newDownloadCmd() *cobra.Command {
	var (
		outputPath string
		toStdout   bool
	)

	cmd := &cobra.Command{
		Use:   "download <result-id>",
		Short: "Download/resolve the actual file from a search result",
		Long: `Automatically retrieves the correct file (.torrent, magnet link, or .nzb)
based on the detected protocol of the selected result — you don't need to
specify the protocol yourself. The ID comes from a previous "gowlarr search".`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			resultID, err := strconv.ParseInt(args[0], 10, 64)
			if err != nil {
				return fmt.Errorf("invalid result id %q: %w", args[0], err)
			}

			st, cfg, err := openStore()
			if err != nil {
				return err
			}
			defer st.Close()

			svc := service.NewDownloadService(st, cfg)
			artifact, err := svc.Resolve(cmd.Context(), resultID)
			if err != nil {
				return err
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
			fmt.Printf("Downloaded: %s\n", path)
			return nil
		},
	}

	cmd.Flags().StringVarP(&outputPath, "output", "o", "", "Output file path (default: name derived from title)")
	cmd.Flags().BoolVar(&toStdout, "stdout", false, "Write content to stdout")

	return cmd
}
