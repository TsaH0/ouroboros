package searchsploit

import (
	"context"
	"encoding/json"
	"strings"

	"sentinel/internal/recon"
)

// Provider implements recon.ReconProvider for searchsploit.
// It is an enrichment provider that searches for known exploits
// matching the detected technologies.
type Provider struct {
	Runner recon.CommandRunner
	// Technologies is the list of technologies to search for.
	Technologies []recon.Technology
}

func (p *Provider) Name() string { return "searchsploit" }

func (p *Provider) Run(ctx context.Context, target string) ([]recon.ReconFinding, error) {
	r := p.Runner
	if r == nil {
		r = recon.DefaultCommandRunner{}
	}

	// Collect unique technology queries.
	searchTerms := p.collectSearchTerms()

	var findings []recon.ReconFinding
	for _, term := range searchTerms {
		out, err := r.Run(ctx, "searchsploit", []string{"--json", term})
		if err != nil {
			// searchsploit returns non-zero if no results, non-fatal.
			continue
		}
		findings = append(findings, parseSearchSploitOutput(string(out))...)
	}

	return findings, nil
}

func (p *Provider) collectSearchTerms() []string {
	seen := make(map[string]bool)
	var terms []string
	for _, t := range p.Technologies {
		term := t.Name
		if t.Version != "" {
			term = t.Name + " " + t.Version
		}
		if !seen[term] && t.Name != "" {
			seen[term] = true
			terms = append(terms, term)
		}
	}
	return terms
}

// parseSearchSploitOutput parses the JSON output of `searchsploit --json`.
// Format: {"RESULTS_SEARCH": [...], "RESULTS_PORT": [...]}
func parseSearchSploitOutput(output string) []recon.ReconFinding {
	type sploitEntry struct {
		Title string `json:"Title"`
		Path  string `json:"Path"`
		Code  string `json:"Code"`
	}
	type sploitResponse struct {
		ResultsSearch []sploitEntry `json:"RESULTS_SEARCH"`
		ResultsPort   []sploitEntry `json:"RESULTS_PORT"`
	}

	// The JSON from searchsploit can have leading non-JSON lines.
	// Find the first '{' and parse from there.
	idx := strings.Index(output, "{")
	if idx < 0 {
		return nil
	}
	jsonPart := output[idx:]

	var resp sploitResponse
	if err := json.Unmarshal([]byte(jsonPart), &resp); err != nil {
		return nil
	}

	var findings []recon.ReconFinding
	for _, e := range resp.ResultsSearch {
		findings = append(findings, recon.ReconFinding{
			Type:       "vulnerability",
			Source:     recon.SourceSearchSploit,
			Title:      e.Title,
			ExploitRef: e.Path,
		})
	}
	for _, e := range resp.ResultsPort {
		findings = append(findings, recon.ReconFinding{
			Type:       "vulnerability",
			Source:     recon.SourceSearchSploit,
			Title:      e.Title,
			ExploitRef: e.Path,
		})
	}

	return findings
}
