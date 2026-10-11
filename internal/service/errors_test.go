package service

import (
	"errors"
	"fmt"
	"testing"

	"github.com/Kcchouette/gowlarr/internal/search"
)

func TestClassifySearchError(t *testing.T) {
	tests := []struct {
		name  string
		err   error
		hints []string
		want  error
	}{
		{
			name:  "newznab http 401",
			err:   errors.New("querying caps from idx: returned HTTP 401"),
			hints: []string{"http 401", "http 403", "invalid api"},
			want:  search.ErrAuthentication,
		},
		{
			name:  "newznab invalid api key",
			err:   errors.New("Newznab error: Invalid API Key (code 100)"),
			hints: []string{"http 401", "http 403", "invalid api"},
			want:  search.ErrAuthentication,
		},
		{
			name:  "cardigann login prefix",
			err:   errors.New("login: connection refused"),
			hints: []string{"login:"},
			want:  search.ErrAuthentication,
		},
		{
			// The cardigann engine emits "login:" in lowercase, but the
			// classifier must keep matching on a lowercased haystack so a
			// capitalized "Login:" is still an auth failure rather than a
			// generic search error.
			name:  "capitalized Login still auth",
			err:   errors.New("Login: connection refused"),
			hints: []string{"login:"},
			want:  search.ErrAuthentication,
		},
		{
			name:  "no hint matches is a search error",
			err:   errors.New("dial tcp: connection timed out"),
			hints: []string{"http 401", "login:"},
			want:  search.ErrSearch,
		},
		{
			name:  "no hints at all is a search error",
			err:   errors.New("unexpected status 500"),
			hints: nil,
			want:  search.ErrSearch,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := classifySearchError(tt.err, tt.hints...)
			if !errors.Is(got, tt.want) {
				t.Errorf("errors.Is(%v, %v) = false, want true", got, tt.want)
			}
			// The original error must stay reachable for logging/inspection.
			if !errors.Is(got, tt.err) {
				t.Errorf("classified error must wrap the original: %v", got)
			}
		})
	}
}

func TestClassifySearchErrorMessages(t *testing.T) {
	// The rendered messages must keep the historical wording so user-visible
	// CLI output is unchanged.
	auth := classifySearchError(errors.New("login: boom"), "login:")
	if got, want := auth.Error(), "authentication failed: login: boom"; got != want {
		t.Errorf("auth message = %q, want %q", got, want)
	}
	generic := classifySearchError(errors.New("boom"))
	if got, want := generic.Error(), "network/parsing error: boom"; got != want {
		t.Errorf("search message = %q, want %q", got, want)
	}
}

func TestClassifySearchErrorWrapsSentinelAndOriginal(t *testing.T) {
	orig := fmt.Errorf("login: underlying failure")
	got := classifySearchError(orig, "login:")
	if !errors.Is(got, search.ErrAuthentication) {
		t.Error("expected ErrAuthentication")
	}
	if !errors.Is(got, orig) {
		t.Error("expected the original error to stay reachable")
	}
}
