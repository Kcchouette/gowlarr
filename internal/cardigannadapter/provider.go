package cardigannadapter

import (
	"context"

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
	if p.inner.Protocol() == cardigannrelease.ProtocolUsenet {
		return model.ProtocolUsenet
	}
	return model.ProtocolTorrent
}

func (p *Provider) Search(ctx context.Context, q search.Query) ([]model.ReleaseInfo, error) {
	releases, err := p.inner.Search(ctx, cardigannengine.Query{
		Keywords:   q.Keywords,
		Categories: q.Categories,
	})
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
	protocol := model.ProtocolTorrent
	if r.Protocol == cardigannrelease.ProtocolUsenet {
		protocol = model.ProtocolUsenet
	}

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
		Protocol:     protocol,
		IndexerID:    r.IndexerID,
		IndexerName:  r.IndexerName,
	}
}
