package cardigannadapter

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Kcchouette/cardigann-go/definition"
	cardigannengine "github.com/Kcchouette/cardigann-go/engine"

	internalsearch "github.com/Kcchouette/gowlarr/internal/search"
)

// localMediaDefinitionYAML is the anonymized inline copy of the local
// media-index definition (definitions-local/, gitignored): absolute JSON search
// path (MeiliSearch-style hits), display-only SPA download, category case
// from media type. Three %s placeholders: links[0], absolute search path
// prefix, details URL, download URL (all the test server).
const localMediaDefinitionYAML = `
id: localmedia
name: Local Media Index
description: "Local JSON media index fixture"
language: fr-FR
type: ddl
encoding: UTF-8
links:
  - %s
caps:
  categorymappings:
    - {id: "2000", cat: Movies, desc: "Movies"}
    - {id: "5000", cat: TV, desc: "Series"}
    - {id: "5070", cat: TV/Anime, desc: "Anime"}
  modes:
    search: [q]
    tv-search: [q]
    movie-search: [q]
settings: []
search:
  headers:
    Accept: ["application/json"]
  paths:
    - path: %s/api/media/search
      response:
        type: json
      inputs:
        q: "{{ .Keywords }}"
  rows:
    selector: "$.hits"
  fields:
    _id:
      selector: id
    _type:
      selector: type
    title:
      selector: title
    details:
      text: "%s/"
    download:
      text: "%s/"
    date:
      selector: addedAt
      optional: true
      filters:
        - name: unixdate
    category:
      text: "{{ .Result._type }}"
      case:
        movie: "2000"
        series: "5000"
        anime: "5070"
        "*": "2000"
`

// localMediaBody is a minimal MeiliSearch-style envelope with epoch-
// seconds addedAt (same unit as production).
const localMediaBody = `{"hits":[
  {"id":"aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee","type":"movie","title":"Demo Film","year":2021,"addedAt":1791755520,"hasLinks":true},
  {"id":"ffffffff-1111-2222-3333-444444444444","type":"series","title":"Demo Show","year":2024,"addedAt":1791755520,"hasLinks":true}
],"estimatedTotalHits":2,"page":1,"hitsPerPage":20}`

// TestLocalMediaDefinition_Search covers absolute-path JSON search, hits
// extraction, unixdate seconds, and type→Newznab category mapping.
func TestLocalMediaDefinition_Search(t *testing.T) {
	var rawQuery string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rawQuery = r.URL.RawQuery
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, localMediaBody)
	}))
	defer server.Close()

	yaml := fmt.Sprintf(localMediaDefinitionYAML, server.URL, server.URL, server.URL, server.URL)
	def, err := definition.Parse([]byte(yaml))
	if err != nil {
		t.Fatalf("parsing fixture yaml: %v", err)
	}
	provider := NewProvider(cardigannengine.NewProvider(def, server.URL, nil))

	releases, err := provider.Search(context.Background(), internalsearch.Query{Keywords: "demo"})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if !strings.Contains(rawQuery, "q=demo") {
		t.Errorf("raw query %q missing q=demo", rawQuery)
	}
	if len(releases) != 2 {
		t.Fatalf("got %d releases, want 2", len(releases))
	}

	film := releases[0]
	if film.Title != "Demo Film" {
		t.Errorf("title = %q", film.Title)
	}
	if film.PublishDate.IsZero() {
		t.Error("date = zero, want epoch 1791755520")
	} else if film.PublishDate.Year() != 2026 {
		t.Errorf("date = %v, want year 2026 from unix seconds", film.PublishDate)
	}
	if len(film.Categories) == 0 || film.Categories[0] != 2000 {
		t.Errorf("categories = %v, want [2000]", film.Categories)
	}

	show := releases[1]
	if show.Title != "Demo Show" {
		t.Errorf("second title = %q", show.Title)
	}
	if len(show.Categories) == 0 || show.Categories[0] != 5000 {
		t.Errorf("second categories = %v, want [5000]", show.Categories)
	}
}
