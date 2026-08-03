package cardigannadapter

import (
	"context"
	"log/slog"
	"net/url"
	"strconv"

	cardigannengine "github.com/Kcchouette/cardigann-go/engine"
	cardigannrelease "github.com/Kcchouette/cardigann-go/release"
	"github.com/Kcchouette/gowlarr/internal/model"
	"github.com/Kcchouette/gowlarr/internal/search"
)

type Provider struct {
	inner *cardigannengine.Provider
}

func NewProvider(inner *cardigannengine.Provider) *Provider {
	return &Provider{inner: inner}
}

func (p *Provider) ID() string   { return p.inner.ID() }
func (p *Provider) Name() string { return p.inner.Name() }

func (p *Provider) Protocol() model.Protocol {
	return toModelProtocol(p.inner.Protocol(), p.inner.ID(), p.inner.Name())
}

func (p *Provider) Search(ctx context.Context, q search.Query) ([]model.ReleaseInfo, error) {
	query := cardigannengine.Query{
		Keywords:   q.Keywords,
		Categories: q.Categories,
		IMDBID:     q.IMDbID,
		TMDBID:     q.TMDBID,
	}
	if q.Season > 0 {
		query.Season = strconv.Itoa(q.Season)
	}
	if q.Episode > 0 {
		query.Ep = strconv.Itoa(q.Episode)
	}

	releases, err := p.inner.Search(ctx, query)
	if err != nil {
		return nil, err
	}
	return ToReleaseInfos(releases), nil
}

// ResolveDownloadLink runs the definition's download.before chain starting
// from the release's stored link, revealing the actual DDL link. The chain
// may perform a side-effecting action (e.g. a "thanks" POST) and therefore
// runs only at explicit link/download time, never during search.
func (p *Provider) ResolveDownloadLink(ctx context.Context, rel model.ReleaseInfo) (model.ReleaseInfo, error) {
	crel := cardigannrelease.Release{Title: rel.Title, DownloadLink: rel.DownloadLink}
	resolved, err := p.inner.ResolveDownloadLink(ctx, crel)
	if err != nil {
		return rel, err
	}
	// The engine returns the raw href extracted from the chain's response
	// (e.g. "attachment.php?aid=1"), which gowlarr's download resolver would
	// reject; absolutize against the definition base URL (== the provider
	// base URL in production).
	base := ""
	if links := p.inner.Def().Links; len(links) > 0 {
		base = links[0]
	}
	rel.DownloadLink = absolutizeLink(resolved.DownloadLink, base)
	rel.InfoHash = resolved.InfoHash
	rel.Unlocked = resolved.Unlocked
	if resolved.Title != "" {
		rel.Title = resolved.Title
	}
	rel.Hosts = append([]string(nil), resolved.Hosts...)
	return rel, nil
}

// absolutizeLink resolves a relative link against base (RFC 3986); absolute
// links (and empty or unparseable values) pass through unchanged.
func absolutizeLink(link, base string) string {
	if link == "" || base == "" {
		return link
	}
	bu, err := url.Parse(base)
	if err != nil {
		return link
	}
	ref, err := url.Parse(link)
	if err != nil {
		return link
	}
	if ref.IsAbs() {
		return link
	}
	return bu.ResolveReference(ref).String()
}

func ToReleaseInfos(releases []cardigannrelease.Release) []model.ReleaseInfo {
	out := make([]model.ReleaseInfo, 0, len(releases))
	for _, r := range releases {
		out = append(out, ToReleaseInfo(r))
	}
	return out
}

func ToReleaseInfo(r cardigannrelease.Release) model.ReleaseInfo {
	return model.ReleaseInfo{
		Title:        r.Title,
		Details:      r.Details,
		DownloadLink: r.DownloadLink,
		InfoHash:     r.InfoHash,
		Size:         r.Size,
		PublishDate:  r.PublishDate,
		Seeders:      r.Seeders,
		Peers:        r.Peers,
		Grabs:        r.Grabs,
		Categories:   append([]int(nil), r.Categories...),
		Protocol:     toModelProtocol(r.Protocol, r.IndexerID, r.IndexerName),
		IndexerID:    r.IndexerID,
		IndexerName:  r.IndexerName,
		Hosts:        append([]string(nil), r.Hosts...),
		Unlocked:     r.Unlocked,
		StreamURL:    r.StreamURL,
	}
}

func toModelProtocol(protocol cardigannrelease.Protocol, indexerID, indexerName string) model.Protocol {
	switch protocol {
	case cardigannrelease.ProtocolUsenet:
		return model.ProtocolUsenet
	case cardigannrelease.ProtocolTorrent:
		return model.ProtocolTorrent
	case cardigannrelease.ProtocolDDL:
		return model.ProtocolDDL
	case cardigannrelease.ProtocolStreaming:
		return model.ProtocolStreaming
	default:
		slog.Warn("unknown cardigann protocol, falling back to torrent", "protocol", string(protocol), "indexer_id", indexerID, "indexer_name", indexerName)
		return model.ProtocolTorrent
	}
}
