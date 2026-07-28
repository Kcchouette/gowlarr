package flaresolverr

import "testing"

func TestDetector_Challenge503(t *testing.T) {
	tests := []struct {
		name   string
		status int
		body   string
		want   bool
	}{
		{"cf-browser-verification", 503, "cf-browser-verification", true},
		{"challenge-platform", 503, "challenge-platform", true},
		{"just a moment", 503, "Just a moment...", true},
		{"checking browser", 503, "Checking your browser", true},
		{"not 503", 200, "cf-browser-verification", false},
		{"normal 503", 503, "Service Unavailable", false},
		{"empty body", 503, "", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := IsCloudflareChallenge(tt.status, []byte(tt.body))
			if got != tt.want {
				t.Errorf("IsCloudflareChallenge(%d, %q) = %v, want %v", tt.status, tt.body, got, tt.want)
			}
		})
	}
}
