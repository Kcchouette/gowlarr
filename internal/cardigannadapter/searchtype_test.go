package cardigannadapter

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Kcchouette/cardigann-go/definition"
	cardigannengine "github.com/Kcchouette/cardigann-go/engine"

	internalsearch "github.com/Kcchouette/gowlarr/internal/search"
)

// searchTypeDefinitionYAML is an invented fixture (no real site) exercising
// the end-to-end forwarding of search.SearchType into the template variable
// .Query.Type, which definitions render as the Newznab "t" parameter.
const searchTypeDefinitionYAML = `
id: typedemo
name: Type Demo
description: SearchType forwarding fixture
language: en-US
type: usenet
encoding: UTF-8
links:
  - %s
caps:
  modes:
    search: [q]
    tv-search: [q, season, ep]
settings: []
search:
  paths:
    - path: /search
      inputs:
        t: "{{ .Query.Type }}"
        q: "{{ .Keywords }}"
      response:
        type: html
  rows:
    selector: table.results tr
  fields:
    title:
      selector: a.title
    download:
      selector: a.dl
      attribute: href
`

const searchTypeBody = `<html><body>
<table class="results">
  <tr><td><a class="title" href="/d1">Example.Release.S01E01</a></td>
      <td><a class="dl" href="/get/1">DL</a></td></tr>
</table>
</body></html>`

// TestSearchTypeForwardedToTemplate verifies the adapter passes
// search.Query.SearchType through as engine Query.Type: the rendered "t"
// parameter carries "tvsearch" when set and the engine default "search"
// when empty.
func TestSearchTypeForwardedToTemplate(t *testing.T) {
	cases := []struct {
		name  string
		query internalsearch.Query
		wantT string
	}{
		{
			name:  "tvsearch forwarded",
			query: internalsearch.Query{Keywords: "example", SearchType: "tvsearch"},
			wantT: "tvsearch",
		},
		{
			name:  "empty type defaults to search",
			query: internalsearch.Query{Keywords: "example"},
			wantT: "search",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var gotT string
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				gotT = r.URL.Query().Get("t")
				w.Header().Set("Content-Type", "text/html")
				fmt.Fprint(w, searchTypeBody)
			}))
			defer server.Close()

			def, err := definition.Parse([]byte(fmt.Sprintf(searchTypeDefinitionYAML, server.URL)))
			if err != nil {
				t.Fatalf("parsing fixture yaml: %v", err)
			}
			inner := cardigannengine.NewProvider(def, server.URL, nil)
			inner.SkipSearchDownload()
			provider := NewProvider(inner)

			releases, err := provider.Search(context.Background(), tc.query)
			if err != nil {
				t.Fatalf("Search: %v", err)
			}
			if len(releases) != 1 {
				t.Fatalf("expected 1 release, got %d", len(releases))
			}
			if gotT != tc.wantT {
				t.Errorf("t = %q, want %q", gotT, tc.wantT)
			}
		})
	}
}
