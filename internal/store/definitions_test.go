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

func TestListDefinitionsWithYAML_Empty(t *testing.T) {
	st := openTestStore(t)

	defs, err := st.ListDefinitionsWithYAML("")
	if err != nil {
		t.Fatalf("ListDefinitionsWithYAML: %v", err)
	}
	if len(defs) != 0 {
		t.Errorf("expected 0 definitions, got %d", len(defs))
	}
}

func TestListDefinitionsWithYAML_WithData(t *testing.T) {
	st := openTestStore(t)

	if err := st.SaveDefinition("alpha", "v11", "sha-a", "yaml-alpha"); err != nil {
		t.Fatalf("SaveDefinition alpha: %v", err)
	}
	if err := st.SaveDefinition("beta", "v11", "sha-b", "yaml-beta"); err != nil {
		t.Fatalf("SaveDefinition beta: %v", err)
	}
	if err := st.SaveDefinition("gamma", "v10", "sha-g", "yaml-gamma-v10"); err != nil {
		t.Fatalf("SaveDefinition gamma v10: %v", err)
	}

	// All versions
	defs, err := st.ListDefinitionsWithYAML("")
	if err != nil {
		t.Fatalf("ListDefinitionsWithYAML: %v", err)
	}
	if len(defs) != 3 {
		t.Fatalf("expected 3 definitions, got %d", len(defs))
	}
	// Ordered by id
	if defs[0].ID != "alpha" || defs[1].ID != "beta" || defs[2].ID != "gamma" {
		t.Errorf("unexpected order: %v %v %v", defs[0].ID, defs[1].ID, defs[2].ID)
	}
	// YAML content preserved
	if defs[0].YAML != "yaml-alpha" {
		t.Errorf("defs[0].YAML = %q, want %q", defs[0].YAML, "yaml-alpha")
	}

	// Filter by version
	defsV11, err := st.ListDefinitionsWithYAML("v11")
	if err != nil {
		t.Fatalf("ListDefinitionsWithYAML v11: %v", err)
	}
	if len(defsV11) != 2 {
		t.Fatalf("expected 2 v11 definitions, got %d", len(defsV11))
	}

	defsV10, err := st.ListDefinitionsWithYAML("v10")
	if err != nil {
		t.Fatalf("ListDefinitionsWithYAML v10: %v", err)
	}
	if len(defsV10) != 1 {
		t.Fatalf("expected 1 v10 definition, got %d", len(defsV10))
	}
	if defsV10[0].ID != "gamma" {
		t.Errorf("v10 definition ID = %q, want %q", defsV10[0].ID, "gamma")
	}
	if defsV10[0].YAML != "yaml-gamma-v10" {
		t.Errorf("v10 definition YAML = %q, want %q", defsV10[0].YAML, "yaml-gamma-v10")
	}
}
