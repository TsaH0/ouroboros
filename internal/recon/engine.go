package recon

import (
	"context"
	"fmt"
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

// Run executes the full recon pipeline for a target.
// It runs all providers concurrently, normalizes results, and returns
// a unified ReconSummary. The context supports cancellation.
func (e *Engine) Run(ctx context.Context, target string) (*ReconSummary, error) {
	// Check cache first.
	if cached, ok := e.cache.Get(target); ok {
		return cached, nil
	}

	var (
		mu          sync.Mutex
		allFindings []ReconFinding
	)

	var wg sync.WaitGroup

	for _, pm := range e.providers {
		wg.Add(1)
		go func(meta ProviderMetadata) {
			defer wg.Done()

			providerCtx := ctx
			timeout := e.defaultTimeout
			if meta.Timeout > 0 {
				timeout = time.Duration(meta.Timeout) * time.Second
			}
			var cancel context.CancelFunc
			providerCtx, cancel = context.WithTimeout(ctx, timeout)
			defer cancel()

			e.progressCh <- ProgressUpdate{Provider: meta.Provider.Name(), Status: "running"}

			findings, err := meta.Provider.Run(providerCtx, target)
			if err != nil {
				e.progressCh <- ProgressUpdate{Provider: meta.Provider.Name(), Status: "error", Error: err}
				// Non-fatal: continue with other providers.
				return
			}

			mu.Lock()
			allFindings = append(allFindings, findings...)
			mu.Unlock()

			e.progressCh <- ProgressUpdate{Provider: meta.Provider.Name(), Status: "done"}
		}(pm)
	}

	// Wait for all providers or cancellation.
	doneCh := make(chan struct{})
	go func() {
		wg.Wait()
		close(doneCh)
	}()

	select {
	case <-doneCh:
	case <-ctx.Done():
		return nil, ctx.Err()
	}

	// Normalize findings into summary.
	summary := normalizeFindings(target, allFindings)
	e.cache.Set(target, summary)

	// Close progress channel after all providers finished.
	close(e.progressCh)

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
