package flaresolverr

import (
	"errors"
	"net"
	"net/url"
	"strings"
)

func ValidateURL(rawURL string) error {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return errors.New("invalid URL")
	}

	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return errors.New("only HTTP and HTTPS URLs are allowed")
	}

	host := parsed.Hostname()
	if host == "" {
		return errors.New("empty hostname")
	}

	// Check for private/internal IPs
	if ip := net.ParseIP(host); ip != nil {
		if ip.IsLoopback() || ip.IsPrivate() || ip.IsUnspecified() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() {
			return errors.New("private/internal IP addresses are not allowed")
		}
	}

	// Block localhost variations
	lowerHost := strings.ToLower(host)
	if lowerHost == "localhost" || strings.HasSuffix(lowerHost, ".localhost") {
		return errors.New("localhost is not allowed")
	}

	return nil
}
