package gau

import (
	"context"
	"fmt"
	"strings"

	"ouroboros/internal/recon"
)

// Provider implements recon.ReconProvider for the gau tool.
type Provider struct {
	Runner recon.CommandRunner
}

func (p *Provider) Name() string { return "gau" }

func (p *Provider) Run(ctx context.Context, target string) ([]recon.ReconFinding, error) {
	r := p.Runner
	if r == nil {
		r = recon.DefaultCommandRunner{}
	}

	out, err := r.Run(ctx, "gau", []string{target})
	if err != nil {
		return nil, fmt.Errorf("gau: %w", err)
	}

	var findings []recon.ReconFinding
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		findings = append(findings, recon.ReconFinding{
			Type:   "url",
			Source: recon.SourceGAU,
			Value:  line,
		})
	}

	return findings, nil
}
