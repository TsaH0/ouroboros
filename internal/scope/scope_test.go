package scope

import (
	"net/url"
	"regexp"
	"testing"
)

func mustURL(s string) *url.URL {
	u, err := url.Parse(s)
	if err != nil {
		panic(err)
	}
	return u
}

func TestMatcher_ExactAllow(t *testing.T) {
	m := NewMatcher([]Rule{
		{Allow: true, Host: regexp.MustCompile(`^example\.com$`)},
	})

	if !m.Evaluate(mustURL("https://example.com/api")) {
		t.Error("expected allow for example.com")
	}
	if m.Evaluate(mustURL("https://evil.com/api")) {
		t.Error("expected deny for evil.com")
	}
}

func TestMatcher_DenyPrecedence(t *testing.T) {
	m := NewMatcher([]Rule{
		{Allow: false, Host: regexp.MustCompile(`^evil\.com$`)},
		{Allow: true, Host: regexp.MustCompile(`.*`)},
	})

	if m.Evaluate(mustURL("https://evil.com/api")) {
		t.Error("expected deny for evil.com (first rule)")
	}
	if !m.Evaluate(mustURL("https://good.com/api")) {
		t.Error("expected allow for good.com (fallthrough)")
	}
}

func TestMatcher_SubdomainRegex(t *testing.T) {
	m := NewMatcher([]Rule{
		{Allow: true, Host: regexp.MustCompile(`.*\.example\.com$`)},
	})

	if !m.Evaluate(mustURL("https://api.example.com/")) {
		t.Error("expected allow for api.example.com")
	}
	if !m.Evaluate(mustURL("https://deep.nested.example.com/")) {
		t.Error("expected allow for deep.nested.example.com")
	}
	if m.Evaluate(mustURL("https://example.org/")) {
		t.Error("expected deny for example.org")
	}
}

func TestMatcher_PortMatching(t *testing.T) {
	m := NewMatcher([]Rule{
		{Allow: true, Host: regexp.MustCompile(`.*`), Port: 8080},
	})

	if !m.Evaluate(mustURL("http://localhost:8080/")) {
		t.Error("expected allow for port 8080")
	}
	if m.Evaluate(mustURL("http://localhost:9090/")) {
		t.Error("expected deny for port 9090")
	}
}

func TestMatcher_SchemeMatching(t *testing.T) {
	m := NewMatcher([]Rule{
		{Allow: true, Host: regexp.MustCompile(`.*`), Scheme: "https"},
	})

	if !m.Evaluate(mustURL("https://example.com/")) {
		t.Error("expected allow for https")
	}
	if m.Evaluate(mustURL("http://example.com/")) {
		t.Error("expected deny for http")
	}
}

func TestMatcher_PathMatching(t *testing.T) {
	m := NewMatcher([]Rule{
		{Allow: true, Host: regexp.MustCompile(`.*`), Path: regexp.MustCompile(`^/api/`)},
	})

	if !m.Evaluate(mustURL("https://example.com/api/users")) {
		t.Error("expected allow for /api/ path")
	}
	if m.Evaluate(mustURL("https://example.com/static/style.css")) {
		t.Error("expected deny for non-/api/ path")
	}
}

func TestMatcher_DefaultDeny(t *testing.T) {
	m := NewMatcher(nil) // no rules

	if m.Evaluate(mustURL("https://example.com/")) {
		t.Error("expected default deny when no rules match")
	}
}
