package cardigannschema

import (
	"strings"
)

// DetectVersion determines the schema version of a Cardigann YAML definition.
// Uses heuristics based on field presence/absence.
func DetectVersion(yaml []byte) string {
	content := string(yaml)

	// v11 indicators
	if strings.Contains(content, "download:") && strings.Contains(content, "selectors:") {
		return "v11"
	}

	// v10 indicators (login.test.selector exists but download.selectors may not)
	if strings.Contains(content, "login:") && strings.Contains(content, "test:") {
		// Check for more specific v10 patterns
		if strings.Contains(content, "selectorinputs:") || strings.Contains(content, "error:") {
			return "v10"
		}
	}

	// v9 and earlier: simpler structures
	if strings.Contains(content, "search:") {
		// If we have search but not the v10+ patterns, it's likely v9 or earlier
		if strings.Contains(content, "paths:") || strings.Contains(content, "rows:") {
			return "v9"
		}
	}

	// Default to v11 if uncertain (most common version)
	return "v11"
}

// normalizationPipeline returns the sequence of normalization functions
// needed to go from sourceVersion to v11.
func normalizationPipeline(sourceVersion string) []func(map[string]interface{}) error {
	switch sourceVersion {
	case "v10":
		return []func(map[string]interface{}) error{normalizeV10toV11}
	case "v9":
		return []func(map[string]interface{}) error{normalizeV9toV10, normalizeV10toV11}
	case "v8":
		return []func(map[string]interface{}) error{normalizeV8toV9, normalizeV9toV10, normalizeV10toV11}
	default:
		return nil
	}
}

func normalizeV10toV11(doc map[string]interface{}) error {
	// v10 -> v11: rename login.test -> login.test.selector
	if login, ok := doc["login"].(map[string]interface{}); ok {
		if test, ok := login["test"].(string); ok {
			// In v11, test is an object with selector field
			login["test"] = map[string]interface{}{
				"selector": test,
			}
		}
	}
	return nil
}

func normalizeV9toV10(doc map[string]interface{}) error {
	// v9 -> v10: add login.test if missing
	if login, ok := doc["login"].(map[string]interface{}); ok {
		if _, hasTest := login["test"]; !hasTest {
			login["test"] = ""
		}
	}
	return nil
}

func normalizeV8toV9(doc map[string]interface{}) error {
	// v8 -> v9: ensure search.paths exists
	if search, ok := doc["search"].(map[string]interface{}); ok {
		if _, hasPaths := search["paths"]; !hasPaths {
			search["paths"] = []interface{}{}
		}
	}
	return nil
}
