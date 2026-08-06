package llm

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"sentinel/internal/model"
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
