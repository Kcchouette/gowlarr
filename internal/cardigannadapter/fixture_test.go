package cardigannadapter

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/Kcchouette/cardigann-go/definition"
	cardigannengine "github.com/Kcchouette/cardigann-go/engine"
	cardigannrelease "github.com/Kcchouette/cardigann-go/release"

	"github.com/Kcchouette/gowlarr/internal/model"
	internalsearch "github.com/Kcchouette/gowlarr/internal/search"
)

// ddlDefinitionYAML is a generic DDL indexer exercising the reply-unlock
// chain pattern: content hidden behind a placeholder until a quick-reply
// POST (carrying the form's CSRF tokens) reveals the real DDL links.
const ddlDefinitionYAML = `
id: ddl-demo
name: DDL Demo
description: demo ddl indexer (reply-unlock chain)
language: fr-FR
type: ddl
encoding: UTF-8
links:
  - http://demo.invalid/
caps:
  modes:
    search: [q]
settings: []
search:
  paths:
    - path: /search.php
  rows:
    # phpBB prosilver result rows alternate bg1/bg2; a site footer block is
    # a bare li.row and must not match.
    selector: li.row.bg1, li.row.bg2
  fields:
    title:
      selector: a.topictitle
    details:
      selector: a.topictitle
      attribute: href
    download:
      selector: a.topictitle
      attribute: href
download:
  # The links are hidden until a reply is posted on the topic (phpBB
  # quick-reply form). onlyiflocked: the POST only runs while the content
  # is still hidden, so an already-revealed topic is not re-commented.
  before:
    - pathselector:
        selector: a.topictitle
        attribute: href
    - method: post
      path: "posting.php?mode=reply&t={{ .DownloadUri.Query.t }}"
      onlyiflocked: true
      inputs:
        message: "Merci beaucoup pour les liens"
        post: "Envoyer"
      selectorinputs:
        creation_time:
          selector: 'input[name="creation_time"]'
          attribute: value
        form_token:
          selector: 'input[name="form_token"]'
          attribute: value
  selectors:
    - selector: "a[href*='uptobox'], a[href*='1fichier']"
      attribute: href
      usebeforeresponse: true
`

// ddlSearchHTML mimics a phpBB search result page: result rows carry the
// bg1/bg2 classes, and the site's footer block is a bare li.row that the
// rows selector must not match.
const ddlSearchHTML = `<html><body>
<ul class="topiclist topics">
  <li class="row bg1"><a class="topictitle" href="./viewtopic.php?t=1">Demo Release One</a></li>
</ul>
<li class="row"><div class="list-inner">SITE INFORMATION</div></li>
</body></html>`

// ddlTopicHTML mimics a topic page before the unlock reply: content hidden
// behind the placeholder and the quick-reply form carrying its CSRF tokens.
const ddlTopicHTML = `<html><body>
<div class="postbody">
  <div class="content">{HIDEBB_MESSAGE_HIDDEN_DESCRIPTION}</div>
</div>
<form method="post" action="./posting.php?mode=reply&amp;t=1" id="qr_postform">
  <input type="hidden" name="creation_time" value="1785000000" />
  <input type="hidden" name="form_token" value="tok123" />
  <textarea name="message"></textarea>
</form>
</body></html>`

// ddlRevealedHTML mimics a topic page whose content is already revealed:
// the DDL links are visible, no unlock action needed.
const ddlRevealedHTML = `<html><body>
<div class="postbody">
  <div class="content">
    <a href="https://uptobox.com/abc123">Uptobox</a>
  </div>
</div>
</body></html>`

// ddlReplyHTML mimics the page returned after the unlock reply POST: the
// hidden DDL links are now visible.
const ddlReplyHTML = `<html><body>
<div class="postbody">
  <div class="content">
    <a href="https://uptobox.com/abc123">Uptobox</a>
    <a href="https://1fichier.com/?xyz">1fichier</a>
  </div>
</div>
</body></html>`

// ddlFixtureServer serves the reply-unlock chain flow and records the
// requests it receives.
func ddlFixtureServer() (*httptest.Server, *[]string) {
	var requests []string
	mux := http.NewServeMux()
	mux.HandleFunc("/search.php", func(w http.ResponseWriter, r *http.Request) {
		requests = append(requests, r.Method+" "+r.URL.Path)
		fmt.Fprint(w, ddlSearchHTML)
	})
	mux.HandleFunc("/viewtopic.php", func(w http.ResponseWriter, r *http.Request) {
		requests = append(requests, r.Method+" "+r.URL.Path+"?"+r.URL.RawQuery)
		fmt.Fprint(w, ddlTopicHTML)
	})
	mux.HandleFunc("/posting.php", func(w http.ResponseWriter, r *http.Request) {
		requests = append(requests, r.Method+" "+r.URL.Path+"?"+r.URL.RawQuery)
		if r.Method != http.MethodPost {
			t := http.StatusMethodNotAllowed
			http.Error(w, "post only", t)
			return
		}
		fmt.Fprint(w, ddlReplyHTML)
	})
	return httptest.NewServer(mux), &requests
}

func TestDdlDefinition_ReplyUnlockChain(t *testing.T) {
	def, err := definition.Parse([]byte(ddlDefinitionYAML))
	if err != nil {
		t.Fatalf("parsing fixture yaml: %v", err)
	}

	server, requests := ddlFixtureServer()
	defer server.Close()

	provider := cardigannengine.NewProvider(def, server.URL, nil)
	releases, err := provider.Search(context.Background(), cardigannengine.Query{Keywords: "demo"})
	if err != nil {
		t.Fatalf("search: %v", err)
	}

	if len(releases) != 1 {
		t.Fatalf("expected 1 release (the footer li.row block must not match), got %d", len(releases))
	}
	rel := releases[0]

	if rel.Protocol != cardigannrelease.ProtocolDDL {
		t.Errorf("Protocol = %q, want %q", rel.Protocol, cardigannrelease.ProtocolDDL)
	}
	if !strings.Contains(rel.DownloadLink, "uptobox.com") {
		t.Errorf("DownloadLink = %q, want an uptobox link", rel.DownloadLink)
	}
	if !rel.Unlocked {
		t.Error("Unlocked = false, want true (reply POST resolved the links)")
	}

	info := ToReleaseInfo(rel)
	if info.Protocol != model.ProtocolDDL {
		t.Errorf("model Protocol = %q, want %q", info.Protocol, model.ProtocolDDL)
	}
	if len(info.Hosts) == 0 || info.Hosts[0] != "uptobox.com" {
		t.Errorf("Hosts = %v, want [uptobox.com]", info.Hosts)
	}

	// Chain: GET search → GET topic (step 0, row link) → POST the unlock
	// reply on posting.php (step 1, carrying the form CSRF tokens).
	if len(*requests) != 3 {
		t.Fatalf("expected 3 requests (search, topic, reply POST), got %d: %v", len(*requests), *requests)
	}
	if (*requests)[0] != "GET /search.php" {
		t.Errorf("first request = %q, want GET /search.php", (*requests)[0])
	}
	if !strings.HasPrefix((*requests)[1], "GET /viewtopic.php") {
		t.Errorf("second request = %q, want the GET of the topic (step 0)", (*requests)[1])
	}
	if !strings.HasPrefix((*requests)[2], "POST /posting.php") {
		t.Errorf("third request = %q, want the reply POST (step 1)", (*requests)[2])
	}
}

func TestProvider_SkipSearchDownloadAndResolve(t *testing.T) {
	def, err := definition.Parse([]byte(ddlDefinitionYAML))
	if err != nil {
		t.Fatalf("parsing fixture yaml: %v", err)
	}

	server, requests := ddlFixtureServer()
	defer server.Close()

	// gowlarr builds providers with SkipSearchDownload: the download chain
	// (side-effecting reply POST) must not run during search.
	inner := cardigannengine.NewProvider(def, server.URL, nil)
	inner.SkipSearchDownload()
	provider := NewProvider(inner)

	releases, err := provider.Search(context.Background(), internalsearch.Query{Keywords: "demo"})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(releases) != 1 {
		t.Fatalf("expected 1 release, got %d", len(releases))
	}
	// Raw row download link kept; the chain did not run.
	if releases[0].DownloadLink != "./viewtopic.php?t=1" {
		t.Errorf("DownloadLink = %q, want the raw row link ./viewtopic.php?t=1", releases[0].DownloadLink)
	}
	if releases[0].Unlocked {
		t.Error("release must not be unlocked when the chain did not run")
	}
	if len(*requests) != 1 || (*requests)[0] != "GET /search.php" {
		t.Errorf("search must only hit search.php, got %v", *requests)
	}

	// Explicit --links resolution runs the chain and reveals the links.
	resolved, err := provider.ResolveDownloadLink(context.Background(), releases[0])
	if err != nil {
		t.Fatalf("ResolveDownloadLink: %v", err)
	}
	if !strings.Contains(resolved.DownloadLink, "uptobox.com") {
		t.Errorf("DownloadLink = %q, want an uptobox link", resolved.DownloadLink)
	}
	if !resolved.Unlocked {
		t.Error("Unlocked = false, want true after the reply POST")
	}
	if len(resolved.Hosts) == 0 || resolved.Hosts[0] != "uptobox.com" {
		t.Errorf("Hosts = %v, want [uptobox.com]", resolved.Hosts)
	}
	if len(*requests) != 3 {
		t.Errorf("expected search + topic GET + reply POST, got %v", *requests)
	}
}

func TestDdlDefinition_ReplyUnlockSkipsWhenRevealed(t *testing.T) {
	def, err := definition.Parse([]byte(ddlDefinitionYAML))
	if err != nil {
		t.Fatalf("parsing fixture yaml: %v", err)
	}

	var requests []string
	mux := http.NewServeMux()
	mux.HandleFunc("/search.php", func(w http.ResponseWriter, r *http.Request) {
		requests = append(requests, r.Method+" "+r.URL.Path)
		fmt.Fprint(w, ddlSearchHTML)
	})
	// The topic is already revealed: links visible, no unlock needed.
	mux.HandleFunc("/viewtopic.php", func(w http.ResponseWriter, r *http.Request) {
		requests = append(requests, r.Method+" "+r.URL.Path)
		fmt.Fprint(w, ddlRevealedHTML)
	})
	// The unlock POST must never be reached.
	mux.HandleFunc("/posting.php", func(w http.ResponseWriter, r *http.Request) {
		requests = append(requests, r.Method+" "+r.URL.Path)
		http.Error(w, "must not be called", http.StatusInternalServerError)
	})
	server := httptest.NewServer(mux)
	defer server.Close()

	provider := cardigannengine.NewProvider(def, server.URL, nil)
	releases, err := provider.Search(context.Background(), cardigannengine.Query{Keywords: "demo"})
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if len(releases) != 1 {
		t.Fatalf("expected 1 release, got %d", len(releases))
	}
	if !strings.Contains(releases[0].DownloadLink, "uptobox.com") {
		t.Errorf("DownloadLink = %q, want the already-revealed uptobox link", releases[0].DownloadLink)
	}
	for _, req := range requests {
		if strings.HasPrefix(req, "POST /posting.php") {
			t.Errorf("unlock POST must be skipped when the content is already revealed: %v", requests)
		}
	}
}

// thelostspaceDefinitionYAML mirrors definitions-local/thelostspace.yml
// (MyBB advanced thread search restricted to release forums + on-demand NZB
// chain), inlined so the test runs in CI without the gitignored definitions
// directory.
const thelostspaceDefinitionYAML = `
id: thelostspace-demo
name: The Lost Space
description: demo usenet forum (MyBB)
language: fr-FR
type: usenet
encoding: UTF-8
links:
  - http://demo.invalid/
caps:
  modes:
    search: [q]
settings: []
search:
  paths:
    - path: search.php?action=do_search
  inputs:
    keywords: "{{ .Keywords }}"
    postthread: 1
    showresults: threads
    $raw: "&forums[]=4&forums[]=5&forums[]=8&forums[]=25&forums[]=70&forums[]=75"
  rows:
    # Only rows carrying a thread link are results: MyBB's "no results" page
    # also renders a table.tborder (message row without showthread links), and
    # matching it would fail every row's title extraction.
    selector: table.tborder tr:has(a[href*="showthread.php?tid="])
  fields:
    title:
      selector: a[href*="showthread.php?tid="]
    details:
      selector: a[href*="showthread.php?tid="]
      attribute: href
    date:
      selector: td:nth-child(7) span.smalltext
      optional: true
      filters:
        - name: regexp
          args: "([0-9]{2}\\.[0-9]{2}\\.[0-9]{4}, [0-9]{2}:[0-9]{2})"
        - name: dateparse
          args: "dd.MM.yyyy, HH:mm"
    download:
      selector: a[href*="showthread.php?tid="]
      attribute: href
download:
  before:
    - pathselector:
        selector: a[href*="showthread.php?tid="]
        attribute: href
  selectors:
    - selector: a[href*="attachment.php?aid="]
      attribute: href
      usebeforeresponse: true
`

// thelostspaceSearchHTML mimics a MyBB advanced thread-search result page:
// title block in the 3rd td (a single showthread link), forum in the 4th,
// last-post date in the 7th td (with trailing "Dernier message" text). A
// thead header row (no showthread link) precedes the results, like the live
// page. The third row is a chit-chat thread in the community forum (fid 11)
// without an attachment; category filtering happens at query level, so it
// still parses.
const thelostspaceSearchHTML = `<html><body>
<table class="tborder">
  <tr class="thead"><td colspan="8">Résultats de la recherche</td></tr>
  <tr class="inline_row">
    <td align="center" class="trow1" width="2%">&nbsp;</td>
    <td align="center" class="trow1" width="2%">&nbsp;</td>
    <td class="trow1"><div class="float_right"><img src="/images/paperclip.png" alt="" title="Ce sujet contient une pièce jointe." /></div><div><span><a href="showthread.php?tid=10&amp;highlight=tsundere" class="subject_editable">BLOOM S01 MULTi 1080p WEB x264 AAC -Tsundere-Raws (NF)</a></span></div></td>
    <td class="trow1"><a href="forumdisplay.php?fid=48">1080p</a></td>
    <td align="center" class="trow1">0</td>
    <td align="center" class="trow1">3</td>
    <td class="trow1" style="white-space: nowrap"><span class="smalltext"> 11.05.2026, 16:20<br /> <a href="showthread.php?tid=10&amp;action=lastpost">Dernier message</a>: <a href="member.php?action=profile&amp;uid=2">Kogami</a> </span></td>
    <td class="trow1" align="center" style="white-space: nowrap"><input type="checkbox" class="checkbox" name="inlinemod_10" id="inlinemod_10" value="1" /></td>
  </tr>
  <tr class="inline_row">
    <td align="center" class="trow2" width="2%">&nbsp;</td>
    <td align="center" class="trow2" width="2%">&nbsp;</td>
    <td class="trow2"><div class="float_right"><img src="/images/paperclip.png" alt="" title="Ce sujet contient une pièce jointe." /></div><div><span><a href="showthread.php?tid=11&amp;highlight=tsundere" class="subject_editable">Dragon Ball Super S01 MULTi 1080p WEB x264 AAC -Tsundere-Raws (ADN)</a></span></div></td>
    <td class="trow2"><a href="forumdisplay.php?fid=48">1080p</a></td>
    <td align="center" class="trow2">0</td>
    <td align="center" class="trow2">1</td>
    <td class="trow2" style="white-space: nowrap"><span class="smalltext"> 12.02.2026, 18:08<br /> <a href="showthread.php?tid=11&amp;action=lastpost">Dernier message</a>: <a href="member.php?action=profile&amp;uid=2">Kogami</a> </span></td>
    <td class="trow2" align="center" style="white-space: nowrap"><input type="checkbox" class="checkbox" name="inlinemod_11" id="inlinemod_11" value="1" /></td>
  </tr>
  <tr class="inline_row">
    <td align="center" class="trow1" width="2%">&nbsp;</td>
    <td align="center" class="trow1" width="2%">&nbsp;</td>
    <td class="trow1"><div><span><a href="showthread.php?tid=99&amp;highlight=tsundere" class="subject_editable">Voili voilou</a></span></div></td>
    <td class="trow1"><a href="forumdisplay.php?fid=11">BLA-BLA</a></td>
    <td align="center" class="trow1">12</td>
    <td align="center" class="trow1">301</td>
    <td class="trow1" style="white-space: nowrap"><span class="smalltext"> 09.01.2026, 21:03<br /> <a href="showthread.php?tid=99&amp;action=lastpost">Dernier message</a>: <a href="member.php?action=profile&amp;uid=9">Tartempion</a> </span></td>
    <td class="trow1" align="center" style="white-space: nowrap"><input type="checkbox" class="checkbox" name="inlinemod_99" id="inlinemod_99" value="1" /></td>
  </tr>
</table>
</body></html>`

// thelostspaceThreadHTML mimics a release thread page: the NZB attachment
// link only exists here, never on the search page.
const thelostspaceThreadHTML = `<html><body>
<div class="postbody">
  <div class="content"><a href="attachment.php?aid=100">BLOOM-S01.nzb</a></div>
</div>
</body></html>`

// thelostspaceJunkThreadHTML mimics a chit-chat thread page: no attachment.
const thelostspaceJunkThreadHTML = `<html><body>
<div class="postbody">
  <div class="content">Voili voilou, rien à télécharger ici.</div>
</div>
</body></html>`

func TestTheLostSpaceDefinition_Search(t *testing.T) {
	def, err := definition.Parse([]byte(thelostspaceDefinitionYAML))
	if err != nil {
		t.Fatalf("parsing fixture yaml: %v", err)
	}

	var searchQuery url.Values
	var requests []string
	mux := http.NewServeMux()
	mux.HandleFunc("/search.php", func(w http.ResponseWriter, r *http.Request) {
		requests = append(requests, r.Method+" "+r.URL.Path+"?"+r.URL.RawQuery)
		searchQuery = r.URL.Query()
		fmt.Fprint(w, thelostspaceSearchHTML)
	})
	server := httptest.NewServer(mux)
	defer server.Close()

	// gowlarr builds providers with SkipSearchDownload: the NZB chain must
	// not run during search.
	inner := cardigannengine.NewProvider(def, server.URL, nil)
	inner.SkipSearchDownload()
	provider := NewProvider(inner)

	releases, err := provider.Search(context.Background(), internalsearch.Query{Keywords: "tsundere"})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(releases) != 3 {
		t.Fatalf("expected 3 releases, got %d", len(releases))
	}

	rel := releases[0]
	if rel.DownloadLink != "showthread.php?tid=10&highlight=tsundere" {
		t.Errorf("DownloadLink = %q, want the raw thread URL", rel.DownloadLink)
	}
	want := time.Date(2026, 5, 11, 16, 20, 0, 0, time.UTC)
	if !rel.PublishDate.Equal(want) {
		t.Errorf("PublishDate = %v, want %v (parsed past the 'Dernier message' text)", rel.PublishDate, want)
	}
	if rel.Protocol != model.ProtocolUsenet {
		t.Errorf("Protocol = %q, want %q", rel.Protocol, model.ProtocolUsenet)
	}
	// The junk thread parses too: category filtering happens at query level.
	if releases[2].Title != "Voili voilou" {
		t.Errorf("releases[2].Title = %q, want %q", releases[2].Title, "Voili voilou")
	}
	if want := time.Date(2026, 1, 9, 21, 3, 0, 0, time.UTC); !releases[2].PublishDate.Equal(want) {
		t.Errorf("junk PublishDate = %v, want %v", releases[2].PublishDate, want)
	}

	if got := searchQuery["forums[]"]; strings.Join(got, ",") != "4,5,8,25,70,75" {
		t.Errorf("forums[] = %v, want [4 5 8 25 70 75]", got)
	}
	if got := searchQuery.Get("showresults"); got != "threads" {
		t.Errorf("showresults = %q, want %q", got, "threads")
	}
	if len(requests) != 1 || !strings.HasPrefix(requests[0], "GET /search.php?") {
		t.Errorf("expected a single GET /search.php, got %v", requests)
	}
}

// mybbNoResultsHTML mimics MyBB's "no results" page for a query with
// no match: a table.tborder whose rows carry no showthread links. Matching
// it as rows would fail every title extraction.
const mybbNoResultsHTML = `<html><body>
<table class="tborder">
  <tr class="thead"><td colspan="2">The Lost Space</td></tr>
  <tr><td>Désolé, mais aucun résultat n&rsquo;a été trouvé avec votre requête. Veuillez réessayer après avoir redéfini les termes de la recherche.</td></tr>
</table>
</body></html>`

func TestNoResultsPage_ReturnsNoReleases(t *testing.T) {
	def, err := definition.Parse([]byte(thelostspaceDefinitionYAML))
	if err != nil {
		t.Fatalf("parsing fixture yaml: %v", err)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/search.php", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, mybbNoResultsHTML)
	})
	server := httptest.NewServer(mux)
	defer server.Close()

	inner := cardigannengine.NewProvider(def, server.URL, nil)
	inner.SkipSearchDownload()
	provider := NewProvider(inner)

	releases, err := provider.Search(context.Background(), internalsearch.Query{Keywords: "The Deer King 2021"})
	if err != nil {
		t.Fatalf("Search must not error on a no-results page: %v", err)
	}
	if len(releases) != 0 {
		t.Fatalf("expected 0 releases on a no-results page, got %d", len(releases))
	}
}

func TestTheLostSpaceDefinition_ResolveLinks(t *testing.T) {
	def, err := definition.Parse([]byte(thelostspaceDefinitionYAML))
	if err != nil {
		t.Fatalf("parsing fixture yaml: %v", err)
	}

	var requests []string
	mux := http.NewServeMux()
	mux.HandleFunc("/showthread.php", func(w http.ResponseWriter, r *http.Request) {
		requests = append(requests, r.Method+" "+r.URL.Path+"?"+r.URL.RawQuery)
		if r.URL.Query().Get("tid") == "99" {
			fmt.Fprint(w, thelostspaceJunkThreadHTML)
			return
		}
		fmt.Fprint(w, thelostspaceThreadHTML)
	})
	server := httptest.NewServer(mux)
	defer server.Close()

	// Absolutization resolves against Def().Links[0]; production passes
	// def.Links[0] as the provider base URL, mirror that here.
	def.Links[0] = server.URL
	provider := NewProvider(cardigannengine.NewProvider(def, server.URL, nil))

	rel := model.ReleaseInfo{
		Title:        "BLOOM S01 MULTi 1080p WEB x264 AAC -Tsundere-Raws (NF)",
		DownloadLink: "showthread.php?tid=10&highlight=tsundere",
		Protocol:     model.ProtocolUsenet,
		IndexerID:    "thelostspace.xyz",
	}
	resolved, err := provider.ResolveDownloadLink(context.Background(), rel)
	if err != nil {
		t.Fatalf("ResolveDownloadLink: %v", err)
	}
	if want := server.URL + "/attachment.php?aid=100"; resolved.DownloadLink != want {
		t.Errorf("DownloadLink = %q, want %q (absolutized)", resolved.DownloadLink, want)
	}

	// A chit-chat thread has no attachment: the chain errors and the raw
	// link is kept (the service layer logs and keeps it).
	junk := model.ReleaseInfo{
		Title:        "Voili voilou",
		DownloadLink: "showthread.php?tid=99&highlight=tsundere",
		Protocol:     model.ProtocolUsenet,
		IndexerID:    "thelostspace.xyz",
	}
	resolvedJunk, err := provider.ResolveDownloadLink(context.Background(), junk)
	if err == nil {
		t.Error("expected an error resolving a thread without an attachment")
	}
	if resolvedJunk.DownloadLink != junk.DownloadLink {
		t.Errorf("DownloadLink = %q, want the raw link kept %q", resolvedJunk.DownloadLink, junk.DownloadLink)
	}
}
