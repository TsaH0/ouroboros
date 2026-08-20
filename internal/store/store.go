package store

import (
	"context"

	"ouroboros/internal/model"
	"ouroboros/internal/recon"
	"ouroboros/internal/scope"
)

// Store is the persistence abstraction for Ouroboros.
// Implementations: MemoryStore, SQLiteStore.
type Store interface {
	// --- Flows ---
	SaveFlow(ctx context.Context, f *model.Flow) error
	GetFlow(ctx context.Context, id string) (*model.Flow, error)
	ListFlows(ctx context.Context) ([]*model.Flow, error)
	DeleteFlow(ctx context.Context, id string) error
	ClearFlows(ctx context.Context) error

	// --- Scope rules ---
	LoadScopeRules(ctx context.Context) ([]scope.Rule, error)
	SaveScopeRule(ctx context.Context, rule *scope.Rule) error
	DeleteScopeRule(ctx context.Context, id string) error

	// --- Scope presets (named Caido-style presets) ---
	ListScopePresets(ctx context.Context) ([]scope.Preset, error)
	SaveScopePreset(ctx context.Context, p *scope.Preset) error
	DeleteScopePreset(ctx context.Context, id string) error
	LoadScopeRulesForPreset(ctx context.Context, presetID string) ([]scope.Rule, error)

	// --- Recon ---
	SaveRecon(ctx context.Context, s *recon.ReconSummary) error
	ListRecon(ctx context.Context) ([]*recon.ReconSummary, error)
	GetRecon(ctx context.Context, target string) (*recon.ReconSummary, error)

	// --- Technologies ---
	SaveTechnology(ctx context.Context, runID string, t recon.Technology) error

	// --- Vulnerabilities ---
	SaveVulnerability(ctx context.Context, runID string, v recon.Vulnerability) error

	// --- Analyses ---
	SaveAnalysis(ctx context.Context, a *model.Analysis) error
	ListAnalyses(ctx context.Context, flowID string) ([]*model.Analysis, error)

	// --- Settings ---
	SaveSetting(ctx context.Context, key, value string) error
	GetSetting(ctx context.Context, key string) (string, error)

	Close() error
}
