package flaresolverr

import "github.com/Kcchouette/gowlarr/internal/netutil"

func ValidateURL(rawURL string) error {
	return netutil.ValidateURL(rawURL)
}
