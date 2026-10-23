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

// localMagnetDefinitionYAML is the anonymized inline copy of the local
// torrent-magnet definitions (definitions-local/, gitignored): public French
// index, search path /recherche/{q}, table rows with WinOption1 hover junk,
// magnet only on the fiche via download.before + usebeforeresponse.
// %s is the test server base URL.
const localMagnetDefinitionYAML = `
id: magnetdemo
name: Magnet Demo
description: "Local French torrent index fixture (magnet on fiche)"
language: fr-FR
type: public
encoding: UTF-8
links:
  - %s
caps:
  categorymappings:
    - {id: "2000", cat: Movies, desc: "Films"}
    - {id: "5000", cat: TV, desc: "Séries"}
    - {id: "3000", cat: Audio, desc: "Musiques"}
    - {id: "4010", cat: PC/Games, desc: "Jeux PC"}
  modes:
    search: [q]
    tv-search: [q]
    movie-search: [q]
settings: []
search:
  paths:
    - path: /recherche/{{ .Keywords }}
  rows:
    selector: table.table-hover tbody tr:has(td.liste-accueil-nom)
  fields:
    title:
      selector: a.ShowOption1
      remove: span.WinOption1
      filters:
        - name: htmldecode
        - name: strip
    details:
      selector: a.ShowOption1
      attribute: href
    download:
      selector: a.ShowOption1
      attribute: href
    category:
      text: "2000"
      case:
        'a[href*="/category/series"]': "5000"
        'a[href*="/category/musiques"]': "3000"
        'a[href*="/category/jeux-pc"]': "4010"
        'a[href*="/category/films"]': "2000"
        "*": "2000"
    seeders:
      selector: td.sources.liste-accueil-sources
      optional: true
    peers:
      selector: td.clients.liste-accueil-clients
      optional: true
download:
  before:
    path: "{{ .DownloadUri }}"
  selectors:
    - selector: 'a[href^="magnet:"]'
      attribute: href
      usebeforeresponse: true
`

// localMagnetSearchHTML mirrors the live /recherche/ table: two rows with
// hover posters, a films and a series category cell.
const localMagnetSearchHTML = `<html><body>
<table class="table table-hover">
<tbody>
<tr>
  <td class="type liste-accueil-type" title="Films"><a href="/category/films"><i class="Films"></i></a></td>
  <td class="liste-accueil-nom"><div class="maxi"><a class="ShowOption1" href="/detail/100">Demo.2024.FRENCH.1080p.WEB.x264-TEST
	<span class="WinOption1">
	<img src="https://cdn.example.invalid/screen/aabbccddeeff00112233445566778899aabbccdd.webp" />
	<div class="info"><div class="maxis">Demo 2024</div>Seeders: 12<br></div>
	</span>
</a></div></td>
  <td class="sources liste-accueil-sources" title="Sources">12</td>
  <td class="clients liste-accueil-clients" title="Clients">3</td>
</tr>
<tr>
  <td class="type liste-accueil-type" title="Séries"><a href="/category/series"><i class="Séries"></i></a></td>
  <td class="liste-accueil-nom"><div class="maxi"><a class="ShowOption1" href="/detail/200">Demo.Show.S01E01.FRENCH.WEB.x264-TEST
	<span class="WinOption1">
	<img src="https://cdn.example.invalid/screen/bbccddeeff00112233445566778899aabbccddee.webp" />
	</span>
</a></div></td>
  <td class="sources liste-accueil-sources" title="Sources">8</td>
  <td class="clients liste-accueil-clients" title="Clients">1</td>
</tr>
</tbody>
</table>
</body></html>`

// localMagnetFicheHTML is the /detail/{id} page: magnet link plus noise.
const localMagnetFicheHTML = `<html><body>
<table class="table">
<tr><td>
  <a href="/get_torrents/aabbccddeeff00112233445566778899aabbccdd">Torrent</a>
  <a href="magnet:?xt=urn:btih:aabbccddeeff00112233445566778899aabbccdd&tr=udp://tracker.example:1337/announce">Magnet</a>
  <a href="https://protector.example/xyz">Direct</a>
</td></tr>
</table>
</body></html>`

// TestLocalMagnetDefinition_Search is the integration test of the local
// torrent-magnet shape: /recherche/ path, table rows, title without hover
// junk, fiche download URL, category case, seeders/peers.
func TestLocalMagnetDefinition_Search(t *testing.T) {
	var rawPath string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rawPath = r.URL.Path
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		fmt.Fprint(w, localMagnetSearchHTML)
	}))
	defer server.Close()

	def, err := definition.Parse([]byte(fmt.Sprintf(localMagnetDefinitionYAML, server.URL)))
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
	if !strings.Contains(rawPath, "/recherche/demo") {
		t.Errorf("path = %q, want /recherche/demo", rawPath)
	}
	if len(releases) != 2 {
		t.Fatalf("got %d releases, want 2", len(releases))
	}

	first := releases[0]
	if first.Title != "Demo.2024.FRENCH.1080p.WEB.x264-TEST" {
		t.Errorf("title = %q, want scene name without WinOption1 junk", first.Title)
	}
	if !strings.Contains(first.DownloadLink, "/detail/100") {
		t.Errorf("download = %q, want fiche /detail/100", first.DownloadLink)
	}
	if len(first.Categories) == 0 || first.Categories[0] != 2000 {
		t.Errorf("categories = %v, want [2000]", first.Categories)
	}
	if first.Seeders != 12 {
		t.Errorf("seeders = %d, want 12", first.Seeders)
	}
	if first.Peers != 3 {
		t.Errorf("peers = %d, want 3", first.Peers)
	}

	second := releases[1]
	if second.Title != "Demo.Show.S01E01.FRENCH.WEB.x264-TEST" {
		t.Errorf("second title = %q", second.Title)
	}
	if len(second.Categories) == 0 || second.Categories[0] != 5000 {
		t.Errorf("second categories = %v, want [5000]", second.Categories)
	}
}

// TestLocalMagnetDefinition_ResolveLinks exercises the download.before chain
// from the stored fiche URL to the magnet href.
func TestLocalMagnetDefinition_ResolveLinks(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		if strings.HasPrefix(r.URL.Path, "/detail/") {
			fmt.Fprint(w, localMagnetFicheHTML)
			return
		}
		fmt.Fprint(w, localMagnetSearchHTML)
	}))
	defer server.Close()

	def, err := definition.Parse([]byte(fmt.Sprintf(localMagnetDefinitionYAML, server.URL)))
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
	if !strings.HasPrefix(resolved.DownloadLink, "magnet:?xt=urn:btih:aabbccdd") {
		t.Errorf("resolved download = %q, want magnet", resolved.DownloadLink)
	}
	if !strings.Contains(resolved.DownloadLink, "tracker.example") {
		t.Errorf("resolved download = %q, want page trackers preserved", resolved.DownloadLink)
	}
}
