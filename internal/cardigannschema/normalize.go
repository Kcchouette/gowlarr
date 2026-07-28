package cardigannschema

import (
	"fmt"

	"gopkg.in/yaml.v3"
)

// NormalizeToV11 transforms a YAML definition from a lower version to v11 format.
// Returns the normalized YAML ready for parsing by cardigann-go.
func NormalizeToV11(raw []byte, sourceVersion string) ([]byte, error) {
	if sourceVersion == "v11" {
		return raw, nil
	}

	var doc map[string]interface{}
	if err := yaml.Unmarshal(raw, &doc); err != nil {
		return nil, fmt.Errorf("parsing yaml: %w", err)
	}

	pipeline := normalizationPipeline(sourceVersion)
	for _, fn := range pipeline {
		if err := fn(doc); err != nil {
			return nil, fmt.Errorf("normalizing from %s: %w", sourceVersion, err)
		}
	}

	normalized, err := yaml.Marshal(doc)
	if err != nil {
		return nil, fmt.Errorf("marshaling normalized yaml: %w", err)
	}

	return normalized, nil
}
