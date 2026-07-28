package store

import (
	"testing"
)

func TestGetDefinitionYAML_Fallback(t *testing.T) {
	st := openTestStore(t)

	// Save a definition in v10 only
	if err := st.SaveDefinition("test-def", "v10", "sha1", "yaml-content-v10"); err != nil {
		t.Fatalf("SaveDefinition: %v", err)
	}

	// Try to get it - should fall back to v10
	raw, version, err := st.GetDefinitionYAMLFallback("test-def")
	if err != nil {
		t.Fatalf("GetDefinitionYAMLFallback: %v", err)
	}
	if version != "v10" {
		t.Errorf("version = %q, want %q", version, "v10")
	}
	if raw != "yaml-content-v10" {
		t.Errorf("raw = %q, want %q", raw, "yaml-content-v10")
	}
}

func TestGetDefinitionYAML_Fallback_Priority(t *testing.T) {
	st := openTestStore(t)

	// Save definitions in multiple versions
	if err := st.SaveDefinition("multi-def", "v11", "sha11", "yaml-v11"); err != nil {
		t.Fatalf("SaveDefinition v11: %v", err)
	}
	if err := st.SaveDefinition("multi-def", "v10", "sha10", "yaml-v10"); err != nil {
		t.Fatalf("SaveDefinition v10: %v", err)
	}

	// Should prefer v11
	_, version, err := st.GetDefinitionYAMLFallback("multi-def")
	if err != nil {
		t.Fatalf("GetDefinitionYAMLFallback: %v", err)
	}
	if version != "v11" {
		t.Errorf("version = %q, want %q", version, "v11")
	}
}

func TestGetDefinitionYAML_Fallback_NotFound(t *testing.T) {
	st := openTestStore(t)

	_, _, err := st.GetDefinitionYAMLFallback("nonexistent")
	if err == nil {
		t.Fatal("expected error for nonexistent definition")
	}
}
