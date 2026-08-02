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
	// Use a local variable instead of mutating t.Base: avoids a data race
	// when multiple goroutines share the same transport with Base == nil.
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

	// The challenge scan can only trigger on 503 (IsCloudflareChallenge gates
	// on that status), so buffering any other response is pure waste (up to
	// 10 MiB per request). Pass the body through untouched; the caller reads
	// and closes it.
	if resp.StatusCode != http.StatusServiceUnavailable {
		return resp, nil
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
