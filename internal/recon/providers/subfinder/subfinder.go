package subfinder

import (
	"context"
	"fmt"
	"strings"

	"ouroboros/internal/recon"
)

// Provider implements recon.ReconProvider for the subfinder tool.
type Provider struct {
	Runner recon.CommandRunner
}

func (p *Provider) Name() string { return "subfinder" }

func (p *Provider) Run(ctx context.Context, target string) ([]recon.ReconFinding, error) {
	r := p.Runner
	if r == nil {
		r = recon.DefaultCommandRunner{}
	}

	out, err := r.Run(ctx, "subfinder", []string{"-d", target, "-silent"})
	if err != nil {
		return nil, fmt.Errorf("subfinder: %w", err)
	}

	var findings []recon.ReconFinding
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		findings = append(findings, recon.ReconFinding{
			Type:   "host",
			Source: recon.SourceSubfinder,
			Value:  line,
		})
	}

	return findings, nil
}
