package flaresolverr

import "testing"

func TestValidator(t *testing.T) {
	tests := []struct {
		name    string
		url     string
		wantErr bool
	}{
		{"valid https", "https://example.com", false},
		{"valid http", "http://example.com", false},
		{"localhost", "http://localhost:8080", true},
		{"localhost suffix", "http://foo.localhost", true},
		{"private ip 10", "http://10.0.0.1", true},
		{"private ip 172", "http://172.16.0.1", true},
		{"private ip 192", "http://192.168.1.1", true},
		{"loopback", "http://127.0.0.1", true},
		{"link local", "http://169.254.0.1", true},
		{"ftp scheme", "ftp://example.com", true},
		{"file scheme", "file:///etc/passwd", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateURL(tt.url)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateURL(%q) error = %v, wantErr %v", tt.url, err, tt.wantErr)
			}
		})
	}
}
