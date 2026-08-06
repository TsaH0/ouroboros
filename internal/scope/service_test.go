package scope

import (
	"net/url"
	"testing"
)

func mustURL(s string) *url.URL {
	u, err := url.Parse(s)
	if err != nil {
		panic(err)
	}
	return u
}

func TestMatcher_LiteralHostInclude(t *testing.T) {
	m, err := NewMatcher([]Rule{
		{Kind: RuleKindHost, Pattern: "example.com", MatchMode: MatchModeLiteral, Action: ActionInclude, Enabled: true, Priority: 10},
	})
	if err != nil {
		t.Fatalf("NewMatcher: %v", err)
	}

	if st := m.Status(mustURL("https://example.com/api")); st != StatusInScope {
		t.Errorf("expected in-scope for example.com, got %s", st)
	}
	if st := m.Status(mustURL("https://evil.com/api")); st != StatusUnknown {
		t.Errorf("expected unknown for evil.com, got %s", st)
	}
}

func TestMatcher_WildcardHost(t *testing.T) {
	m, err := NewMatcher([]Rule{
		{Kind: RuleKindHost, Pattern: "*.example.com", MatchMode: MatchModeWildcard, Action: ActionInclude, Enabled: true, Priority: 10},
	})
	if err != nil {
		t.Fatalf("NewMatcher: %v", err)
	}

	if st := m.Status(mustURL("https://api.example.com/path")); st != StatusInScope {
		t.Errorf("expected in-scope for api.example.com, got %s", st)
	}
	if st := m.Status(mustURL("https://example.com/path")); st != StatusInScope {
		t.Errorf("expected in-scope for example.com (bare domain), got %s", st)
	}
	if st := m.Status(mustURL("https://evil.com/path")); st != StatusUnknown {
		t.Errorf("expected unknown for evil.com, got %s", st)
	}
}

func TestMatcher_RegexHost(t *testing.T) {
	m, err := NewMatcher([]Rule{
		{Kind: RuleKindHost, Pattern: `.*\.corp\.example\.com`, MatchMode: MatchModeRegex, Action: ActionInclude, Enabled: true, Priority: 10},
	})
	if err != nil {
		t.Fatalf("NewMatcher: %v", err)
	}

	if st := m.Status(mustURL("https://api.corp.example.com/")); st != StatusInScope {
		t.Errorf("expected in-scope for api.corp.example.com, got %s", st)
	}
	if st := m.Status(mustURL("https://evil.com/")); st != StatusUnknown {
		t.Errorf("expected unknown for evil.com, got %s", st)
	}
}

func TestMatcher_PathRule(t *testing.T) {
	m, err := NewMatcher([]Rule{
		{Kind: RuleKindPath, Pattern: "/admin/*", MatchMode: MatchModeWildcard, Action: ActionInclude, Enabled: true, Priority: 10},
	})
	if err != nil {
		t.Fatalf("NewMatcher: %v", err)
	}

	if st := m.Status(mustURL("https://example.com/admin/users")); st != StatusInScope {
		t.Errorf("expected in-scope for /admin/users, got %s", st)
	}
	if st := m.Status(mustURL("https://example.com/api")); st != StatusUnknown {
		t.Errorf("expected unknown for /api, got %s", st)
	}
}

func TestMatcher_ExcludeOverridesInclude(t *testing.T) {
	m, err := NewMatcher([]Rule{
		{Kind: RuleKindHost, Pattern: "*.example.com", MatchMode: MatchModeWildcard, Action: ActionInclude, Enabled: true, Priority: 10},
		{Kind: RuleKindHost, Pattern: "evil.example.com", MatchMode: MatchModeLiteral, Action: ActionExclude, Enabled: true, Priority: 10},
	})
	if err != nil {
		t.Fatalf("NewMatcher: %v", err)
	}

	if st := m.Status(mustURL("https://good.example.com/")); st != StatusInScope {
		t.Errorf("expected in-scope for good.example.com, got %s", st)
	}
	if st := m.Status(mustURL("https://evil.example.com/")); st != StatusOutOfScope {
		t.Errorf("expected out-of-scope for evil.example.com, got %s", st)
	}
}

func TestMatcher_PriorityOrder(t *testing.T) {
	m, err := NewMatcher([]Rule{
		{Kind: RuleKindHost, Pattern: "*.example.com", MatchMode: MatchModeWildcard, Action: ActionExclude, Enabled: true, Priority: 5},
		{Kind: RuleKindHost, Pattern: "api.example.com", MatchMode: MatchModeLiteral, Action: ActionInclude, Enabled: true, Priority: 10},
	})
	if err != nil {
		t.Fatalf("NewMatcher: %v", err)
	}

	// Higher priority (10) wins: api.example.com is included despite the wildcard exclude.
	if st := m.Status(mustURL("https://api.example.com/")); st != StatusInScope {
		t.Errorf("expected in-scope for api.example.com (priority 10 include), got %s", st)
	}
	// Other subdomains match the exclude (priority 5).
	if st := m.Status(mustURL("https://other.example.com/")); st != StatusOutOfScope {
		t.Errorf("expected out-of-scope for other.example.com, got %s", st)
	}
}

func TestMatcher_DisabledRuleSkipped(t *testing.T) {
	m, err := NewMatcher([]Rule{
		{Kind: RuleKindHost, Pattern: "*.example.com", MatchMode: MatchModeWildcard, Action: ActionInclude, Enabled: false, Priority: 10},
	})
	if err != nil {
		t.Fatalf("NewMatcher: %v", err)
	}

	if st := m.Status(mustURL("https://api.example.com/")); st != StatusUnknown {
		t.Errorf("expected unknown for disabled rule, got %s", st)
	}
}

func TestMatcher_DefaultUnknown(t *testing.T) {
	m, err := NewMatcher(nil)
	if err != nil {
		t.Fatalf("NewMatcher: %v", err)
	}

	if st := m.Status(mustURL("https://example.com/")); st != StatusUnknown {
		t.Errorf("expected unknown with no rules, got %s", st)
	}
}

func TestMatcher_EvaluateBool(t *testing.T) {
	m, err := NewMatcher([]Rule{
		{Kind: RuleKindHost, Pattern: "example.com", MatchMode: MatchModeLiteral, Action: ActionInclude, Enabled: true, Priority: 10},
	})
	if err != nil {
		t.Fatalf("NewMatcher: %v", err)
	}

	if !m.Evaluate(mustURL("https://example.com/api")) {
		t.Error("expected Evaluate true for in-scope")
	}
	if m.Evaluate(mustURL("https://evil.com/api")) {
		t.Error("expected Evaluate false for unknown")
	}
}

func TestMatcher_HostStatus(t *testing.T) {
	m, err := NewMatcher([]Rule{
		{Kind: RuleKindHost, Pattern: "example.com", MatchMode: MatchModeLiteral, Action: ActionInclude, Enabled: true, Priority: 10},
	})
	if err != nil {
		t.Fatalf("NewMatcher: %v", err)
	}

	if st := m.HostStatus("example.com"); st != StatusInScope {
		t.Errorf("expected in-scope for example.com, got %s", st)
	}
	if st := m.HostStatus("evil.com"); st != StatusUnknown {
		t.Errorf("expected unknown for evil.com, got %s", st)
	}
}

func TestMatcher_URLLiteral(t *testing.T) {
	m, err := NewMatcher([]Rule{
		{Kind: RuleKindURL, Pattern: `https://example\.com/admin`, MatchMode: MatchModeRegex, Action: ActionInclude, Enabled: true, Priority: 10},
	})
	if err != nil {
		t.Fatalf("NewMatcher: %v", err)
	}

	if st := m.Status(mustURL("https://example.com/admin")); st != StatusInScope {
		t.Errorf("expected in-scope for exact URL, got %s", st)
	}
	if st := m.Status(mustURL("https://example.com/admin/users")); st != StatusUnknown {
		t.Errorf("expected unknown for different path, got %s", st)
	}
}
