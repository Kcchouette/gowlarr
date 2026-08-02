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

// japanfanSearchHTML mimics a phpBB search result page (two topics, one row
// is skipped by rows.after).
const japanfanSearchHTML = `<html><body>
<ul class="topiclist">
  <li class="row"><a class="topictitle" href="./viewtopic.php?t=1">Bleach S01E01</a></li>
  <li class="row"><a class="topictitle" href="./viewtopic.php?t=2">Naruto S01E02 [1F]</a></li>
</ul>
</body></html>`

// japanfanTopicHTML mimics a viewtopic page before the thanks POST: content
// hidden by hideBBcode and the thanks link present.
const japanfanTopicHTML = `<html><body>
<div class="postbody">
  <div class="content">{HIDEBB_MESSAGE_HIDDEN_DESCRIPTION}</div>
  <a id="lnk_thanks_post_1" href="./viewtopic.php?f=5&p=1&thanks=1&to_id=2&from_id=3">Merci</a>
</div>
</body></html>`

// japanfanThanksHTML mimics the viewtopic page after the thanks POST: the
// hidden DDL links are now visible.
const japanfanThanksHTML = `<html><body>
<div class="postbody">
  <div class="content">
    <a href="https://uptobox.com/abc123">Uptobox</a>
    <a href="https://1fichier.com/?xyz">1fichier</a>
  </div>
</div>
</body></html>`

func TestJapanfanDefinition_ThanksChain(t *testing.T) {
	def := loadLocalDefinition(t, "japanfan")

	var requests []string
	mux := http.NewServeMux()
	mux.HandleFunc("/search.php", func(w http.ResponseWriter, r *http.Request) {
		requests = append(requests, r.Method+" "+r.URL.Path)
		fmt.Fprint(w, japanfanSearchHTML)
	})
	mux.HandleFunc("/viewtopic.php", func(w http.ResponseWriter, r *http.Request) {
		requests = append(requests, r.Method+" "+r.URL.Path+"?"+r.URL.RawQuery)
		if r.Method == http.MethodPost {
			fmt.Fprint(w, japanfanThanksHTML)
			return
		}
		fmt.Fprint(w, japanfanTopicHTML)
	})
	server := httptest.NewServer(mux)
	defer server.Close()

	provider := cardigannengine.NewProvider(def, server.URL, nil)
	releases, err := provider.Search(context.Background(), cardigannengine.Query{Keywords: "bleach"})
	if err != nil {
		t.Fatalf("search: %v", err)
	}

	if len(releases) != 1 {
		t.Fatalf("expected 1 release (one row skipped by after: 1), got %d", len(releases))
	}
	rel := releases[0]

	if rel.Protocol != cardigannrelease.ProtocolDDL {
		t.Errorf("Protocol = %q, want %q", rel.Protocol, cardigannrelease.ProtocolDDL)
	}
	if !strings.Contains(rel.DownloadLink, "uptobox.com") {
		t.Errorf("DownloadLink = %q, want an uptobox link", rel.DownloadLink)
	}
	if !rel.Unlocked {
		t.Error("Unlocked = false, want true (thanks POST resolved the links)")
	}

	info := ToReleaseInfo(rel)
	if info.Protocol != model.ProtocolDDL {
		t.Errorf("model Protocol = %q, want %q", info.Protocol, model.ProtocolDDL)
	}
	if len(info.Hosts) == 0 || info.Hosts[0] != "uptobox.com" {
		t.Errorf("Hosts = %v, want [uptobox.com]", info.Hosts)
	}

	// Chain: GET search → GET viewtopic (step 0, row link) → POST thanks
	// (step 1, thanks link from the topic page).
	if len(requests) != 3 {
		t.Fatalf("expected 3 requests (search, topic, thanks), got %d: %v", len(requests), requests)
	}
	if requests[0] != "GET /search.php" {
		t.Errorf("first request = %q, want GET /search.php", requests[0])
	}
	if !strings.HasPrefix(requests[1], "GET /viewtopic.php") {
		t.Errorf("second request = %q, want the GET of the topic (step 0)", requests[1])
	}
	if !strings.HasPrefix(requests[2], "POST /viewtopic.php") {
		t.Errorf("third request = %q, want the thanks POST (step 1)", requests[2])
	}
}

// thelostspaceSearchHTML mimics a MyBB search result page with two threads
// carrying NZB attachments.
const thelostspaceSearchHTML = `<html><body>
<table class="tborder">
  <tr>
    <td><a href="showthread.php?tid=10">Bleach S01</a></td>
    <td><a href="attachment.php?aid=100">bleach-s01.nzb</a></td>
  </tr>
  <tr>
    <td><a href="showthread.php?tid=11">Naruto S02</a></td>
    <td><a href="attachment.php?aid=101">naruto-s02.nzb</a></td>
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
	releases, err := provider.Search(context.Background(), cardigannengine.Query{Keywords: "bleach"})
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
