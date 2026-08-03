package cardigannadapter

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/Kcchouette/cardigann-go/definition"
	cardigannengine "github.com/Kcchouette/cardigann-go/engine"

	"github.com/Kcchouette/gowlarr/internal/model"
	internalsearch "github.com/Kcchouette/gowlarr/internal/search"
)

// hydraddlDefinitionYAML mirrors definitions-local/hydraddl.yml (the DDL
// section of hydracker.com): a search.before step resolves the title id from
// the public title search, then the search path lists every DDL lien of that
// title. Inlined so the test runs in CI without the gitignored definitions
// directory. %s is replaced by the test server base URL.
const hydraddlDefinitionYAML = `
id: hydraddl
name: Hydra DDL
description: direct-download section of hydracker.com (fixture)
language: fr-FR
type: ddl
encoding: UTF-8
links:
  - %s
caps:
  categorymappings:
    - {id: 1000, cat: Console, desc: "Console"}
    - {id: 2000, cat: Movies, desc: "Movies"}
    - {id: 3000, cat: Audio, desc: "Audio"}
    - {id: 4000, cat: PC, desc: "PC"}
    - {id: 5000, cat: TV, desc: "TV"}
    - {id: 7000, cat: Books, desc: "Books"}
    - {id: 8000, cat: Other, desc: "Other"}
  modes:
    search: [q]
    tv-search: [q, season]
    movie-search: [q]
settings:
  - name: apikey
    type: text
    label: API Token
search:
  before:
    - name: title
      path: api/v1/titles
      inputs:
        query: "{{ .Keywords }}"
      response:
        type: json
      selectors:
        id: {selector: "$.pagination.data.0.id"}
        name: {selector: "$.pagination.data.0.name"}
        type: {selector: "$.pagination.data.0.type"}
  paths:
    - path: "api/v1/titles/{{ .Before.title.id }}/content/liens"
      inputs:
        loader: linksdl
        season: "{{ .Query.Season }}"
      response:
        type: json
  headers:
    Accept: ["application/json"]
    Authorization: ["Bearer {{ .Config.apikey }}"]
  rows:
    selector: "$.liens"
  fields:
    _season:      {selector: saison, optional: true}
    _episode:     {selector: episode, optional: true}
    _full_saison: {selector: full_saison, optional: true}
    _quality:     {selector: "qual.qual", optional: true}
    _hostname:    {selector: "host.name", optional: true}
    title:
      text: '{{ .Before.title.name }}{{ if eq .Result._full_saison "1" }} S{{ .Result._season }} (saison complete){{ else }} S{{ .Result._season }}E{{ .Result._episode }}{{ end }} - {{ .Result._quality }} [{{ .Result._hostname }}]'
    size:
      selector: taille
    date:
      selector: created_at
      filters:
        - name: dateparse
    category:
      text: "{{ .Before.title.type }}"
      case:
        series: 5000
        movie: 2000
        music: 3000
        animes: 5000
        ebook: 7000
        games: 4000
        app: 4000
        "*": 8000
    details:
      text: "https://hydracker.com/titles/{{ .Before.title.id }}"
    download:
      text: '{{ if eq .Result._full_saison "1" }}https://hydracker.com/titles/{{ .Before.title.id }}/season/{{ .Result._season }}/download{{ else }}https://hydracker.com/titles/{{ .Before.title.id }}/season/{{ .Result._season }}/episode/{{ .Result._episode }}/download{{ end }}'
`

const hydraddlTitleJSON = `{"pagination":{"data":[{"id":179751,"name":"House of the Dragon","type":"series"}]}}`

const hydraddlLiensJSON = `{"liens":[
	{"id":1,"saison":1,"episode":1,"full_saison":0,"taille":9800362833,"created_at":"2022-12-31T03:21:19.000000Z","host":{"name":"1Fichier"},"qual":{"qual":"Blu-Ray 1080p"}},
	{"id":2,"saison":2,"episode":0,"full_saison":1,"taille":19022414451,"created_at":"2024-08-05T05:53:19.000000Z","host":{"name":"1Fichier"},"qual":{"qual":"ULTRA HD (x265)"}}
]}`

// hydraddlServer serves the two-step API and records the bearer header and
// the requests it receives. expectedBearer is asserted on every request
// ("" disables the assertion).
func hydraddlServer(t *testing.T, titleBody string, liensBody string, statusForSearch int, expectedBearer string) (*httptest.Server, *[]string, *int) {
	t.Helper()
	var requests []string
	var searchCalls int
	var mu sync.Mutex
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/titles", func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		requests = append(requests, r.Method+" "+r.URL.Path+"?"+r.URL.RawQuery)
		mu.Unlock()
		if expectedBearer != "" {
			if got := r.Header.Get("Authorization"); got != expectedBearer {
				t.Errorf("title lookup: Authorization = %q, want %q", got, expectedBearer)
			}
		}
		fmt.Fprint(w, titleBody)
	})
	mux.HandleFunc("/api/v1/titles/179751/content/liens", func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		searchCalls++
		requests = append(requests, r.Method+" "+r.URL.Path+"?"+r.URL.RawQuery)
		mu.Unlock()
		if expectedBearer != "" {
			if got := r.Header.Get("Authorization"); got != expectedBearer {
				t.Errorf("liens: Authorization = %q, want %q", got, expectedBearer)
			}
		}
		if statusForSearch != http.StatusOK {
			w.WriteHeader(statusForSearch)
			fmt.Fprint(w, `{"message":"Unauthenticated."}`)
			return
		}
		fmt.Fprint(w, liensBody)
	})
	return httptest.NewServer(mux), &requests, &searchCalls
}

func TestHydraDdlDefinition_Search(t *testing.T) {
	server, requests, searchCalls := hydraddlServer(t, hydraddlTitleJSON, hydraddlLiensJSON, http.StatusOK, "Bearer sekret")
	defer server.Close()

	def, err := definition.Parse([]byte(fmt.Sprintf(hydraddlDefinitionYAML, server.URL)))
	if err != nil {
		t.Fatalf("parsing fixture yaml: %v", err)
	}
	inner := cardigannengine.NewProvider(def, server.URL, map[string]string{"apikey": "sekret"})
	inner.SkipSearchDownload()
	provider := NewProvider(inner)

	releases, err := provider.Search(context.Background(), internalsearch.Query{Keywords: "house of the dragon"})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(releases) != 2 {
		t.Fatalf("expected 2 releases, got %d", len(releases))
	}

	ep := releases[0]
	if ep.Title != "House of the Dragon S1E1 - Blu-Ray 1080p [1Fichier]" {
		t.Errorf("Title = %q", ep.Title)
	}
	if ep.Size != 9800362833 {
		t.Errorf("Size = %d, want 9800362833", ep.Size)
	}
	if ep.PublishDate.Format("2006-01-02") != "2022-12-31" {
		t.Errorf("PublishDate = %v, want 2022-12-31", ep.PublishDate)
	}
	if len(ep.Categories) != 1 || ep.Categories[0] != 5000 {
		t.Errorf("Categories = %v, want [5000] (TV)", ep.Categories)
	}
	if ep.Details != "https://hydracker.com/titles/179751" {
		t.Errorf("Details = %q", ep.Details)
	}
	if ep.DownloadLink != "https://hydracker.com/titles/179751/season/1/episode/1/download" {
		t.Errorf("DownloadLink = %q", ep.DownloadLink)
	}
	if ep.Protocol != model.ProtocolDDL {
		t.Errorf("Protocol = %q, want %q", ep.Protocol, model.ProtocolDDL)
	}

	season := releases[1]
	if season.Title != "House of the Dragon S2 (saison complete) - ULTRA HD (x265) [1Fichier]" {
		t.Errorf("season Title = %q", season.Title)
	}
	if season.DownloadLink != "https://hydracker.com/titles/179751/season/2/download" {
		t.Errorf("season DownloadLink = %q", season.DownloadLink)
	}

	if len(*requests) != 2 {
		t.Fatalf("expected 2 requests (title lookup + liens), got %d: %v", len(*requests), *requests)
	}
	if !contains(*requests, "GET /api/v1/titles?query=house+of+the+dragon") {
		t.Errorf("expected the title lookup with the encoded query, got %v", *requests)
	}
	if !contains(*requests, "GET /api/v1/titles/179751/content/liens?loader=linksdl") {
		t.Errorf("expected the liens request with loader=linksdl, got %v", *requests)
	}
	_ = searchCalls
}

func TestHydraDdlDefinition_NoTitleMatch(t *testing.T) {
	server, requests, searchCalls := hydraddlServer(t, `{"pagination":{"data":[]}}`, hydraddlLiensJSON, http.StatusOK, "Bearer sekret")
	defer server.Close()

	def, err := definition.Parse([]byte(fmt.Sprintf(hydraddlDefinitionYAML, server.URL)))
	if err != nil {
		t.Fatalf("parsing fixture yaml: %v", err)
	}
	provider := NewProvider(cardigannengine.NewProvider(def, server.URL, map[string]string{"apikey": "sekret"}))

	releases, err := provider.Search(context.Background(), internalsearch.Query{Keywords: "zzzznothing"})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(releases) != 0 {
		t.Fatalf("expected 0 releases, got %d", len(releases))
	}
	if *searchCalls != 0 {
		t.Fatalf("liens request must not fire when no title matches, got %d calls", *searchCalls)
	}
	if len(*requests) != 1 {
		t.Fatalf("expected only the title lookup, got %v", *requests)
	}
}

// The liens request answers 401: the search path failure must surface as an
// error, not a silent empty result. (A 401 on the search.before step itself
// is covered by the fork engine test TestSearchBefore_HTTPFailure_AbortsWithError.)
func TestHydraDdlDefinition_SearchPathUnauthorized(t *testing.T) {
	server, _, _ := hydraddlServer(t, hydraddlTitleJSON, hydraddlLiensJSON, http.StatusUnauthorized, "")
	defer server.Close()

	def, err := definition.Parse([]byte(fmt.Sprintf(hydraddlDefinitionYAML, server.URL)))
	if err != nil {
		t.Fatalf("parsing fixture yaml: %v", err)
	}
	provider := NewProvider(cardigannengine.NewProvider(def, server.URL, map[string]string{"apikey": "expired"}))

	if _, err := provider.Search(context.Background(), internalsearch.Query{Keywords: "house of the dragon"}); err == nil {
		t.Fatal("Search must surface an error on 401 (expired token), not a silent empty result")
	}
}

func contains(ss []string, want string) bool {
	for _, s := range ss {
		if s == want {
			return true
		}
	}
	return false
}
