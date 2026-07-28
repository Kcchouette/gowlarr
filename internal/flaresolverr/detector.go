package flaresolverr

import "bytes"

var cloudflareMarkers = []string{
	"cf-browser-verification",
	"challenge-platform",
	"Just a moment...",
	"Checking if the site connection is secure",
	"Enable JavaScript and cookies to continue",
	"Verifying you are human",
	"Checking your browser",
}

func IsCloudflareChallenge(status int, body []byte) bool {
	if status != 503 {
		return false
	}
	for _, marker := range cloudflareMarkers {
		if bytes.Contains(body, []byte(marker)) {
			return true
		}
	}
	return false
}
