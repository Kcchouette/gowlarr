package cli

import (
	"fmt"
	"io"
	"log/slog"
	"net/url"
	"strings"

	"github.com/Kcchouette/cardigann-go/definition"
	"github.com/spf13/cobra"

	"github.com/Kcchouette/gowlarr/internal/service"
	"github.com/Kcchouette/gowlarr/internal/store"
)

func newIndexerCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "indexer",
		Short: "Manage configured indexers (persistent instances of a definition)",
	}
	cmd.AddCommand(newIndexerAddCmd())
	cmd.AddCommand(newIndexerListCmd())
	cmd.AddCommand(newIndexerRemoveCmd())
	cmd.AddCommand(newIndexerTestCmd())
	cmd.AddCommand(newIndexerEnableCmd(true))
	cmd.AddCommand(newIndexerEnableCmd(false))
	return cmd
}

func newIndexerAddCmd() *cobra.Command {
	var (
		id       string
		version  string
		proxyURL string
		settings []string
	)
	cmd := &cobra.Command{
		Use:   "add <query>",
		Short: "Add an indexer instance (by definition ID, domain, or name)",
		Long: `Add an indexer instance from a cached definition.
The query can be a definition ID (e.g. "1337x"), a domain (e.g. "abn.lol"),
or a partial name (e.g. "abnormal"). The system resolves it automatically.
The reserved aliases "newznab", "nzb" and "usenet" add a native Newznab
indexer (base URL, optional API key) instead.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			st, cfg, err := openStore()
			if err != nil {
				return err
			}
			defer st.Close()

			settingsMap, err := parseSettingFlags(settings)
			if err != nil {
				return err
			}

			svc := service.NewIndexerService(st, cfg)
			errw := cmd.ErrOrStderr() // prompts and summary are diagnostics: keep stdout clean

			// Reserved aliases add a native Newznab indexer, resolved before
			// any definition lookup (there is no cached definition for it).
			// Only baseurl is required: Spotweb-style public endpoints omit
			// the apikey parameter entirely.
			if isNewznabAlias(args[0]) {
				def := newznabSyntheticDefinition()
				if settingsMap, err = promptNewznabSettings(def, settingsMap, errw); err != nil {
					return err
				}
				instanceID := id
				if instanceID == "" {
					instanceID = newznabDefaultID(settingsMap["baseurl"])
				}
				if err := svc.AddNewznab(instanceID, settingsMap["baseurl"], settingsMap["apikey"], proxyURL); err != nil {
					return err
				}
				fmt.Printf("Indexer %q added (native Newznab).\n", instanceID)
				return nil
			}

			// Resolve the query to a definition ID.
			resolved, err := svc.ResolveDefinition(args[0], version)
			if err != nil {
				return err
			}
			if len(resolved) == 0 {
				return fmt.Errorf("no definition found for %q; run `gowlarr defs sync` and retry", args[0])
			}

			best := resolved[0]
			if best.Score < 60 {
				fmt.Printf("Multiple matches for %q:\n", args[0])
				limit := len(resolved)
				if limit > 5 {
					limit = 5
				}
				for _, r := range resolved[:limit] {
					domain := r.Domain
					if domain == "" {
						domain = "-"
					}
					fmt.Printf("  %-25s %-20s %s\n", r.ID, r.Name, domain)
				}
				return fmt.Errorf("be more specific: use the exact definition ID or a more precise domain")
			}

			defID := best.ID
			instanceID := id
			if instanceID == "" {
				if best.Domain != "" {
					instanceID = best.Domain
				} else {
					instanceID = defID
				}
			}
			fmt.Printf("Resolved %q → definition %q (%s)\n", args[0], defID, best.Name)

			// Load the definition with exactly the version the service will
			// persist (best.Version), so the prompted settings match what
			// the engine sees.
			def, err := service.NewDefinitionService(st, cfg).Load(defID, best.Version)
			if err != nil {
				return err
			}

			missing := requiredSettings(def, settingsMap)
			if len(missing) > 0 {
				if settingsMap, err = promptMissingSettings(def, settingsMap, errw); err != nil {
					return err
				}
			} else {
				printSettingsSummary(def, settingsMap, errw)
			}

			if err := svc.Add(defID, instanceID, best.Version, proxyURL, settingsMap); err != nil {
				return err
			}

			fmt.Printf("Indexer %q added (definition %q).\n", instanceID, defID)
			return nil
		},
	}
	cmd.Flags().StringVar(&id, "id", "", "Instance ID (default: same as definition)")
	cmd.Flags().StringVar(&version, "version", "", "Definition schema version (default: local then v11)")
	cmd.Flags().StringVar(&proxyURL, "proxy", "", "Dedicated proxy URL (http://... or socks5://...)")
	cmd.Flags().StringArrayVar(&settings, "setting", nil, "Setting in key=value format (repeatable)")
	return cmd
}

func parseSettingFlags(settings []string) (map[string]string, error) {
	out := make(map[string]string, len(settings))
	for _, s := range settings {
		parts := strings.SplitN(s, "=", 2)
		if len(parts) != 2 {
			return nil, fmt.Errorf("invalid --setting %q (expected key=value)", s)
		}
		out[parts[0]] = parts[1]
	}
	return out, nil
}

// promptMissingSettings asks interactively (or errors in non-tty mode) for
// the definition's required settings not yet provided, then prints the
// settings summary. Returns the final settings map.
func promptMissingSettings(def definition.IndexerDefinition, settingsMap map[string]string, errw io.Writer) (map[string]string, error) {
	missing := requiredSettings(def, settingsMap)
	if len(missing) > 0 && !stdinIsTerminal() {
		names := make([]string, 0, len(missing))
		for _, s := range missing {
			names = append(names, s.Name)
		}
		// The definition's info texts are the help the user needs
		// (e.g. where to find the API key) — show them in the error.
		for _, s := range def.Settings {
			if s.Type == "info" && s.Default != "" {
				fmt.Fprintln(errw, s.Default)
			}
		}
		return nil, fmt.Errorf("definition %q requires settings: %s (provide them with --setting key=value)", def.ID, strings.Join(names, ", "))
	}
	if len(missing) > 0 {
		var err error
		if settingsMap, err = runPrompts(def, settingsMap, newStdinPrompter(errw), errw); err != nil {
			return nil, err
		}
	}
	printSettingsSummary(def, settingsMap, errw)
	return settingsMap, nil
}

// newznabSyntheticDefinition is the in-memory definition backing native
// Newznab adds: it drives the settings prompts (required baseurl + optional
// apikey) without any cached Cardigann definition. apikey stays declared so
// printSettingsSummary masks it when provided.
func newznabSyntheticDefinition() definition.IndexerDefinition {
	return definition.IndexerDefinition{
		ID:   store.NewznabDefinitionID,
		Name: "Newznab (generic usenet)",
		Type: "usenet",
		Settings: []definition.Setting{
			{Name: "baseurl", Type: "text", Label: "Base URL"},
			{Name: "apikey", Type: "password", Label: "API Key"},
			{Name: "info", Type: "info", Default: "Base URL: the indexer's API endpoint root, e.g. https://api.nzbgeek.info/. Indexers exposing the API under a custom path (e.g. https://example.invalid/api/v1/newznab) take the full path. API key: your indexer profile/API settings page — optional for open Spotweb instances (leave empty; the parameter is then omitted)."},
		},
	}
}

// isNewznabAlias reports whether the query is a reserved alias for the
// native Newznab indexer (exact, case-insensitive match). Intentionally
// narrow: any real definition (e.g. "usenet-crawler") is resolved normally.
func isNewznabAlias(query string) bool {
	switch strings.ToLower(strings.TrimSpace(query)) {
	case "newznab", "nzb", "usenet":
		return true
	}
	return false
}

// newznabDefaultID returns the default instance ID for a Newznab base URL
// (its hostname), falling back to "newznab" when unparseable.
func newznabDefaultID(baseURL string) string {
	u, err := url.Parse(baseURL)
	if err != nil || u.Hostname() == "" {
		return store.NewznabDefinitionID
	}
	return u.Hostname()
}

func newIndexerListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List configured indexers",
		RunE: func(cmd *cobra.Command, args []string) error {
			st, cfg, err := openStore()
			if err != nil {
				return err
			}
			defer st.Close()

			svc := service.NewIndexerService(st, cfg)
			configs, err := svc.List(false)
			if err != nil {
				return err
			}
			if len(configs) == 0 {
				fmt.Println("No indexers configured. Use `gowlarr indexer add <definition-id>`.")
				return nil
			}
			fmt.Printf("%-20s %-20s %-8s %s\n", "ID", "DEFINITION", "PROTO", "ENABLED")
			for _, c := range configs {
				fmt.Printf("%-20s %-20s %-8s %v\n", c.ID, c.DefinitionID, c.Protocol, c.Enabled)
			}
			return nil
		},
	}
}

func newIndexerRemoveCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "remove <id>",
		Short: "Remove a configured indexer",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			st, cfg, err := openStore()
			if err != nil {
				return err
			}
			defer st.Close()

			svc := service.NewIndexerService(st, cfg)
			if err := svc.Remove(args[0]); err != nil {
				return err
			}
			fmt.Printf("Indexer %q removed.\n", args[0])
			return nil
		},
	}
}

func newIndexerEnableCmd(enable bool) *cobra.Command {
	use, short := "enable <id>", "Enable a configured indexer"
	if !enable {
		use, short = "disable <id>", "Disable a configured indexer"
	}
	return &cobra.Command{
		Use:   use,
		Short: short,
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			st, cfg, err := openStore()
			if err != nil {
				return err
			}
			defer st.Close()

			svc := service.NewIndexerService(st, cfg)
			if err := svc.Enable(args[0], enable); err != nil {
				return err
			}
			fmt.Printf("Indexer %q updated (enabled=%v).\n", args[0], enable)
			return nil
		},
	}
}

func newIndexerTestCmd() *cobra.Command {
	var version string
	cmd := &cobra.Command{
		Use:   "test <id>",
		Short: "Test connectivity/authentication of a configured indexer",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			st, cfg, err := openStore()
			if err != nil {
				return err
			}
			defer st.Close()

			svc := service.NewIndexerService(st, cfg)
			if err := svc.Test(cmd.Context(), args[0], version); err != nil {
				slog.Error("indexer test failed", "id", args[0], "err", err)
				return err
			}
			fmt.Printf("✅ %s: OK (connection + parsing functional)\n", args[0])
			return nil
		},
	}
	cmd.Flags().StringVar(&version, "version", "", "Definition schema version (default: local then v11)")
	return cmd
}
