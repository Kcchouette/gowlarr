package netutil

import (
	"context"
	"net"
	"testing"
)

func TestValidateURL(t *testing.T) {
	originalLookupHost := lookupHost
	t.Cleanup(func() { lookupHost = originalLookupHost })

	lookupHost = func(ctx context.Context, host string) ([]string, error) {
		switch host {
		case "public.example":
			return []string{"93.184.216.34"}, nil
		case "private.example":
			return []string{"10.0.0.1"}, nil
		default:
			return nil, &net.DNSError{Err: "no such host", Name: host}
		}
	}

	tests := []struct {
		name    string
		url     string
		wantErr bool
	}{
		{"public hostname", "https://public.example/path", false},
		{"private resolved hostname", "https://private.example/path", true},
		{"public ip literal", "https://1.1.1.1/path", false},
		{"private ip literal", "https://192.168.1.1/path", true},
		{"localhost", "http://localhost:8080", true},
		{"bad scheme", "ftp://public.example/path", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateURL(tt.url)
			if (err != nil) != tt.wantErr {
				t.Fatalf("ValidateURL(%q) error = %v, wantErr %v", tt.url, err, tt.wantErr)
			}
		})
	}
}
