package store

import (
	"context"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"ouroboros/internal/model"
	"ouroboros/internal/recon"
	"ouroboros/internal/scope"
)

func TestSQLiteStore_CRUD(t *testing.T) {
	s := newSQLiteStore(t)
	ctx := context.Background()

	// Flow round-trip.
	f := &model.Flow{
		ID:        "flow-1",
		Host:      "example.com",
		Scheme:    "https",
		Port:      443,
		StartTime: time.Now(),
		Duration:  100 * time.Millisecond,
		State:     model.FlowCompleted,
		ScopeStatus: model.ScopeInScope,
		Tags:      []string{"test"},
		Notes:     "test note",
		Request: &model.Message{
			Method:      "GET",
			URL:         "https://example.com/api",
			HTTPVersion: "HTTP/1.1",
			Headers:     map[string][]string{"Host": {"example.com"}},
			Body:        []byte("request body"),
		},
		Response: &model.Message{
			StatusCode:  200,
			HTTPVersion: "HTTP/1.1",
			Headers:     map[string][]string{"Content-Type": {"text/plain"}},
			Body:        []byte("response body"),
		},
	}
	if err := s.SaveFlow(ctx, f); err != nil {
		t.Fatalf("SaveFlow: %v", err)
	}

	got, err := s.GetFlow(ctx, "flow-1")
	if err != nil {
		t.Fatalf("GetFlow: %v", err)
	}
	if got == nil {
		t.Fatal("GetFlow returned nil")
	}
	if got.Host != "example.com" {
		t.Errorf("host = %q, want example.com", got.Host)
	}
	if got.ScopeStatus != model.ScopeInScope {
		t.Errorf("scope_status = %q, want in_scope", got.ScopeStatus)
	}
	if string(got.Request.Body) != "request body" {
		t.Errorf("request body = %q, want %q", got.Request.Body, "request body")
	}
	if string(got.Response.Body) != "response body" {
		t.Errorf("response body = %q, want %q", got.Response.Body, "response body")
	}
	if len(got.Tags) != 1 || got.Tags[0] != "test" {
		t.Errorf("tags = %v, want [test]", got.Tags)
	}

	// List.
	flows, err := s.ListFlows(ctx)
	if err != nil {
		t.Fatalf("ListFlows: %v", err)
	}
	if len(flows) != 1 {
		t.Fatalf("expected 1 flow, got %d", len(flows))
	}

	// Scope rules.
	r := scope.Rule{
		ID:        "rule-1",
		Kind:      scope.RuleKindHost,
		Pattern:   "example.com",
		MatchMode: scope.MatchModeLiteral,
		Action:    scope.ActionInclude,
		Enabled:   true,
		Priority:  10,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	if err := s.SaveScopeRule(ctx, &r); err != nil {
		t.Fatalf("SaveScopeRule: %v", err)
	}
	rules, err := s.LoadScopeRules(ctx)
	if err != nil {
		t.Fatalf("LoadScopeRules: %v", err)
	}
	if len(rules) != 1 {
		t.Fatalf("expected 1 rule, got %d", len(rules))
	}
	if err := s.DeleteScopeRule(ctx, "rule-1"); err != nil {
		t.Fatalf("DeleteScopeRule: %v", err)
	}
	rules, _ = s.LoadScopeRules(ctx)
	if len(rules) != 0 {
		t.Fatalf("expected 0 rules after delete, got %d", len(rules))
	}

	// Recon.
	summary := &recon.ReconSummary{
		Target:    "example.com",
		CreatedAt: time.Now(),
		Hosts:     []recon.Host{{Hostname: "www.example.com"}},
	}
	if err := s.SaveRecon(ctx, summary); err != nil {
		t.Fatalf("SaveRecon: %v", err)
	}
	loaded, err := s.GetRecon(ctx, "example.com")
	if err != nil {
		t.Fatalf("GetRecon: %v", err)
	}
	if loaded == nil {
		t.Fatal("GetRecon returned nil")
	}
	if len(loaded.Hosts) != 1 || loaded.Hosts[0].Hostname != "www.example.com" {
		t.Errorf("recon host = %+v", loaded.Hosts)
	}

	// Technologies.
	tech := recon.Technology{Name: "nginx", Version: "1.20", Host: "www.example.com", Source: "whatweb"}
	if err := s.SaveTechnology(ctx, "run-1", tech); err != nil {
		t.Fatalf("SaveTechnology: %v", err)
	}

	// Vulnerabilities.
	vuln := recon.Vulnerability{CVE: "CVE-2023-1234", Title: "Test vuln", Source: "searchsploit"}
	if err := s.SaveVulnerability(ctx, "run-1", vuln); err != nil {
		t.Fatalf("SaveVulnerability: %v", err)
	}

	// Analyses.
	analysis := &model.Analysis{
		ID:        "analysis-1",
		FlowID:    "flow-1",
		Kind:      "flow",
		Provider:  "openai",
		Model:     "gpt-4",
		Summary:   "test analysis",
		RawJSON:   `{"test": true}`,
		CreatedAt: time.Now(),
	}
	if err := s.SaveAnalysis(ctx, analysis); err != nil {
		t.Fatalf("SaveAnalysis: %v", err)
	}
	analyses, err := s.ListAnalyses(ctx, "flow-1")
	if err != nil {
		t.Fatalf("ListAnalyses: %v", err)
	}
	if len(analyses) != 1 {
		t.Fatalf("expected 1 analysis, got %d", len(analyses))
	}

	// Settings.
	if err := s.SaveSetting(ctx, "theme", "dark"); err != nil {
		t.Fatalf("SaveSetting: %v", err)
	}
	val, err := s.GetSetting(ctx, "theme")
	if err != nil {
		t.Fatalf("GetSetting: %v", err)
	}
	if val != "dark" {
		t.Errorf("setting = %q, want dark", val)
	}
	val, _ = s.GetSetting(ctx, "missing")
	if val != "" {
		t.Errorf("missing setting = %q, want empty", val)
	}
}

func TestSQLiteStore_MigrationIdempotent(t *testing.T) {
	path := filepath.Join(t.TempDir(), "test.db")
	s1, err := NewSQLiteStore(path)
	if err != nil {
		t.Fatalf("first open: %v", err)
	}
	s1.Close()

	s2, err := NewSQLiteStore(path)
	if err != nil {
		t.Fatalf("second open: %v", err)
	}
	s2.Close()
}

func TestSQLiteStore_Concurrent(t *testing.T) {
	s := newSQLiteStore(t)
	ctx := context.Background()

	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			f := &model.Flow{ID: string(rune(n)), Host: "example.com"}
			_ = s.SaveFlow(ctx, f)
		}(i)
	}
	wg.Wait()

	flows, err := s.ListFlows(ctx)
	if err != nil {
		t.Fatalf("ListFlows: %v", err)
	}
	if len(flows) != 50 {
		t.Fatalf("expected 50 flows, got %d", len(flows))
	}
}

func newSQLiteStore(t *testing.T) *SQLiteStore {
	t.Helper()
	path := filepath.Join(t.TempDir(), "test.db")
	s, err := NewSQLiteStore(path)
	if err != nil {
		t.Fatalf("NewSQLiteStore: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}
