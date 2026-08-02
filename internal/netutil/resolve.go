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

// SetDNSOverrides installs the global hostname→IP overrides. An empty or nil
// map disables overrides. Not safe for concurrent mutation; call once at
// startup.
func SetDNSOverrides(overrides map[string]string) {
	dnsOverrides = overrides
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
