package flaresolverr

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestClient_Get(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1" {
			t.Errorf("expected /v1, got %s", r.URL.Path)
		}
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}

		var req map[string]interface{}
		json.NewDecoder(r.Body).Decode(&req)

		if req["cmd"] != "request.get" {
			t.Errorf("expected cmd=request.get, got %v", req["cmd"])
		}

		resp := Response{
			Status: "ok",
			Solution: struct {
				URL      string   `json:"url"`
				Status   int      `json:"status"`
				Response string   `json:"response"`
				Cookies  []Cookie `json:"cookies"`
			}{
				URL:      "https://example.com",
				Status:   200,
				Response: "<html>test</html>",
			},
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	client := NewClient(server.URL)
	resp, err := client.Get(context.Background(), Request{URL: "https://example.com"})
	if err != nil {
		t.Fatalf("Get: %v", err)
	}

	if resp.Solution.Status != 200 {
		t.Errorf("status = %d, want 200", resp.Solution.Status)
	}
	if resp.Solution.Response != "<html>test</html>" {
		t.Errorf("response = %q, want %q", resp.Solution.Response, "<html>test</html>")
	}
}

func TestClient_Session(t *testing.T) {
	var createdSession string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req map[string]interface{}
		json.NewDecoder(r.Body).Decode(&req)

		switch req["cmd"] {
		case "sessions.create":
			createdSession = req["session"].(string)
			w.WriteHeader(http.StatusOK)
		case "sessions.destroy":
			if req["session"] != createdSession {
				t.Errorf("destroy session %v != created %v", req["session"], createdSession)
			}
			w.WriteHeader(http.StatusOK)
		default:
			w.WriteHeader(http.StatusBadRequest)
		}
	}))
	defer server.Close()

	client := NewClient(server.URL)
	ctx := context.Background()

	if err := client.CreateSession(ctx, "test-session"); err != nil {
		t.Fatalf("CreateSession: %v", err)
	}

	if createdSession != "test-session" {
		t.Errorf("created session = %q, want %q", createdSession, "test-session")
	}

	if err := client.DestroySession(ctx, "test-session"); err != nil {
		t.Fatalf("DestroySession: %v", err)
	}
}

func TestClient_Unreachable(t *testing.T) {
	client := NewClient("http://localhost:19999")
	_, err := client.Get(context.Background(), Request{URL: "https://example.com"})
	if err == nil {
		t.Fatal("expected error for unreachable FlareSolverr")
	}
}
