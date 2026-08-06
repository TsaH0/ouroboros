package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
)

// Role is the message role in a chat conversation.
type Role string

const (
	RoleSystem    Role = "system"
	RoleUser      Role = "user"
	RoleAssistant Role = "assistant"
)

// Message is a single chat message.
type Message struct {
	Role    Role   `json:"role"`
	Content string `json:"content"`
}

// ChatRequest is the provider-agnostic chat request.
type ChatRequest struct {
	Model    string
	Messages []Message
	Schema   json.RawMessage // nil = text, non-nil = structured JSON output
}

// ChatResponse is the provider-agnostic chat response.
type ChatResponse struct {
	Content      string
	FinishReason string
}

// Provider is the abstraction over LLM backends.
type Provider interface {
	ChatCompletion(ctx context.Context, req ChatRequest) (*ChatResponse, error)
}

// --- OpenAI Provider ---

type openaiProvider struct {
	baseURL    string
	apiKey     string
	httpClient *http.Client
}

// NewOpenAIProvider creates an OpenAI provider.
func NewOpenAIProvider(apiKey string) Provider {
	return &openaiProvider{
		baseURL:    "https://api.openai.com/v1",
		apiKey:     apiKey,
		httpClient: &http.Client{},
	}
}

func (p *openaiProvider) ChatCompletion(ctx context.Context, req ChatRequest) (*ChatResponse, error) {
	type apiMessage struct {
		Role    string `json:"role"`
		Content string `json:"content"`
	}

	body := map[string]any{
		"model":       req.Model,
		"messages":    []apiMessage{},
		"stream":      false,
		"temperature": 0.2,
	}
	for _, m := range req.Messages {
		body["messages"] = append(body["messages"].([]apiMessage), apiMessage{Role: string(m.Role), Content: m.Content})
	}

	if req.Schema != nil {
		body["response_format"] = map[string]any{
			"type": "json_schema",
			"json_schema": map[string]any{
				"name":   "analysis_result",
				"strict": true,
				"schema":  json.RawMessage(req.Schema),
			},
		}
	}

	b, _ := json.Marshal(body)
	httpReq, err := http.NewRequestWithContext(ctx, "POST", p.baseURL+"/chat/completions", bytes.NewReader(b))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Authorization", "Bearer "+p.apiKey)
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := p.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("openai request: %w", err)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("openai error %d: %s", resp.StatusCode, string(respBody))
	}

	var result struct {
		Choices []struct {
			Message      struct {
				Content string `json:"content"`
			} `json:"message"`
			FinishReason string `json:"finish_reason"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, fmt.Errorf("openai unmarshal: %w", err)
	}
	if len(result.Choices) == 0 {
		return nil, fmt.Errorf("openai empty response")
	}

	return &ChatResponse{
		Content:      result.Choices[0].Message.Content,
		FinishReason: result.Choices[0].FinishReason,
	}, nil
}

// --- Ollama Provider ---

type ollamaProvider struct {
	baseURL    string
	httpClient *http.Client
}

// NewOllamaProvider creates an Ollama provider at the given base URL.
func NewOllamaProvider(baseURL string) Provider {
	if baseURL == "" {
		baseURL = "http://localhost:11434"
	}
	return &ollamaProvider{
		baseURL:    baseURL,
		httpClient: &http.Client{},
	}
}

func (p *ollamaProvider) ChatCompletion(ctx context.Context, req ChatRequest) (*ChatResponse, error) {
	type apiMessage struct {
		Role    string `json:"role"`
		Content string `json:"content"`
	}

	body := map[string]any{
		"model":    req.Model,
		"messages": []apiMessage{},
		"stream":   false,
		"options": map[string]any{
			"temperature": 0.2,
		},
	}
	for _, m := range req.Messages {
		body["messages"] = append(body["messages"].([]apiMessage), apiMessage{Role: string(m.Role), Content: m.Content})
	}

	if req.Schema != nil {
		var schema map[string]any
		_ = json.Unmarshal(req.Schema, &schema)
		body["format"] = schema
	}

	b, _ := json.Marshal(body)
	httpReq, err := http.NewRequestWithContext(ctx, "POST", p.baseURL+"/api/chat", bytes.NewReader(b))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := p.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("ollama request: %w", err)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("ollama error %d: %s", resp.StatusCode, string(respBody))
	}

	var result struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
		Done         bool   `json:"done"`
		DoneReason   string `json:"done_reason,omitempty"`
	}
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, fmt.Errorf("ollama unmarshal: %w", err)
	}

	return &ChatResponse{
		Content:      result.Message.Content,
		FinishReason: result.DoneReason,
	}, nil
}

// --- Provider Factory ---

// NewProvider creates a provider from environment variables.
// Prefers OpenAI if OPENAI_API_KEY is set; falls back to Ollama at localhost.
func NewProvider() (Provider, string, error) {
	if key := os.Getenv("OPENAI_API_KEY"); key != "" {
		return NewOpenAIProvider(key), "gpt-4o-mini", nil
	}
	return NewOllamaProvider(""), "llama3.2", nil
}
