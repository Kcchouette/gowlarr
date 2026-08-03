package cardigannadapter

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Kcchouette/cardigann-go/definition"
	cardigannengine "github.com/Kcchouette/cardigann-go/engine"
	cardigannrelease "github.com/Kcchouette/cardigann-go/release"

	"github.com/Kcchouette/gowlarr/internal/model"
	internalsearch "github.com/Kcchouette/gowlarr/internal/search"
)

// loadLocalDefinition reads one of the canonical definitions from
// definitions-local/. The directory is gitignored (definitions live in a
// separate repo), so the test skips when the files are absent.
func loadLocalDefinition(t *testing.T, id string) definition.IndexerDefinition {
	t.Helper()
	path := filepath.Join("..", "..", "definitions-local", id+".yml")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Skipf("local definition %s not found (%v); clone the definitions repo into definitions-local/", path, err)
	}
	def, err := definition.Parse(raw)
	if err != nil {
		t.Fatalf("parsing local definition %s: %v", path, err)
	}
	return def
}

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

// thelostspaceSearchHTML mimics a MyBB search result page with two threads
// carrying NZB attachments.
const thelostspaceSearchHTML = `<html><body>
<table class="tborder">
  <tr>
    <td><a href="showthread.php?tid=10">Demo Thread One</a></td>
    <td><a href="attachment.php?aid=100">demo-one.nzb</a></td>
  </tr>
  <tr>
    <td><a href="showthread.php?tid=11">Demo Thread Two</a></td>
    <td><a href="attachment.php?aid=101">demo-two.nzb</a></td>
  </tr>
</table>
</body></html>`

func TestTheLostSpaceDefinition_NZBAttachment(t *testing.T) {
	def := loadLocalDefinition(t, "thelostspace")

	var requests []string
	mux := http.NewServeMux()
	mux.HandleFunc("/search.php", func(w http.ResponseWriter, r *http.Request) {
		requests = append(requests, r.Method+" "+r.URL.Path)
		fmt.Fprint(w, thelostspaceSearchHTML)
	})
	server := httptest.NewServer(mux)
	defer server.Close()

	provider := cardigannengine.NewProvider(def, server.URL, nil)
	releases, err := provider.Search(context.Background(), cardigannengine.Query{Keywords: "demo"})
	if err != nil {
		t.Fatalf("search: %v", err)
	}

	if len(releases) != 2 {
		t.Fatalf("expected 2 releases, got %d", len(releases))
	}
	for _, rel := range releases {
		if rel.Protocol != cardigannrelease.ProtocolUsenet {
			t.Errorf("Protocol = %q, want %q", rel.Protocol, cardigannrelease.ProtocolUsenet)
		}
		if !strings.Contains(rel.DownloadLink, "attachment.php?aid=") {
			t.Errorf("DownloadLink = %q, want an attachment.php link", rel.DownloadLink)
		}
	}
	if len(requests) != 1 || requests[0] != "GET /search.php" {
		t.Errorf("expected a single GET /search.php, got %v", requests)
	}
}
