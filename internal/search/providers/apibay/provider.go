// Package providers/apibay implémente un provider torrent réel et sans
// authentification, utilisé comme premier indexeur public de démonstration
// pour la Slice A du MVP (avant que le moteur Cardigann complet — Slice B —
// ne soit implémenté). Il interroge l'API JSON non-officielle "apibay"
// (miroir de données The Pirate Bay), qui ne nécessite ni login ni
// scraping HTML.
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

// baseURL est une variable (pas une constante) pour permettre aux tests de
// pointer vers un serveur httptest local plutôt que vers le vrai apibay.org.
var baseURL = "https://apibay.org/q.php"

// Provider interroge apibay.org (protocole torrent, indexeur public, sans login).
type Provider struct {
	HTTPClient *http.Client
}

// New construit un provider apibay avec un client HTTP par défaut si aucun n'est fourni.
func New() *Provider {
	return &Provider{HTTPClient: &http.Client{Timeout: 15 * time.Second}}
}

func (p *Provider) ID() string               { return "apibay" }
func (p *Provider) Name() string             { return "The Pirate Bay (apibay)" }
func (p *Provider) Protocol() model.Protocol { return model.ProtocolTorrent }

// item représente un enregistrement brut de la réponse JSON apibay.
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

// Search interroge apibay.org et normalise les résultats en model.ReleaseInfo.
// Les liens de téléchargement sont reconstruits en magnet URI directement
// (apibay ne fournit que le info_hash + nom, pas de page de détails scrapée).
func (p *Provider) Search(ctx context.Context, q search.Query) ([]model.ReleaseInfo, error) {
	if q.Keywords == "" {
		return nil, fmt.Errorf("apibay requires non-empty keywords (no browse-all support)")
	}

	reqURL := fmt.Sprintf("%s?q=%s", baseURL, url.QueryEscape(q.Keywords))
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

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading apibay response: %w", err)
	}

	var items []item
	if err := json.Unmarshal(body, &items); err != nil {
		return nil, fmt.Errorf("parsing apibay response: %w", err)
	}

	releases := make([]model.ReleaseInfo, 0, len(items))
	for _, it := range items {
		// apibay renvoie un enregistrement factice {"id":"0", "name":"No results returned"}
		// quand la recherche ne trouve rien.
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
