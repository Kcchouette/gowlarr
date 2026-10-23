package cardigannadapter

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Kcchouette/cardigann-go/definition"
	cardigannengine "github.com/Kcchouette/cardigann-go/engine"

	"github.com/Kcchouette/gowlarr/internal/model"
	internalsearch "github.com/Kcchouette/gowlarr/internal/search"
)

// localNZBIndexDefinitionYAML is the anonymized inline copy of the local
// nzbindex definition (definitions-local/, gitignored): JSON search at
// /api/search with epoch posted and byte size. %s is the test server base URL.
const localNZBIndexDefinitionYAML = `
id: localnzbindex
name: Local NZBIndex
description: "Local JSON usenet search fixture"
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
    - path: /api/search
      response:
        type: json
      inputs:
        q: "{{ .Keywords }}"
        max: "100"
  rows:
    selector: "$.data.content"
  fields:
    title:
      selector: name
    _id:
      selector: id
    download:
      text: "/download/{{ .Result._id }}.nzb"
    size:
      selector: size
    date:
      selector: posted
      filters:
        - name: unixdate
    poster:
      selector: poster
      optional: true
`

// localNZBIndexBody exercises a full item, an item without poster, and the
// envelope fields (error/errorMessage/page) the live API returns.
const localNZBIndexBody = `{
  "data": {
    "content": [
      {
        "id": "aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee",
        "name": "Example.Show.S01E01.1080p.WEB",
        "poster": "poster@example.invalid",
        "posted": 1791735048,
        "size": 3177685,
        "fileCount": 4,
        "complete": false,
        "groups": ["alt.binaries.example"]
      },
      {
        "id": "ffffffff-0000-1111-2222-333333333333",
        "name": "Example.Show.S01E02.720p.WEB",
        "poster": null,
        "posted": 1791451048,
        "size": 2763139,
        "fileCount": 2,
        "complete": true,
        "groups": []
      }
    ],
    "page": {"size": 100, "number": 0, "totalElements": 2, "totalPages": 1}
  },
  "error": false,
  "errorMessage": ""
}`

// TestLocalNZBIndexDefinition_Search is the integration test of the local
// nzbindex JSON shape: request mapping, field extraction, epoch date, and
// download path template.
func TestLocalNZBIndexDefinition_Search(t *testing.T) {
	var rawQuery string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/search" {
			t.Errorf("path = %q, want /api/search", r.URL.Path)
		}
		rawQuery = r.URL.RawQuery
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, localNZBIndexBody)
	}))
	defer server.Close()

	def, err := definition.Parse([]byte(fmt.Sprintf(localNZBIndexDefinitionYAML, server.URL)))
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
	if rawQuery != "max=100&q=example" && rawQuery != "q=example&max=100" {
		t.Errorf("query = %q, want q + max", rawQuery)
	}

	if len(releases) != 2 {
		t.Fatalf("expected 2 releases, got %d", len(releases))
	}

	first := releases[0]
	if first.Title != "Example.Show.S01E01.1080p.WEB" {
		t.Errorf("title = %q", first.Title)
	}
	if first.Size != 3177685 {
		t.Errorf("size = %d, want 3177685", first.Size)
	}
	if first.DownloadLink != "/download/aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee.nzb" {
		t.Errorf("download = %q", first.DownloadLink)
	}
	wantDate := time.Unix(1791735048, 0).UTC()
	if !first.PublishDate.Equal(wantDate) {
		t.Errorf("PublishDate = %v, want %v", first.PublishDate, wantDate)
	}

	// Null poster: optional field must not fail the row.
	if releases[1].Title != "Example.Show.S01E02.720p.WEB" {
		t.Errorf("second title = %q", releases[1].Title)
	}
	if releases[1].DownloadLink != "/download/ffffffff-0000-1111-2222-333333333333.nzb" {
		t.Errorf("second download = %q", releases[1].DownloadLink)
	}
}
