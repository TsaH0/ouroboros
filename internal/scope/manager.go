package scope

import (
	"context"
	"fmt"
	"net/url"
	"sync"
	"sync/atomic"
	"time"

	"github.com/oklog/ulid/v2"

	"ouroboros/internal/model"
)

// Manager is a thread-safe scope rule manager backed by a RuleStore.
// It maintains a compiled Matcher that is atomically swapped on every mutation.
type Manager struct {
	mu      sync.Mutex
	store   RuleStore
	rules   []Rule
	matcher atomic.Pointer[Matcher]
}

// NewManager creates a Manager and loads rules from the store.
// If the store is nil, the manager starts with an empty rule set.
func NewManager(store RuleStore) *Manager {
	m := &Manager{store: store}
	m.rebuild()
	return m
}

// Load reloads rules from the store and rebuilds the matcher.
func (m *Manager) Load(ctx context.Context) error {
	if m.store == nil {
		return nil
	}
	rules, err := m.store.LoadScopeRules(ctx)
	if err != nil {
		return fmt.Errorf("load scope rules: %w", err)
	}
	m.mu.Lock()
	m.rules = rules
	m.mu.Unlock()
	m.rebuild()
	return nil
}

// Rules returns a sorted copy of the current rules.
func (m *Manager) Rules() []Rule {
	m.mu.Lock()
	defer m.mu.Unlock()
	r := make([]Rule, len(m.rules))
	copy(r, m.rules)
	return r
}

// AddRule adds a rule, assigns an ID, persists it, and rebuilds the matcher.
func (m *Manager) AddRule(ctx context.Context, r Rule) (Rule, error) {
	if err := validateRule(r); err != nil {
		return r, err
	}
	now := time.Now()
	r.ID = ulid.MustNewDefault(now).String()
	r.CreatedAt = now
	r.UpdatedAt = now

	if m.store != nil {
		if err := m.store.SaveScopeRule(ctx, &r); err != nil {
			return r, fmt.Errorf("save rule: %w", err)
		}
	}

	m.mu.Lock()
	m.rules = append(m.rules, r)
	m.mu.Unlock()
	m.rebuild()
	return r, nil
}

// DeleteRule removes a rule by ID, persists the deletion, and rebuilds.
func (m *Manager) DeleteRule(ctx context.Context, id string) error {
	if m.store != nil {
		if err := m.store.DeleteScopeRule(ctx, id); err != nil {
			return fmt.Errorf("delete rule: %w", err)
		}
	}

	m.mu.Lock()
	filtered := make([]Rule, 0, len(m.rules))
	for _, r := range m.rules {
		if r.ID != id {
			filtered = append(filtered, r)
		}
	}
	m.rules = filtered
	m.mu.Unlock()
	m.rebuild()
	return nil
}

// SetEnabled toggles a rule's enabled state, persists, and rebuilds.
func (m *Manager) SetEnabled(ctx context.Context, id string, enabled bool) error {
	m.mu.Lock()
	var found bool
	for i := range m.rules {
		if m.rules[i].ID == id {
			m.rules[i].Enabled = enabled
			m.rules[i].UpdatedAt = time.Now()
			found = true
			break
		}
	}
	rules := make([]Rule, len(m.rules))
	copy(rules, m.rules)
	m.mu.Unlock()

	if !found {
		return fmt.Errorf("rule %s not found", id)
	}

	if m.store != nil {
		// Persist the updated rule.
		for i := range rules {
			if rules[i].ID == id {
				if err := m.store.SaveScopeRule(ctx, &rules[i]); err != nil {
					return fmt.Errorf("save rule: %w", err)
				}
				break
			}
		}
	}

	m.mu.Lock()
	m.rules = rules
	m.mu.Unlock()
	m.rebuild()
	return nil
}

// ReplaceRules replaces all scope rules in-memory (no store I/O).
// Used by project load to swap the active ruleset.
func (m *Manager) ReplaceRules(rules []Rule) {
	m.mu.Lock()
	m.rules = make([]Rule, len(rules))
	copy(m.rules, rules)
	m.mu.Unlock()
	m.rebuild()
}

// Evaluate returns true if the URL is explicitly in scope.
func (m *Manager) Evaluate(u *url.URL) bool {
	matcher := m.matcher.Load()
	if matcher == nil {
		return false
	}
	return matcher.Evaluate(u)
}

// Status returns the tri-state scope status for a URL.
func (m *Manager) Status(u *url.URL) model.ScopeStatus {
	matcher := m.matcher.Load()
	if matcher == nil {
		return StatusUnknown
	}
	return matcher.Status(u)
}

// HostStatus returns the scope status for a bare hostname.
func (m *Manager) HostStatus(host string) model.ScopeStatus {
	u := &url.URL{Scheme: "https", Host: host, Path: "/"}
	return m.Status(u)
}

// rebuild atomically replaces the compiled matcher.
func (m *Manager) rebuild() {
	m.mu.Lock()
	rules := make([]Rule, len(m.rules))
	copy(rules, m.rules)
	m.mu.Unlock()

	matcher, err := NewMatcher(rules)
	if err != nil {
		// A rule that was previously valid should not fail now.
		// If it does, log and keep the old matcher.
		return
	}
	m.matcher.Store(matcher)
}

// validateRule checks that a rule has valid fields.
func validateRule(r Rule) error {
	if r.Pattern == "" {
		return fmt.Errorf("pattern is required")
	}
	switch r.Kind {
	case RuleKindHost, RuleKindPath, RuleKindURL:
	default:
		return fmt.Errorf("invalid rule kind: %s", r.Kind)
	}
	switch r.MatchMode {
	case MatchModeLiteral, MatchModeWildcard, MatchModeRegex:
	default:
		return fmt.Errorf("invalid match mode: %s", r.MatchMode)
	}
	switch r.Action {
	case ActionInclude, ActionExclude:
	default:
		return fmt.Errorf("invalid action: %s", r.Action)
	}
	return nil
}
