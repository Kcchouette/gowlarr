package flaresolverr

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"strings"
)

const maxBufferedResponseBodySize = 10 << 20

type FlareSolverrTransport struct {
	Base          http.RoundTripper
	FlareClient   *Client
	Session       string
	SkipValidator bool // For testing: skip URL validation
}

func (t *FlareSolverrTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	// Utilise une variable locale plutôt que de muter t.Base : évite une
	// data race si plusieurs goroutines partagent le même transport avec
	// Base == nil.
	base := t.Base
	if base == nil {
		base = http.DefaultTransport
	}

	// Validate URL before making request (skip in tests)
	if !t.SkipValidator {
		if err := ValidateURL(req.URL.String()); err != nil {
			return nil, err
		}
	}

	resp, err := base.RoundTrip(req)
	if err != nil {
		return nil, err
	}

	// Read body to check for Cloudflare challenge
	body, err := readAllBounded(resp.Body, maxBufferedResponseBodySize)
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
			Header:     copyFallbackHeaders(resp.Header),
			Request:    req,
		}, nil
	}

	// Reconstruct response with original body
	resp.Body = io.NopCloser(bytes.NewReader(body))
	return resp, nil
}

func readAllBounded(r io.Reader, limit int64) ([]byte, error) {
	body, err := io.ReadAll(io.LimitReader(r, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(body)) > limit {
		return nil, fmt.Errorf("response body too large (max %d bytes)", limit)
	}
	return body, nil
}

func copyFallbackHeaders(src http.Header) http.Header {
	dst := make(http.Header)
	if values := src.Values("Content-Type"); len(values) > 0 {
		dst["Content-Type"] = append([]string(nil), values...)
	}
	return dst
}
