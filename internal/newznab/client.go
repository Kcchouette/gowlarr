// Package newznab implements a native client for the generic Newznab protocol
// (standard usenet indexers, outside the Cardigann engine). It is part of the
// MVP (Slice F), explicitly requested by the user to retain Prowlarr's usenet
// support.
//
// Prowlarr's "Usenet" goes through web indexers that expose the Newznab API
// (HTTP/REST + RSS), never through a direct NNTP connection — this module
// remains a simple HTTP client, not an NNTP client.
package newznab

import (
	"bufio"
	"context"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"golang.org/x/net/proxy"

	"github.com/Kcchouette/gowlarr/internal/model"
	"github.com/Kcchouette/gowlarr/internal/netutil"
	"github.com/Kcchouette/gowlarr/internal/search"
)

// Client queries a usenet indexer speaking the standard Newznab protocol.
type Client struct {
	BaseURL     string
	APIKey      string
	IndexerID   string
	IndexerName string
	HTTPClient  *http.Client
	// apiURL is the full API endpoint (BaseURL + "/api" for a bare origin,
	// BaseURL verbatim when it carries a custom path), computed once.
	apiURL string
}

// sharedTransport is a tuned transport shared by all Newznab clients:
// it keeps idle connections alive and pooled instead of relying on
// http.DefaultTransport's tight per-host limit. Proxy: ProxyFromEnvironment
// preserves the default transport's env-var proxy behavior.
var sharedTransport = &http.Transport{
	Proxy:               http.ProxyFromEnvironment,
	MaxIdleConns:        100,
	MaxIdleConnsPerHost: 10,
	IdleConnTimeout:     90 * time.Second,
}

// New builds a generic Newznab client. The shared transport is always
// cloned (never mutated); when custom DNS servers are configured, a
// resolver-backed DialContext is installed on the clone.
func New(id, name, baseURL, apiKey string) *Client {
	transport := cloneSharedTransport()
	transportWithDNSServers(transport)
	return newClient(id, name, baseURL, apiKey, transport)
}

// cloneSharedTransport returns a per-client copy of the shared transport.
// ForceAttemptHTTP2 is set unconditionally on purpose: Clone() first runs the
// source transport's HTTP/2 auto-setup, so the copy inherits the source's
// post-setup TLSClientConfig (which already advertises ALPN "h2") but NOT its
// TLSNextProto map. Without the force, protocols() treats that inherited
// TLSClientConfig as a custom TLS config, disables HTTP/2, and the client
// then offers h2 via ALPN while reading the response as HTTP/1.1 — any
// h2-speaking server breaks the request with "malformed HTTP response".
func cloneSharedTransport() *http.Transport {
	transport := sharedTransport.Clone()
	transport.ForceAttemptHTTP2 = true
	return transport
}

// transportWithDNSServers installs the custom-resolver DialContext when
// DNS servers are configured. No-op otherwise. Must not be applied on
// top of a SOCKS5 DialContext (would bypass the proxy).
func transportWithDNSServers(tr *http.Transport) {
	if !netutil.HasDNSServers() {
		return
	}
	tr.DialContext = netutil.DialContextFor()
	tr.ForceAttemptHTTP2 = true // custom DialContext disables HTTP/2 by default
}

// apiEndpoint returns the API endpoint for a base URL: the standard Newznab
// endpoint is <base>/api, but an indexer exposing its API under a custom
// path (e.g. api/v1/newznab) passes the full path verbatim.
func apiEndpoint(baseURL string) string {
	trimmed := strings.TrimRight(baseURL, "/")
	u, err := url.Parse(trimmed)
	if err != nil || u.Path == "" {
		return trimmed + "/api"
	}
	return trimmed
}

// newClient builds the client around a pre-built transport.
func newClient(id, name, baseURL, apiKey string, transport *http.Transport) *Client {
	trimmed := strings.TrimRight(baseURL, "/")
	return &Client{
		BaseURL:     trimmed,
		APIKey:      apiKey,
		IndexerID:   id,
		IndexerName: name,
		HTTPClient:  &http.Client{Timeout: 20 * time.Second, Transport: transport}, // Newznab typically responds quickly; 20s gives network margin without blocking the UI too long.
		apiURL:      apiEndpoint(trimmed),
	}
}

// NewWithTransport builds a generic Newznab client honoring an optional
// per-indexer proxy (http://, https://, socks5://). The proxy scheme is
// validated; an unsupported scheme returns an error. Custom DNS servers
// (netutil) apply to the direct path and to the HTTP-proxy hostname, but
// not on top of a SOCKS5 dialer (remote resolution, proxy preserved).
func NewWithTransport(id, name, baseURL, apiKey, proxyURL string) (*Client, error) {
	transport := cloneSharedTransport()

	if proxyURL != "" {
		u, err := url.Parse(proxyURL)
		if err != nil {
			return nil, fmt.Errorf("parsing proxy URL: %w", err)
		}
		switch u.Scheme {
		case "http", "https":
			transport.Proxy = http.ProxyURL(u)
			// The destination is resolved by the HTTP proxy, but the proxy
			// hostname itself is dialed directly — honor custom DNS servers.
			transportWithDNSServers(transport)
		case "socks5":
			transport.Proxy = nil // the socks5 dialer IS the proxy path; an env proxy must not also apply
			dialer, err := proxy.FromURL(u, proxy.Direct)
			if err != nil {
				return nil, fmt.Errorf("building socks5 dialer: %w", err)
			}
			// Prefer the context-aware dialer so request deadlines propagate
			// to the SOCKS5 handshake; fall back to the plain Dial. No
			// DNS-server DialContext on top: it would bypass the proxy.
			if cd, ok := dialer.(proxy.ContextDialer); ok {
				transport.DialContext = cd.DialContext
			} else {
				transport.DialContext = func(ctx context.Context, network, addr string) (net.Conn, error) {
					return dialer.Dial(network, addr)
				}
			}
		default:
			return nil, fmt.Errorf("unsupported proxy scheme %q (only http/https/socks5)", u.Scheme)
		}
	} else {
		transportWithDNSServers(transport)
	}

	return newClient(id, name, baseURL, apiKey, transport), nil
}

func (c *Client) ID() string               { return c.IndexerID }
func (c *Client) Name() string             { return c.IndexerName }
func (c *Client) Protocol() model.Protocol { return model.ProtocolUsenet }

const (
	SearchTypeSearch   = "search"
	SearchTypeTVSearch = "tvsearch"
	SearchTypeMovie    = "movie"
	SearchTypeMusic    = "music"
	SearchTypeBook     = "book"
)

// rssFeed / rssItem / rssAttr model the RSS 2.0 + newznab:attr extension
// response returned by the Newznab API (t=search|tvsearch|movie|music|book).
type rssFeed struct {
	XMLName xml.Name `xml:"rss"`
	Channel struct {
		Items []rssItem `xml:"item"`
	} `xml:"channel"`
}

type rssItem struct {
	Title string `xml:"title"`
	Link  string `xml:"link"`
	// Enclosure carries the downloadable asset per the Newznab spec; <link>
	// may point at a details page instead (feeds of some anime mirrors do).
	Enclosure struct {
		URL string `xml:"url,attr"`
	} `xml:"enclosure"`
	GUID    string    `xml:"guid"`
	PubDate string    `xml:"pubDate"`
	Attrs   []rssAttr `xml:"attr"`
}

type rssAttr struct {
	Name  string `xml:"name,attr"`
	Value string `xml:"value,attr"`
}

// errorXML models the <error code="..." description="..."/> response the
// Newznab API returns for failed requests (e.g. an invalid API key).
type errorXML struct {
	Code        string `xml:"code,attr"`
	Description string `xml:"description,attr"`
}

func (it rssItem) attr(name string) string {
	for _, a := range it.Attrs {
		if strings.EqualFold(a.Name, name) {
			return a.Value
		}
	}
	return ""
}

// Search builds the request with the appropriate search type
// and parses the RSS + newznab:attr response.
func (c *Client) Search(ctx context.Context, q search.Query) ([]model.ReleaseInfo, error) {
	params := url.Values{}

	// Use SearchType from query, default to "search"
	searchType := q.SearchType
	if searchType == "" {
		searchType = SearchTypeSearch
	}
	params.Set("t", searchType)
	// Spotweb-style public Newznab endpoints answer only when the apikey
	// parameter is omitted; an empty apikey= is treated as invalid.
	if c.APIKey != "" {
		params.Set("apikey", c.APIKey)
	}
	if q.Keywords != "" {
		params.Set("q", q.Keywords)
	}
	if len(q.Categories) > 0 {
		cats := make([]string, len(q.Categories))
		for i, cat := range q.Categories {
			cats[i] = strconv.Itoa(cat)
		}
		params.Set("cat", strings.Join(cats, ","))
	}

	// Type-specific parameters
	if q.Season > 0 {
		params.Set("season", strconv.Itoa(q.Season))
	}
	if q.Episode > 0 {
		params.Set("ep", strconv.Itoa(q.Episode))
	}
	if q.IMDbID != "" {
		params.Set("imdbid", q.IMDbID)
	}
	if q.TMDBID != "" {
		params.Set("tmdbid", q.TMDBID)
	}

	reqURL := fmt.Sprintf("%s?%s", c.apiURL, params.Encode())
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return nil, fmt.Errorf("building newznab request: %w", err)
	}

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("querying newznab indexer %s: %w", c.IndexerID, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("newznab indexer %s returned HTTP %d", c.IndexerID, resp.StatusCode)
	}

	// A bad API key is reported as HTTP 200 + <error code="100"
	// description="Invalid API Key"/> (not a 4xx): sniff the root element
	// with a non-consuming peek so the error is surfaced as an
	// authentication failure instead of a misleading RSS parse error.
	br := bufio.NewReader(resp.Body)
	head, err := br.Peek(1024)
	if err != nil && !errors.Is(err, io.EOF) {
		return nil, fmt.Errorf("reading newznab response from %s: %w", c.IndexerID, err)
	}
	headLower := strings.ToLower(string(head))
	if !strings.Contains(headLower, "<rss") && strings.Contains(headLower, "<error") {
		var e errorXML
		// Require at least one attribute: an unrelated element merely
		// containing the substring "error" must not produce "unknown error
		// (code )" — it falls through to the RSS parse error instead.
		if err := xml.NewDecoder(br).Decode(&e); err == nil && (e.Code != "" || e.Description != "") {
			desc := e.Description
			if desc == "" {
				desc = "unknown error"
			}
			return nil, fmt.Errorf("newznab indexer %s: %s (code %s)", c.IndexerID, desc, e.Code)
		}
	}

	var feed rssFeed
	if err := xml.NewDecoder(br).Decode(&feed); err != nil {
		return nil, fmt.Errorf("parsing newznab RSS response from %s: %w", c.IndexerID, err)
	}

	releases := make([]model.ReleaseInfo, 0, len(feed.Channel.Items))
	for _, it := range feed.Channel.Items {
		size, _ := strconv.ParseInt(it.attr("size"), 10, 64)
		var categories []int
		if catID, err := strconv.Atoi(it.attr("category")); err == nil {
			categories = []int{catID}
		}

		pubDate := parseRSSDate(it.PubDate)

		// The enclosure is the download asset (Newznab spec); fall back to
		// <link>, then the GUID, for feeds that leave it out.
		downloadLink := it.Enclosure.URL
		if downloadLink == "" {
			downloadLink = it.Link
		}
		if downloadLink == "" {
			downloadLink = it.GUID
		}

		releases = append(releases, model.ReleaseInfo{
			Title:        it.Title,
			Details:      it.GUID,
			DownloadLink: downloadLink,
			Size:         size,
			PublishDate:  pubDate,
			Categories:   categories,
			Protocol:     model.ProtocolUsenet,
			IndexerID:    c.IndexerID,
			IndexerName:  c.IndexerName,
		})
	}

	return releases, nil
}

// parseRSSDate attempts the usual RSS date formats (RFC1123Z is the
// standard, but some indexers vary slightly). RFC3339 is tried early because
// it is common in modern feeds; the formats are disjoint so the order is
// semantically irrelevant, only a matter of average attempt count.
func parseRSSDate(raw string) time.Time {
	layouts := []string{
		time.RFC1123Z,
		time.RFC1123,
		time.RFC3339,
		"Mon, 2 Jan 2006 15:04:05 -0700",
	}
	for _, layout := range layouts {
		if t, err := time.Parse(layout, raw); err == nil {
			return t.UTC()
		}
	}
	return time.Time{}
}
