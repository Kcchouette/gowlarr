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
	"context"
	"encoding/xml"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/Kcchouette/gowlarr/internal/model"
	"github.com/Kcchouette/gowlarr/internal/search"
)

// Client queries a usenet indexer speaking the standard Newznab protocol.
type Client struct {
	BaseURL     string
	APIKey      string
	IndexerID   string
	IndexerName string
	HTTPClient  *http.Client
}

// New builds a generic Newznab client.
func New(id, name, baseURL, apiKey string) *Client {
	return &Client{
		BaseURL:     strings.TrimRight(baseURL, "/"),
		APIKey:      apiKey,
		IndexerID:   id,
		IndexerName: name,
		HTTPClient:  &http.Client{Timeout: 20 * time.Second}, // Newznab typically responds quickly; 20s gives network margin without blocking the UI too long.
	}
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
	Title   string    `xml:"title"`
	Link    string    `xml:"link"`
	GUID    string    `xml:"guid"`
	PubDate string    `xml:"pubDate"`
	Attrs   []rssAttr `xml:"attr"`
}

type rssAttr struct {
	Name  string `xml:"name,attr"`
	Value string `xml:"value,attr"`
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
	params.Set("apikey", c.APIKey)
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

	reqURL := fmt.Sprintf("%s/api?%s", c.BaseURL, params.Encode())
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

	var feed rssFeed
	if err := xml.NewDecoder(resp.Body).Decode(&feed); err != nil {
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

		downloadLink := it.Link
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
// standard, but some indexers vary slightly).
func parseRSSDate(raw string) time.Time {
	layouts := []string{
		time.RFC1123Z,
		time.RFC1123,
		"Mon, 2 Jan 2006 15:04:05 -0700",
		time.RFC3339,
	}
	for _, layout := range layouts {
		if t, err := time.Parse(layout, raw); err == nil {
			return t.UTC()
		}
	}
	return time.Time{}
}

// Caps represents the capabilities of a Newznab indexer.
type Caps struct {
	SearchTypes []string
	Categories  []Category
}

type Category struct {
	ID   int
	Name string
}

// capsXML represents the XML response from /api?t=caps
type capsXML struct {
	XMLName   xml.Name `xml:"caps"`
	Searching struct {
		Search struct {
			Available string `xml:"available,attr"`
		} `xml:"search"`
		TVSearch struct {
			Available string `xml:"available,attr"`
		} `xml:"tv-search"`
		MovieSearch struct {
			Available string `xml:"available,attr"`
		} `xml:"movie-search"`
		MusicSearch struct {
			Available string `xml:"available,attr"`
		} `xml:"music-search"`
		BookSearch struct {
			Available string `xml:"available,attr"`
		} `xml:"book-search"`
	} `xml:"searching"`
	Categories struct {
		Category []capsCategory `xml:"category"`
	} `xml:"categories"`
}

type capsCategory struct {
	ID     string `xml:"id,attr"`
	Name   string `xml:"name,attr"`
	Subcat []struct {
		ID   string `xml:"id,attr"`
		Name string `xml:"name,attr"`
	} `xml:"subcat"`
}

// Caps queries the indexer for its capabilities.
func (c *Client) Caps(ctx context.Context) (Caps, error) {
	reqURL := fmt.Sprintf("%s/api?t=caps&apikey=%s", c.BaseURL, url.QueryEscape(c.APIKey))
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return Caps{}, fmt.Errorf("building caps request: %w", err)
	}

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return Caps{}, fmt.Errorf("querying caps from %s: %w", c.IndexerID, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return Caps{}, fmt.Errorf("caps from %s returned HTTP %d", c.IndexerID, resp.StatusCode)
	}

	var caps capsXML
	if err := xml.NewDecoder(resp.Body).Decode(&caps); err != nil {
		return Caps{}, fmt.Errorf("parsing caps from %s: %w", c.IndexerID, err)
	}

	result := Caps{}

	// Determine available search types
	if caps.Searching.Search.Available == "yes" {
		result.SearchTypes = append(result.SearchTypes, SearchTypeSearch)
	}
	if caps.Searching.TVSearch.Available == "yes" {
		result.SearchTypes = append(result.SearchTypes, SearchTypeTVSearch)
	}
	if caps.Searching.MovieSearch.Available == "yes" {
		result.SearchTypes = append(result.SearchTypes, SearchTypeMovie)
	}
	if caps.Searching.MusicSearch.Available == "yes" {
		result.SearchTypes = append(result.SearchTypes, SearchTypeMusic)
	}
	if caps.Searching.BookSearch.Available == "yes" {
		result.SearchTypes = append(result.SearchTypes, SearchTypeBook)
	}

	// Parse categories
	for _, cat := range caps.Categories.Category {
		id, _ := strconv.Atoi(cat.ID)
		result.Categories = append(result.Categories, Category{ID: id, Name: cat.Name})
		for _, sub := range cat.Subcat {
			subID, _ := strconv.Atoi(sub.ID)
			result.Categories = append(result.Categories, Category{ID: subID, Name: sub.Name})
		}
	}

	return result, nil
}
