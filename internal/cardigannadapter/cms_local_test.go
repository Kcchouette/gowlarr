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

// localCMSDefinitionYAML is the anonymized inline copy of the local
// local CMS definition (definitions-local/, gitignored): CMS search
// via a keyword query param + title cards. %s is the test server base URL.
const localCMSDefinitionYAML = `
id: localfreetele
name: Local CMS Demo
description: "Local French DDL CMS fixture"
language: fr-FR
type: ddl
encoding: UTF-8
links:
  - %s
caps:
  categorymappings:
    - {id: "2000", cat: Movies, desc: "Films"}
  modes:
    search: [q]
    movie-search: [q]
settings: []
search:
  paths:
    - path: /1/recherche1/1.html
      inputs:
        rech_fiche: "{{ .Keywords }}"
  rows:
    selector: div.content
  fields:
    _title:
      selector: div.titre1 a
      filters:
        - name: htmldecode
        - name: strip
    title:
      text: "{{ .Result._title }}"
    details:
      selector: div.titre1 a
      attribute: href
    download:
      selector: div.titre1 a
      attribute: href
    date:
      selector: div.link_cat
      optional: true
      filters:
        - name: regexp
          args: "le ([0-9]{2}-[0-9]{2}-[0-9]{4})"
        - name: dateparse
          args: "02-01-2006"
    category:
      text: "2000"
`

// localCMSSearchHTML mirrors the live search results: highlighted title
// spans inside .titre1 plus a link_cat metadata line with a French date.
const localCMSSearchHTML = `<html><body>
<div class="content">
  <div class="titre1">
    <a href="films-bluray-720p-1080p/100-demo-bluray.html"><span style="background-color: yellow;"><font color="red">Demo</font></span> Bluray</a>
  </div>
  <div class="link_cat">Ajouté par <b>user</b> dans <b>Films</b> le 23-06-2026</div>
</div>
</body></html>`

// TestLocalCMSDefinition_Search covers the rech_fiche query mapping,
// titre1 rows, htmldecode of highlight markup, and "le dd-mm-yyyy" dates.
func TestLocalCMSDefinition_Search(t *testing.T) {
	var rawQuery string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rawQuery = r.URL.RawQuery
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		fmt.Fprint(w, localCMSSearchHTML)
	}))
	defer server.Close()

	def, err := definition.Parse([]byte(fmt.Sprintf(localCMSDefinitionYAML, server.URL)))
	if err != nil {
		t.Fatalf("parsing fixture yaml: %v", err)
	}
	provider := NewProvider(cardigannengine.NewProvider(def, server.URL, nil))

	releases, err := provider.Search(context.Background(), internalsearch.Query{Keywords: "demo"})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if !strings.Contains(rawQuery, "rech_fiche=demo") {
		t.Errorf("raw query %q missing rech_fiche=demo", rawQuery)
	}
	if len(releases) != 1 {
		t.Fatalf("got %d releases, want 1", len(releases))
	}
	rel := releases[0]
	if rel.Title != "Demo Bluray" {
		t.Errorf("title = %q, want %q", rel.Title, "Demo Bluray")
	}
	if !strings.Contains(rel.DownloadLink, "100-demo-bluray.html") {
		t.Errorf("download = %q", rel.DownloadLink)
	}
	if rel.PublishDate.IsZero() {
		t.Error("date = zero, want 2026-06-23")
	} else if rel.PublishDate.Year() != 2026 || int(rel.PublishDate.Month()) != 6 || rel.PublishDate.Day() != 23 {
		t.Errorf("date = %v, want 2026-06-23", rel.PublishDate)
	}
}
