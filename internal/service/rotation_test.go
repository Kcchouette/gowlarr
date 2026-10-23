package service

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Kcchouette/cardigann-go/definition"
	"gopkg.in/yaml.v3"
)

// withLocalDefs points LocalDefsDir discovery at a temp dir via the
// working directory (LocalDefsDir checks ./definitions-local first).
func withLocalDefs(t *testing.T, files map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	local := filepath.Join(dir, "definitions-local")
	if err := os.MkdirAll(local, 0o755); err != nil {
		t.Fatal(err)
	}
	for name, content := range files {
		if err := os.WriteFile(filepath.Join(local, name), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(wd) })
	return local
}

const fixtureYAML = `# Site rotates domains; put the current origin first in links.
id: rotfix
name: RotFix
description: rotation fixture
language: fr-FR
type: ddl
links:
  - https://example.org/
search:
  paths:
    - path: /
      response:
        type: html
  rows:
    selector: div.x
  fields:
    title:
      text: t
`

func TestRewriteLinksFirstEntry_LegacyLinksBlock(t *testing.T) {
	newRaw, ok := rewriteLinksFirstEntry(fixtureYAML, "https://example.com")
	if !ok {
		t.Fatal("rewrite refused")
	}
	if !strings.Contains(newRaw, "# Site rotates domains; put the current origin first in links.") {
		t.Error("header comment lost")
	}
	if !strings.Contains(newRaw, "  - https://example.com/") {
		t.Errorf("links[0] not rewritten:\n%s", newRaw)
	}
	if strings.Contains(newRaw, "rotated-from") {
		t.Errorf("no rotated-from trail expected:\n%s", newRaw)
	}
	// Exact legacylinks block shape pinned (insert before the next root key).
	if !strings.Contains(newRaw, "legacylinks:\n  - https://example.org/\nsearch:") {
		t.Errorf("legacylinks block shape wrong:\n%s", newRaw)
	}
	def, err := definition.Parse([]byte(newRaw))
	if err != nil {
		t.Fatalf("rewritten YAML does not parse: %v", err)
	}
	if len(def.Links) == 0 || def.Links[0] != "https://example.com/" {
		t.Errorf("parsed links[0] = %v", def.Links)
	}
	if len(def.LegacyLinks) != 1 || def.LegacyLinks[0] != "https://example.org/" {
		t.Errorf("parsed legacylinks = %v", def.LegacyLinks)
	}
}

func TestRewriteLinksFirstEntry_QuotedLinkPreserved(t *testing.T) {
	in := strings.Replace(fixtureYAML, "- https://example.org/", `- "https://example.org/"`, 1)
	newRaw, ok := rewriteLinksFirstEntry(in, "https://example.com")
	if !ok {
		t.Fatal("rewrite refused")
	}
	if !strings.Contains(newRaw, `- "https://example.com/"`) {
		t.Errorf("quoting not preserved:\n%s", newRaw)
	}
	if !strings.Contains(newRaw, "legacylinks:") {
		t.Errorf("legacylinks missing:\n%s", newRaw)
	}
}

func TestRewriteLinksFirstEntry_FlowLinksRefused(t *testing.T) {
	in := "id: f\nlinks: [https://example.org/]\n"
	if _, ok := rewriteLinksFirstEntry(in, "https://example.com"); ok {
		t.Fatal("flow-style links must refuse to write")
	}
}

func TestRewriteLinksFirstEntry_UserCommentKept(t *testing.T) {
	in := strings.Replace(fixtureYAML,
		"- https://example.org/",
		"- https://example.org/  # my pinned origin",
		1)
	newRaw, ok := rewriteLinksFirstEntry(in, "https://example.com")
	if !ok {
		t.Fatal("rewrite refused")
	}
	if !strings.Contains(newRaw, "# my pinned origin") {
		t.Errorf("existing line comment lost:\n%s", newRaw)
	}
	if strings.Contains(newRaw, "rotated-from") {
		t.Errorf("no note should be added:\n%s", newRaw)
	}
}

func TestRewriteLinksFirstEntry_RotatedFromStripped(t *testing.T) {
	cases := []struct{ name, in, wantRest string }{
		{"note only", "- https://example.org/ # rotated-from: https://a.example/", ""},
		{"user pin join", "- https://example.org/ # my pin | rotated-from: https://a.example/", " # my pin"},
		{"user comment", "- https://example.org/ # my pin", " # my pin"},
		{"pipe suffix preserved", "- https://example.org/ # a| | rotated-from: https://a.example/", " # a|"},
		{"double note", "- https://example.org/ # a | rotated-from: X | rotated-from: Y", " # a"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			in := strings.Replace(fixtureYAML, "- https://example.org/", tc.in, 1)
			newRaw, ok := rewriteLinksFirstEntry(in, "https://example.com")
			if !ok {
				t.Fatal("rewrite refused")
			}
			rewrittenLine := ""
			for _, l := range strings.Split(newRaw, "\n") {
				if strings.Contains(l, "https://example.com/") {
					rewrittenLine = l
					break
				}
			}
			if rewrittenLine == "" {
				t.Fatalf("rewritten line not found:\n%s", newRaw)
			}
			rest := ""
			// Rest = everything after the (unquoted) token.
			unquoted := "https://example.com/"
			if i := strings.Index(rewrittenLine, unquoted); i >= 0 {
				rest = rewrittenLine[i+len(unquoted):]
			}
			if rest != tc.wantRest {
				t.Errorf("rest = %q, want %q\nline: %s", rest, tc.wantRest, rewrittenLine)
			}
			if strings.Contains(newRaw, "rotated-from") {
				t.Errorf("note not stripped:\n%s", newRaw)
			}
		})
	}
}

func TestRewriteLinksFirstEntry_LegacyLinksAppend(t *testing.T) {
	in := strings.Replace(fixtureYAML,
		"search:",
		"legacylinks:\n  - https://a.example/\n  - https://b.example/\nsearch:",
		1)
	newRaw, ok := rewriteLinksFirstEntry(in, "https://example.com")
	if !ok {
		t.Fatal("rewrite refused")
	}
	if !strings.Contains(newRaw, "legacylinks:\n  - https://a.example/\n  - https://b.example/\n  - https://example.org/\nsearch:") {
		t.Errorf("legacylinks append wrong:\n%s", newRaw)
	}
	def, err := definition.Parse([]byte(newRaw))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(def.LegacyLinks) != 3 {
		t.Errorf("legacylinks = %v", def.LegacyLinks)
	}
}

func TestRewriteLinksFirstEntry_LegacyLinksDedup(t *testing.T) {
	in := strings.Replace(fixtureYAML,
		"search:",
		"legacylinks:\n  - https://example.org/\nsearch:",
		1)
	newRaw, ok := rewriteLinksFirstEntry(in, "https://example.com")
	if !ok {
		t.Fatal("rewrite refused")
	}
	if strings.Count(newRaw, "https://example.org/") != 1 {
		t.Errorf("duplicate legacy entry:\n%s", newRaw)
	}
	if !strings.Contains(newRaw, "- https://example.com/") {
		t.Errorf("links[0] rewrite missing:\n%s", newRaw)
	}
}

func TestRewriteLinksFirstEntry_LegacyLinksFlow(t *testing.T) {
	in := strings.Replace(fixtureYAML,
		"search:",
		"legacylinks: [https://a.example/, https://b.example/]\nsearch:",
		1)
	newRaw, ok := rewriteLinksFirstEntry(in, "https://example.com")
	if !ok {
		t.Fatal("rewrite refused")
	}
	if !strings.Contains(newRaw, "legacylinks: [https://a.example/, https://b.example/, https://example.org/]") {
		t.Errorf("flow append wrong:\n%s", newRaw)
	}
	def, err := definition.Parse([]byte(newRaw))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(def.LegacyLinks) != 3 {
		t.Errorf("legacylinks = %v", def.LegacyLinks)
	}

	// A "]" inside a trailing comment must not be mistaken for the closer.
	inComment := strings.Replace(fixtureYAML,
		"search:",
		"legacylinks: [https://a.example/] # see [docs]\nsearch:",
		1)
	newRaw3, ok := rewriteLinksFirstEntry(inComment, "https://example.com")
	if !ok {
		t.Fatal("rewrite refused")
	}
	if !strings.Contains(newRaw3, "legacylinks: [https://a.example/, https://example.org/] # see [docs]") {
		t.Errorf("comment bracket defeated the search:\n%s", newRaw3)
	}

	// IPv6 item defeats the bracket search: history skipped, rewrite kept.
	// The item must be quoted or the bare "[" starts a nested flow seq.
	inIPv6 := strings.Replace(fixtureYAML,
		"search:",
		"legacylinks: [\"https://[::1]/\"]\nsearch:",
		1)
	newRaw2, ok := rewriteLinksFirstEntry(inIPv6, "https://example.com")
	if !ok {
		t.Fatal("rewrite refused")
	}
	if !strings.Contains(newRaw2, `legacylinks: ["https://[::1]/"]`) {
		t.Errorf("flow should be untouched for IPv6 items:\n%s", newRaw2)
	}
	if !strings.Contains(newRaw2, "- https://example.com/") {
		t.Errorf("links[0] rewrite missing on IPv6 guard:\n%s", newRaw2)
	}
}

func TestRewriteLinksFirstEntry_LegacyLinksFlowEmpty(t *testing.T) {
	in := strings.Replace(fixtureYAML, "search:", "legacylinks: []\nsearch:", 1)
	newRaw, ok := rewriteLinksFirstEntry(in, "https://example.com")
	if !ok {
		t.Fatal("rewrite refused")
	}
	if !strings.Contains(newRaw, "legacylinks: [https://example.org/]") {
		t.Errorf("empty flow insert wrong:\n%s", newRaw)
	}
}

func TestRewriteLinksFirstEntry_LegacyLinksNonSequence(t *testing.T) {
	in := strings.Replace(fixtureYAML, "search:", "legacylinks:\n  foo: bar\nsearch:", 1)
	newRaw, ok := rewriteLinksFirstEntry(in, "https://example.com")
	if !ok {
		t.Fatal("rewrite must still apply")
	}
	if !strings.Contains(newRaw, "- https://example.com/") {
		t.Errorf("links[0] rewrite missing:\n%s", newRaw)
	}
	if !strings.Contains(newRaw, "legacylinks:\n  foo: bar") {
		t.Errorf("non-sequence legacylinks must be left alone:\n%s", newRaw)
	}
	// Still valid YAML (cardigann definition.Parse would reject the wrong
	// legacylinks type, but the raw file must remain well-formed).
	var probe map[string]any
	if err := yaml.Unmarshal([]byte(newRaw), &probe); err != nil {
		t.Errorf("YAML must stay well-formed: %v", err)
	}
}

func TestRewriteLinksFirstEntry_LegacyLinksAtEOF(t *testing.T) {
	// links is the last root key; file ends with a newline.
	in := "id: eof\nlinks:\n  - https://example.org/\n"
	newRaw, ok := rewriteLinksFirstEntry(in, "https://example.com")
	if !ok {
		t.Fatal("rewrite refused")
	}
	if !strings.Contains(newRaw, "links:\n  - https://example.com/\nlegacylinks:\n  - https://example.org/\n") {
		t.Errorf("EOF block shape wrong:\n%s", newRaw)
	}
	if !strings.HasSuffix(newRaw, "\n") {
		t.Errorf("trailing newline lost:\n%q", newRaw)
	}
	if strings.Contains(newRaw, "\n\n") {
		t.Errorf("unexpected blank line:\n%q", newRaw)
	}
}

func TestRewriteLinksFirstEntry_LegacyLinksGrowOnSecondRotation(t *testing.T) {
	first, ok := rewriteLinksFirstEntry(fixtureYAML, "https://example.com")
	if !ok {
		t.Fatal("first rewrite refused")
	}
	second, ok := rewriteLinksFirstEntry(first, "https://example.net")
	if !ok {
		t.Fatal("second rewrite refused")
	}
	if !strings.Contains(second, "- https://example.net/") {
		t.Errorf("second rotation did not replace origin:\n%s", second)
	}
	def, err := definition.Parse([]byte(second))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(def.LegacyLinks) != 2 ||
		def.LegacyLinks[0] != "https://example.org/" ||
		def.LegacyLinks[1] != "https://example.com/" {
		t.Errorf("legacylinks history = %v, want [org, com]", def.LegacyLinks)
	}
}

func TestMaybePersistRotation_LegacyLinksEndToEnd(t *testing.T) {
	local := withLocalDefs(t, map[string]string{"rotfix.yml": fixtureYAML})
	st := openTestStore(t)
	if err := st.SaveDefinition("rotfix", "local", "", fixtureYAML); err != nil {
		t.Fatal(err)
	}

	maybePersistRotation(st, "rotfix", "https://example.org/", "https://example.com/")

	raw, err := os.ReadFile(filepath.Join(local, "rotfix.yml"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), "https://example.com/") {
		t.Errorf("file not rewritten:\n%s", raw)
	}
	if !strings.Contains(string(raw), "legacylinks:\n  - https://example.org/") {
		t.Errorf("legacylinks missing in file:\n%s", raw)
	}
	cached, err := st.GetDefinitionYAML("rotfix", "local")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(cached, "https://example.com/") || !strings.Contains(cached, "legacylinks:") {
		t.Errorf("DB cache not updated:\n%s", cached)
	}
}

func TestMaybePersistRotation_PrivateIPRejected(t *testing.T) {
	withLocalDefs(t, map[string]string{"rotfix.yml": fixtureYAML})
	st := openTestStore(t)
	maybePersistRotation(st, "rotfix", "https://example.org/", "http://127.0.0.1/")
	raw, _ := os.ReadFile(filepath.Join(LocalDefsDir(), "rotfix.yml"))
	if strings.Contains(string(raw), "127.0.0.1") {
		t.Errorf("private IP persisted:\n%s", raw)
	}
}

func TestMaybePersistRotation_HostileHostRejected(t *testing.T) {
	withLocalDefs(t, map[string]string{"rotfix.yml": fixtureYAML})
	st := openTestStore(t)
	maybePersistRotation(st, "rotfix", "https://example.org/", `https://evil".example/`)
	raw, _ := os.ReadFile(filepath.Join(LocalDefsDir(), "rotfix.yml"))
	if strings.Contains(string(raw), "evil") {
		t.Errorf("hostile host persisted:\n%s", raw)
	}
}

func TestMaybePersistRotation_NoLocalFileNoop(t *testing.T) {
	withLocalDefs(t, nil) // empty definitions-local
	st := openTestStore(t)
	// Must not panic or error.
	maybePersistRotation(st, "upstreamdef", "https://example.org/", "https://example.com/")
}

func TestMaybePersistRotation_EqualOriginsNoop(t *testing.T) {
	local := withLocalDefs(t, map[string]string{"rotfix.yml": fixtureYAML})
	st := openTestStore(t)
	maybePersistRotation(st, "rotfix", "https://example.org/", "https://example.org/")
	raw, _ := os.ReadFile(filepath.Join(local, "rotfix.yml"))
	if string(raw) != fixtureYAML {
		t.Error("equal origins must not touch the file")
	}
}

func TestMaybePersistRotation_DefaultPortCanonical(t *testing.T) {
	local := withLocalDefs(t, map[string]string{"rotfix.yml": fixtureYAML})
	st := openTestStore(t)
	// https://example.org:443 ≡ https://old.example → no rotation.
	maybePersistRotation(st, "rotfix", "https://example.org/", "https://example.org:443/")
	raw, _ := os.ReadFile(filepath.Join(local, "rotfix.yml"))
	if string(raw) != fixtureYAML {
		t.Error("default-port-only difference must not rotate")
	}
}

func TestFindLocalDefinition_MatchesByIDNotFilename(t *testing.T) {
	withLocalDefs(t, map[string]string{"weird-name.yml": fixtureYAML})
	path, raw, ok := findLocalDefinition("rotfix")
	if !ok {
		t.Fatal("expected to find def by parsed id")
	}
	if filepath.Base(path) != "weird-name.yml" {
		t.Errorf("path = %q", path)
	}
	if !strings.Contains(raw, "id: rotfix") {
		t.Error("wrong file content")
	}
}

func TestCanonicalOrigin(t *testing.T) {
	cases := []struct {
		in   string
		want string
		ok   bool
	}{
		{"https://example.org/", "https://example.org", true},
		{"https://example.org:443/x", "https://example.org", true},
		{"http://example.org:80/", "http://example.org", true},
		{"HTTP://Example.org/", "http://example.org", true},
		{"not-a-url", "", false},
		{"", "", false},
	}
	for _, tc := range cases {
		got, ok := canonicalOrigin(tc.in)
		if ok != tc.ok || got != tc.want {
			t.Errorf("canonicalOrigin(%q) = (%q,%v), want (%q,%v)", tc.in, got, ok, tc.want, tc.ok)
		}
	}
}
