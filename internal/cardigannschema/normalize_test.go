package cardigannschema

import (
	"testing"

	"gopkg.in/yaml.v3"
)

func TestDetectVersion_V11(t *testing.T) {
	yaml := []byte(`
id: test
name: Test Indexer
login:
  test:
    selector: "div.logged-in"
download:
  selectors:
    - selector: "a.download"
`)
	got := DetectVersion(yaml)
	if got != "v11" {
		t.Errorf("DetectVersion() = %q, want %q", got, "v11")
	}
}

func TestDetectVersion_V10(t *testing.T) {
	yaml := []byte(`
id: test
name: Test Indexer
login:
  test: "div.logged-in"
  selectorinputs:
    username:
      selector: "input[name=username]"
`)
	got := DetectVersion(yaml)
	if got != "v10" {
		t.Errorf("DetectVersion() = %q, want %q", got, "v10")
	}
}

func TestDetectVersion_Unknown(t *testing.T) {
	yaml := []byte(`
id: test
name: Test Indexer
`)
	got := DetectVersion(yaml)
	if got != "v11" {
		t.Errorf("DetectVersion() = %q, want %q", got, "v11")
	}
}

func TestNormalize_V10toV11(t *testing.T) {
	input := []byte(`
id: test
login:
  test: "div.logged-in"
search:
  paths:
    - path: "/search?q={{.Keywords}}"
`)
	got, err := NormalizeToV11(input, "v10")
	if err != nil {
		t.Fatalf("NormalizeToV11: %v", err)
	}

	var doc map[string]interface{}
	if err := yaml.Unmarshal(got, &doc); err != nil {
		t.Fatalf("parsing result: %v", err)
	}

	login := doc["login"].(map[string]interface{})
	test := login["test"].(map[string]interface{})
	if test["selector"] != "div.logged-in" {
		t.Errorf("login.test.selector = %q, want %q", test["selector"], "div.logged-in")
	}
}

func TestNormalize_V11Passthrough(t *testing.T) {
	input := []byte(`
id: test
name: Test
`)
	got, err := NormalizeToV11(input, "v11")
	if err != nil {
		t.Fatalf("NormalizeToV11: %v", err)
	}

	if string(got) != string(input) {
		t.Errorf("v11 should pass through unchanged")
	}
}

func TestNormalize_InvalidYAML(t *testing.T) {
	input := []byte(`{{{invalid yaml`)
	_, err := NormalizeToV11(input, "v10")
	if err == nil {
		t.Fatal("expected error for invalid YAML")
	}
}
