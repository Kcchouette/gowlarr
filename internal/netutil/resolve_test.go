package netutil

import (
	"context"
	"errors"
	"net"
	"testing"
)

func TestOverrideDialContext(t *testing.T) {
	var dialed []string
	base := func(ctx context.Context, network, addr string) (net.Conn, error) {
		dialed = append(dialed, addr)
		return nil, errors.New("dial not needed in test")
	}

	overrides := map[string]string{"hydracker.com": "104.21.16.105"}
	dial := OverrideDialContext(base, overrides)

	_, _ = dial(context.Background(), "tcp", "hydracker.com:443")
	if len(dialed) != 1 || dialed[0] != "104.21.16.105:443" {
		t.Fatalf("overridden host should dial the fixed IP with original port, got %v", dialed)
	}

	_, _ = dial(context.Background(), "tcp", "japanfan.org:80")
	if len(dialed) != 2 || dialed[1] != "japanfan.org:80" {
		t.Fatalf("non-overridden host should be delegated unchanged, got %v", dialed)
	}
}

func TestOverrideDialContextNilBase(t *testing.T) {
	// A nil base must not panic: the standard dialer replaces it, and an
	// overridden host is rewritten to the fixed IP before the dial.
	overrides := map[string]string{"example.com": "93.184.216.34"}
	got := ""
	dial := OverrideDialContext(func(ctx context.Context, network, addr string) (net.Conn, error) {
		got = addr
		return nil, errors.New("no dial in test")
	}, overrides)

	_, _ = dial(context.Background(), "tcp", "example.com:443")
	if got != "93.184.216.34:443" {
		t.Fatalf("dialed %q, want %q", got, "93.184.216.34:443")
	}

	// nil base: the wrapper must still rewrite before delegating to the
	// default dialer. A cancelled context fails the dial immediately
	// (no network dependency), proving the wrapper does not panic.
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	dialNil := OverrideDialContext(nil, overrides)
	if _, err := dialNil(ctx, "tcp", "example.com:443"); err == nil {
		t.Fatal("expected a dial error on a cancelled context, got nil")
	}
}

func TestSetDNSOverridesLowercasesKeys(t *testing.T) {
	originalOverrides := dnsOverrides
	t.Cleanup(func() { dnsOverrides = originalOverrides })

	SetDNSOverrides(map[string]string{"HydraTracker.COM": "104.21.16.105"})

	addrs, err := lookupHost(context.Background(), "hydratracker.com")
	if err != nil {
		t.Fatalf("lookupHost with mixed-case override key: %v", err)
	}
	if len(addrs) != 1 || addrs[0] != "104.21.16.105" {
		t.Fatalf("lookupHost returned %v, want [104.21.16.105]", addrs)
	}

	SetDNSOverrides(nil)
}

func TestLookupHostWithOverrides(t *testing.T) {
	originalOverrides := dnsOverrides
	t.Cleanup(func() { dnsOverrides = originalOverrides })

	SetDNSOverrides(map[string]string{"hydracker.com": "104.21.16.105"})

	// The override map short-circuits the real resolver: the fixed IP is
	// returned without any DNS lookup.
	addrs, err := lookupHost(context.Background(), "hydracker.com")
	if err != nil {
		t.Fatalf("lookupHost with override: %v", err)
	}
	if len(addrs) != 1 || addrs[0] != "104.21.16.105" {
		t.Fatalf("lookupHost override returned %v, want [104.21.16.105]", addrs)
	}

	// Non-overridden host still goes through the real resolver; .invalid is
	// guaranteed by RFC 2606 to never resolve.
	if _, err := lookupHost(context.Background(), "nope.invalid"); err == nil {
		t.Fatal("non-overridden host should still go through the real resolver")
	}

	SetDNSOverrides(nil)
}
