package flaresolverr

import (
	"bytes"
	"io"
	"net/http"
	"strings"
)

type FlareSolverrTransport struct {
	Base          http.RoundTripper
	FlareClient   *Client
	Session       string
	SkipValidator bool // For testing: skip URL validation
}

func (t *FlareSolverrTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if t.Base == nil {
		t.Base = http.DefaultTransport
	}

	// Validate URL before making request (skip in tests)
	if !t.SkipValidator {
		if err := ValidateURL(req.URL.String()); err != nil {
			return nil, err
		}
	}

	resp, err := t.Base.RoundTrip(req)
	if err != nil {
		return nil, err
	}

	// Read body to check for Cloudflare challenge
	body, err := io.ReadAll(resp.Body)
	resp.Body.Close()
	if err != nil {
		return nil, err
	}

	if IsCloudflareChallenge(resp.StatusCode, body) && t.FlareClient != nil {
		// Retry via FlareSolverr
		fsResp, err := t.FlareClient.Get(req.Context(), Request{
			URL:        req.URL.String(),
			Session:    t.Session,
			MaxTimeout: 60000,
		})
		if err != nil {
			// Return original response if FlareSolverr fails
			return &http.Response{
				StatusCode: resp.StatusCode,
				Body:       io.NopCloser(bytes.NewReader(body)),
				Header:     resp.Header,
				Request:    req,
			}, nil
		}

		return &http.Response{
			StatusCode: fsResp.Solution.Status,
			Body:       io.NopCloser(strings.NewReader(fsResp.Solution.Response)),
			Header:     make(http.Header),
			Request:    req,
		}, nil
	}

	// Reconstruct response with original body
	resp.Body = io.NopCloser(bytes.NewReader(body))
	return resp, nil
}
