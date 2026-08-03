package service

import (
	"context"
	"errors"
	"testing"

	"github.com/Kcchouette/gowlarr/internal/model"
	"github.com/Kcchouette/gowlarr/internal/search"
)

// fakeResolverProvider implements search.Provider and the optional
// linkResolver contract (download.before resolution on demand).
type fakeResolverProvider struct {
	id    string
	errOn map[string]error
}

func (f *fakeResolverProvider) ID() string               { return f.id }
func (f *fakeResolverProvider) Name() string             { return f.id }
func (f *fakeResolverProvider) Protocol() model.Protocol { return model.ProtocolDDL }
func (f *fakeResolverProvider) Search(context.Context, search.Query) ([]model.ReleaseInfo, error) {
	return nil, nil
}
func (f *fakeResolverProvider) ResolveDownloadLink(_ context.Context, rel model.ReleaseInfo) (model.ReleaseInfo, error) {
	if err := f.errOn[rel.Title]; err != nil {
		return rel, err
	}
	rel.DownloadLink = "https://resolved.example/" + rel.Title
	rel.Hosts = []string{"uptobox.com"}
	rel.Unlocked = true
	return rel, nil
}

// plainProvider is a search.Provider without download-chain resolution.
type plainProvider struct{ id string }

func (p *plainProvider) ID() string               { return p.id }
func (p *plainProvider) Name() string             { return p.id }
func (p *plainProvider) Protocol() model.Protocol { return model.ProtocolDDL }
func (p *plainProvider) Search(context.Context, search.Query) ([]model.ReleaseInfo, error) {
	return nil, nil
}

func TestSearch_NoDefaultIndexers(t *testing.T) {
	// With no configured indexers, a search must return nothing: the old
	// behavior seeded the built-in apibay provider unconditionally.
	svc := NewSearchService(openTestStore(t), testConfig())
	result, err := svc.Search(context.Background(), SearchParams{Keywords: "hunter"})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(result.Releases) != 0 {
		t.Errorf("expected 0 releases without configured indexers (no default apibay), got %d", len(result.Releases))
	}
	if len(result.Errors) != 0 {
		t.Errorf("expected 0 provider errors, got %v", result.Errors)
	}
}

func TestResolveLinks(t *testing.T) {
	svc := NewSearchService(openTestStore(t), testConfig())
	providers := []search.Provider{
		&fakeResolverProvider{id: "ddl-a"},
		&plainProvider{id: "plain"},
		&fakeResolverProvider{id: "ddl-b", errOn: map[string]error{"Boom": errors.New("chain failed")}},
	}
	releases := []model.ReleaseInfo{
		{Title: "Alpha", IndexerID: "ddl-a"},
		{Title: "NoResolver", IndexerID: "plain"},
		{Title: "Boom", IndexerID: "ddl-b"},
		{Title: "Unknown", IndexerID: "ghost"},
	}

	got := svc.resolveLinks(context.Background(), providers, releases)
	if len(got) != 4 {
		t.Fatalf("expected 4 releases, got %d", len(got))
	}
	byTitle := make(map[string]model.ReleaseInfo, len(got))
	for _, r := range got {
		byTitle[r.Title] = r
	}

	// Matching resolver: link, hosts and unlock flag are updated.
	alpha := byTitle["Alpha"]
	if alpha.DownloadLink != "https://resolved.example/Alpha" || !alpha.Unlocked || len(alpha.Hosts) != 1 || alpha.Hosts[0] != "uptobox.com" {
		t.Errorf("Alpha not resolved: %+v", alpha)
	}
	// Provider without the optional contract: untouched.
	if nr := byTitle["NoResolver"]; nr.DownloadLink != "" || nr.Unlocked {
		t.Errorf("NoResolver must stay untouched, got %+v", nr)
	}
	// Resolver error: the raw release is kept.
	if b := byTitle["Boom"]; b.DownloadLink != "" || b.Unlocked {
		t.Errorf("Boom must keep the raw release on resolution error, got %+v", b)
	}
	// No provider for this indexer: untouched.
	if u := byTitle["Unknown"]; u.DownloadLink != "" || u.Unlocked {
		t.Errorf("Unknown must stay untouched, got %+v", u)
	}
}

func TestResolveLinks_EmptyInput(t *testing.T) {
	svc := NewSearchService(openTestStore(t), testConfig())
	got := svc.resolveLinks(context.Background(), nil, nil)
	if len(got) != 0 {
		t.Errorf("expected 0 releases, got %d", len(got))
	}
}
