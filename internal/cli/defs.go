package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/Kcchouette/cardigann-go/definition"
	"github.com/Kcchouette/cardigann-go/defs"
	"github.com/spf13/cobra"

	"github.com/Kcchouette/gowlarr/internal/store"
)

func newDefsCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "defs",
		Short: "Manage local Cardigann definition cache (Prowlarr/Indexers)",
		Long: `Synchronize and browse Cardigann definitions. YAML files are NEVER
redistributed with Gowlarr: they are fetched on demand from the
Prowlarr/Indexers GitHub repo and cached locally for your use only.`,
	}
	cmd.AddCommand(newDefsSyncCmd())
	cmd.AddCommand(newDefsListCmd())
	cmd.AddCommand(newDefsShowCmd())
	return cmd
}

func newDefsSyncCmd() *cobra.Command {
	var version string
	cmd := &cobra.Command{
		Use:   "sync",
		Short: "Download/update Cardigann definitions from GitHub",
		RunE: func(cmd *cobra.Command, args []string) error {
			st, cfg, err := openStore()
			if err != nil {
				return err
			}
			defer st.Close()

			fetcher := defs.NewFetcher()
			if etag := loadStoredETag(cfg.DefsCacheDir); etag != "" {
				fetcher.SetIfNoneMatch(etag)
			}

			var raws []defs.RawDefinition
			if version != "" {
				raws, err = fetcher.FetchVersion(cmd.Context(), version)
				if err != nil {
					return fmt.Errorf("synchronizing definitions %s: %w", version, err)
				}
			} else {
				raws, err = fetcher.FetchAll(cmd.Context())
				if err != nil {
					return fmt.Errorf("synchronizing definitions: %w", err)
				}
			}

			if fetcher.NotModified() {
				fmt.Println("Definitions already up to date.")
			} else {
				storeETag(cfg.DefsCacheDir, fetcher.LastETag())

				for _, raw := range raws {
					v := raw.Version
					if version != "" {
						v = version
					}
					if err := st.SaveDefinition(raw.ID, v, raw.SHA, raw.YAML); err != nil {
						return err
					}
				}
			}

			// Repo-local definitions (definitions-local/*.yml) are ingested
			// regardless of the remote sync result, so `indexer add` can
			// resolve them even when the remote corpus is unchanged.
			localCount, err := ingestLocalDefinitions(st)
			if err != nil {
				return err
			}
			if localCount > 0 {
				fmt.Printf("%d local definition(s) ingested from definitions-local/.\n", localCount)
			}

			if version != "" {
				fmt.Printf("%d definition(s) %s synchronized.\n", len(raws), version)
			} else {
				byVersion := make(map[string]int)
				for _, raw := range raws {
					byVersion[raw.Version]++
				}
				for v, count := range byVersion {
					fmt.Printf("%d definition(s) %s synchronized.\n", count, v)
				}
				fmt.Printf("Total: %d definition(s) across %d version(s).\n", len(raws), len(byVersion))
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&version, "version", "", "Sync a specific version only (default: all)")
	return cmd
}

func newDefsListCmd() *cobra.Command {
	var version string
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List Cardigann definitions in local cache",
		RunE: func(cmd *cobra.Command, args []string) error {
			st, _, err := openStore()
			if err != nil {
				return err
			}
			defer st.Close()

			metas, err := st.ListDefinitions(version)
			if err != nil {
				return err
			}
			if len(metas) == 0 {
				fmt.Println("No definitions in cache. Run `gowlarr defs sync` first.")
				return nil
			}
			for _, m := range metas {
				fmt.Printf("%-30s %-6s %s\n", m.ID, m.Version, m.DownloadedAt.Format("2006-01-02 15:04"))
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&version, "version", "", "Filter by version (empty = all)")
	return cmd
}

func newDefsShowCmd() *cobra.Command {
	var version string
	cmd := &cobra.Command{
		Use:   "show <definition-id>",
		Short: "Show a Cardigann definition in cache (summary)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			st, _, err := openStore()
			if err != nil {
				return err
			}
			defer st.Close()

			raw, err := st.GetDefinitionYAML(args[0], version)
			if err != nil {
				return err
			}
			def, err := definition.Parse([]byte(raw))
			if err != nil {
				return fmt.Errorf("parsing cached definition: %w", err)
			}
			fmt.Printf("id:          %s\n", def.ID)
			fmt.Printf("name:        %s\n", def.Name)
			fmt.Printf("type:        %s\n", def.Type)
			fmt.Printf("login:       %s\n", methodOrNone(def.Login.Method))
			fmt.Printf("links:       %v\n", def.Links)
			return nil
		},
	}
	cmd.Flags().StringVar(&version, "version", "v11", "Schema version")
	return cmd
}

// localDefsDir returns the repo-local definitions directory: first
// ./definitions-local relative to the working directory, then relative to
// the executable (so an installed binary still finds the repo's local defs).
func localDefsDir() string {
	if wd, err := os.Getwd(); err == nil {
		dir := filepath.Join(wd, "definitions-local")
		if info, statErr := os.Stat(dir); statErr == nil && info.IsDir() {
			return dir
		}
	}
	if exe, err := os.Executable(); err == nil {
		dir := filepath.Join(filepath.Dir(exe), "definitions-local")
		if info, statErr := os.Stat(dir); statErr == nil && info.IsDir() {
			return dir
		}
	}
	return ""
}

// ingestLocalDefinitions upserts the repo-local definitions
// (definitions-local/*.yml) into the cache under version "local". Returns the
// number of ingested definitions.
func ingestLocalDefinitions(st *store.Store) (int, error) {
	dir := localDefsDir()
	if dir == "" {
		return 0, nil
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return 0, fmt.Errorf("reading local definitions dir: %w", err)
	}

	count := 0
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".yml") {
			continue
		}
		raw, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			return count, fmt.Errorf("reading local definition %s: %w", e.Name(), err)
		}
		def, err := definition.Parse(raw)
		if err != nil {
			return count, fmt.Errorf("parsing local definition %s: %w", e.Name(), err)
		}
		if err := st.SaveDefinition(def.ID, "local", "", string(raw)); err != nil {
			return count, err
		}
		count++
	}
	return count, nil
}

func methodOrNone(method string) string {
	if method == "" {
		return "(none)"
	}
	return method
}

// etagFilePath returns the path of the persisted defs ETag file, or "" when
// the cache dir is unset (no persistence).
func etagFilePath(dir string) string {
	if dir == "" {
		return ""
	}
	return filepath.Join(dir, ".defs-etag")
}

// loadStoredETag reads the ETag persisted by the previous sync ("" if none).
func loadStoredETag(dir string) string {
	path := etagFilePath(dir)
	if path == "" {
		return ""
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(data))
}

// storeETag persists the last ETag so the next sync can send If-None-Match
// (best effort: failures are ignored — the sync still works without it).
func storeETag(dir string, etag string) {
	if etag == "" {
		return
	}
	path := etagFilePath(dir)
	if path == "" {
		return
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return
	}
	_ = os.WriteFile(path, []byte(etag), 0o600)
}
