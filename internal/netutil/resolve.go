package netutil

import (
	"context"
	"net"
	"strings"
	"sync/atomic"
)

// dnsOverrides maps hostnames to fixed IP addresses, applied at dial time
// and DNS lookup time (equivalent to curl's --resolve). Configured once at
// startup and read from request paths, so it is held behind an atomic pointer
// to keep the read side lock-free and race-free.
var dnsOverrides atomic.Pointer[map[string]string]

// SetDNSOverrides installs the global hostname→IP overrides. Keys are
// normalized to lowercase (hostnames are case-insensitive). An empty or nil
// map disables overrides. Safe to call concurrently with readers.
func SetDNSOverrides(overrides map[string]string) {
	if len(overrides) == 0 {
		dnsOverrides.Store(nil)
		return
	}
	normalized := make(map[string]string, len(overrides))
	for host, ip := range overrides {
		normalized[strings.ToLower(host)] = ip
	}
	dnsOverrides.Store(&normalized)
}

// currentDNSOverrides returns the installed override map (nil when none).
func currentDNSOverrides() map[string]string {
	if m := dnsOverrides.Load(); m != nil {
		return *m
	}
	return nil
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
