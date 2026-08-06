package llm

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"

	"ouroboros/internal/model"
	"ouroboros/internal/recon"
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
	provider := &mockProvider{response: `{"summary":"clean","findings":[]}`}
	analyzer := NewAnalyzer(provider, "test-model")

	flow := &model.Flow{
		Request:  &model.Message{Method: "GET", URL: "https://example.com/", HTTPVersion: "HTTP/1.1", Headers: map[string][]string{}},
		Response: &model.Message{StatusCode: 200, HTTPVersion: "HTTP/1.1", Headers: map[string][]string{}, Body: []byte("ok")},
	}

	result, err := analyzer.AnalyzeFlow(context.Background(), flow, nil)
	if err != nil {
		t.Fatalf("analyze: %v", err)
	}
	if len(result.Findings) != 0 {
		t.Fatalf("expected 0 findings, got %d", len(result.Findings))
	}
}

func TestAnalyzer_AnalyzeFlow_WithFindings(t *testing.T) {
	provider := &mockProvider{response: `{"summary":"xss found","findings":[{"severity":"high","title":"XSS","description":"Reflected XSS in query parameter","cves":["CVE-2024-0001"]}]}`}
	analyzer := NewAnalyzer(provider, "test-model")

	flow := &model.Flow{
		Request:  &model.Message{Method: "GET", URL: "https://example.com/?q=<script>", HTTPVersion: "HTTP/1.1", Headers: map[string][]string{}},
		Response: &model.Message{StatusCode: 200, HTTPVersion: "HTTP/1.1", Headers: map[string][]string{"Content-Type": {"text/html"}}, Body: []byte("<script>alert(1)</script>")},
	}

	result, err := analyzer.AnalyzeFlow(context.Background(), flow, nil)
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

func TestAnalyzer_AnalyzeFlow_WithContext(t *testing.T) {
	provider := &mockProvider{response: `{"summary":"ok","findings":[]}`}
	analyzer := NewAnalyzer(provider, "test-model")

	prior := []Message{
		{Role: RoleUser, Content: "prior bulk analysis context"},
		{Role: RoleAssistant, Content: "understood"},
	}

	flow := &model.Flow{Request: &model.Message{Method: "GET", URL: "https://example.com/"}}
	_, err := analyzer.AnalyzeFlow(context.Background(), flow, prior)
	if err != nil {
		t.Fatalf("analyze with context: %v", err)
	}
}

func TestAnalyzer_AnalyzeBulk(t *testing.T) {
	provider := &mockProvider{response: `{"summary":"1 suspicious","findings":[{"flow_id":"abc12345","severity":"high","title":"SQL injection","why":"id param unescaped","suggestion":"use parameterized queries"}]}`}
	analyzer := NewAnalyzer(provider, "test-model")

	flows := []*model.Flow{
		{ID: "01JABC12345", Host: "example.com", Request: &model.Message{Method: "GET", URL: "https://example.com/?id=1'OR'1"}},
	}

	result, err := analyzer.AnalyzeBulk(context.Background(), flows)
	if err != nil {
		t.Fatalf("bulk analyze: %v", err)
	}
	if len(result.Findings) != 1 {
		t.Fatalf("expected 1 finding, got %d", len(result.Findings))
	}
	if result.Findings[0].FlowID != "abc12345" {
		t.Fatalf("flow_id = %s", result.Findings[0].FlowID)
	}
	if result.Findings[0].Suggestion != "use parameterized queries" {
		t.Fatalf("suggestion = %s", result.Findings[0].Suggestion)
	}
}

func TestAnalyzer_AnalyzeFlow_ProviderError(t *testing.T) {
	provider := &mockProvider{err: fmt.Errorf("network error")}
	analyzer := NewAnalyzer(provider, "test-model")

	flow := &model.Flow{}
	_, err := analyzer.AnalyzeFlow(context.Background(), flow, nil)
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestAnalyzer_AnalyzeFlow_InvalidJSON(t *testing.T) {
	provider := &mockProvider{response: `not json`}
	analyzer := NewAnalyzer(provider, "test-model")

	flow := &model.Flow{}
	_, err := analyzer.AnalyzeFlow(context.Background(), flow, nil)
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

func TestGetBulkSchema(t *testing.T) {
	schema := getBulkSchema()
	var s map[string]any
	if err := json.Unmarshal(schema, &s); err != nil {
		t.Fatalf("bulk schema is invalid JSON: %v", err)
	}
	props, ok := s["properties"].(map[string]any)
	if !ok {
		t.Fatal("missing properties")
	}
	if _, ok := props["summary"]; !ok {
		t.Fatal("missing summary in bulk schema")
	}
}

func TestGetReconSchema(t *testing.T) {
	schema := getReconSchema()
	var s map[string]any
	if err := json.Unmarshal(schema, &s); err != nil {
		t.Fatalf("recon schema is invalid JSON: %v", err)
	}
	props, ok := s["properties"].(map[string]any)
	if !ok {
		t.Fatal("missing properties in recon schema")
	}
	required := []string{"summary", "high_priority", "interesting_hosts", "interesting_endpoints", "recommended_testing_order", "interesting_patterns", "reasoning"}
	for _, key := range required {
		if _, ok := props[key]; !ok {
			t.Fatalf("recon schema missing property: %s", key)
		}
	}
}

func TestAnalyzer_AnalyzeRecon(t *testing.T) {
	resp := `{"summary":"focus on admin panel","high_priority":["admin panel at /admin"],"interesting_hosts":["admin.example.com"],"interesting_endpoints":["/admin"],"recommended_testing_order":["1. Test /admin for auth bypass","2. Test /api for IDOR"],"interesting_patterns":["admin endpoints exposed without rate limiting"],"reasoning":["Admin panel accessible from external network"]}`
	provider := &mockProvider{response: resp}
	analyzer := NewAnalyzer(provider, "test-model")

	summary := &recon.ReconSummary{
		Target:       "example.com",
		Hosts:        []recon.Host{{Hostname: "admin.example.com", Sources: []recon.Source{recon.SourceSubfinder}}},
		Endpoints:    []recon.Endpoint{{URL: "http://example.com/admin", Host: "example.com", Path: "/admin", Category: recon.CatAdmin, Score: 100, Sources: []recon.Source{recon.SourceGAU}}},
		Technologies: []recon.Technology{{Name: "Apache", Version: "2.4.41", Host: "example.com", Source: recon.SourceWhatWeb}},
	}

	result, err := analyzer.AnalyzeRecon(context.Background(), summary)
	if err != nil {
		t.Fatalf("analyze recon: %v", err)
	}
	if result.Summary != "focus on admin panel" {
		t.Fatalf("summary = %s", result.Summary)
	}
	if len(result.HighPriority) != 1 {
		t.Fatalf("high_priority count = %d", len(result.HighPriority))
	}
	if len(result.RecommendedOrder) != 2 {
		t.Fatalf("recommended order count = %d", len(result.RecommendedOrder))
	}
}

func TestAnalyzer_AnalyzeRecon_ProviderError(t *testing.T) {
	provider := &mockProvider{err: fmt.Errorf("network error")}
	analyzer := NewAnalyzer(provider, "test-model")

	summary := &recon.ReconSummary{Target: "example.com"}
	_, err := analyzer.AnalyzeRecon(context.Background(), summary)
	if err == nil {
		t.Fatal("expected error for provider failure")
	}
}

func TestAnalyzer_AnalyzeRecon_InvalidJSON(t *testing.T) {
	provider := &mockProvider{response: `not json`}
	analyzer := NewAnalyzer(provider, "test-model")

	summary := &recon.ReconSummary{Target: "example.com"}
	_, err := analyzer.AnalyzeRecon(context.Background(), summary)
	if err == nil {
		t.Fatal("expected error for invalid JSON")
	}
}
