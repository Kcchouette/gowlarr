package netutil

import (
	"context"
	"net"
	"strings"
	"sync/atomic"
)

// dnsServerResolver holds the custom resolver installed by SetDNSServers.
// nil = system resolver. Configured once at startup; read from dial and
// lookup paths behind an atomic pointer (same pattern as the former
// dnsOverrides map).
var dnsServerResolver atomic.Pointer[net.Resolver]

// dnsServerRR rotates the first-tried server across Dial calls. UDP
// connect() succeeds even against a blackholed server, so a fixed order
// would always exit on the first address; round-robin spreads first tries.
var dnsServerRR atomic.Uint64

// SetDNSServers installs the global DNS servers used for every outbound
// name resolution. Empty or nil disables the custom resolver (system
// behavior). Entries are normalized: "1.1.1.1" → "1.1.1.1:53",
// "fd01::1" and "[fd01::1]" → "[fd01::1]:53", "1.1.1.1:5353" kept as-is.
// Blank entries are skipped. Safe to call concurrently with readers.
func SetDNSServers(servers []string) {
	if len(servers) == 0 {
		dnsServerResolver.Store(nil)
		return
	}
	addrs := make([]string, 0, len(servers))
	for _, s := range servers {
		if a := normalizeDNSServer(s); a != "" {
			addrs = append(addrs, a)
		}
	}
	if len(addrs) == 0 {
		dnsServerResolver.Store(nil)
		return
	}
	dnsServerRR.Store(0)
	dnsServerResolver.Store(newDNSServerResolver(addrs))
}

// HasDNSServers reports whether a custom resolver is active. Call sites use
// it to decide whether to install a custom DialContext on transports.
func HasDNSServers() bool { return dnsServerResolver.Load() != nil }

// currentDNSServerResolver returns the installed resolver (nil when none).
func currentDNSServerResolver() *net.Resolver { return dnsServerResolver.Load() }

// DialContextFor returns a DialContext that resolves hostnames through the
// custom DNS servers when configured, via net.Dialer{Resolver: …}. The
// resolver is looked up lazily at each dial (order-proof if SetDNSServers
// runs later). Without custom servers it is a plain system dialer.
func DialContextFor() func(ctx context.Context, network, addr string) (net.Conn, error) {
	return func(ctx context.Context, network, addr string) (net.Conn, error) {
		return (&net.Dialer{Resolver: currentDNSServerResolver()}).DialContext(ctx, network, addr)
	}
}

// normalizeDNSServer returns addr:port for a config entry, or "" for blank.
func normalizeDNSServer(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	if _, _, err := net.SplitHostPort(s); err == nil {
		return s // already host:port (IPv4:port or [v6]:port)
	}
	// Bare host/IP. Strip any surrounding brackets first: JoinHostPort
	// re-brackets inputs that already look like "[v6]", producing
	// "[[v6]]:53" otherwise.
	s = strings.TrimPrefix(s, "[")
	s = strings.TrimSuffix(s, "]")
	return net.JoinHostPort(s, "53")
}

// newDNSServerResolver builds a pure-Go resolver that queries addrs in
// order, starting each Dial at a round-robin offset. Dial ignores the
// address parameter (the resolv.conf server the Go resolver would use)
// and dials the configured servers instead. The network parameter ("udp"
// or "tcp") is forwarded unchanged — the Go resolver handles UDP→TCP
// fallback on truncation by re-calling Dial. Failover is connect-level:
// a blackholed-but-routed UDP server "succeeds" at connect, so liveness
// is decided by the DNS exchange timeout above us; round-robin only
// spreads which server is tried first. The inner net.Dialer deliberately
// has no Resolver: if an entry is a hostname, it is resolved by the
// system, avoiding recursion.
func newDNSServerResolver(addrs []string) *net.Resolver {
	return &net.Resolver{
		PreferGo: true,
		Dial: func(ctx context.Context, network, _ string) (net.Conn, error) {
			var d net.Dialer
			// Modulo before int conversion: an unmasked int64→int cast can
			// go negative after 2^63 dials and panic the index.
			start := int((dnsServerRR.Add(1) - 1) % uint64(len(addrs)))
			var lastErr error
			for i := range addrs {
				srv := addrs[(start+i)%len(addrs)]
				conn, err := d.DialContext(ctx, network, srv)
				if err == nil {
					return conn, nil
				}
				lastErr = err
			}
			return nil, lastErr
		},
	}
}
