package netutil

import (
	"context"
	"errors"
	"net"
	"testing"
	"time"
)

func TestNormalizeDNSServer(t *testing.T) {
	tests := []struct {
		in, want string
	}{
		{"1.1.1.1", "1.1.1.1:53"},
		{"1.1.1.1:5353", "1.1.1.1:5353"},
		{"fd01::1", "[fd01::1]:53"},
		{"[fd01::1]", "[fd01::1]:53"},
		{"[fd01::1]:5353", "[fd01::1]:5353"},
		{"  ", ""},
		{"", ""},
	}
	for _, tt := range tests {
		if got := normalizeDNSServer(tt.in); got != tt.want {
			t.Errorf("normalizeDNSServer(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestSetDNSServers_EmptyDisables(t *testing.T) {
	original := dnsServerResolver.Load()
	t.Cleanup(func() { dnsServerResolver.Store(original) })

	SetDNSServers(nil)
	if HasDNSServers() {
		t.Fatal("SetDNSServers(nil) must disable the custom resolver")
	}
	SetDNSServers([]string{"  "})
	if HasDNSServers() {
		t.Fatal("SetDNSServers with only blank entries must disable the custom resolver")
	}
}

// fakeDNSServer is a minimal UDP listener that records the source of every
// datagram it receives, so tests can prove Dial redirected to it.
type fakeDNSServer struct {
	conn net.PacketConn
	ch   chan string
}

func newFakeDNSServer(t *testing.T) *fakeDNSServer {
	t.Helper()
	conn, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("ListenPacket: %v", err)
	}
	f := &fakeDNSServer{conn: conn, ch: make(chan string, 8)}
	go func() {
		buf := make([]byte, 512)
		for {
			n, _, err := conn.ReadFrom(buf)
			if err != nil {
				return
			}
			if n > 0 {
				f.ch <- "pkt"
			}
		}
	}()
	t.Cleanup(func() { conn.Close() })
	return f
}

func (f *fakeDNSServer) addr() string { return f.conn.LocalAddr().String() }

func (f *fakeDNSServer) waitPacket(t *testing.T) {
	t.Helper()
	select {
	case <-f.ch:
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for a DNS packet on the fake server")
	}
}

func TestDNSServerResolver_DialRedirects(t *testing.T) {
	original := dnsServerResolver.Load()
	t.Cleanup(func() { dnsServerResolver.Store(original) })

	fake := newFakeDNSServer(t)
	SetDNSServers([]string{fake.addr()})

	// .test is reserved (RFC 6761) and will never resolve, but the query
	// must reach our fake server first — that is what we assert.
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	_, _ = currentDNSServerResolver().LookupHost(ctx, "example.test")
	fake.waitPacket(t)
}

func TestLookupHost_UsesCustomResolver(t *testing.T) {
	originalResolver := dnsServerResolver.Load()
	t.Cleanup(func() { dnsServerResolver.Store(originalResolver) })

	fake := newFakeDNSServer(t)
	SetDNSServers([]string{fake.addr()})

	// The package-level lookupHost var (validate.go) is the production
	// closure; validate_test restores it via t.Cleanup after each swap.
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	_, _ = lookupHost(ctx, "example.test")
	fake.waitPacket(t)
}

func TestDNSServerResolver_RoundRobin(t *testing.T) {
	original := dnsServerResolver.Load()
	originalRR := dnsServerRR.Load()
	t.Cleanup(func() {
		dnsServerResolver.Store(original)
		dnsServerRR.Store(originalRR)
	})

	fakeA := newFakeDNSServer(t)
	fakeB := newFakeDNSServer(t)
	SetDNSServers([]string{fakeA.addr(), fakeB.addr()})

	r := currentDNSServerResolver()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	// First Dial starts at offset 0 (fakeA), second at offset 1 (fakeB).
	_, _ = r.LookupHost(ctx, "one.test")
	fakeA.waitPacket(t)
	_, _ = r.LookupHost(ctx, "two.test")
	fakeB.waitPacket(t)
}

func TestDialContextFor_CancelledContext(t *testing.T) {
	original := dnsServerResolver.Load()
	t.Cleanup(func() { dnsServerResolver.Store(original) })

	SetDNSServers([]string{"127.0.0.1:53"})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := DialContextFor()(ctx, "tcp", "example.com:443"); err == nil {
		t.Fatal("expected a dial error on a cancelled context, got nil")
	}
}

func TestDialContextFor_NoServersIsSystemDialer(t *testing.T) {
	original := dnsServerResolver.Load()
	t.Cleanup(func() { dnsServerResolver.Store(original) })

	SetDNSServers(nil)
	// A cancelled context fails immediately without any network dependency,
	// proving the dialer is wired (not a nil func) and does not panic.
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := DialContextFor()(ctx, "tcp", "example.com:443"); err == nil {
		t.Fatal("expected a dial error on a cancelled context, got nil")
	}
	var opErr *net.OpError
	if _, err := DialContextFor()(ctx, "tcp", "example.com:443"); !errors.As(err, &opErr) {
		t.Logf("dial error type: %T (%v)", err, err)
	}
}
