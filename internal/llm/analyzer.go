package llm

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

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
	Findings []Finding `json:"findings"`
}

// Analyzer performs security analysis on captured HTTP flows.
type Analyzer struct {
	provider Provider
	model    string
}

func NewAnalyzer(provider Provider, model string) *Analyzer {
	return &Analyzer{provider: provider, model: model}
}

// AnalyzeFlow sends a flow to the LLM for security analysis.
func (a *Analyzer) AnalyzeFlow(ctx context.Context, flow *model.Flow) (*AnalysisResult, error) {
	schema := getAnalysisSchema()

	req := ChatRequest{
		Model:    a.model,
		Messages: buildAnalysisMessages(flow),
		Schema:   schema,
	}

	resp, err := a.provider.ChatCompletion(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("llm chat: %w", err)
	}

	var result AnalysisResult
	if err := json.Unmarshal([]byte(resp.Content), &result); err != nil {
		return nil, fmt.Errorf("unmarshal analysis: %w", err)
	}

	return &result, nil
}

func buildAnalysisMessages(flow *model.Flow) []Message {
	var b strings.Builder
	b.WriteString("Analyze the following HTTP request/response for security vulnerabilities.\n\n")
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
			b.WriteString(fmt.Sprintf("\nBody:\n%s\n", string(flow.Request.Body)))
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
			b.WriteString(fmt.Sprintf("\nBody:\n%s\n", string(flow.Response.Body)))
		}
	}

	b.WriteString("\nInstructions:\n")
	b.WriteString("- Only report concrete, verifiable security issues.\n")
	b.WriteString("- Do not speculate or report hypothetical vulnerabilities.\n")
	b.WriteString("- If no issues are found, return an empty findings array.\n")
	b.WriteString("- Severity must be one of: critical, high, medium, low.\n")
	b.WriteString("- CVEs are optional; only include confirmed CVEs.\n")

	return []Message{
		{Role: RoleSystem, Content: "You are a security analyst specializing in web application security. You analyze HTTP traffic and report findings in a structured JSON format. You are conservative — only report issues you are confident about."},
		{Role: RoleUser, Content: b.String()},
	}
}

func getAnalysisSchema() json.RawMessage {
	s := `{
		"type": "object",
		"properties": {
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
		"required": ["findings"]
	}`
	return json.RawMessage(s)
}
