package netutil

import (
	"context"
	"fmt"
	"net"
	"net/url"
	"strings"
	"time"
)

const dnsLookupTimeout = 3 * time.Second

var lookupHost = func(ctx context.Context, host string) ([]string, error) {
	return net.DefaultResolver.LookupHost(ctx, host)
}

// ValidateURL performs a best-effort pre-request SSRF check for outbound
// http/https URLs. It only validates the URL before the request starts: it does
// not protect against DNS rebinding at actual dial time, it does not
// re-validate redirect targets, and cannot see through per-indexer HTTP/SOCKS
// proxies where the final destination is opaque to this check.
func ValidateURL(rawURL string) error {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return fmt.Errorf("invalid URL: %w", err)
	}

	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return fmt.Errorf("only HTTP and HTTPS URLs are allowed")
	}

	host := parsed.Hostname()
	if host == "" {
		return fmt.Errorf("empty hostname")
	}

	if isBlockedHostname(host) {
		return fmt.Errorf("localhost is not allowed")
	}

	if ip := net.ParseIP(host); ip != nil {
		if isBlockedIP(ip) {
			return fmt.Errorf("private/internal IP addresses are not allowed")
		}
		return nil
	}

	lookupCtx, cancel := context.WithTimeout(context.Background(), dnsLookupTimeout)
	defer cancel()

	addrs, err := lookupHost(lookupCtx, host)
	if err != nil {
		return fmt.Errorf("resolving hostname %q: %w", host, err)
	}
	if len(addrs) == 0 {
		return fmt.Errorf("resolving hostname %q: no IP addresses found", host)
	}

	for _, addr := range addrs {
		ip := net.ParseIP(addr)
		if ip == nil {
			return fmt.Errorf("resolving hostname %q: invalid IP address %q", host, addr)
		}
		if isBlockedIP(ip) {
			return fmt.Errorf("private/internal IP addresses are not allowed")
		}
	}

	return nil
}

func isBlockedHostname(host string) bool {
	lowerHost := strings.ToLower(host)
	return lowerHost == "localhost" || strings.HasSuffix(lowerHost, ".localhost")
}

func isBlockedIP(ip net.IP) bool {
	return ip.IsLoopback() || ip.IsPrivate() || ip.IsUnspecified() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast()
}
