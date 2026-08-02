package service

import (
	"testing"

	"github.com/Kcchouette/gowlarr/internal/config"
)

func TestEffectiveProxyURLPrefersIndexerSpecific(t *testing.T) {
	cfg := config.Config{HTTPProxy: "http://global.proxy:8080"}

	if got := effectiveProxyURL("", cfg); got != "http://global.proxy:8080" {
		t.Fatalf("effectiveProxyURL() = %q, want %q", got, "http://global.proxy:8080")
	}
	if got := effectiveProxyURL("socks5://indexer.proxy:1080", cfg); got != "socks5://indexer.proxy:1080" {
		t.Fatalf("effectiveProxyURL(indexer) = %q, want %q", got, "socks5://indexer.proxy:1080")
	}
}
