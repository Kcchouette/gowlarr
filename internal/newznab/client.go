// Package newznab implémente un client natif pour le protocole Newznab
// générique (indexeurs usenet standard, hors moteur Cardigann). Il fait
// partie du MVP (Slice F), à la demande explicite de l'utilisateur pour
// conserver le support usenet de Prowlarr.
//
// Le "Usenet" de Prowlarr passe par des indexeurs Web qui exposent l'API
// Newznab (HTTP/REST + RSS), jamais par une connexion NNTP directe — ce
// module reste donc un simple client HTTP, pas un client NNTP.
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

// Client interroge un indexeur usenet parlant le protocole Newznab standard.
type Client struct {
	BaseURL    string
	APIKey     string
	IndexerID  string
	IndexerName string
	HTTPClient *http.Client
}

// New construit un client Newznab générique.
func New(id, name, baseURL, apiKey string) *Client {
	return &Client{
		BaseURL:     strings.TrimRight(baseURL, "/"),
		APIKey:      apiKey,
		IndexerID:   id,
		IndexerName: name,
		HTTPClient:  &http.Client{Timeout: 20 * time.Second},
	}
}

func (c *Client) ID() string               { return c.IndexerID }
func (c *Client) Name() string              { return c.IndexerName }
func (c *Client) Protocol() model.Protocol { return model.ProtocolUsenet }

// rssFeed / rssItem / rssAttr modélisent la réponse RSS 2.0 + extension
// newznab:attr renvoyée par l'API Newznab (t=search|tvsearch|movie|music|book).
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

// Search construit la requête `t=search` (ou tvsearch/movie/music/book selon
// les catégories demandées reste un raffinement post-MVP ; le MVP couvre la
// recherche générique par mots-clés) et parse la réponse RSS + newznab:attr.
func (c *Client) Search(ctx context.Context, q search.Query) ([]model.ReleaseInfo, error) {
	params := url.Values{}
	params.Set("t", "search")
	params.Set("apikey", c.APIKey)
	params.Set("q", q.Keywords)
	if len(q.Categories) > 0 {
		cats := make([]string, len(q.Categories))
		for i, cat := range q.Categories {
			cats[i] = strconv.Itoa(cat)
		}
		params.Set("cat", strings.Join(cats, ","))
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

// parseRSSDate tente les formats de date usuels d'un flux RSS (RFC1123Z est
// le format standard, mais certains indexeurs varient légèrement).
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
