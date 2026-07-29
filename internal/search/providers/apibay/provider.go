// Package providers/apibay implements a real, unauthenticated torrent provider
// used as the first public demo indexer for the MVP Slice A (before the full
// Cardigann engine — Slice B — is implemented). It queries the unofficial
// "apibay" JSON API (a Pirate Bay data mirror) that requires neither login
// nor HTML scraping.
package apibay

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"github.com/Kcchouette/gowlarr/internal/model"
	"github.com/Kcchouette/gowlarr/internal/search"
)

const (
	defaultBaseURL        = "https://apibay.org/q.php"
	maxAPIBayResponseSize = 32 << 20
)

// Provider queries apibay.org (torrent protocol, public indexer, no login).
type Provider struct {
	HTTPClient *http.Client
	BaseURL    string
}

// New builds an apibay provider with a default HTTP client if none is provided.
func New() *Provider {
	return &Provider{
		HTTPClient: &http.Client{Timeout: 15 * time.Second}, // Simple public JSON API, short timeout to stay responsive.
		BaseURL:    defaultBaseURL,
	}
}

func (p *Provider) ID() string               { return "apibay" }
func (p *Provider) Name() string             { return "The Pirate Bay (apibay)" }
func (p *Provider) Protocol() model.Protocol { return model.ProtocolTorrent }

// item represents a raw record from the apibay JSON response.
type item struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	InfoHash string `json:"info_hash"`
	Leechers string `json:"leechers"`
	Seeders  string `json:"seeders"`
	NumFiles string `json:"num_files"`
	Size     string `json:"size"`
	Added    string `json:"added"`
	Category string `json:"category"`
}

// Search queries apibay.org and normalizes results into model.ReleaseInfo.
// Download links are reconstructed as magnet URIs directly (apibay only
// provides info_hash + name, no scraped detail page).
func (p *Provider) Search(ctx context.Context, q search.Query) ([]model.ReleaseInfo, error) {
	if q.Keywords == "" {
		return nil, fmt.Errorf("apibay requires non-empty keywords (no browse-all support)")
	}

	reqURL := fmt.Sprintf("%s?q=%s", p.BaseURL, url.QueryEscape(q.Keywords))
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return nil, fmt.Errorf("building apibay request: %w", err)
	}
	req.Header.Set("User-Agent", "gowlarr/0.1 (+https://github.com/Kcchouette/gowlarr)")

	resp, err := p.HTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("querying apibay: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("apibay returned HTTP %d", resp.StatusCode)
	}

	body, err := readAllBounded(resp.Body, maxAPIBayResponseSize)
	if err != nil {
		return nil, fmt.Errorf("reading apibay response: %w", err)
	}

	var items []item
	if err := json.Unmarshal(body, &items); err != nil {
		return nil, fmt.Errorf("parsing apibay response: %w", err)
	}

	releases := make([]model.ReleaseInfo, 0, len(items))
	for _, it := range items {
		// apibay returns a sentinel record {"id":"0", "name":"No results returned"}
		// when the search finds nothing.
		if it.ID == "0" || it.InfoHash == "0000000000000000000000000000000000000000" {
			continue
		}

		size, _ := strconv.ParseInt(it.Size, 10, 64)
		seeders, _ := strconv.Atoi(it.Seeders)
		leechers, _ := strconv.Atoi(it.Leechers)
		addedUnix, _ := strconv.ParseInt(it.Added, 10, 64)
		category, _ := strconv.Atoi(it.Category)

		magnet := fmt.Sprintf("magnet:?xt=urn:btih:%s&dn=%s", it.InfoHash, url.QueryEscape(it.Name))

		releases = append(releases, model.ReleaseInfo{
			Title:        it.Name,
			DownloadLink: magnet,
			InfoHash:     it.InfoHash,
			Size:         size,
			PublishDate:  time.Unix(addedUnix, 0).UTC(),
			Seeders:      seeders,
			Peers:        seeders + leechers,
			Categories:   []int{category},
			Protocol:     model.ProtocolTorrent,
			IndexerID:    p.ID(),
			IndexerName:  p.Name(),
		})
	}

	return releases, nil
}

func readAllBounded(r io.Reader, limit int64) ([]byte, error) {
	body, err := io.ReadAll(io.LimitReader(r, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(body)) > limit {
		return nil, fmt.Errorf("response body too large (max %d bytes)", limit)
	}
	return body, nil
}
