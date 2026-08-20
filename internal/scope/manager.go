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
// It also manages named Presets: a set of named rule collections.
// Exactly one preset is "active" at a time; its rules drive the Matcher.
type Manager struct {
	mu             sync.Mutex
	store          RuleStore
	presetStore    PresetStore
	rules          []Rule   // rules for the active preset (or global if none)
	activePresetID string   // empty = global / no preset selected
	presets        []Preset // cached list of all presets
	matcher        atomic.Pointer[Matcher]
}

// NewManager creates a Manager and loads rules from the store.
// If the store also implements PresetStore, preset management is enabled.
func NewManager(store RuleStore) *Manager {
	m := &Manager{store: store}
	if ps, ok := store.(PresetStore); ok {
		m.presetStore = ps
	}
	m.rebuild()
	return m
}

// Load reloads rules (and presets) from the store and rebuilds the matcher.
func (m *Manager) Load(ctx context.Context) error {
	if m.store == nil {
		return nil
	}
	// Load presets first.
	if m.presetStore != nil {
		presets, err := m.presetStore.ListScopePresets(ctx)
		if err != nil {
			return fmt.Errorf("load scope presets: %w", err)
		}
		m.mu.Lock()
		m.presets = presets
		m.mu.Unlock()
	}
	// Load rules: for the active preset if one is set, otherwise all rules.
	if m.activePresetID != "" {
		return m.loadPresetRules(ctx, m.activePresetID)
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

// loadPresetRules fetches rules for a specific preset and rebuilds the matcher.
func (m *Manager) loadPresetRules(ctx context.Context, presetID string) error {
	if m.presetStore == nil {
		return nil
	}
	rules, err := m.presetStore.LoadScopeRulesForPreset(ctx, presetID)
	if err != nil {
		return fmt.Errorf("load rules for preset %s: %w", presetID, err)
	}
	m.mu.Lock()
	m.rules = rules
	m.mu.Unlock()
	m.rebuild()
	return nil
}

// Rules returns a copy of the currently active rules.
func (m *Manager) Rules() []Rule {
	m.mu.Lock()
	defer m.mu.Unlock()
	r := make([]Rule, len(m.rules))
	copy(r, m.rules)
	return r
}

// --- Preset management ---

// ListPresets returns all known presets (cached).
func (m *Manager) ListPresets() []Preset {
	m.mu.Lock()
	defer m.mu.Unlock()
	p := make([]Preset, len(m.presets))
	copy(p, m.presets)
	return p
}

// ActivePresetID returns the active preset's ID (empty = global mode).
func (m *Manager) ActivePresetID() string {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.activePresetID
}

// ActivePresetName returns the active preset name, or "Global" if none.
func (m *Manager) ActivePresetName() string {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.activePresetID == "" {
		return "Global"
	}
	for _, p := range m.presets {
		if p.ID == m.activePresetID {
			return p.Name
		}
	}
	return "Unknown"
}

// CreatePreset creates and persists a new named scope preset.
func (m *Manager) CreatePreset(ctx context.Context, name string) (Preset, error) {
	if name == "" {
		return Preset{}, fmt.Errorf("preset name is required")
	}
	now := time.Now()
	p := Preset{
		ID:        ulid.MustNewDefault(now).String(),
		Name:      name,
		CreatedAt: now,
		UpdatedAt: now,
	}
	if m.presetStore != nil {
		if err := m.presetStore.SaveScopePreset(ctx, &p); err != nil {
			return p, fmt.Errorf("save preset: %w", err)
		}
	}
	m.mu.Lock()
	m.presets = append(m.presets, p)
	m.mu.Unlock()
	return p, nil
}

// DeletePreset removes a preset and all its rules, switches to global if it was active.
func (m *Manager) DeletePreset(ctx context.Context, id string) error {
	if m.presetStore != nil {
		// Delete rules belonging to this preset first.
		rules, _ := m.presetStore.LoadScopeRulesForPreset(ctx, id)
		for _, r := range rules {
			_ = m.store.DeleteScopeRule(ctx, r.ID)
		}
		if err := m.presetStore.DeleteScopePreset(ctx, id); err != nil {
			return fmt.Errorf("delete preset: %w", err)
		}
	}
	m.mu.Lock()
	filtered := make([]Preset, 0, len(m.presets))
	for _, p := range m.presets {
		if p.ID != id {
			filtered = append(filtered, p)
		}
	}
	m.presets = filtered
	wasActive := m.activePresetID == id
	if wasActive {
		m.activePresetID = ""
		m.rules = nil
	}
	m.mu.Unlock()
	m.rebuild()
	return nil
}

// ActivatePreset switches the active preset and reloads its rules for display.
// This is a display-only change — flow DB records are NOT modified.
// Pass an empty id to return to global (all-rules) mode.
func (m *Manager) ActivatePreset(ctx context.Context, id string) error {
	if id == "" {
		// Global mode: load all rules.
		if m.store != nil {
			rules, err := m.store.LoadScopeRules(ctx)
			if err != nil {
				return fmt.Errorf("load global rules: %w", err)
			}
			m.mu.Lock()
			m.activePresetID = ""
			m.rules = rules
			m.mu.Unlock()
			m.rebuild()
		}
		return nil
	}
	if err := m.loadPresetRules(ctx, id); err != nil {
		return err
	}
	m.mu.Lock()
	m.activePresetID = id
	m.mu.Unlock()
	return nil
}

// AddRule adds a rule to the currently active preset, persists it, and rebuilds.
func (m *Manager) AddRule(ctx context.Context, r Rule) (Rule, error) {
	if err := validateRule(r); err != nil {
		return r, err
	}
	now := time.Now()
	r.ID = ulid.MustNewDefault(now).String()
	r.CreatedAt = now
	r.UpdatedAt = now
	m.mu.Lock()
	r.PresetID = m.activePresetID
	m.mu.Unlock()

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

// AddRuleInMemory adds a rule without persisting to the store.
// Used for session-only scope toggles (s key, i key) so the
// next session starts clean.
func (m *Manager) AddRuleInMemory(r Rule) (Rule, error) {
	if err := validateRule(r); err != nil {
		return r, err
	}
	now := time.Now()
	r.ID = ulid.MustNewDefault(now).String()
	r.CreatedAt = now
	r.UpdatedAt = now

	m.mu.Lock()
	r.PresetID = m.activePresetID
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

// RemoveHostRules deletes all literal host rules matching the given hostname.
// Returns the number of rules removed.
func (m *Manager) RemoveHostRules(ctx context.Context, host string) int {
	var removed int
	m.mu.Lock()
	filtered := make([]Rule, 0, len(m.rules))
	for _, r := range m.rules {
		if r.Kind == RuleKindHost && r.MatchMode == MatchModeLiteral && r.Pattern == host {
			removed++
			if m.store != nil {
				_ = m.store.DeleteScopeRule(ctx, r.ID)
			}
		} else {
			filtered = append(filtered, r)
		}
	}
	m.rules = filtered
	m.mu.Unlock()
	if removed > 0 {
		m.rebuild()
	}
	return removed
}

// RemoveHostRulesInMemory removes literal host rules without persisting the deletion.
// Used for session-only scope toggles.
func (m *Manager) RemoveHostRulesInMemory(host string) int {
	var removed int
	m.mu.Lock()
	filtered := make([]Rule, 0, len(m.rules))
	for _, r := range m.rules {
		if r.Kind == RuleKindHost && r.MatchMode == MatchModeLiteral && r.Pattern == host {
			removed++
		} else {
			filtered = append(filtered, r)
		}
	}
	m.rules = filtered
	m.mu.Unlock()
	if removed > 0 {
		m.rebuild()
	}
	return removed
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
