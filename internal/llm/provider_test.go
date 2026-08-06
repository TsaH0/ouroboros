package llm

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestGeminiProviderChatCompletion(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/models/gemini-test:generateContent" {
			t.Fatalf("path = %s", r.URL.Path)
		}
		if got := r.URL.Query().Get("key"); got != "test-key" {
			t.Fatalf("key = %q", got)
		}
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		if _, ok := body["systemInstruction"]; !ok {
			t.Fatal("missing systemInstruction")
		}
		cfg := body["generationConfig"].(map[string]any)
		if cfg["responseMimeType"] != "application/json" {
			t.Fatalf("responseMimeType = %v", cfg["responseMimeType"])
		}
		if _, ok := cfg["responseSchema"]; !ok {
			t.Fatal("missing responseSchema")
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"candidates":[{"content":{"parts":[{"text":"{\"findings\":[]}"}]},"finishReason":"STOP"}]}`))
	}))
	defer server.Close()

	provider := NewGeminiProvider(server.URL, "test-key")
	resp, err := provider.ChatCompletion(context.Background(), ChatRequest{
		Model: "gemini-test",
		Messages: []Message{
			{Role: RoleSystem, Content: "system"},
			{Role: RoleUser, Content: "user"},
		},
		Schema: json.RawMessage(`{"type":"object"}`),
	})
	if err != nil {
		t.Fatalf("ChatCompletion: %v", err)
	}
	if resp.Content != `{"findings":[]}` {
		t.Fatalf("content = %q", resp.Content)
	}
	if resp.FinishReason != "STOP" {
		t.Fatalf("finish = %q", resp.FinishReason)
	}
}
