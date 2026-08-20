package repeater

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"

	"github.com/TsaH0/ouroboros/internal/model"
	"github.com/TsaH0/ouroboros/internal/scope"
)

// ErrOutOfScope is returned when the target URL is not in scope.
var ErrOutOfScope = errors.New("target is out of scope")

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
	ReplayOverride(ctx context.Context, flow *model.Flow, edits Edits) (*model.Message, error)
}

// HTTPService is the concrete implementation using net/http.
type HTTPService struct {
	client *http.Client
	scope  scope.Service
}

// NewHTTPService creates a new HTTPService with an optional scope checker.
// If sc is nil, scope enforcement is disabled.
func NewHTTPService(sc scope.Service) *HTTPService {
	return &HTTPService{
		client: &http.Client{
			Timeout: 30 * time.Second,
		},
		scope: sc,
	}
}

func (s *HTTPService) Replay(ctx context.Context, flow *model.Flow, edits Edits) (*model.Message, error) {
	if s.scope != nil {
		target := edits.URL
		if target == "" && flow.Request != nil {
			target = flow.Request.URL
		}
		if u, err := url.Parse(target); err == nil {
			if st := s.scope.Status(u); st != model.ScopeInScope {
				return nil, ErrOutOfScope
			}
		}
	}
	return s.replay(ctx, flow, edits)
}

func (s *HTTPService) ReplayOverride(ctx context.Context, flow *model.Flow, edits Edits) (*model.Message, error) {
	return s.replay(ctx, flow, edits)
}

func (s *HTTPService) replay(ctx context.Context, flow *model.Flow, edits Edits) (*model.Message, error) {
	method := edits.Method
	if method == "" && flow.Request != nil {
		method = flow.Request.Method
	}

	urlStr := edits.URL
	if urlStr == "" && flow.Request != nil {
		urlStr = flow.Request.URL
	}

	body := edits.Body
	if body == nil && flow.Request != nil {
		body = flow.Request.Body
	}

	req, err := http.NewRequestWithContext(ctx, method, urlStr, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	for k, vals := range edits.Headers {
		req.Header[k] = vals
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
		StatusCode:  resp.StatusCode,
		HTTPVersion: formatHTTPVersion(resp.ProtoMajor, resp.ProtoMinor),
		Headers:     cloneHeaders(resp.Header),
		Body:        respBody,
	}, nil
}

func formatHTTPVersion(major, minor int) string {
	if major == 1 {
		if minor == 0 {
			return "HTTP/1.0"
		}
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
