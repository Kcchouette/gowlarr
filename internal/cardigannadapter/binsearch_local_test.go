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

// localBinsearchDefinitionYAML is the anonymized inline copy of the local
// binsearch definition (definitions-local/, gitignored): HTML table search
// with checkbox-name ids and /nzb?id= downloads. %s is the test server base
// URL. Identity fields differ from the production file on purpose.
const localBinsearchDefinitionYAML = `
id: localbinsearch
name: Local Binsearch
description: "Local HTML usenet search fixture"
language: en-US
encoding: UTF-8
type: usenet
links:
  - %s
caps:
  modes:
    search: [q]
settings: []
search:
  paths:
    - path: /search
      inputs:
        q: "{{ .Keywords }}"
        max: "100"
  rows:
    selector: table.result-table tr
    after: 1
  fields:
    title:
      selector: a[href^="/details/"]
      filters:
        - name: htmldecode
        - name: strip
    details:
      selector: a[href^="/details/"]
      attribute: href
    _id:
      selector: input[type=checkbox]
      attribute: name
    download:
      text: "/nzb?id={{ .Result._id }}"
    date:
      selector: "td:last-child"
      filters:
        - name: timeago
`

// localBinsearchHTML mirrors the live result table: header row + two result
// rows (one with HTML entity in the title).
const localBinsearchHTML = `<html><body>
<table class="result-table">
  <tr><th></th><th>Subject</th><th>Age</th></tr>
  <tr>
    <td>1.</td>
    <td><input type="checkbox" name="dG9rZW4x"/></td>
    <td><a href="/details/dG9rZW4x">Example &quot;Show&quot; S01E01</a></td>
    <td>11 days</td>
  </tr>
  <tr>
    <td>2.</td>
    <td><input type="checkbox" name="dG9rZW4y"/></td>
    <td><a href="/details/dG9rZW4y">Example Show S01E02</a></td>
    <td>1 month</td>
  </tr>
</table>
</body></html>`

// TestLocalBinsearchDefinition_Search is the integration test of the local
// binsearch shape: row skip, htmldecode, download template from checkbox
// name, and relative age → PublishDate via timeago.
func TestLocalBinsearchDefinition_Search(t *testing.T) {
	var rawQuery string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rawQuery = r.URL.RawQuery
		w.Header().Set("Content-Type", "text/html")
		fmt.Fprint(w, localBinsearchHTML)
	}))
	defer server.Close()

	def, err := definition.Parse([]byte(fmt.Sprintf(localBinsearchDefinitionYAML, server.URL)))
	if err != nil {
		t.Fatalf("parsing fixture yaml: %v", err)
	}
	provider := NewProvider(cardigannengine.NewProvider(def, server.URL, nil))

	if provider.Protocol() != model.ProtocolUsenet {
		t.Errorf("protocol = %q, want usenet", provider.Protocol())
	}

	releases, err := provider.Search(context.Background(), internalsearch.Query{Keywords: "example"})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if !strings.Contains(rawQuery, "q=example") {
		t.Errorf("query %q missing q=example", rawQuery)
	}

	if len(releases) != 2 {
		t.Fatalf("expected 2 releases (header skipped), got %d", len(releases))
	}

	first := releases[0]
	if first.Title != `Example "Show" S01E01` {
		t.Errorf("title = %q (htmldecode)", first.Title)
	}
	if first.DownloadLink != server.URL+"/nzb?id=dG9rZW4x" {
		t.Errorf("download = %q", first.DownloadLink)
	}
	if first.Details != server.URL+"/details/dG9rZW4x" {
		t.Errorf("details = %q", first.Details)
	}
	if first.PublishDate.IsZero() {
		t.Error("first release lost its date (timeago)")
	}
	// "11 days" → approximately now-11d; allow generous skew.
	if age := time.Since(first.PublishDate); age < 9*24*time.Hour || age > 13*24*time.Hour {
		t.Errorf("PublishDate age = %v, want ~11 days", age)
	}

	second := releases[1]
	if second.Title != "Example Show S01E02" {
		t.Errorf("second title = %q", second.Title)
	}
	if second.DownloadLink != server.URL+"/nzb?id=dG9rZW4y" {
		t.Errorf("second download = %q", second.DownloadLink)
	}
}

// TestLocalBinsearchDefinition_LinkylessRow documents current engine
// behavior: a row without a checkbox yields an empty download link (the
// _id template input is missing and the rendered path is discarded).
func TestLocalBinsearchDefinition_LinkylessRow(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		fmt.Fprint(w, `<html><body>
<table class="result-table">
  <tr><th></th><th>Subject</th><th>Age</th></tr>
  <tr>
    <td>1.</td>
    <td></td>
    <td><a href="/details/bm9pZA">No Checkbox</a></td>
    <td>2 hours</td>
  </tr>
</table>
</body></html>`)
	}))
	defer server.Close()

	def, err := definition.Parse([]byte(fmt.Sprintf(localBinsearchDefinitionYAML, server.URL)))
	if err != nil {
		t.Fatalf("parsing fixture yaml: %v", err)
	}
	provider := NewProvider(cardigannengine.NewProvider(def, server.URL, nil))

	releases, err := provider.Search(context.Background(), internalsearch.Query{Keywords: "x"})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(releases) != 1 {
		t.Fatalf("expected 1 release, got %d", len(releases))
	}
	if releases[0].Title != "No Checkbox" {
		t.Errorf("title = %q", releases[0].Title)
	}
	if releases[0].DownloadLink != "" {
		t.Errorf("linkless row DownloadLink = %q, want empty", releases[0].DownloadLink)
	}
}
