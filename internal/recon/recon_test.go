package recon

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"
)

// --- Normalization & Dedup Tests ---

func TestNormalizeURL_SchemeAndHost(t *testing.T) {
	tests := []struct {
		in   string
		want string
	}{
		{"HTTP://Example.COM/path", "http://example.com/path"},
		{"http://example.com:80/path", "http://example.com/path"},
		{"https://example.com:443/path", "https://example.com/path"},
		{"http://example.com/path/#frag", "http://example.com/path"},
		{"http://example.com/path/", "http://example.com/path"},
		{"http://example.com/", "http://example.com/"},
		{"  http://example.com/x  ", "http://example.com/x"},
		{"", ""},
		{"not-a-url", "http://not-a-url"},
	}
	for _, tt := range tests {
		got := NormalizeURL(tt.in)
		if got != tt.want {
			t.Errorf("NormalizeURL(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestNormalizeURL_QuerySorting(t *testing.T) {
	a := NormalizeURL("http://example.com/api?b=2&a=1")
	b := NormalizeURL("http://example.com/api?a=1&b=2")
	if a != b {
		t.Errorf("query param order not normalized:\n  a=%s\n  b=%s", a, b)
	}
}

func TestDedupHosts_MergesSources(t *testing.T) {
	hosts := []Host{
		{Hostname: "sub.example.com", Sources: []Source{SourceSubfinder}},
		{Hostname: "sub.example.com", Sources: []Source{SourceGAU}},
		{Hostname: "other.example.com", Sources: []Source{SourceSubfinder}},
	}
	result := DeduplicateHosts(hosts)
	if len(result) != 2 {
		t.Fatalf("expected 2 hosts, got %d", len(result))
	}
	var sub *Host
	for i := range result {
		if result[i].Hostname == "sub.example.com" {
			sub = &result[i]
		}
	}
	if sub == nil {
		t.Fatal("sub.example.com not found")
	}
	if len(sub.Sources) != 2 {
		t.Fatalf("expected 2 sources, got %d", len(sub.Sources))
	}
}

func TestDedupEndpoints_MergesSameURL(t *testing.T) {
	endpoints := []Endpoint{
		{URL: "http://example.com/admin", Category: CatAdmin, Score: 100, Sources: []Source{SourceGAU}},
		{URL: "http://example.com/admin", Category: CatAdmin, Score: 100, Sources: []Source{SourceWayback}},
	}
	result := DeduplicateEndpoints(endpoints)
	if len(result) != 1 {
		t.Fatalf("expected 1 endpoint, got %d", len(result))
	}
	if len(result[0].Sources) != 2 {
		t.Fatalf("expected 2 sources, got %d", len(result[0].Sources))
	}
}

func TestDedupTechs_MergesIdentical(t *testing.T) {
	techs := []Technology{
		{Name: "Apache", Version: "2.4.41", Host: "example.com", Source: SourceWhatWeb, Confidence: "100"},
		{Name: "Apache", Version: "2.4.41", Host: "example.com", Source: SourceWhatWeb, Confidence: "50"},
		{Name: "Nginx", Version: "1.18", Host: "example.com", Source: SourceWhatWeb, Confidence: "100"},
	}
	result := DeduplicateTechnologies(techs)
	if len(result) != 2 {
		t.Fatalf("expected 2 techs, got %d", len(result))
	}
}

func TestDedupVulns_RemovesDuplicates(t *testing.T) {
	vulns := []Vulnerability{
		{CVE: "CVE-2024-0001", Title: "RCE in Apache", Source: SourceSearchSploit},
		{CVE: "CVE-2024-0001", Title: "RCE in Apache", Source: SourceSearchSploit},
		{CVE: "", Title: "Info Disclosure", Source: SourceSearchSploit},
	}
	result := DeduplicateVulns(vulns)
	if len(result) != 2 {
		t.Fatalf("expected 2 vulns, got %d", len(result))
	}
}

// --- Classification Tests ---

func TestClassifyEndpoint_Categories(t *testing.T) {
	tests := []struct {
		url      string
		category EndpointCategory
		score    int
	}{
		{"http://example.com/admin", CatAdmin, 100},
		{"http://example.com/login", CatAuth, 90},
		{"http://example.com/graphql", CatGraphQL, 80},
		{"http://example.com/upload", CatUpload, 75},
		{"http://example.com/swagger.json", CatSwagger, 70},
		{"http://example.com/debug", CatDebug, 65},
		{"http://example.com/.env", CatConfig, 60},
		{"http://example.com/api/users", CatAPI, 50},
		{"http://example.com/webhook/stripe", CatWebhook, 45},
		{"http://example.com/.well-known/security.txt", CatSecurity, 40},
		{"http://example.com/docs", CatDocs, 20},
		{"http://example.com/health", CatHealth, 10},
		{"http://example.com/random/page", CatGeneric, 0},
	}
	for _, tt := range tests {
		cat, score := ClassifyEndpoint(tt.url)
		if cat != tt.category {
			t.Errorf("ClassifyEndpoint(%q) category = %s, want %s", tt.url, cat, tt.category)
		}
		if score != tt.score {
			t.Errorf("ClassifyEndpoint(%q) score = %d, want %d", tt.url, score, tt.score)
		}
	}
}

// --- Cache Tests ---

func TestCache_GetSet(t *testing.T) {
	c := NewCache()
	s := &ReconSummary{Target: "example.com"}

	if _, ok := c.Get("example.com"); ok {
		t.Fatal("expected miss on empty cache")
	}

	c.Set("example.com", s)
	got, ok := c.Get("example.com")
	if !ok {
		t.Fatal("expected hit after Set")
	}
	if got.Target != "example.com" {
		t.Fatalf("got target %s", got.Target)
	}
	if c.Size() != 1 {
		t.Fatalf("size = %d, want 1", c.Size())
	}
}

func TestCache_Clear(t *testing.T) {
	c := NewCache()
	c.Set("a", &ReconSummary{Target: "a"})
	c.Set("b", &ReconSummary{Target: "b"})
	if c.Size() != 2 {
		t.Fatalf("size = %d, want 2", c.Size())
	}
	c.Clear()
	if c.Size() != 0 {
		t.Fatalf("after clear, size = %d, want 0", c.Size())
	}
}

func TestCache_ConcurrentAccess(t *testing.T) {
	c := NewCache()
	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			c.Set("target", &ReconSummary{Target: "target"})
			c.Get("target")
		}(i)
	}
	wg.Wait()
	if c.Size() != 1 {
		t.Fatalf("size = %d, want 1", c.Size())
	}
}

// --- Engine Tests ---

type mockProvider struct {
	name     string
	delay    time.Duration
	findings []ReconFinding
	err      error
}

func (m *mockProvider) Name() string { return m.name }

func (m *mockProvider) Run(ctx context.Context, target string) ([]ReconFinding, error) {
	select {
	case <-time.After(m.delay):
		return m.findings, m.err
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

type mockSummaryAwareProvider struct {
	prepared []Technology
}

func (m *mockSummaryAwareProvider) Name() string { return "summary-aware" }

func (m *mockSummaryAwareProvider) Prepare(summary *ReconSummary) {
	m.prepared = append([]Technology(nil), summary.Technologies...)
}

func (m *mockSummaryAwareProvider) Run(context.Context, string) ([]ReconFinding, error) {
	if len(m.prepared) == 0 {
		return nil, fmt.Errorf("missing technologies")
	}
	return []ReconFinding{{
		Type:       "vulnerability",
		Source:     SourceSearchSploit,
		Title:      "Known exploit for " + m.prepared[0].Name,
		ExploitRef: "exploits/example.txt",
	}}, nil
}

func TestEngine_DefersSummaryAwareProviders(t *testing.T) {
	enricher := &mockSummaryAwareProvider{}
	engine := NewEngine(NewCache(), DefaultCommandRunner{}, []ProviderMetadata{
		{Provider: &mockProvider{name: "technology", findings: []ReconFinding{{
			Type:       "technology",
			Source:     SourceWhatWeb,
			Value:      "example.com",
			Technology: "Apache",
			Version:    "2.4.41",
		}}}, Role: RoleEnrichment},
		{Provider: enricher, Role: RoleEnrichment},
	})

	summary, err := engine.Run(context.Background(), "example.com")
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if len(enricher.prepared) != 1 || enricher.prepared[0].Name != "Apache" {
		t.Fatalf("prepared technologies = %#v", enricher.prepared)
	}
	if summary.VulnCount() != 1 {
		t.Fatalf("vulnerabilities = %d, want 1", summary.VulnCount())
	}
}

func TestEngine_RunBasic(t *testing.T) {
	providers := []ProviderMetadata{
		{Provider: &mockProvider{name: "p1", delay: 10 * time.Millisecond, findings: []ReconFinding{
			{Type: "host", Source: SourceSubfinder, Value: "sub.example.com"},
		}}, Role: RoleDiscovery, Timeout: 5},
		{Provider: &mockProvider{name: "p2", delay: 10 * time.Millisecond, findings: []ReconFinding{
			{Type: "url", Source: SourceGAU, Value: "http://example.com/admin"},
		}}, Role: RoleDiscovery, Timeout: 5},
	}
	cache := NewCache()
	engine := NewEngine(cache, DefaultCommandRunner{}, providers)
	summary, err := engine.Run(context.Background(), "example.com")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if summary.HostCount() != 1 {
		t.Fatalf("host count = %d, want 1", summary.HostCount())
	}
	if summary.EndpointCount() != 1 {
		t.Fatalf("endpoint count = %d, want 1", summary.EndpointCount())
	}
	if len(summary.InterestingEndpoints()) != 1 {
		t.Fatalf("interesting endpoints = %d, want 1", len(summary.InterestingEndpoints()))
	}
}

func TestEngine_CacheHitSkipsProviders(t *testing.T) {
	providers := []ProviderMetadata{
		{Provider: &mockProvider{name: "p1", delay: 10 * time.Millisecond, findings: []ReconFinding{
			{Type: "host", Source: SourceSubfinder, Value: "cached.example.com"},
		}}, Role: RoleDiscovery, Timeout: 5},
	}
	cache := NewCache()
	engine := NewEngine(cache, DefaultCommandRunner{}, providers)

	_, err := engine.Run(context.Background(), "example.com")
	if err != nil {
		t.Fatalf("first run: %v", err)
	}

	summary, err := engine.Run(context.Background(), "example.com")
	if err != nil {
		t.Fatalf("second run: %v", err)
	}
	if summary.HostCount() != 1 {
		t.Fatalf("cached host count = %d, want 1", summary.HostCount())
	}
	if summary.Hosts[0].Hostname != "cached.example.com" {
		t.Fatalf("cached hostname = %s", summary.Hosts[0].Hostname)
	}
}

func TestEngine_CanRunDifferentTargetsSequentially(t *testing.T) {
	providers := []ProviderMetadata{
		{Provider: &mockProvider{name: "p1", findings: []ReconFinding{
			{Type: "host", Source: SourceSubfinder, Value: "sub.example.com"},
		}}, Role: RoleDiscovery, Timeout: 5},
	}
	engine := NewEngine(NewCache(), DefaultCommandRunner{}, providers)

	for _, target := range []string{"first.example", "second.example"} {
		if _, err := engine.Run(context.Background(), target); err != nil {
			t.Fatalf("run %q: %v", target, err)
		}
	}
}

func TestEngine_Cancellation(t *testing.T) {
	providers := []ProviderMetadata{
		{Provider: &mockProvider{name: "slow", delay: 5 * time.Second, findings: []ReconFinding{
			{Type: "host", Source: SourceSubfinder, Value: "slow.example.com"},
		}}, Role: RoleDiscovery, Timeout: 0},
	}
	cache := NewCache()
	engine := NewEngine(cache, DefaultCommandRunner{}, providers)

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	_, err := engine.Run(ctx, "example.com")
	if err == nil {
		t.Fatal("expected cancellation error")
	}
}

func TestEngine_ProviderErrorNonFatal(t *testing.T) {
	providers := []ProviderMetadata{
		{Provider: &mockProvider{name: "fail", delay: 10 * time.Millisecond, err: context.DeadlineExceeded}, Role: RoleDiscovery, Timeout: 1},
		{Provider: &mockProvider{name: "ok", delay: 10 * time.Millisecond, findings: []ReconFinding{
			{Type: "host", Source: SourceGAU, Value: "ok.example.com"},
		}}, Role: RoleDiscovery, Timeout: 5},
	}
	cache := NewCache()
	engine := NewEngine(cache, DefaultCommandRunner{}, providers)

	summary, err := engine.Run(context.Background(), "example.com")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if summary.HostCount() != 1 {
		t.Fatalf("host count = %d, want 1 (failed provider should be non-fatal)", summary.HostCount())
	}
}

func TestEngine_AllProvidersFailReturnsDiagnostics(t *testing.T) {
	cache := NewCache()
	engine := NewEngine(cache, DefaultCommandRunner{}, []ProviderMetadata{
		{Provider: &mockProvider{name: "missing-a", err: context.DeadlineExceeded}, Role: RoleDiscovery},
		{Provider: &mockProvider{name: "missing-b", err: context.Canceled}, Role: RoleEnrichment},
	})

	summary, err := engine.Run(context.Background(), "example.com")
	if err == nil {
		t.Fatal("expected all-provider failure")
	}
	if summary == nil {
		t.Fatal("expected summary with provider diagnostics")
	}
	if len(summary.Providers) != 2 {
		t.Fatalf("provider statuses = %d, want 2", len(summary.Providers))
	}
	if !strings.Contains(err.Error(), "missing-a") || !strings.Contains(err.Error(), "missing-b") {
		t.Fatalf("error does not name failed providers: %v", err)
	}
	if cache.Size() != 0 {
		t.Fatal("all-failed result must not be cached")
	}
}

func TestEngine_RunAsync(t *testing.T) {
	providers := []ProviderMetadata{
		{Provider: &mockProvider{name: "p1", delay: 10 * time.Millisecond, findings: []ReconFinding{
			{Type: "host", Source: SourceSubfinder, Value: "async.example.com"},
		}}, Role: RoleDiscovery, Timeout: 5},
	}
	cache := NewCache()
	engine := NewEngine(cache, DefaultCommandRunner{}, providers)

	resultCh := engine.RunAsync(context.Background(), "example.com")
	select {
	case result := <-resultCh:
		if result.Err != nil {
			t.Fatalf("async run: %v", result.Err)
		}
		if result.Summary.HostCount() != 1 {
			t.Fatalf("async host count = %d", result.Summary.HostCount())
		}
	case <-time.After(5 * time.Second):
		t.Fatal("async run timed out")
	}
}

// --- ReconSummary Tests ---

func TestReconSummary_SortedEndpoints(t *testing.T) {
	s := &ReconSummary{
		Endpoints: []Endpoint{
			{URL: "http://example.com/health", Score: 10},
			{URL: "http://example.com/admin", Score: 100},
			{URL: "http://example.com/api", Score: 50},
		},
	}
	sorted := s.SortedEndpoints()
	if sorted[0].URL != "http://example.com/admin" {
		t.Fatalf("first sorted endpoint = %s, want admin", sorted[0].URL)
	}
	if sorted[1].URL != "http://example.com/api" {
		t.Fatalf("second sorted endpoint = %s, want api", sorted[1].URL)
	}
}

func TestReconSummary_InterestingEndpoints(t *testing.T) {
	s := &ReconSummary{
		Endpoints: []Endpoint{
			{URL: "http://example.com/admin", Score: 100},
			{URL: "http://example.com/page", Score: 0},
			{URL: "http://example.com/api", Score: 50},
		},
	}
	interesting := s.InterestingEndpoints()
	if len(interesting) != 2 {
		t.Fatalf("interesting count = %d, want 2", len(interesting))
	}
}

func TestFormatSummary(t *testing.T) {
	s := &ReconSummary{
		Hosts:        []Host{{Hostname: "a.example.com"}},
		Endpoints:    []Endpoint{{URL: "http://example.com/admin", Score: 100}},
		Technologies: []Technology{{Name: "Apache", Host: "example.com"}},
	}
	got := FormatSummary(s)
	want := "1 hosts, 1 endpoints (1 interesting), 1 technologies, 0 vulnerabilities"
	if got != want {
		t.Fatalf("FormatSummary = %q, want %q", got, want)
	}
}
