package cardigannadapter

import (
	"context"
	"log/slog"
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
	}
}

func toModelProtocol(protocol cardigannrelease.Protocol, indexerID, indexerName string) model.Protocol {
	switch protocol {
	case cardigannrelease.ProtocolUsenet:
		return model.ProtocolUsenet
	case cardigannrelease.ProtocolTorrent:
		return model.ProtocolTorrent
	default:
		slog.Warn("unknown cardigann protocol, falling back to torrent", "protocol", string(protocol), "indexer_id", indexerID, "indexer_name", indexerName)
		return model.ProtocolTorrent
	}
}
