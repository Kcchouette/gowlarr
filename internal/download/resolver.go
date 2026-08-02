// Package download resolves a model.ReleaseInfo into a concrete file
// (.torrent/.nzb) or magnet link, based on the detected protocol of the
// result — the user never has to choose the protocol manually.
package download

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/Kcchouette/gowlarr/internal/model"
	"github.com/Kcchouette/gowlarr/internal/netutil"
)

// Artifact represents the file obtained after resolving a ReleaseInfo.
type Artifact struct {
	// Filename suggested for saving to disk.
	Filename string
	// IsMagnet indicates that Content is a plain magnet: text link, not a binary stream.
	IsMagnet bool
	// Content is the file content (.torrent/.nzb) or the magnet link text.
	Content []byte
}

// httpDoer is satisfied by both *http.Client and *httpclient.IndexerClient
// (persistent cookies/rate-limit, Slice C), for indexers requiring an
// authenticated session before download.
type httpDoer interface {
	Do(*http.Request) (*http.Response, error)
}

// Resolver resolves a ReleaseInfo into a downloadable Artifact.
type Resolver struct {
	HTTPClient  httpDoer
	AuthHeaders map[string]string
}

// NewResolverWithClient builds a Resolver with a specific HTTP client.
func NewResolverWithClient(client httpDoer, authHeaders map[string]string) *Resolver {
	return &Resolver{HTTPClient: client, AuthHeaders: authHeaders}
}

// Resolve downloads/resolves the actual link of a ReleaseInfo:
//   - magnet link: returned as-is (no binary stream to download);
//   - usenet protocol: direct .nzb stream retrieval;
//   - non-magnet torrent protocol: direct .torrent stream retrieval.
//
// Indexers requiring an authenticated session reuse the already-connected
// HTTP client (r.HTTPClient, potentially an httpclient.IndexerClient with
// persistent cookies — see Slice C).
//
// Note: resolution via an intermediate page (`download.selectors` Cardigann)
// is not supported — the actual structure of this mechanism (Filters,
// Before/session priming, Method, Infohash) is richer than a simple
// selector+attribute, and a partial branch would silently break indexers
// that actually depend on it. Full support would be a separate project.
func (r *Resolver) Resolve(ctx context.Context, release model.ReleaseInfo) (Artifact, error) {
	// DDL and streaming results are display-only by design: Gowlarr never
	// downloads them automatically (no debrid/aria2 in v1). The user copies
	// the links from `gowlarr search --links`.
	switch release.Protocol {
	case model.ProtocolDDL, model.ProtocolStreaming:
		return Artifact{}, fmt.Errorf("protocol %q is display-only: use `gowlarr search --links` to view the link(s)", release.Protocol)
	}

	if strings.HasPrefix(release.DownloadLink, "magnet:") {
		return Artifact{
			Filename: sanitizeFilename(release.Title) + ".magnet.txt",
			IsMagnet: true,
			Content:  []byte(release.DownloadLink),
		}, nil
	}

	// trustedHost is derived from the initial link provided by the indexer:
	// AuthHeaders (indexer credentials) must never be sent to a different
	// host, even if a redirect later points to an arbitrary third-party host
	// (risk of credential leakage, per security audit).
	trustedHost := hostOf(release.DownloadLink)

	body, err := r.fetch(ctx, release.DownloadLink, trustedHost)
	if err != nil {
		return Artifact{}, fmt.Errorf("downloading release %q from %s: %w", release.Title, release.IndexerID, err)
	}

	ext := ".torrent"
	if release.Protocol == model.ProtocolUsenet {
		ext = ".nzb"
	}

	return Artifact{
		Filename: sanitizeFilename(release.Title) + ext,
		Content:  body,
	}, nil
}

func (r *Resolver) fetch(ctx context.Context, link string, trustedHost string) ([]byte, error) {
	// Best-effort pre-request SSRF check only: this does not protect against
	// DNS rebinding at actual dial time, does not re-validate redirects, and
	// cannot see through per-indexer HTTP/SOCKS proxies where the final
	// destination is opaque to this check.
	if err := netutil.ValidateURL(link); err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, link, nil)
	if err != nil {
		return nil, fmt.Errorf("building download request: %w", err)
	}
	req.Header.Set("User-Agent", "gowlarr/0.1 (+https://github.com/Kcchouette/gowlarr)")

	// AuthHeaders (indexer credentials) are only attached if the request
	// targets the original trusted host: an intermediate page or a link
	// extracted by a selector must never be able to siphon these credentials
	// to an arbitrary third-party host.
	if trustedHost != "" && hostOf(link) == trustedHost {
		for header, value := range r.AuthHeaders {
			req.Header.Set(header, value)
		}
	}

	resp, err := r.HTTPClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("http status %d", resp.StatusCode)
	}

	return io.ReadAll(io.LimitReader(resp.Body, 64<<20)) // 64 MiB max, memory safety guard.
}

// hostOf returns the hostname (without port) of rawURL, or an empty string
// if the URL is invalid.
func hostOf(rawURL string) string {
	u, err := url.Parse(rawURL)
	if err != nil {
		return ""
	}
	return u.Hostname()
}

func sanitizeFilename(name string) string {
	replacer := strings.NewReplacer(
		"/", "_", "\\", "_", ":", "_", "*", "_", "?", "_",
		"\"", "_", "<", "_", ">", "_", "|", "_",
	)
	clean := replacer.Replace(name)
	if len(clean) > 150 {
		clean = clean[:150]
	}
	if clean == "" {
		clean = "release"
	}
	return clean
}
