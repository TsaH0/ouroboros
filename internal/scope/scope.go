package scope

import (
	"net/url"
	"regexp"
)

// Rule defines a single scope matching rule.
type Rule struct {
	Allow bool
	// Scheme is an optional exact match on the URL scheme (e.g. "https").
	Scheme string
	// Host is an optional regex matching the hostname.
	Host *regexp.Regexp
	// Port is an optional exact port match. 0 means any port.
	Port int
	// Path is an optional regex matching the URL path.
	Path *regexp.Regexp
}

// Service evaluates whether a URL is within the authorized scope.
type Service interface {
	Evaluate(u *url.URL) bool
}

// Matcher implements Service by iterating through an ordered list of rules.
// The first matching rule determines the result. If no rule matches, the
// default is deny (false).
type Matcher struct {
	rules []Rule
}

func NewMatcher(rules []Rule) *Matcher {
	return &Matcher{rules: rules}
}

func (m *Matcher) Evaluate(u *url.URL) bool {
	for _, r := range m.rules {
		if r.Scheme != "" && r.Scheme != u.Scheme {
			continue
		}
		if r.Host != nil && !r.Host.MatchString(u.Hostname()) {
			continue
		}
		if r.Port != 0 {
			p := u.Port()
			if p == "" {
				// Default port for scheme.
				if (u.Scheme == "https" && r.Port != 443) ||
					(u.Scheme == "http" && r.Port != 80) {
					continue
				}
			} else if p != itoa(r.Port) {
				continue
			}
		}
		if r.Path != nil && !r.Path.MatchString(u.Path) {
			continue
		}
		return r.Allow
	}
	return false
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	return string(buf[i:])
}
