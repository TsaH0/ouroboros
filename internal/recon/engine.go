package recon

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"
)

// ProgressUpdate is sent to the UI to show provider progress.
type ProgressUpdate struct {
	Provider string
	Status   string // "running", "done", "error"
	Error    error
}

// Engine orchestrates the recon pipeline.
type Engine struct {
	cache          *Cache
	providers      []ProviderMetadata
	runner         CommandRunner
	progressCh     chan ProgressUpdate
	defaultTimeout time.Duration
}

// NewEngine creates a recon engine with the given providers and cache.
func NewEngine(cache *Cache, runner CommandRunner, providers []ProviderMetadata) *Engine {
	return &Engine{
		cache:          cache,
		providers:      providers,
		runner:         runner,
		progressCh:     make(chan ProgressUpdate, 32),
		defaultTimeout: 120 * time.Second,
	}
}

// ProgressChan returns a read-only channel for progress updates.
func (e *Engine) ProgressChan() <-chan ProgressUpdate {
	return e.progressCh
}

// SetDefaultTimeout configures the default per-provider timeout.
func (e *Engine) SetDefaultTimeout(d time.Duration) {
	e.defaultTimeout = d
}

// Run executes the recon pipeline. Independent providers run concurrently;
// summary-aware enrichers run afterward with the normalized initial results.
func (e *Engine) Run(ctx context.Context, target string) (*ReconSummary, error) {
	// Check cache first.
	if cached, ok := e.cache.Get(target); ok {
		return cached, nil
	}

	var (
		mu               sync.Mutex
		allFindings      []ReconFinding
		providerStatuses []ProviderStatus
	)

	runProviders := func(providers []ProviderMetadata) error {
		var wg sync.WaitGroup
		for _, pm := range providers {
			wg.Add(1)
			go func(meta ProviderMetadata) {
				defer wg.Done()

				timeout := e.defaultTimeout
				if meta.Timeout > 0 {
					timeout = time.Duration(meta.Timeout) * time.Second
				}
				providerCtx, cancel := context.WithTimeout(ctx, timeout)
				defer cancel()

				e.progressCh <- ProgressUpdate{Provider: meta.Provider.Name(), Status: "running"}

				findings, err := meta.Provider.Run(providerCtx, target)
				status := ProviderStatus{
					Name:     meta.Provider.Name(),
					Role:     meta.Role,
					Status:   "done",
					Findings: len(findings),
				}
				if err != nil {
					status.Status = "error"
					status.Error = err.Error()
				}

				mu.Lock()
				providerStatuses = append(providerStatuses, status)
				if err == nil {
					allFindings = append(allFindings, findings...)
				}
				mu.Unlock()

				e.progressCh <- ProgressUpdate{
					Provider: meta.Provider.Name(),
					Status:   status.Status,
					Error:    err,
				}
			}(pm)
		}

		doneCh := make(chan struct{})
		go func() {
			wg.Wait()
			close(doneCh)
		}()

		select {
		case <-doneCh:
			return nil
		case <-ctx.Done():
			return ctx.Err()
		}
	}

	var initial, deferred []ProviderMetadata
	for _, pm := range e.providers {
		if _, ok := pm.Provider.(SummaryAwareProvider); ok {
			deferred = append(deferred, pm)
		} else {
			initial = append(initial, pm)
		}
	}

	if err := runProviders(initial); err != nil {
		return nil, err
	}

	initialSummary := normalizeFindings(target, allFindings)
	for _, pm := range deferred {
		pm.Provider.(SummaryAwareProvider).Prepare(initialSummary)
	}
	if err := runProviders(deferred); err != nil {
		return nil, err
	}

	sort.Slice(providerStatuses, func(i, j int) bool {
		return providerStatuses[i].Name < providerStatuses[j].Name
	})

	summary := normalizeFindings(target, allFindings)
	summary.Providers = providerStatuses

	var failures []string
	for _, status := range providerStatuses {
		if status.Status == "error" {
			failures = append(failures, status.Name+": "+status.Error)
		}
	}
	if len(providerStatuses) == 0 {
		return summary, fmt.Errorf("no recon providers configured")
	}
	if len(failures) == len(providerStatuses) {
		return summary, fmt.Errorf("all recon providers failed: %s", strings.Join(failures, "; "))
	}

	e.cache.Set(target, summary)
	return summary, nil
}

// RunAsync starts the pipeline in a goroutine and returns immediately.
// The summary is delivered via the returned channel. Progress updates
// flow through ProgressChan().
func (e *Engine) RunAsync(ctx context.Context, target string) <-chan RunResult {
	resultCh := make(chan RunResult, 1)
	go func() {
		summary, err := e.Run(ctx, target)
		resultCh <- RunResult{Summary: summary, Err: err}
		close(resultCh)
	}()
	return resultCh
}

// RunResult carries the pipeline result.
type RunResult struct {
	Summary *ReconSummary
	Err     error
}

// normalizeFindings converts raw provider findings into a unified summary.
func normalizeFindings(target string, findings []ReconFinding) *ReconSummary {
	var (
		hosts     []Host
		endpoints []Endpoint
		techs     []Technology
		vulns     []Vulnerability
	)

	for _, f := range findings {
		switch f.Type {
		case "host":
			hosts = append(hosts, Host{
				Hostname: f.Value,
				Sources:  []Source{f.Source},
			})
		case "url":
			normURL := NormalizeURL(f.Value)
			if normURL == "" {
				continue
			}
			host := extractHost(normURL)
			path := extractPath(normURL)
			cat, score := ClassifyEndpoint(normURL)
			endpoints = append(endpoints, Endpoint{
				URL:      normURL,
				Host:     host,
				Path:     path,
				Category: cat,
				Score:    score,
				Sources:  []Source{f.Source},
			})
		case "technology":
			techs = append(techs, Technology{
				Name:       f.Technology,
				Version:    f.Version,
				Confidence: f.Confidence,
				Host:       f.Value,
				Source:     f.Source,
			})
		case "vulnerability":
			vulns = append(vulns, Vulnerability{
				CVE:        f.CVE,
				Title:      f.Title,
				ExploitRef: f.ExploitRef,
				Source:     f.Source,
			})
		}
	}

	return &ReconSummary{
		Target:          target,
		Hosts:           DeduplicateHosts(hosts),
		Endpoints:       DeduplicateEndpoints(endpoints),
		Technologies:    DeduplicateTechnologies(techs),
		Vulnerabilities: DeduplicateVulns(vulns),
		CreatedAt:       time.Now(),
	}
}

// FormatSummary returns a human-readable one-liner for a summary.
func FormatSummary(s *ReconSummary) string {
	if s == nil {
		return "no results"
	}
	return fmt.Sprintf("%d hosts, %d endpoints (%d interesting), %d technologies, %d vulnerabilities",
		s.HostCount(), s.EndpointCount(), len(s.InterestingEndpoints()), s.TechCount(), s.VulnCount())
}
