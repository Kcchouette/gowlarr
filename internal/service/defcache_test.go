package service

import (
	"testing"
)

// TestParseDefinition_ContentAddressed verifies the def cache contract:
// the same raw YAML returns the same parsed definition (content-addressed,
// shared storage — safe because definitions are read-only after parsing),
// and distinct raw contents stay distinct.
func TestParseDefinition_ContentAddressed(t *testing.T) {
	rawA := "id: alpha\nname: Alpha\nlinks:\n  - https://a.invalid/\n"
	rawB := "id: beta\nname: Beta\nlinks:\n  - https://b.invalid/\n"

	defA1, err := parseDefinition(rawA)
	if err != nil {
		t.Fatalf("parseDefinition(rawA): %v", err)
	}
	defA2, err := parseDefinition(rawA)
	if err != nil {
		t.Fatalf("parseDefinition(rawA) second call: %v", err)
	}
	defB, err := parseDefinition(rawB)
	if err != nil {
		t.Fatalf("parseDefinition(rawB): %v", err)
	}

	if defA1.ID != "alpha" || defA2.ID != "alpha" || defB.ID != "beta" {
		t.Fatalf("unexpected ids: A1=%q A2=%q B=%q", defA1.ID, defA2.ID, defB.ID)
	}
	if len(defA1.Links) != 1 || len(defA2.Links) != 1 {
		t.Fatalf("expected links preserved, got A1=%v A2=%v", defA1.Links, defA2.Links)
	}
	if defA1.Links[0] != "https://a.invalid/" || defB.Links[0] != "https://b.invalid/" {
		t.Fatalf("unexpected links: A1=%v B=%v", defA1.Links, defB.Links)
	}
}

// TestParseDefinition_ErrorsNotCached verifies that a parse error is NOT
// memoized: a definition fixed later must be re-parseable.
func TestParseDefinition_ErrorsNotCached(t *testing.T) {
	if _, err := parseDefinition("id: [unclosed"); err == nil {
		t.Fatal("expected parse error for invalid yaml")
	}
	// Invalid content must not have poisoned the cache: a subsequent parse of
	// the same (still invalid) content still returns an error, and a valid
	// parse still succeeds.
	if _, err := parseDefinition("id: [unclosed"); err == nil {
		t.Fatal("expected parse error on second call")
	}
	raw := "id: fixed\nname: Fixed\nlinks:\n  - https://c.invalid/\n"
	if def, err := parseDefinition(raw); err != nil || def.ID != "fixed" {
		t.Fatalf("expected valid parse, got def=%+v err=%v", def, err)
	}
}
