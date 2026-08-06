package repeater

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"

	"ouroboros/internal/model"
)

// Edits holds user-modified fields for replaying a request.
type Edits struct {
	Method  string
	URL     string
	Headers map[string][]string
	Body    []byte
}

// Service replays captured flows with optional modifications.
type Service interface {
	Replay(ctx context.Context, flow *model.Flow, edits Edits) (*model.Message, error)
}

// HTTPService is the concrete implementation using net/http.
type HTTPService struct {
	client *http.Client
}

func NewHTTPService() *HTTPService {
	return &HTTPService{
		client: &http.Client{
			Transport: &http.Transport{
				DisableCompression: true,
			},
		},
	}
}

func (s *HTTPService) Replay(ctx context.Context, flow *model.Flow, edits Edits) (*model.Message, error) {
	method := edits.Method
	if method == "" {
		if flow.Request != nil {
			method = flow.Request.Method
		} else {
			method = http.MethodGet
		}
	}

	url := edits.URL
	if url == "" {
		if flow.Request != nil {
			url = flow.Request.URL
		} else {
			return nil, fmt.Errorf("no URL provided and flow has no request")
		}
	}

	body := edits.Body
	if body == nil && flow.Request != nil {
		body = flow.Request.Body
	}

	req, err := http.NewRequestWithContext(ctx, method, url, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	for k, vals := range edits.Headers {
		for _, v := range vals {
			req.Header.Add(k, v)
		}
	}
	if req.Header.Get("Content-Length") == "" && len(body) > 0 {
		req.ContentLength = int64(len(body))
	}

	resp, err := s.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("send request: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response body: %w", err)
	}

	return &model.Message{
		Method:      method,
		URL:         url,
		HTTPVersion: formatHTTPVersion(resp.ProtoMajor, resp.ProtoMinor),
		StatusCode:  resp.StatusCode,
		Headers:     cloneHeaders(resp.Header),
		Body:        respBody,
	}, nil
}

func formatHTTPVersion(major, minor int) string {
	if major == 1 && minor == 1 {
		return "HTTP/1.1"
	}
	if major == 2 {
		return "HTTP/2"
	}
	return fmt.Sprintf("HTTP/%d.%d", major, minor)
}

func cloneHeaders(h http.Header) map[string][]string {
	if h == nil {
		return nil
	}
	m := make(map[string][]string, len(h))
	for k, v := range h {
		m[k] = append([]string(nil), v...)
	}
	return m
}
