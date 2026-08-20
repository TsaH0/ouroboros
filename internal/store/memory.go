package store

import (
	"context"
	"sync"
	"time"

	"github.com/oklog/ulid/v2"

	"github.com/TsaH0/ouroboros/internal/model"
	"github.com/TsaH0/ouroboros/internal/recon"
	"github.com/TsaH0/ouroboros/internal/scope"
)

// MemoryStore is a concurrency-safe in-memory implementation of Store.
type MemoryStore struct {
	mu    sync.RWMutex
	flows []*model.Flow
	byID  map[string]*model.Flow

	rules   []scope.Rule
	presets []scope.Preset
	recons  map[string]*recon.ReconSummary
	reconBy map[string]*recon.ReconSummary // target → summary
	techs   []recon.Technology
	vulns   []recon.Vulnerability
	analyses []model.Analysis
	settings map[string]string
}

// NewMemoryStore creates a new MemoryStore.
func NewMemoryStore() *MemoryStore {
	return &MemoryStore{
		byID:     make(map[string]*model.Flow),
		recons:   make(map[string]*recon.ReconSummary),
		reconBy:  make(map[string]*recon.ReconSummary),
		settings: make(map[string]string),
	}
}

// --- Flows ---

func (s *MemoryStore) SaveFlow(_ context.Context, f *model.Flow) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.byID[f.ID] = f
	s.flows = append(s.flows, f)
	return nil
}

func (s *MemoryStore) GetFlow(_ context.Context, id string) (*model.Flow, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	f, ok := s.byID[id]
	if !ok {
		return nil, nil
	}
	return f, nil
}

func (s *MemoryStore) ListFlows(_ context.Context) ([]*model.Flow, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	result := make([]*model.Flow, len(s.flows))
	copy(result, s.flows)
	return result, nil
}

func (s *MemoryStore) DeleteFlow(_ context.Context, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.byID, id)
	filtered := make([]*model.Flow, 0, len(s.flows))
	for _, f := range s.flows {
		if f.ID != id {
			filtered = append(filtered, f)
		}
	}
	s.flows = filtered
	return nil
}

func (s *MemoryStore) ClearFlows(_ context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.byID = make(map[string]*model.Flow)
	s.flows = nil
	return nil
}

// --- Scope rules ---

func (s *MemoryStore) LoadScopeRules(_ context.Context) ([]scope.Rule, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	r := make([]scope.Rule, len(s.rules))
	copy(r, s.rules)
	return r, nil
}

func (s *MemoryStore) SaveScopeRule(_ context.Context, rule *scope.Rule) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i, r := range s.rules {
		if r.ID == rule.ID {
			s.rules[i] = *rule
			return nil
		}
	}
	s.rules = append(s.rules, *rule)
	return nil
}

func (s *MemoryStore) DeleteScopeRule(_ context.Context, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	filtered := make([]scope.Rule, 0, len(s.rules))
	for _, r := range s.rules {
		if r.ID != id {
			filtered = append(filtered, r)
		}
	}
	s.rules = filtered
	return nil
}

// --- Scope presets ---

func (s *MemoryStore) ListScopePresets(_ context.Context) ([]scope.Preset, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	p := make([]scope.Preset, len(s.presets))
	copy(p, s.presets)
	return p, nil
}

func (s *MemoryStore) SaveScopePreset(_ context.Context, p *scope.Preset) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i, existing := range s.presets {
		if existing.ID == p.ID {
			s.presets[i] = *p
			return nil
		}
	}
	s.presets = append(s.presets, *p)
	return nil
}

func (s *MemoryStore) DeleteScopePreset(_ context.Context, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	filtered := make([]scope.Preset, 0, len(s.presets))
	for _, p := range s.presets {
		if p.ID != id {
			filtered = append(filtered, p)
		}
	}
	s.presets = filtered
	return nil
}

func (s *MemoryStore) LoadScopeRulesForPreset(_ context.Context, presetID string) ([]scope.Rule, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var result []scope.Rule
	for _, r := range s.rules {
		if r.PresetID == presetID {
			result = append(result, r)
		}
	}
	return result, nil
}

// --- Recon ---

func (s *MemoryStore) SaveRecon(_ context.Context, summary *recon.ReconSummary) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	id := ulid.MustNewDefault(time.Now()).String()
	s.recons[id] = summary
	s.reconBy[summary.Target] = summary
	return nil
}

func (s *MemoryStore) ListRecon(_ context.Context) ([]*recon.ReconSummary, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	result := make([]*recon.ReconSummary, 0, len(s.recons))
	for _, v := range s.recons {
		result = append(result, v)
	}
	return result, nil
}

func (s *MemoryStore) GetRecon(_ context.Context, target string) (*recon.ReconSummary, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	summary, ok := s.reconBy[target]
	if !ok {
		return nil, nil
	}
	return summary, nil
}

// --- Technologies ---

func (s *MemoryStore) SaveTechnology(_ context.Context, runID string, t recon.Technology) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.techs = append(s.techs, t)
	return nil
}

// --- Vulnerabilities ---

func (s *MemoryStore) SaveVulnerability(_ context.Context, runID string, v recon.Vulnerability) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.vulns = append(s.vulns, v)
	return nil
}

// --- Analyses ---

func (s *MemoryStore) SaveAnalysis(_ context.Context, a *model.Analysis) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.analyses = append(s.analyses, *a)
	return nil
}

func (s *MemoryStore) ListAnalyses(_ context.Context, flowID string) ([]*model.Analysis, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var result []*model.Analysis
	for i := range s.analyses {
		if s.analyses[i].FlowID == flowID {
			result = append(result, &s.analyses[i])
		}
	}
	return result, nil
}

// --- Settings ---

func (s *MemoryStore) SaveSetting(_ context.Context, key, value string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.settings[key] = value
	return nil
}

func (s *MemoryStore) GetSetting(_ context.Context, key string) (string, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	v, ok := s.settings[key]
	if !ok {
		return "", nil
	}
	return v, nil
}

// Close is a no-op for the in-memory store.
func (s *MemoryStore) Close() error { return nil }
