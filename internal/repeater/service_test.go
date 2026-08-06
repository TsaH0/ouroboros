package repeater

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"sentinel/internal/model"
)

func TestHTTPService_Replay_Passthrough(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("method = %s, want POST", r.Method)
		}
		if r.URL.Path != "/api/test" {
			t.Errorf("path = %s, want /api/test", r.URL.Path)
		}
		body, _ := io.ReadAll(r.Body)
		if string(body) != `{"key":"value"}` {
			t.Errorf("body = %s, want {\"key\":\"value\"}", string(body))
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"ok":true}`))
	}))
	defer upstream.Close()

	svc := NewHTTPService()
	flow := &model.Flow{
		Request: &model.Message{
			Method:      http.MethodPost,
			URL:         upstream.URL + "/api/test",
			HTTPVersion: "HTTP/1.1",
			Headers:     map[string][]string{"Content-Type": {"application/json"}},
			Body:        []byte(`{"key":"value"}`),
		},
	}

	resp, err := svc.Replay(context.Background(), flow, Edits{})
	if err != nil {
		t.Fatalf("replay: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	if string(resp.Body) != `{"ok":true}` {
		t.Fatalf("body = %s, want {\"ok\":true}", string(resp.Body))
	}
	if resp.Headers["Content-Type"][0] != "application/json" {
		t.Fatalf("content-type = %s", resp.Headers["Content-Type"][0])
	}
}

func TestHTTPService_Replay_WithEdits(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut {
			t.Errorf("method = %s, want PUT", r.Method)
		}
		if !strings.Contains(r.URL.String(), "/api/updated") {
			t.Errorf("path = %s, want /api/updated", r.URL.Path)
		}
		body, _ := io.ReadAll(r.Body)
		if string(body) != "new body" {
			t.Errorf("body = %s, want new body", string(body))
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer upstream.Close()

	svc := NewHTTPService()
	flow := &model.Flow{
		Request: &model.Message{
			Method: http.MethodPost,
			URL:    upstream.URL + "/api/original",
			Body:   []byte("original body"),
		},
	}

	resp, err := svc.Replay(context.Background(), flow, Edits{
		Method:  http.MethodPut,
		URL:     upstream.URL + "/api/updated",
		Body:    []byte("new body"),
		Headers: map[string][]string{"X-Custom": {"header"}},
	})
	if err != nil {
		t.Fatalf("replay: %v", err)
	}
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("status = %d, want 204", resp.StatusCode)
	}
}

func TestHTTPService_Replay_NoURL(t *testing.T) {
	svc := NewHTTPService()
	flow := &model.Flow{}
	_, err := svc.Replay(context.Background(), flow, Edits{})
	if err == nil {
		t.Fatal("expected error with no URL")
	}
}
