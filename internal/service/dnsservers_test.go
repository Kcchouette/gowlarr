package service

import (
	"context"
	"net"
	"net/http"
	"testing"

	"github.com/Kcchouette/cardigann-go/httpclient"

	"github.com/Kcchouette/gowlarr/internal/netutil"
)

func TestApplyDNSServers_InstallsDialContext(t *testing.T) {
	netutil.SetDNSServers([]string{"127.0.0.1:53"})
	t.Cleanup(func() { netutil.SetDNSServers(nil) })

	client, err := httpclient.New(httpclient.Options{IndexerID: "test"})
	if err != nil {
		t.Fatalf("httpclient.New: %v", err)
	}
	applyDNSServers(client)

	tr, ok := client.HTTPClient().Transport.(*http.Transport)
	if !ok {
		t.Fatalf("expected *http.Transport, got %T", client.HTTPClient().Transport)
	}
	if tr.DialContext == nil {
		t.Fatal("expected DialContext when DNS servers are configured")
	}
	if !tr.ForceAttemptHTTP2 {
		t.Fatal("expected ForceAttemptHTTP2 with a custom DialContext")
	}
}

func TestApplyDNSServers_NoServersLeavesTransport(t *testing.T) {
	netutil.SetDNSServers(nil)

	client, err := httpclient.New(httpclient.Options{IndexerID: "test"})
	if err != nil {
		t.Fatalf("httpclient.New: %v", err)
	}
	before := client.HTTPClient().Transport
	applyDNSServers(client)
	if client.HTTPClient().Transport != before {
		t.Fatal("transport must be untouched when no DNS servers are configured")
	}
}

func TestApplyDNSServers_PreservesExistingDialContext(t *testing.T) {
	netutil.SetDNSServers([]string{"127.0.0.1:53"})
	t.Cleanup(func() { netutil.SetDNSServers(nil) })

	client, err := httpclient.New(httpclient.Options{IndexerID: "test"})
	if err != nil {
		t.Fatalf("httpclient.New: %v", err)
	}
	// Simulate the SOCKS5 path: a transport with its own DialContext.
	tr := client.HTTPClient().Transport.(*http.Transport).Clone()
	existing := func(ctx context.Context, network, addr string) (net.Conn, error) {
		return nil, net.ErrClosed
	}
	tr.DialContext = existing
	client.HTTPClient().Transport = tr

	applyDNSServers(client)

	got := client.HTTPClient().Transport.(*http.Transport)
	// The function must skip (not clone/replace) when DialContext != nil;
	// the transport pointer itself must be unchanged.
	if got != tr {
		t.Fatal("transport with a pre-existing DialContext (socks5) must not be replaced")
	}
	if got.DialContext == nil {
		t.Fatal("existing DialContext must be preserved")
	}
}
