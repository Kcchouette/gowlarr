package service

import (
	"testing"

	"github.com/Kcchouette/cardigann-go/definition"
)

func seedTestDefinitions(t *testing.T, svc *IndexerService) {
	t.Helper()
	defs := []struct {
		id, version, yaml string
	}{
		{"1337x", "v11", `id: 1337x
name: 1337x
type: public
links:
  - https://1337x.to
search:
  paths:
    - path: /search
      response:
        type: html
  rows:
    selector: table tbody tr
  fields:
    title:
      selector: td.name a:nth-child(2)
`},
		{"abnormal-api", "v11", `id: abnormal-api
name: Abnormal
type: semi-private
links:
  - https://abn.lol
search:
  paths:
    - path: /torrents
      response:
        type: html
  rows:
    selector: .torrent-list . torrent-item
  fields:
    title:
      selector: .name a
`},
		{"nzbgeek", "v11", `id: nzbgeek
name: NZBGeek
type: usenet
links:
  - https://nzbgeek.info
search:
  paths:
    - path: /api
      response:
        type: xml
  rows:
    selector: item
  fields:
    title:
      selector: title
`},
	}
	for _, d := range defs {
		if err := svc.store.SaveDefinition(d.id, d.version, "sha-"+d.id, d.yaml); err != nil {
			t.Fatalf("SaveDefinition %s: %v", d.id, err)
		}
	}
}

func TestResolveDefinition_EmptyStore(t *testing.T) {
	st := openTestStore(t)
	svc := NewIndexerService(st, testConfig())

	results, err := svc.ResolveDefinition("1337x", "v11")
	if err != nil {
		t.Fatalf("ResolveDefinition: %v", err)
	}
	if len(results) != 0 {
		t.Errorf("expected 0 results from empty store, got %d", len(results))
	}
}

func TestResolveDefinition_ExactDomain(t *testing.T) {
	st := openTestStore(t)
	svc := NewIndexerService(st, testConfig())
	seedTestDefinitions(t, svc)

	results, err := svc.ResolveDefinition("abn.lol", "v11")
	if err != nil {
		t.Fatalf("ResolveDefinition: %v", err)
	}
	if len(results) == 0 {
		t.Fatal("expected at least 1 result")
	}
	best := results[0]
	if best.ID != "abnormal-api" {
		t.Errorf("best.ID = %q, want %q", best.ID, "abnormal-api")
	}
	if best.Name != "Abnormal" {
		t.Errorf("best.Name = %q, want %q", best.Name, "Abnormal")
	}
	if best.Domain != "abn.lol" {
		t.Errorf("best.Domain = %q, want %q", best.Domain, "abn.lol")
	}
	if best.Score != 100 {
		t.Errorf("best.Score = %d, want 100", best.Score)
	}
}

func TestResolveDefinition_ExactID(t *testing.T) {
	st := openTestStore(t)
	svc := NewIndexerService(st, testConfig())
	seedTestDefinitions(t, svc)

	results, err := svc.ResolveDefinition("1337x", "v11")
	if err != nil {
		t.Fatalf("ResolveDefinition: %v", err)
	}
	if len(results) == 0 {
		t.Fatal("expected at least 1 result")
	}
	best := results[0]
	if best.ID != "1337x" {
		t.Errorf("best.ID = %q, want %q", best.ID, "1337x")
	}
	if best.Score < 60 {
		t.Errorf("best.Score = %d, want >= 60", best.Score)
	}
}

func TestResolveDefinition_PartialName(t *testing.T) {
	st := openTestStore(t)
	svc := NewIndexerService(st, testConfig())
	seedTestDefinitions(t, svc)

	results, err := svc.ResolveDefinition("abnorm", "v11")
	if err != nil {
		t.Fatalf("ResolveDefinition: %v", err)
	}
	if len(results) == 0 {
		t.Fatal("expected at least 1 result")
	}
	found := false
	for _, r := range results {
		if r.ID == "abnormal-api" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected abnormal-api in results, got: %v", results)
	}
}

func TestResolveDefinition_NoMatch(t *testing.T) {
	st := openTestStore(t)
	svc := NewIndexerService(st, testConfig())
	seedTestDefinitions(t, svc)

	results, err := svc.ResolveDefinition("zzzzz-never-exists", "v11")
	if err != nil {
		t.Fatalf("ResolveDefinition: %v", err)
	}
	if len(results) != 0 {
		t.Errorf("expected 0 results for nonexistent query, got %d: %v", len(results), results)
	}
}

func TestScoreMatch(t *testing.T) {
	cases := []struct {
		name  string
		query string
		def   definition.IndexerDefinition
		want  int
	}{
		{
			name:  "exact domain match",
			query: "abn.lol",
			def: definition.IndexerDefinition{
				ID:    "abnormal-api",
				Name:  "Abnormal",
				Links: []string{"https://abn.lol"},
			},
			want: 100,
		},
		{
			name:  "domain contains query",
			query: "abn",
			def: definition.IndexerDefinition{
				ID:    "abnormal-api",
				Name:  "Abnormal",
				Links: []string{"https://abn.lol"},
			},
			want: 80,
		},
		{
			name:  "exact ID match (no domain overlap)",
			query: "my-tracker",
			def: definition.IndexerDefinition{
				ID:    "my-tracker",
				Name:  "My Tracker",
				Links: []string{"https://other.example.com"},
			},
			want: 60,
		},
		{
			name:  "ID contains query (no domain overlap)",
			query: "my-track",
			def: definition.IndexerDefinition{
				ID:    "my-tracker",
				Name:  "My Tracker",
				Links: []string{"https://other.example.com"},
			},
			want: 50,
		},
		{
			name:  "name contains query",
			query: "abnormal",
			def: definition.IndexerDefinition{
				ID:    "abn-api",
				Name:  "Abnormal Tracker",
				Links: []string{"https://other.example.com"},
			},
			want: 40,
		},
		{
			name:  "no match",
			query: "zzzzz",
			def: definition.IndexerDefinition{
				ID:    "1337x",
				Name:  "1337x",
				Links: []string{"https://1337x.to"},
			},
			want: 0,
		},
		{
			name:  "empty links handled",
			query: "1337x",
			def: definition.IndexerDefinition{
				ID:    "1337x",
				Name:  "1337x",
				Links: nil,
			},
			want: 60,
		},
		{
			name:  "case insensitive",
			query: "ABNORMAL",
			def: definition.IndexerDefinition{
				ID:    "abnormal-api",
				Name:  "Abnormal",
				Links: []string{"https://abn.lol"},
			},
			want: 50, // ID contains query (case-insensitive)
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := scoreMatch(tc.query, tc.def)
			if got != tc.want {
				t.Errorf("scoreMatch(%q, ...) = %d, want %d", tc.query, got, tc.want)
			}
		})
	}
}

func TestResolveDefinition_LocalPriority(t *testing.T) {
	st := openTestStore(t)
	svc := NewIndexerService(st, testConfig())

	ddlYAML := `id: japanfan
name: JapanFan
type: ddl
links:
  - https://japanfan.org/phpBB3/
search:
  paths:
    - path: /search
      response:
        type: html
  rows:
    selector: li.row
  fields:
    title:
      selector: a.topictitle
`

	// Same definition ID in the v11 corpus and in the local definitions.
	if err := svc.store.SaveDefinition("japanfan", "v11", "sha-v11", ddlYAML); err != nil {
		t.Fatalf("SaveDefinition v11: %v", err)
	}
	if err := svc.store.SaveDefinition("japanfan", "local", "sha-local", ddlYAML); err != nil {
		t.Fatalf("SaveDefinition local: %v", err)
	}

	// Resolving without a version must pick the local definition (priority)
	// and deduplicate by ID.
	results, err := svc.ResolveDefinition("japanfan", "")
	if err != nil {
		t.Fatalf("ResolveDefinition: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 deduplicated result, got %d", len(results))
	}
	if results[0].Version != "local" {
		t.Errorf("Version = %q, want local (highest priority)", results[0].Version)
	}

	// Add without a version resolves the local definition and records ddl.
	if err := svc.Add("japanfan", "", "", "", nil); err != nil {
		t.Fatalf("Add: %v", err)
	}
	configs, err := svc.List(false)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(configs) != 1 {
		t.Fatalf("expected 1 indexer config, got %d", len(configs))
	}
	if configs[0].Protocol != "ddl" {
		t.Errorf("Protocol = %q, want ddl", configs[0].Protocol)
	}
}
