package flaresolverr

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

type Client struct {
	BaseURL    string
	HTTPClient *http.Client
}

type Request struct {
	URL        string
	Session    string
	MaxTimeout int
}

type Response struct {
	Solution struct {
		URL      string   `json:"url"`
		Status   int      `json:"status"`
		Response string   `json:"response"`
		Cookies  []Cookie `json:"cookies"`
	} `json:"solution"`
	Status  string `json:"status"`
	Message string `json:"message"`
}

type Cookie struct {
	Name    string  `json:"name"`
	Value   string  `json:"value"`
	Domain  string  `json:"domain"`
	Path    string  `json:"path"`
	Expires float64 `json:"expires"`
}

func NewClient(baseURL string) *Client {
	if baseURL == "" {
		baseURL = "http://localhost:8191"
	}
	return &Client{
		BaseURL: baseURL,
		HTTPClient: &http.Client{
			Timeout: 90 * time.Second, // FlareSolverr drives a headless browser and can legitimately take much longer.
		},
	}
}

const maxFlareSolverrResponseSize = 32 << 20

func (c *Client) Get(ctx context.Context, req Request) (Response, error) {
	if req.MaxTimeout == 0 {
		req.MaxTimeout = 60000
	}

	payload := map[string]interface{}{
		"cmd":        "request.get",
		"url":        req.URL,
		"maxTimeout": req.MaxTimeout,
	}
	if req.Session != "" {
		payload["session"] = req.Session
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return Response{}, fmt.Errorf("marshaling request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.BaseURL+"/v1", bytes.NewReader(body))
	if err != nil {
		return Response{}, fmt.Errorf("building request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := c.HTTPClient.Do(httpReq)
	if err != nil {
		return Response{}, fmt.Errorf("calling FlareSolverr: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := readAllBounded(resp.Body, maxFlareSolverrResponseSize)
	if err != nil {
		return Response{}, fmt.Errorf("reading response: %w", err)
	}

	var fsResp Response
	if err := json.Unmarshal(respBody, &fsResp); err != nil {
		return Response{}, fmt.Errorf("parsing response: %w", err)
	}

	return fsResp, nil
}

func (c *Client) CreateSession(ctx context.Context, name string) error {
	payload := map[string]interface{}{
		"cmd":     "sessions.create",
		"session": name,
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.BaseURL+"/v1", bytes.NewReader(body))
	if err != nil {
		return err
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := c.HTTPClient.Do(httpReq)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("create session returned HTTP %d", resp.StatusCode)
	}
	return nil
}

func (c *Client) DestroySession(ctx context.Context, name string) error {
	payload := map[string]interface{}{
		"cmd":     "sessions.destroy",
		"session": name,
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.BaseURL+"/v1", bytes.NewReader(body))
	if err != nil {
		return err
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := c.HTTPClient.Do(httpReq)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("destroy session returned HTTP %d", resp.StatusCode)
	}
	return nil
}
