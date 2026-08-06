package intercept

import (
	"regexp"
	"testing"

	"ouroboros/internal/model"
)

func TestMatcher_NoRules(t *testing.T) {
	m := NewMatcher(nil)
	flow := &model.Flow{Host: "example.com", Request: &model.Message{Method: "GET", URL: "http://example.com/"}}
	if m.Evaluate(flow) {
		t.Error("expected false with no rules")
	}
}

func TestMatcher_HostMatch(t *testing.T) {
	m := NewMatcher([]Rule{
		{Allow: true, Host: regexp.MustCompile(`example\.com`)},
	})
	flow := &model.Flow{Host: "example.com", Request: &model.Message{Method: "GET", URL: "http://example.com/"}}
	if !m.Evaluate(flow) {
		t.Error("expected true for matching host")
	}
	flow2 := &model.Flow{Host: "other.com", Request: &model.Message{Method: "GET", URL: "http://other.com/"}}
	if m.Evaluate(flow2) {
		t.Error("expected false for non-matching host")
	}
}

func TestMatcher_MethodMatch(t *testing.T) {
	m := NewMatcher([]Rule{
		{Allow: true, Method: "POST"},
	})
	flow := &model.Flow{Host: "example.com", Request: &model.Message{Method: "POST", URL: "http://example.com/"}}
	if !m.Evaluate(flow) {
		t.Error("expected true for POST")
	}
	flow2 := &model.Flow{Host: "example.com", Request: &model.Message{Method: "GET", URL: "http://example.com/"}}
	if m.Evaluate(flow2) {
		t.Error("expected false for GET")
	}
}

func TestMatcher_PathMatch(t *testing.T) {
	m := NewMatcher([]Rule{
		{Allow: true, Path: regexp.MustCompile(`/api/`)},
	})
	flow := &model.Flow{Host: "example.com", Request: &model.Message{Method: "GET", URL: "http://example.com/api/users"}}
	if !m.Evaluate(flow) {
		t.Error("expected true for /api/ path")
	}
	flow2 := &model.Flow{Host: "example.com", Request: &model.Message{Method: "GET", URL: "http://example.com/static/style.css"}}
	if m.Evaluate(flow2) {
		t.Error("expected false for non-/api/ path")
	}
}

func TestMatcher_FirstRuleWins(t *testing.T) {
	m := NewMatcher([]Rule{
		{Allow: true, Host: regexp.MustCompile(`example\.com`)},
		{Allow: false, Host: regexp.MustCompile(`.*`)},
	})
	flow := &model.Flow{Host: "example.com", Request: &model.Message{Method: "GET", URL: "http://example.com/"}}
	if !m.Evaluate(flow) {
		t.Error("expected first rule to match (allow)")
	}
	flow2 := &model.Flow{Host: "other.com", Request: &model.Message{Method: "GET", URL: "http://other.com/"}}
	if m.Evaluate(flow2) {
		t.Error("expected second rule to match (deny)")
	}
}
