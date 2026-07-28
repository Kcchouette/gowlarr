// Package download résout un model.ReleaseInfo en fichier concret
// (.torrent/.nzb) ou en lien magnet, selon le protocole détecté sur le
// résultat — l'utilisateur n'a jamais à choisir manuellement le protocole.
package download

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/Kcchouette/cardigann-go/selector"
	"github.com/Kcchouette/gowlarr/internal/model"
)

// Artifact représente le fichier obtenu après résolution d'un ReleaseInfo.
type Artifact struct {
	// Filename suggéré pour l'enregistrement sur disque.
	Filename string
	// IsMagnet indique que Content est un simple lien texte magnet:, pas un flux binaire.
	IsMagnet bool
	// Content est le contenu du fichier (.torrent/.nzb) ou le texte du lien magnet.
	Content []byte
}

// httpDoer est satisfait aussi bien par *http.Client que par
// *httpclient.IndexerClient (cookies persistés/rate-limit, Slice C), pour les
// indexeurs nécessitant une session authentifiée avant téléchargement.
type httpDoer interface {
	Do(*http.Request) (*http.Response, error)
}

// Resolver résout un ReleaseInfo en Artifact téléchargeable.
type Resolver struct {
	HTTPClient  httpDoer
	AuthHeaders map[string]string
}

// NewResolverWithClient construit un Resolver avec un client HTTP spécifique.
func NewResolverWithClient(client httpDoer, authHeaders map[string]string) *Resolver {
	return &Resolver{HTTPClient: client, AuthHeaders: authHeaders}
}

// DownloadSelectorStep décrit une étape de résolution intermédiaire Cardigann
// (`download.selectors`) : suivre une page HTML intermédiaire pour extraire
// le lien réel de téléchargement avant de le récupérer.
type DownloadSelectorStep struct {
	Selector  string
	Attribute string
}

// Resolve télécharge/résout le lien réel d'un ReleaseInfo :
//   - lien magnet: renvoyé tel quel (pas de flux binaire à télécharger) ;
//   - protocole usenet: récupération directe du flux .nzb ;
//   - protocole torrent non-magnet: récupération directe du flux .torrent,
//     après avoir suivi les éventuelles étapes intermédiaires (steps) si
//     l'indexeur ne fournit pas de lien direct.
//
// Les indexeurs nécessitant une session authentifiée réutilisent le client
// HTTP déjà connecté (r.HTTPClient, potentiellement un
// httpclient.IndexerClient avec cookies persistés — cf. Slice C).
func (r *Resolver) Resolve(ctx context.Context, release model.ReleaseInfo, steps ...DownloadSelectorStep) (Artifact, error) {
	if strings.HasPrefix(release.DownloadLink, "magnet:") {
		return Artifact{
			Filename: sanitizeFilename(release.Title) + ".magnet.txt",
			IsMagnet: true,
			Content:  []byte(release.DownloadLink),
		}, nil
	}

	link := release.DownloadLink
	for _, step := range steps {
		resolved, err := r.followIntermediatePage(ctx, link, step)
		if err != nil {
			return Artifact{}, fmt.Errorf("following download selector %q: %w", step.Selector, err)
		}
		link = resolved
		if strings.HasPrefix(link, "magnet:") {
			return Artifact{
				Filename: sanitizeFilename(release.Title) + ".magnet.txt",
				IsMagnet: true,
				Content:  []byte(link),
			}, nil
		}
	}

	body, err := r.fetch(ctx, link)
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

// followIntermediatePage récupère link (page HTML intermédiaire) et en
// extrait le lien réel via le sélecteur/attribut fourni.
func (r *Resolver) followIntermediatePage(ctx context.Context, link string, step DownloadSelectorStep) (string, error) {
	body, err := r.fetch(ctx, link)
	if err != nil {
		return "", err
	}
	doc, err := selector.NewHTMLDocument(string(body))
	if err != nil {
		return "", fmt.Errorf("parsing intermediate page: %w", err)
	}
	nodes, err := doc.Find(step.Selector)
	if err != nil || len(nodes) == 0 {
		return "", fmt.Errorf("selector %q matched nothing on intermediate page", step.Selector)
	}
	attr := step.Attribute
	if attr == "" {
		attr = "href"
	}
	value, ok := nodes[0].Attr(attr)
	if !ok {
		return "", fmt.Errorf("attribute %q not found on selected node", attr)
	}
	return value, nil
}

func (r *Resolver) fetch(ctx context.Context, link string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, link, nil)
	if err != nil {
		return nil, fmt.Errorf("building download request: %w", err)
	}
	req.Header.Set("User-Agent", "gowlarr/0.1 (+https://github.com/Kcchouette/gowlarr)")

	// Add auth headers if provided
	for header, value := range r.AuthHeaders {
		req.Header.Set(header, value)
	}

	resp, err := r.HTTPClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("http status %d", resp.StatusCode)
	}

	return io.ReadAll(io.LimitReader(resp.Body, 64<<20)) // 64 Mio max, garde-fou mémoire.
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
