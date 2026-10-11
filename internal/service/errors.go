package service

import (
	"fmt"
	"strings"

	"github.com/Kcchouette/gowlarr/internal/search"
)

// classifySearchError labels a provider failure as an authentication error or
// a generic search error, and wraps it so callers can match with errors.Is.
//
// hints are matched case-insensitively, so callers pass the substrings in any
// casing. The haystack is lowercased rather than the hints, which means a
// cardigann message such as "Login: ..." matches the "login:" hint just as the
// old strings.Contains(err.Error(), "login:") check would have on a
// lowercase-only message.
func classifySearchError(err error, hints ...string) error {
	lower := strings.ToLower(err.Error())
	for _, hint := range hints {
		if strings.Contains(lower, strings.ToLower(hint)) {
			return fmt.Errorf("%w: %w", search.ErrAuthentication, err)
		}
	}
	return fmt.Errorf("%w: %w", search.ErrSearch, err)
}
