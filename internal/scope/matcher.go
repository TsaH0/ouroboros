package scope

import (
	"fmt"
	"net/url"
	"regexp"
	"sort"
	"strings"
)

// compiledRule is a rule ready for matching.
type compiledRule struct {
	rule    Rule
	matcher *regexp.Regexp // compiled pattern, anchored
}

// Matcher evaluates URLs against a set of compiled rules.
// It is immutable after construction and safe for concurrent use.
type Matcher struct {
	rules []compiledRule
}

// NewMatcher compiles rules and returns a Matcher.
// Rules are sorted by descending priority (higher = evaluated first).
// Ties are broken by exclude-first, then by creation order.
func NewMatcher(rules []Rule) (*Matcher, error) {
	compiled := make([]compiledRule, 0, len(rules))
	for _, r := range rules {
		if !r.Enabled {
			continue
		}
		re, err := compilePattern(r.Kind, r.Pattern, r.MatchMode)
		if err != nil {
			return nil, fmt.Errorf("rule %s: %w", r.ID, err)
		}
		compiled = append(compiled, compiledRule{rule: r, matcher: re})
	}
	sort.Slice(compiled, func(i, j int) bool {
		pi, pj := compiled[i].rule.Priority, compiled[j].rule.Priority
		if pi != pj {
			return pi > pj // higher priority first
		}
		// Exclude beats include at the same priority (safety).
		ai, aj := compiled[i].rule.Action, compiled[j].rule.Action
		if ai != aj {
			return ai == ActionExclude
		}
		return compiled[i].rule.CreatedAt.Before(compiled[j].rule.CreatedAt)
	})
	return &Matcher{rules: compiled}, nil
}

// Evaluate returns true if the URL is explicitly in scope.
// It implements the legacy Service interface for the proxy.
func (m *Matcher) Evaluate(u *url.URL) bool {
	return m.Status(u) == StatusInScope
}

// Status returns the tri-state scope status for a URL.
func (m *Matcher) Status(u *url.URL) Status {
	for _, cr := range m.rules {
		if matchRule(cr, u) {
			if cr.rule.Action == ActionInclude {
				return StatusInScope
			}
			return StatusOutOfScope
		}
	}
	return StatusUnknown
}

// HostStatus returns the scope status for a bare hostname.
// Path rules are ignored (no path to match).
func (m *Matcher) HostStatus(host string) Status {
	u := &url.URL{Scheme: "https", Host: host, Path: "/"}
	return m.Status(u)
}

// compilePattern converts a rule pattern into an anchored regexp.
func compilePattern(kind RuleKind, pattern string, mode MatchMode) (*regexp.Regexp, error) {
	var rePattern string
	switch mode {
	case MatchModeLiteral:
		rePattern = regexp.QuoteMeta(pattern)
	case MatchModeWildcard:
		rePattern = wildcardToRegex(pattern)
	case MatchModeRegex:
		rePattern = pattern
	default:
		return nil, fmt.Errorf("unknown match mode: %s", mode)
	}

	switch kind {
	case RuleKindHost:
		rePattern = "^(?i:" + rePattern + ")$"
	case RuleKindPath:
		rePattern = "^(?:" + rePattern + ")$"
	case RuleKindURL:
		rePattern = "^(?:" + rePattern + ")$"
	default:
		return nil, fmt.Errorf("unknown rule kind: %s", kind)
	}

	re, err := regexp.Compile(rePattern)
	if err != nil {
		return nil, fmt.Errorf("compile pattern %q: %w", pattern, err)
	}
	return re, nil
}

// wildcardToRegex converts a glob-style pattern to a regex.
// * matches any sequence of characters, ? matches any single character.
// When the pattern starts with "*." the leading ".*" is wrapped in an optional
// group so that "*.example.com" also matches the bare "example.com".
func wildcardToRegex(pattern string) string {
	if strings.HasPrefix(pattern, "*.") {
		body := regexp.QuoteMeta(pattern[2:])
		return "(.*\\." + body + "|" + body + ")"
	}
	var b strings.Builder
	for _, c := range pattern {
		switch c {
		case '*':
			b.WriteString(".*")
		case '?':
			b.WriteString(".")
		default:
			b.WriteString(regexp.QuoteMeta(string(c)))
		}
	}
	return b.String()
}

// matchRule checks whether a compiled rule matches a URL.
func matchRule(cr compiledRule, u *url.URL) bool {
	var target string
	switch cr.rule.Kind {
	case RuleKindHost:
		target = u.Hostname()
	case RuleKindPath:
		target = u.Path
	case RuleKindURL:
		target = u.String()
	default:
		return false
	}
	return cr.matcher.MatchString(target)
}
