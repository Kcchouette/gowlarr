package cli

import (
	"github.com/Kcchouette/cardigann-go/definition"
)

// requiredSettings returns the settings that must be supplied before the
// definition is usable: text/password settings without a Default, in
// definition order, skipping keys already present in provided (presence
// counts even with an empty value — consistent with --setting key=).
// Settings with a Default are injected by the engine at search time
// (configOnce), so they are never prompted for.
func requiredSettings(def definition.IndexerDefinition, provided map[string]string) []definition.Setting {
	required := make([]definition.Setting, 0, len(def.Settings))
	for _, s := range def.Settings {
		if (s.Type == "text" || s.Type == "password") && s.Default == "" {
			if _, ok := provided[s.Name]; !ok {
				required = append(required, s)
			}
		}
	}
	return required
}
