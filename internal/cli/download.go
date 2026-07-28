package cli

import (
	"fmt"
	"os"

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
		Short: "Télécharger/résoudre le fichier réel d'un résultat de recherche",
		Long: `Récupère automatiquement le bon fichier (.torrent, lien magnet, ou .nzb)
selon le protocole détecté du résultat sélectionné — vous n'avez pas à
préciser vous-même le protocole. L'ID provient d'un précédent "gowlarr search".`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			var resultID int64
			if _, err := fmt.Sscanf(args[0], "%d", &resultID); err != nil {
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
			fmt.Printf("Téléchargé : %s\n", path)
			return nil
		},
	}

	cmd.Flags().StringVarP(&outputPath, "output", "o", "", "Chemin du fichier de sortie (défaut: nom dérivé du titre)")
	cmd.Flags().BoolVar(&toStdout, "stdout", false, "Écrire le contenu sur la sortie standard")

	return cmd
}
