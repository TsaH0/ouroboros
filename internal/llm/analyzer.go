package llm

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"sentinel/internal/model"
	"sentinel/internal/recon"
)

// Finding is a single security finding from LLM analysis.
type Finding struct {
	Severity    string   `json:"severity"`
	Title       string   `json:"title"`
	Description string   `json:"description"`
	CVEs        []string `json:"cves"`
}

// AnalysisResult is the structured output of LLM security analysis.
type AnalysisResult struct {
	Summary  string    `json:"summary"`
	Findings []Finding `json:"findings"`
}

// BulkFinding is a finding tied to a specific flow in bulk analysis.
type BulkFinding struct {
	FlowID     string `json:"flow_id"`
	Severity   string `json:"severity"`
	Title      string `json:"title"`
	Why        string `json:"why"`
	Suggestion string `json:"suggestion"`
}

// BulkAnalysisResult is the structured output of bulk flow analysis.
type BulkAnalysisResult struct {
	Summary  string        `json:"summary"`
	Findings []BulkFinding `json:"findings"`
}

// ReconAnalysisResult is the LLM's prioritized analysis of a recon summary.
type ReconAnalysisResult struct {
	Summary              string   `json:"summary"`
	HighPriority         []string `json:"high_priority"`
	InterestingHosts     []string `json:"interesting_hosts"`
	InterestingEndpoints []string `json:"interesting_endpoints"`
	RecommendedOrder     []string `json:"recommended_testing_order"`
	InterestingPatterns  []string `json:"interesting_patterns"`
	Reasoning            []string `json:"reasoning"`
}

// Analyzer performs security analysis on captured HTTP flows.
type Analyzer struct {
	provider Provider
	model    string
}

func NewAnalyzer(provider Provider, model string) *Analyzer {
	return &Analyzer{provider: provider, model: model}
}

// AnalyzeFlow sends a single flow to the LLM for security analysis,
// optionally including prior conversation context.
func (a *Analyzer) AnalyzeFlow(ctx context.Context, flow *model.Flow, priorContext []Message) (*AnalysisResult, error) {
	req := ChatRequest{
		Model:    a.model,
		Messages: buildAnalysisMessages(flow, priorContext),
		Schema:   getAnalysisSchema(),
	}

	resp, err := a.provider.ChatCompletion(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("llm chat: %w", err)
	}

	var result AnalysisResult
	if err := json.Unmarshal([]byte(resp.Content), &result); err != nil {
		return nil, fmt.Errorf("unmarshal analysis: %w (content: %s)", err, truncate(resp.Content, 200))
	}

	return &result, nil
}

// AnalyzeBulk sends a compact summary of all flows to the LLM and returns
// which flows look suspicious, why, and what to do about it.
func (a *Analyzer) AnalyzeBulk(ctx context.Context, flows []*model.Flow) (*BulkAnalysisResult, error) {
	req := ChatRequest{
		Model:    a.model,
		Messages: buildBulkMessages(flows),
		Schema:   getBulkSchema(),
	}

	resp, err := a.provider.ChatCompletion(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("llm chat: %w", err)
	}

	var result BulkAnalysisResult
	if err := json.Unmarshal([]byte(resp.Content), &result); err != nil {
		return nil, fmt.Errorf("unmarshal bulk analysis: %w (content: %s)", err, truncate(resp.Content, 200))
	}

	return &result, nil
}

// AnalyzeRecon sends a compact recon summary to the LLM and receives
// prioritized attack surface analysis.
func (a *Analyzer) AnalyzeRecon(ctx context.Context, summary *recon.ReconSummary) (*ReconAnalysisResult, error) {
	req := ChatRequest{
		Model:    a.model,
		Messages: buildReconMessages(summary),
		Schema:   getReconSchema(),
	}

	resp, err := a.provider.ChatCompletion(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("llm chat: %w", err)
	}

	var result ReconAnalysisResult
	if err := json.Unmarshal([]byte(resp.Content), &result); err != nil {
		return nil, fmt.Errorf("unmarshal recon analysis: %w (content: %s)", err, truncate(resp.Content, 200))
	}

	return &result, nil
}

func buildReconMessages(s *recon.ReconSummary) []Message {
	var b strings.Builder
	b.WriteString("You are a web security analyst. Below is a recon summary.\n")
	b.WriteString("Prioritize the attack surface. Recommend manual investigation.\n")
	b.WriteString("Explain why endpoints are interesting. Suggest which to open in Repeater.\n")
	b.WriteString("Do NOT invent CVEs, technologies, or versions. Do NOT claim vulnerabilities are confirmed.\n")
	b.WriteString("Be concise. No filler.\n\n")

	b.WriteString(fmt.Sprintf("Target: %s\n", s.Target))
	b.WriteString(fmt.Sprintf("Hosts: %d\n", s.HostCount()))
	b.WriteString(fmt.Sprintf("Endpoints: %d (%d interesting)\n", s.EndpointCount(), len(s.InterestingEndpoints())))
	b.WriteString(fmt.Sprintf("Technologies: %d\n", s.TechCount()))
	b.WriteString(fmt.Sprintf("Potential Vulnerabilities: %d\n\n", s.VulnCount()))

	if s.TechCount() > 0 {
		b.WriteString("Technologies:\n")
		for _, t := range s.Technologies {
			v := t.Version
			if v == "" {
				v = "?"
			}
			b.WriteString(fmt.Sprintf("- %s %s (host: %s)\n", t.Name, v, t.Host))
		}
		b.WriteString("\n")
	}

	endpoints := s.InterestingEndpoints()
	if len(endpoints) > 50 {
		endpoints = endpoints[:50]
	}
	if len(endpoints) > 0 {
		b.WriteString("Interesting endpoints:\n")
		for _, e := range endpoints {
			b.WriteString(fmt.Sprintf("- [%s] %s\n", e.Category, e.URL))
		}
		b.WriteString("\n")
	}

	if s.VulnCount() > 0 {
		b.WriteString("Potential vulnerabilities:\n")
		for _, v := range s.Vulnerabilities {
			cve := v.CVE
			if cve == "" {
				cve = "N/A"
			}
			b.WriteString(fmt.Sprintf("- %s: %s\n", cve, v.Title))
		}
		b.WriteString("\n")
	}

	return []Message{
		{Role: RoleSystem, Content: "You are a web security analyst. Analyze recon summaries and prioritize attack surface. Be concise. Do not invent information not present in the data. Return structured JSON."},
		{Role: RoleUser, Content: b.String()},
	}
}

func getReconSchema() json.RawMessage {
	s := `{
		"type": "object",
		"properties": {
			"summary": {"type": "string"},
			"high_priority": {"type": "array", "items": {"type": "string"}},
			"interesting_hosts": {"type": "array", "items": {"type": "string"}},
			"interesting_endpoints": {"type": "array", "items": {"type": "string"}},
			"recommended_testing_order": {"type": "array", "items": {"type": "string"}},
			"interesting_patterns": {"type": "array", "items": {"type": "string"}},
			"reasoning": {"type": "array", "items": {"type": "string"}}
		},
		"required": ["summary", "high_priority", "interesting_hosts", "interesting_endpoints", "recommended_testing_order", "interesting_patterns", "reasoning"]
	}`
	return json.RawMessage(s)
}

func buildBulkMessages(flows []*model.Flow) []Message {
	var b strings.Builder
	b.WriteString("You are analyzing HTTP traffic from a proxy history. Below is a compact list of all captured requests.\n")
	b.WriteString("For each request that looks suspicious, report: the flow_id, severity, a short title, why it's suspicious, and what to do about it.\n")
	b.WriteString("Be concise. No filler words. Context window is limited.\n")
	b.WriteString("Only flag requests that are genuinely suspicious. Skip normal traffic.\n")
	b.WriteString("Severity: critical, high, medium, low.\n\n")

	b.WriteString("=== CAPTURED TRAFFIC ===\n")
	for i, f := range flows {
		shortID := f.ID
		if len(shortID) > 8 {
			shortID = shortID[len(shortID)-8:]
		}
		method := ""
		url := ""
		status := 0
		host := f.Host
		if f.Request != nil {
			method = f.Request.Method
			url = f.Request.URL
		}
		if f.Response != nil {
			status = f.Response.StatusCode
		}
		ts := f.StartTime.Format("15:04:05")
		dur := f.Duration.Round(time.Millisecond)
		b.WriteString(fmt.Sprintf("[%d] id=%s %s %s host=%s status=%d %s %s\n",
			i+1, shortID, method, url, host, status, ts, dur))
	}

	b.WriteString("\nAnalyze and report suspicious requests only.\n")

	return []Message{
		{Role: RoleSystem, Content: "You are a web security analyst. You analyze HTTP traffic summaries and flag suspicious requests. Be extremely concise — no pleasantries, no filler, no disclaimers. Context window is limited."},
		{Role: RoleUser, Content: b.String()},
	}
}

func buildAnalysisMessages(flow *model.Flow, priorContext []Message) []Message {
	var b strings.Builder
	b.WriteString("Analyze this HTTP request/response for security issues.\n")
	b.WriteString("Be concise. No filler. Context window is limited.\n\n")

	b.WriteString("=== REQUEST ===\n")
	if flow.Request != nil {
		b.WriteString(fmt.Sprintf("Method: %s\n", flow.Request.Method))
		b.WriteString(fmt.Sprintf("URL: %s\n", flow.Request.URL))
		b.WriteString(fmt.Sprintf("Version: %s\n", flow.Request.HTTPVersion))
		for k, vals := range flow.Request.Headers {
			for _, v := range vals {
				b.WriteString(fmt.Sprintf("%s: %s\n", k, v))
			}
		}
		if len(flow.Request.Body) > 0 {
			body := truncate(string(flow.Request.Body), 4096)
			b.WriteString(fmt.Sprintf("\nBody:\n%s\n", body))
		}
	}

	b.WriteString("\n=== RESPONSE ===\n")
	if flow.Response != nil {
		b.WriteString(fmt.Sprintf("Status: %d\n", flow.Response.StatusCode))
		b.WriteString(fmt.Sprintf("Version: %s\n", flow.Response.HTTPVersion))
		for k, vals := range flow.Response.Headers {
			for _, v := range vals {
				b.WriteString(fmt.Sprintf("%s: %s\n", k, v))
			}
		}
		if len(flow.Response.Body) > 0 {
			body := truncate(string(flow.Response.Body), 4096)
			b.WriteString(fmt.Sprintf("\nBody:\n%s\n", body))
		}
	}

	b.WriteString("\nInstructions:\n")
	b.WriteString("- Only report concrete issues. No speculation.\n")
	b.WriteString("- If no issues, return empty findings and a one-line summary.\n")
	b.WriteString("- Severity: critical, high, medium, low.\n")
	b.WriteString("- CVEs only if confirmed.\n")

	msgs := []Message{
		{Role: RoleSystem, Content: "You are a web security analyst. Report findings in structured JSON. Be concise — no filler, no disclaimers. Context window is limited."},
	}
	msgs = append(msgs, priorContext...)
	msgs = append(msgs, Message{Role: RoleUser, Content: b.String()})
	return msgs
}

func getAnalysisSchema() json.RawMessage {
	s := `{
		"type": "object",
		"properties": {
			"summary": {"type": "string"},
			"findings": {
				"type": "array",
				"items": {
					"type": "object",
					"properties": {
						"severity": {"type": "string", "enum": ["critical", "high", "medium", "low"]},
						"title": {"type": "string"},
						"description": {"type": "string"},
						"cves": {"type": "array", "items": {"type": "string"}}
					},
					"required": ["severity", "title", "description"]
				}
			}
		},
		"required": ["summary", "findings"]
	}`
	return json.RawMessage(s)
}

func getBulkSchema() json.RawMessage {
	s := `{
		"type": "object",
		"properties": {
			"summary": {"type": "string"},
			"findings": {
				"type": "array",
				"items": {
					"type": "object",
					"properties": {
						"flow_id": {"type": "string"},
						"severity": {"type": "string", "enum": ["critical", "high", "medium", "low"]},
						"title": {"type": "string"},
						"why": {"type": "string"},
						"suggestion": {"type": "string"}
					},
					"required": ["flow_id", "severity", "title", "why", "suggestion"]
				}
			}
		},
		"required": ["summary", "findings"]
	}`
	return json.RawMessage(s)
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + "...(truncated)"
}
