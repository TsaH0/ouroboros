package intercept

import (
	"regexp"

	"ouroboros/internal/model"
)

// Rule defines a single intercept matching rule.
type Rule struct {
	// Allow when true means intercept matching flows; when false means skip.
	Allow bool
	// Host is an optional regex matching the hostname.
	Host *regexp.Regexp
	// Path is an optional regex matching the URL path.
	Path *regexp.Regexp
	// Method is an optional exact match on the HTTP method (e.g. "POST").
	Method string
}

// Service evaluates whether a flow should be intercepted.
type Service interface {
	Evaluate(flow *model.Flow) bool
}

// Matcher implements Service by iterating through an ordered list of rules.
// The first matching rule determines the result. If no rule matches, the
// default is not to intercept (false).
type Matcher struct {
	rules []Rule
}

func NewMatcher(rules []Rule) *Matcher {
	return &Matcher{rules: rules}
}

func (m *Matcher) Evaluate(flow *model.Flow) bool {
	for _, r := range m.rules {
		if r.Method != "" && r.Method != flow.Request.Method {
			continue
		}
		if r.Host != nil && !r.Host.MatchString(flow.Host) {
			continue
		}
		if r.Path != nil && !r.Path.MatchString(flow.Request.URL) {
			continue
		}
		return r.Allow
	}
	return false
}
