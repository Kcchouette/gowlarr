package server

import (
	"encoding/xml"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"strings"

	"github.com/Kcchouette/gowlarr/internal/model"
	"github.com/Kcchouette/gowlarr/internal/service"
)

func (s *Server) handleAPI(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	t := q.Get("t")

	switch t {
	case "search", "tvsearch", "movie", "music", "book":
		s.handleSearch(w, r, t)
	case "caps":
		s.handleCaps(w, r)
	case "serverping":
		s.handlePing(w, r)
	default:
		http.Error(w, "unsupported t parameter", http.StatusBadRequest)
	}
}

func (s *Server) handleSearch(w http.ResponseWriter, r *http.Request, t string) {
	q := r.URL.Query()

	keywords := strings.TrimSpace(q.Get("q"))
	if keywords == "" {
		http.Error(w, "missing required parameter: q", http.StatusBadRequest)
		return
	}

	// Build the transport-agnostic params, then convert once via the service
	// mapping so the HTTP and CLI entry paths share a single Query contract.
	params := service.SearchParams{
		Keywords:   keywords,
		SearchType: t,
	}

	if catStr := q.Get("cat"); catStr != "" {
		for _, c := range strings.Split(catStr, ",") {
			if id, err := strconv.Atoi(strings.TrimSpace(c)); err == nil {
				// Intentionally permissive with *arr clients: invalid
				// categories are silently ignored instead of failing
				// the entire request.
				params.Categories = append(params.Categories, id)
			}
		}
	}

	if seasonStr := q.Get("season"); seasonStr != "" {
		if season, err := strconv.Atoi(seasonStr); err == nil {
			// Same permissive compatibility for season/ep: unparseable
			// values are silently ignored instead of returning 400.
			params.Season = season
		}
	}
	if epStr := q.Get("ep"); epStr != "" {
		if ep, err := strconv.Atoi(epStr); err == nil {
			params.Episode = ep
		}
	}
	if imdbid := q.Get("imdbid"); imdbid != "" {
		params.IMDbID = imdbid
	}
	if tmdbid := q.Get("tmdbid"); tmdbid != "" {
		params.TMDBID = tmdbid
	}

	result := s.engine.Search(r.Context(), service.QueryFromParams(params))

	w.Header().Set("Content-Type", "application/xml; charset=utf-8")
	fmt.Fprint(w, `<?xml version="1.0" encoding="UTF-8"?>`)
	if err := WriteTorznabResponse(w, result.Releases, "gowlarr"); err != nil {
		// The response stream has already been partially written (headers +
		// XML prolog): we can no longer emit a proper HTTP error code or
		// return err.Error() to the client (would leak internal details).
		// Log server-side only.
		slog.Error("writing torznab response", "err", err)
	}
}

// capsModes is the set of Torznab/Newznab search modes advertised in caps.
var capsModes = service.CapsSearchModes

// capsAvailable reports the modes supported by the enabled indexers. A nil
// store (server tests) keeps the historical all-yes behavior; the aggregation
// itself lives in the service layer.
func (s *Server) capsAvailable() map[string]bool {
	if s.store == nil {
		avail := make(map[string]bool, len(capsModes))
		for _, m := range capsModes {
			avail[m] = true
		}
		return avail
	}
	return service.AvailableSearchModes(s.store)
}

func (s *Server) handleCaps(w http.ResponseWriter, r *http.Request) {
	avail := s.capsAvailable()
	yesNo := func(m string) string {
		if avail[m] {
			return "yes"
		}
		return "no"
	}

	w.Header().Set("Content-Type", "application/xml; charset=utf-8")
	fmt.Fprint(w, `<?xml version="1.0" encoding="UTF-8"?>`)
	fmt.Fprintf(w, `<caps>
  <searching>
    <search available="%s" />
    <tv-search available="%s" />
    <movie-search available="%s" />
    <music-search available="%s" />
    <book-search available="%s" />
  </searching>
  <categories>
    <category id="2000" name="Movies" />
    <category id="3000" name="TV" />
    <category id="4000" name="Audio" />
    <category id="5000" name="Books" />
    <category id="6000" name="PC" />
    <category id="7000" name="Console" />
    <category id="8000" name="XXX" />
  </categories>
</caps>`, yesNo("search"), yesNo("tv-search"), yesNo("movie-search"), yesNo("music-search"), yesNo("book-search"))
}

func (s *Server) handlePing(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/xml; charset=utf-8")
	fmt.Fprint(w, `<?xml version="1.0" encoding="UTF-8"?>
<rss version="2.0">
  <channel>
    <title>Gowlarr</title>
    <description>OK</description>
  </channel>
</rss>`)
}

// enclosureType returns the MIME type of the download enclosure for a
// release, per protocol: NZB for usenet, .torrent otherwise.
func enclosureType(p model.Protocol) string {
	if p == model.ProtocolUsenet {
		return "application/x-nzb"
	}
	return "application/x-bittorrent"
}

// WriteTorznabResponse writes a Torznab-compatible RSS XML response.
func WriteTorznabResponse(w http.ResponseWriter, releases []model.ReleaseInfo, indexerName string) error {
	type torznabAttr struct {
		XMLName xml.Name `xml:"torznab:attr"`
		Name    string   `xml:"name,attr"`
		Value   string   `xml:"value,attr"`
	}

	type enclosure struct {
		URL    string `xml:"url,attr"`
		Length string `xml:"length,attr"`
		Type   string `xml:"type,attr"`
	}

	type rssItem struct {
		Title     string        `xml:"title"`
		Link      string        `xml:"link"`
		GUID      string        `xml:"guid"`
		PubDate   string        `xml:"pubDate"`
		Category  []string      `xml:"category"`
		Enclosure enclosure     `xml:"enclosure"`
		Attrs     []torznabAttr `xml:"torznab:attr"`
	}

	type channel struct {
		Title       string    `xml:"title"`
		Description string    `xml:"description"`
		Link        string    `xml:"link"`
		Items       []rssItem `xml:"item"`
	}

	type rssFeed struct {
		XMLName xml.Name `xml:"rss"`
		Version string   `xml:"version,attr"`
		Atom    string   `xml:"xmlns:atom,attr"`
		Torznab string   `xml:"xmlns:torznab,attr"`
		Channel channel  `xml:"channel"`
	}

	feed := rssFeed{
		Version: "2.0",
		Atom:    "http://www.w3.org/2005/Atom",
		Torznab: "http://torznab.github.io/schemas/2015/feed",
		Channel: channel{
			Title:       indexerName,
			Description: "Gowlarr Torznab/Newznab Server",
			Link:        "/",
		},
	}

	for _, r := range releases {
		cats := make([]string, len(r.Categories))
		for i, c := range r.Categories {
			cats[i] = strconv.Itoa(c)
		}

		item := rssItem{
			Title:    r.Title,
			Link:     r.DownloadLink,
			GUID:     r.DownloadLink,
			Category: cats,
			Enclosure: enclosure{
				URL:    r.DownloadLink,
				Length: strconv.FormatInt(r.Size, 10),
				Type:   enclosureType(r.Protocol),
			},
			Attrs: []torznabAttr{
				{Name: "size", Value: strconv.FormatInt(r.Size, 10)},
				{Name: "seeders", Value: strconv.Itoa(r.Seeders)},
				{Name: "peers", Value: strconv.Itoa(r.Peers)},
				{Name: "infohash", Value: r.InfoHash},
			},
		}
		feed.Channel.Items = append(feed.Channel.Items, item)
	}

	w.Header().Set("Content-Type", "application/xml; charset=utf-8")
	encoder := xml.NewEncoder(w)
	encoder.Indent("", "  ")
	return encoder.Encode(feed)
}
