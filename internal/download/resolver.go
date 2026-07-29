// Package download résout un model.ReleaseInfo en fichier concret
// (.torrent/.nzb) ou en lien magnet, selon le protocole détecté sur le
// résultat — l'utilisateur n'a jamais à choisir manuellement le protocole.
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

// Resolve télécharge/résout le lien réel d'un ReleaseInfo :
//   - lien magnet: renvoyé tel quel (pas de flux binaire à télécharger) ;
//   - protocole usenet: récupération directe du flux .nzb ;
//   - protocole torrent non-magnet: récupération directe du flux .torrent.
//
// Les indexeurs nécessitant une session authentifiée réutilisent le client
// HTTP déjà connecté (r.HTTPClient, potentiellement un
// httpclient.IndexerClient avec cookies persistés — cf. Slice C).
//
// Note : la résolution via une page intermédiaire (`download.selectors`
// Cardigann) n'est pas supportée — la structure réelle de ce mécanisme
// (Filters, Before/priming de session, Method, Infohash) est plus riche
// qu'un simple sélecteur+attribut, et un branchement partiel romprait
// silencieusement les indexeurs qui en dépendent réellement. Un support
// complet serait un chantier séparé.
func (r *Resolver) Resolve(ctx context.Context, release model.ReleaseInfo) (Artifact, error) {
	if strings.HasPrefix(release.DownloadLink, "magnet:") {
		return Artifact{
			Filename: sanitizeFilename(release.Title) + ".magnet.txt",
			IsMagnet: true,
			Content:  []byte(release.DownloadLink),
		}, nil
	}

	// trustedHost est dérivé du lien initial fourni par l'indexeur : les
	// AuthHeaders (credentials indexeur) ne doivent jamais être envoyés vers
	// un hôte différent, même si un redirect pointe ensuite vers un hôte
	// tiers arbitraire (risque de fuite de credentials, cf. audit sécurité).
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

	// Les AuthHeaders (credentials indexeur) ne sont attachés que si la
	// requête cible bien l'hôte de confiance initial : une page
	// intermédiaire ou un lien extrait par un sélecteur ne doit jamais
	// pouvoir siphonner ces credentials vers un hôte tiers arbitraire.
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

	return io.ReadAll(io.LimitReader(resp.Body, 64<<20)) // 64 Mio max, garde-fou mémoire.
}

// hostOf renvoie l'hôte (sans port) de rawURL, ou une chaîne vide si
// l'URL est invalide.
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
