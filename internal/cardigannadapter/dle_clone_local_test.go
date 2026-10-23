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

// localDLEDefinitionYAML is the anonymized inline copy of the local DLE
// clone definitions (definitions-local/, gitignored): French DDL HTML search
// with ?p=films&search= paths, card rows, and a download.before chain that
// extracts protector hrefs via usebeforeresponse. %s is the test server base
// URL. Identity fields differ from production files on purpose.
const localDLEDefinitionYAML = `
id: dledemo
name: DLE Demo
description: "Local DLE-style French DDL fixture"
language: fr-FR
type: ddl
encoding: UTF-8
links:
  - %s
caps:
  categorymappings:
    - {id: "2000", cat: Movies, desc: "Films"}
    - {id: "5000", cat: TV, desc: "Séries"}
  modes:
    search: [q]
    tv-search: [q]
    movie-search: [q]
settings: []
search:
  paths:
    - path: /?p=films
      categories: [2000]
      inputs:
        search: "{{ .Keywords }}"
  rows:
    selector: div.cover_global
  fields:
    _title:
      selector: div.cover_infos_title a
      filters:
        - name: htmldecode
        - name: strip
    title:
      text: "{{ .Result._title }}"
    details:
      selector: div.cover_infos_title a
      attribute: href
    download:
      selector: div.cover_infos_title a
      attribute: href
    date:
      selector: time
      optional: true
      filters:
        - name: dateparse
          args: "02-01-2006"
    category:
      text: "2000"
      case:
        'a[href*="?p=serie&id="]': "5000"
        'a[href*="?p=film&id="]': "2000"
        "*": "2000"

download:
  before:
    path: "{{ .DownloadUri }}"
  selectors:
    - selector: 'a[href*="protector.example"]'
      attribute: href
      usebeforeresponse: true
`

// localDLESearchHTML mirrors the live results page: two cover cards with
// relative item links, a French-style release date, and a sidebar link that
// must not match the row selector.
const localDLESearchHTML = `<html><body>
<div class="sidebar"><a href="?p=film&id=99999-noise">Noise</a></div>
<div class="cover_global">
  <div style="height:193px;">
    <a href="?p=film&id=100-demo-one"><img class="mainimg" src="/img/1.webp"></a>
  </div>
  <div class="cover_infos_title">
    <a href="?p=film&id=100-demo-one">Demo One &amp; More</a>
    <span class="detail_release"><b>DVDRIP</b> <b>(FRENCH)</b></span>
  </div>
  <time>03-11-2018</time>
</div>
<div class="cover_global">
  <div class="cover_infos_title">
    <a href="?p=serie&id=200-demo-show">Demo Show</a>
  </div>
</div>
</body></html>`

// localDLEItemHTML is the fiche page returned by download.before: one
// protector link and one noise link that must be ignored.
const localDLEItemHTML = `<html><body>
<div class="download_liens">
  <a href="https://protector.example/abc123?fn=x">Lien</a>
  <a href="https://unrelated.invalid/noise">noise</a>
</div>
</body></html>`

// TestLocalDLEDefinition_Search is the integration test of the local DLE
// clone shape: Newznab path categories, card rows, htmldecode titles, href
// details/download, dd-MM-yyyy dates, category case from p=film|serie.
func TestLocalDLEDefinition_Search(t *testing.T) {
	var rawQuery string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rawQuery = r.URL.RawQuery
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		if strings.Contains(r.URL.RawQuery, "id=") {
			fmt.Fprint(w, localDLEItemHTML)
			return
		}
		fmt.Fprint(w, localDLESearchHTML)
	}))
	defer server.Close()

	def, err := definition.Parse([]byte(fmt.Sprintf(localDLEDefinitionYAML, server.URL)))
	if err != nil {
		t.Fatalf("parsing fixture yaml: %v", err)
	}
	inner := cardigannengine.NewProvider(def, server.URL, nil)
	inner.SkipSearchDownload()
	provider := NewProvider(inner)

	releases, err := provider.Search(context.Background(), internalsearch.Query{Keywords: "demo"})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if !strings.Contains(rawQuery, "search=demo") {
		t.Errorf("raw query %q missing search=demo", rawQuery)
	}
	if len(releases) != 2 {
		t.Fatalf("got %d releases, want 2", len(releases))
	}

	first := releases[0]
	if first.Title != "Demo One & More" {
		t.Errorf("title = %q, want %q", first.Title, "Demo One & More")
	}
	if !strings.Contains(first.DownloadLink, "id=100-demo-one") {
		t.Errorf("download = %q, want id=100-demo-one", first.DownloadLink)
	}
	if first.PublishDate.IsZero() {
		t.Error("date = zero, want 2018-11-03")
	} else if first.PublishDate.Year() != 2018 || int(first.PublishDate.Month()) != 11 || first.PublishDate.Day() != 3 {
		t.Errorf("date = %v, want 2018-11-03", first.PublishDate)
	}
	if len(first.Categories) == 0 || first.Categories[0] != 2000 {
		t.Errorf("categories = %v, want [2000]", first.Categories)
	}

	second := releases[1]
	if second.Title != "Demo Show" {
		t.Errorf("second title = %q", second.Title)
	}
	if len(second.Categories) == 0 || second.Categories[0] != 5000 {
		t.Errorf("second categories = %v, want [5000]", second.Categories)
	}
}

// TestLocalDLEDefinition_ResolveLinks exercises the download.before chain:
// path GET of the fiche + usebeforeresponse selector for the protector host.
func TestLocalDLEDefinition_ResolveLinks(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		if strings.Contains(r.URL.RawQuery, "id=100") {
			fmt.Fprint(w, localDLEItemHTML)
			return
		}
		fmt.Fprint(w, localDLESearchHTML)
	}))
	defer server.Close()

	def, err := definition.Parse([]byte(fmt.Sprintf(localDLEDefinitionYAML, server.URL)))
	if err != nil {
		t.Fatalf("parsing fixture yaml: %v", err)
	}
	inner := cardigannengine.NewProvider(def, server.URL, nil)
	inner.SkipSearchDownload()
	provider := NewProvider(inner)

	releases, err := provider.Search(context.Background(), internalsearch.Query{Keywords: "demo"})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(releases) == 0 {
		t.Fatal("no releases")
	}

	resolved, err := provider.ResolveDownloadLink(context.Background(), releases[0])
	if err != nil {
		t.Fatalf("ResolveDownloadLink: %v", err)
	}
	if !strings.Contains(resolved.DownloadLink, "protector.example") {
		t.Errorf("resolved download = %q, want protector.example", resolved.DownloadLink)
	}
}
