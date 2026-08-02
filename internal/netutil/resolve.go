package netutil

import (
	"context"
	"net"
	"strings"
)

// dnsOverrides maps hostnames to fixed IP addresses, applied at dial time
// and DNS lookup time (equivalent to curl's --resolve). Configured via
// config.DNSOverrides and wired in by the service layer.
var dnsOverrides map[string]string

// SetDNSOverrides installs the global hostname→IP overrides. Keys are
// normalized to lowercase (hostnames are case-insensitive). An empty or nil
// map disables overrides. Not safe for concurrent mutation; call once at
// startup.
func SetDNSOverrides(overrides map[string]string) {
	if len(overrides) == 0 {
		dnsOverrides = nil
		return
	}
	normalized := make(map[string]string, len(overrides))
	for host, ip := range overrides {
		normalized[strings.ToLower(host)] = ip
	}
	dnsOverrides = normalized
}

// OverrideDialContext wraps a base DialContext so that connections to
// overridden hostnames dial the fixed IP instead, keeping the original port.
// Hosts not in the override map are delegated to base. base may be nil, in
// which case the standard resolver is used for non-overridden hosts.
func OverrideDialContext(base func(ctx context.Context, network, addr string) (net.Conn, error), overrides map[string]string) func(ctx context.Context, network, addr string) (net.Conn, error) {
	if base == nil {
		base = (&net.Dialer{}).DialContext
	}
	return func(ctx context.Context, network, addr string) (net.Conn, error) {
		host, port, err := net.SplitHostPort(addr)
		if err != nil {
			// Not host:port (unusual) — let the base dialer handle it.
			return base(ctx, network, addr)
		}
		if ip, ok := overrides[strings.ToLower(host)]; ok {
			return base(ctx, network, net.JoinHostPort(ip, port))
		}
		return base(ctx, network, addr)
	}
}
