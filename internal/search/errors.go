package search

import "errors"

// ErrAuthentication marks a provider failure caused by bad or missing
// credentials (HTTP 401/403, an invalid API key, or a rejected login).
var ErrAuthentication = errors.New("authentication failed")

// ErrSearch marks any other provider failure: network errors, timeouts,
// unexpected HTTP status codes, or HTML/XML that could not be parsed.
var ErrSearch = errors.New("network/parsing error")
