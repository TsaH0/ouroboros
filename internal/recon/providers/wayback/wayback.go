package wayback

import (
	"context"
	"fmt"
	"strings"

	"ouroboros/internal/recon"
)

// Provider implements recon.ReconProvider for waybackurls.
type Provider struct {
	Runner recon.CommandRunner
}

func (p *Provider) Name() string { return "waybackurls" }

func (p *Provider) Run(ctx context.Context, target string) ([]recon.ReconFinding, error) {
	r := p.Runner
	if r == nil {
		r = recon.DefaultCommandRunner{}
	}

	out, err := r.Run(ctx, "waybackurls", []string{target})
	if err != nil {
		return nil, fmt.Errorf("waybackurls: %w", err)
	}

	var findings []recon.ReconFinding
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		findings = append(findings, recon.ReconFinding{
			Type:   "url",
			Source: recon.SourceWayback,
			Value:  line,
		})
	}

	return findings, nil
}
