package scope

import (
	"context"
	"time"

	"github.com/TsaH0/ouroboros/internal/model"
)

// RuleKind classifies what part of a URL the rule matches against.
type RuleKind string

const (
	RuleKindHost RuleKind = "host" // pattern matches the hostname
	RuleKindPath RuleKind = "path" // pattern matches the URL path
	RuleKindURL  RuleKind = "url"  // pattern matches the full URL string
)

// MatchMode selects how the pattern is interpreted.
type MatchMode string

const (
	MatchModeLiteral  MatchMode = "literal"  // exact match (case-insensitive for host)
	MatchModeWildcard MatchMode = "wildcard"  // glob-style: * matches any sequence, ? matches one char
	MatchModeRegex    MatchMode = "regex"     // full regular expression (anchored)
)

// Action is the rule's effect when it matches.
type Action string

const (
	ActionInclude Action = "include" // matching → in scope
	ActionExclude Action = "exclude" // matching → out of scope
)

// Rule is a single scope rule. It is persisted in SQLite.
type Rule struct {
	ID        string    `json:"id"`
	PresetID  string    `json:"preset_id"` // empty = belongs to the Default preset
	Kind      RuleKind  `json:"kind"`
	Pattern   string    `json:"pattern"`
	MatchMode MatchMode `json:"match_mode"`
	Action    Action    `json:"action"`
	Enabled   bool      `json:"enabled"`
	Priority  int       `json:"priority"`
	Note      string    `json:"note"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// Preset is a named collection of scope rules (like a Caido Scope Preset).
// Multiple presets can exist; exactly one is "active" at a time in the Manager.
type Preset struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// Status is a convenience alias for the tri-state scope status.
type Status = model.ScopeStatus

const (
	StatusInScope    Status = model.ScopeInScope
	StatusOutOfScope Status = model.ScopeOutOfScope
	StatusUnknown    Status = model.ScopeUnknown
)

// RuleStore is the persistence interface consumed by Manager for rules.
type RuleStore interface {
	LoadScopeRules(ctx context.Context) ([]Rule, error)
	SaveScopeRule(ctx context.Context, rule *Rule) error
	DeleteScopeRule(ctx context.Context, id string) error
}

// PresetStore is the persistence interface for named scope presets.
type PresetStore interface {
	ListScopePresets(ctx context.Context) ([]Preset, error)
	SaveScopePreset(ctx context.Context, p *Preset) error
	DeleteScopePreset(ctx context.Context, id string) error
	// LoadScopeRulesForPreset returns all rules belonging to the given preset ID.
	// An empty presetID returns rules with no preset (legacy / Default preset rules).
	LoadScopeRulesForPreset(ctx context.Context, presetID string) ([]Rule, error)
}
