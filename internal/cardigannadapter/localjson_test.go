package cardigannadapter

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Kcchouette/cardigann-go/definition"
	cardigannengine "github.com/Kcchouette/cardigann-go/engine"

	"github.com/Kcchouette/gowlarr/internal/model"
	internalsearch "github.com/Kcchouette/gowlarr/internal/search"
)

// localJSONDefinitionYAML is the anonymized inline copy of the local JSON
// definition (definitions-local/, gitignored): a usenet indexer over a
// keyless JSON API returning {items: [{filename, nzb, size, date, anidb}]}
// with epoch-seconds dates and nullable fields. Inlined so the test runs in
// CI without the gitignored definitions directory. %s is replaced by the
// test server base URL. Identity fields (id/name/description/links) differ
// from the production file on purpose — no real site appears in tests.
const localJSONDefinitionYAML = `
id: localjson
name: Local JSON Indexer
description: "Local JSON usenet indexer fixture (no auth)"
language: en-US
encoding: UTF-8
type: usenet
links:
  - %s
caps:
  categorymappings:
    - {id: 5070, cat: TV/Anime, desc: "Anime"}
  modes:
    search: [q]
    tv-search: [q, season, ep]
settings: []
search:
  headers:
    Accept: ["application/json"]
  paths:
    - path: "/"
      response:
        type: json
      inputs:
        name: "{{ .Keywords }}"
        season: "{{ .Query.Season }}"
        episode: "{{ .Query.Ep }}"
  rows:
    selector: "$.items"
  fields:
    _anidb:
      selector: anidb
      optional: true
    title:
      selector: filename
    download:
      selector: nzb
      optional: true
    details:
      text: '{{ if .Result._anidb }}https://example.invalid/anime/{{ .Result._anidb }}{{ end }}'
    size:
      selector: size
    date:
      selector: date
      filters:
        - name: unixdate
    category:
      text: "5070"
`

// localJSONBody exercises the three interesting shapes: a full item, an
// item with a null optional series id, and an item without any download link.
const localJSONBody = `{"total_count":3,"items":[
  {"filename":"Example.Show.S01E01.1080p.WEB-DL-GRP","anidb":12345,"size":123456789,"date":1792340016,"nzb":"http://example.invalid/get/1.nzb"},
  {"filename":"Example.Show.S01E02.1080p.WEB-DL-GRP","anidb":null,"size":987654,"date":1792340000,"nzb":"http://example.invalid/get/2.nzb"},
  {"filename":"Example.Show.S01E03.NoLink","anidb":12345,"size":555,"date":1792340010}
]}`

// TestLocalJSONDefinition_Search is the integration test of the local
// definition against a fixture JSON API: request mapping (name/season/
// episode, no empty params), field extraction (title, size, absolute link,
// epoch date → RFC3339, category), nullable-field handling, and the
// observed behavior of items without a download link.
func TestLocalJSONDefinition_Search(t *testing.T) {
	var rawQuery string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rawQuery = r.URL.RawQuery
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, localJSONBody)
	}))
	defer server.Close()

	def, err := definition.Parse([]byte(fmt.Sprintf(localJSONDefinitionYAML, server.URL)))
	if err != nil {
		t.Fatalf("parsing fixture yaml: %v", err)
	}
	inner := cardigannengine.NewProvider(def, server.URL, nil)
	inner.SkipSearchDownload()
	provider := NewProvider(inner)

	if provider.Protocol() != model.ProtocolUsenet {
		t.Errorf("protocol = %q, want usenet", provider.Protocol())
	}

	releases, err := provider.Search(context.Background(), internalsearch.Query{
		Keywords:   "example",
		SearchType: "tvsearch",
		Season:     2,
		Episode:    4,
	})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}

	// Request mapping: name + season + episode, nothing empty.
	for _, want := range []string{"name=example", "season=2", "episode=4"} {
		if !strings.Contains(rawQuery, want) {
			t.Errorf("query %q missing %q", rawQuery, want)
		}
	}
	if strings.Contains(rawQuery, "season=&") || strings.HasSuffix(rawQuery, "season=") ||
		strings.Contains(rawQuery, "episode=&") || strings.HasSuffix(rawQuery, "episode=") {
		t.Errorf("query %q contains an empty season/episode param", rawQuery)
	}

	// The JSON rows were parsed: response type json must be in effect.
	if len(releases) != 3 {
		t.Fatalf("expected 3 releases (rows from $.items), got %d", len(releases))
	}

	first := releases[0]
	if first.Title != "Example.Show.S01E01.1080p.WEB-DL-GRP" {
		t.Errorf("title = %q", first.Title)
	}
	if first.Size != 123456789 {
		t.Errorf("size = %d, want 123456789", first.Size)
	}
	if first.DownloadLink != "http://example.invalid/get/1.nzb" {
		t.Errorf("download link = %q (absolute passthrough)", first.DownloadLink)
	}
	wantDate := time.Unix(1792340016, 0).UTC()
	if !first.PublishDate.Equal(wantDate) {
		t.Errorf("PublishDate = %v, want %v (epoch → RFC3339 via unixdate)", first.PublishDate, wantDate)
	}
	if first.Details != "https://example.invalid/anime/12345" {
		t.Errorf("details = %q, want conditional id URL", first.Details)
	}
	if len(first.Categories) != 1 || first.Categories[0] != 5070 {
		t.Errorf("categories = %v, want [5070]", first.Categories)
	}

	// Null id: the conditional details template must render nothing at all.
	if releases[1].Details != "" {
		t.Errorf("details for null id = %q, want empty", releases[1].Details)
	}
	if releases[1].PublishDate.IsZero() {
		t.Error("second release lost its date")
	}

	// Item without a download link: observed engine behavior (kept as-is,
	// empty link, so download fails loudly at resolve time rather than
	// silently dropping a catalog entry).
	if releases[2].DownloadLink != "" {
		t.Errorf("linkless item DownloadLink = %q, want empty", releases[2].DownloadLink)
	}
	if releases[2].Title != "Example.Show.S01E03.NoLink" {
		t.Errorf("linkless item title = %q", releases[2].Title)
	}
}

// TestLocalJSONDefinition_EmptySeasonOmitted verifies that a keyword-only
// search sends neither season nor episode (filterInputs drops the rendered
// empties, which is what keeps the API's empty-param edge cases away).
func TestLocalJSONDefinition_EmptySeasonOmitted(t *testing.T) {
	var rawQuery string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rawQuery = r.URL.RawQuery
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"total_count":0,"items":[]}`)
	}))
	defer server.Close()

	def, err := definition.Parse([]byte(fmt.Sprintf(localJSONDefinitionYAML, server.URL)))
	if err != nil {
		t.Fatalf("parsing fixture yaml: %v", err)
	}
	inner := cardigannengine.NewProvider(def, server.URL, nil)
	inner.SkipSearchDownload()
	provider := NewProvider(inner)

	if _, err := provider.Search(context.Background(), internalsearch.Query{Keywords: "example"}); err != nil {
		t.Fatalf("Search: %v", err)
	}
	if strings.Contains(rawQuery, "season") || strings.Contains(rawQuery, "episode") {
		t.Errorf("query %q must omit absent season/episode", rawQuery)
	}
}
