package llm

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"

	"sentinel/internal/model"
)

type mockProvider struct {
	response string
	err      error
}

func (m *mockProvider) ChatCompletion(ctx context.Context, req ChatRequest) (*ChatResponse, error) {
	if m.err != nil {
		return nil, m.err
	}
	return &ChatResponse{Content: m.response, FinishReason: "stop"}, nil
}

func TestAnalyzer_AnalyzeFlow_NoFindings(t *testing.T) {
	provider := &mockProvider{response: `{"findings":[]}`}
	analyzer := NewAnalyzer(provider, "test-model")

	flow := &model.Flow{
		Request: &model.Message{Method: "GET", URL: "https://example.com/", HTTPVersion: "HTTP/1.1", Headers: map[string][]string{}},
		Response: &model.Message{StatusCode: 200, HTTPVersion: "HTTP/1.1", Headers: map[string][]string{}, Body: []byte("ok")},
	}

	result, err := analyzer.AnalyzeFlow(context.Background(), flow)
	if err != nil {
		t.Fatalf("analyze: %v", err)
	}
	if len(result.Findings) != 0 {
		t.Fatalf("expected 0 findings, got %d", len(result.Findings))
	}
}

func TestAnalyzer_AnalyzeFlow_WithFindings(t *testing.T) {
	provider := &mockProvider{response: `{"findings":[{"severity":"high","title":"XSS","description":"Reflected XSS in query parameter","cves":["CVE-2024-0001"]}]}`}
	analyzer := NewAnalyzer(provider, "test-model")

	flow := &model.Flow{
		Request: &model.Message{Method: "GET", URL: "https://example.com/?q=<script>", HTTPVersion: "HTTP/1.1", Headers: map[string][]string{}},
		Response: &model.Message{StatusCode: 200, HTTPVersion: "HTTP/1.1", Headers: map[string][]string{"Content-Type": {"text/html"}}, Body: []byte("<script>alert(1)</script>")},
	}

	result, err := analyzer.AnalyzeFlow(context.Background(), flow)
	if err != nil {
		t.Fatalf("analyze: %v", err)
	}
	if len(result.Findings) != 1 {
		t.Fatalf("expected 1 finding, got %d", len(result.Findings))
	}
	f := result.Findings[0]
	if f.Severity != "high" {
		t.Fatalf("severity = %s, want high", f.Severity)
	}
	if f.Title != "XSS" {
		t.Fatalf("title = %s, want XSS", f.Title)
	}
	if len(f.CVEs) != 1 || f.CVEs[0] != "CVE-2024-0001" {
		t.Fatalf("cves = %v", f.CVEs)
	}
}

func TestAnalyzer_AnalyzeFlow_ProviderError(t *testing.T) {
	provider := &mockProvider{err: fmt.Errorf("network error")}
	analyzer := NewAnalyzer(provider, "test-model")

	flow := &model.Flow{}
	_, err := analyzer.AnalyzeFlow(context.Background(), flow)
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestAnalyzer_AnalyzeFlow_InvalidJSON(t *testing.T) {
	provider := &mockProvider{response: `not json`}
	analyzer := NewAnalyzer(provider, "test-model")

	flow := &model.Flow{}
	_, err := analyzer.AnalyzeFlow(context.Background(), flow)
	if err == nil {
		t.Fatal("expected error for invalid JSON")
	}
}

func TestGetAnalysisSchema(t *testing.T) {
	schema := getAnalysisSchema()
	var s map[string]any
	if err := json.Unmarshal(schema, &s); err != nil {
		t.Fatalf("schema is invalid JSON: %v", err)
	}
	if s["type"] != "object" {
		t.Fatal("schema root is not object")
	}
}
