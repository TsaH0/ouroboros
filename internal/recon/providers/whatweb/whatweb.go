package whatweb

import (
	"context"
	"fmt"
	"strings"

	"sentinel/internal/recon"
)

// Provider implements recon.ReconProvider for WhatWeb technology detection.
type Provider struct {
	Runner recon.CommandRunner
}

func (p *Provider) Name() string { return "whatweb" }

// Run executes whatweb against the target URL and parses the technology output.
// WhatWeb output format: http://example.com [200 OK] Apache[2.4.41], PHP[7.4], ...
func (p *Provider) Run(ctx context.Context, target string) ([]recon.ReconFinding, error) {
	r := p.Runner
	if r == nil {
		r = recon.DefaultCommandRunner{}
	}

	if !strings.Contains(target, "://") {
		target = "http://" + target
	}

	out, err := r.Run(ctx, "whatweb", []string{"--no-errors", "--color=never", target})
	if err != nil {
		return nil, fmt.Errorf("whatweb: %w", err)
	}

	return parseWhatWebOutput(string(out), target), nil
}

// parseWhatWebOutput parses whatweb single-site output.
// Example: "http://example.com [200 OK] Apache[2.4.41], PHP[7.4], Country[RESERVED][ZZ]"
func parseWhatWebOutput(output, target string) []recon.ReconFinding {
	line := strings.TrimSpace(output)
	if line == "" {
		return nil
	}

	// Find the URL prefix, then the bracketed tech section.
	bracketIdx := strings.Index(line, "[")
	if bracketIdx < 0 {
		return nil
	}

	host := strings.TrimSpace(line[:bracketIdx])
	host = strings.TrimPrefix(host, "http://")
	host = strings.TrimPrefix(host, "https://")

	rest := line[bracketIdx:]

	// Skip the status code bracket, e.g. "[200 OK]"
	techSection := rest
	if closeIdx := strings.Index(rest, "]"); closeIdx >= 0 {
		techSection = rest[closeIdx+1:]
	}

	var findings []recon.ReconFinding
	for _, part := range splitTechnologies(techSection) {
		part = strings.TrimSpace(part)
		if part == "" || strings.HasPrefix(part, "Country[") || strings.HasPrefix(part, "Title[") {
			continue
		}
		name, version := parseTechPart(part)
		_ = target // target is the full URL, we want host
		findings = append(findings, recon.ReconFinding{
			Type:       "technology",
			Source:     recon.SourceWhatWeb,
			Value:      host,
			Technology: name,
			Version:    version,
			Confidence: "100",
		})
	}

	return findings
}

// splitTechnologies splits the comma-separated tech list but respects brackets.
func splitTechnologies(s string) []string {
	var parts []string
	depth := 0
	start := 0
	for i, c := range s {
		switch c {
		case '[':
			depth++
		case ']':
			depth--
		case ',':
			if depth == 0 {
				parts = append(parts, s[start:i])
				start = i + 1
			}
		}
	}
	if start < len(s) {
		parts = append(parts, s[start:])
	}
	return parts
}

// parseTechPart extracts name and version from a tech fragment like "Apache[2.4.41]".
func parseTechPart(part string) (name, version string) {
	part = strings.TrimSpace(part)
	idx := strings.Index(part, "[")
	if idx < 0 {
		return part, ""
	}
	name = part[:idx]
	version = strings.Trim(part[idx:], "[]")
	version = strings.TrimSuffix(version, "]")
	return name, version
}
