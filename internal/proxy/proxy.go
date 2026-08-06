package proxy

import (
	"context"
	"crypto/rand"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"charm.land/bubbletea/v2"
	"github.com/oklog/ulid/v2"

	"sentinel/internal/intercept"
	"sentinel/internal/model"
	"sentinel/internal/msg"
	"sentinel/internal/scope"
	"sentinel/internal/store"
)

var hopHeaders = []string{
	"Proxy-Connection",
	"Proxy-Authorization",
	"Proxy-Authenticate",
	"Connection",
	"Keep-Alive",
	"Transfer-Encoding",
	"TE",
	"Trailer",
	"Upgrade",
}

// InterceptResult is sent on the intercept channel to unblock a paused flow.
type InterceptResult struct {
	Action string // "forward" or "drop"
}

// Proxy is an HTTP intercepting forward proxy.
type Proxy struct {
	store        *store.InMemoryFlowStore
	program      *tea.Program
	scope        scope.Service
	interceptSvc intercept.Service
	transport    http.RoundTripper
	interceptCh  sync.Map // map[string]chan InterceptResult
	ca           *CACert
}

func New(s *store.InMemoryFlowStore, p *tea.Program, sc scope.Service, is intercept.Service) *Proxy {
	return &Proxy{
		store:        s,
		program:      p,
		scope:        sc,
		interceptSvc: is,
		transport: &http.Transport{
			MaxIdleConns:        100,
			MaxIdleConnsPerHost: 10,
			IdleConnTimeout:     90 * time.Second,
			DisableCompression:  true,
			ForceAttemptHTTP2:   false,
		},
	}
}

func (p *Proxy) SetCA(ca *CACert) {
	p.ca = ca
}

// SetProgram sets the tea.Program for sending events to the TUI.
func (p *Proxy) SetProgram(program *tea.Program) {
	p.program = program
}

func (p *Proxy) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodConnect {
		p.handleConnect(w, r)
		return
	}
	p.handleHTTP(w, r)
}

func (p *Proxy) handleHTTP(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	flowID := newULID()

	reqBody, err := io.ReadAll(r.Body)
	r.Body.Close()
	if err != nil {
		http.Error(w, "failed to read request body", http.StatusInternalServerError)
		return
	}

	reqMsg := &model.Message{
		Method:      r.Method,
		URL:         r.URL.String(),
		HTTPVersion: formatHTTPVersion(r.ProtoMajor, r.ProtoMinor),
		Headers:     cloneHeaders(r.Header),
		Body:        reqBody,
	}

	flow := &model.Flow{
		ID:         flowID,
		StartTime:  start,
		ClientAddr: r.RemoteAddr,
		Scheme:     "http",
		Host:       r.Host,
		Request:    reqMsg,
		State:      model.FlowPending,
	}
	p.sendEvent(msg.FlowStarted{Flow: flow})

	// Check intercept.
	if p.interceptSvc != nil && p.interceptSvc.Evaluate(flow) {
		flow.State = model.FlowIntercepted
		_ = p.store.Save(context.Background(), flow)
		p.sendEvent(msg.InterceptionRequired{FlowID: flow.ID})

		ch := make(chan InterceptResult, 1)
		p.interceptCh.Store(flow.ID, ch)
		select {
		case result := <-ch:
			p.interceptCh.Delete(flow.ID)
			if result.Action == "drop" {
				flow.State = model.FlowDropped
				p.finalizeFlow(flow, nil)
				http.Error(w, "intercepted and dropped", http.StatusForbidden)
				return
			}
		case <-time.After(5 * time.Minute):
			p.interceptCh.Delete(flow.ID)
			flow.State = model.FlowDropped
			p.finalizeFlow(flow, nil)
			http.Error(w, "intercept timeout", http.StatusGatewayTimeout)
			return
		}
	}

	targetURL := r.URL
	if !targetURL.IsAbs() {
		targetURL = &url.URL{
			Scheme:   "http",
			Host:     r.Host,
			Path:     r.URL.Path,
			RawQuery: r.URL.RawQuery,
		}
	}

	outReq, err := http.NewRequestWithContext(r.Context(), r.Method, targetURL.String(), nil)
	if err != nil {
		flow.Error = err.Error()
		flow.State = model.FlowFailed
		p.finalizeFlow(flow, nil)
		http.Error(w, "failed to create upstream request", http.StatusInternalServerError)
		return
	}
	copyHeaders(outReq.Header, r.Header)
	removeHopHeaders(outReq.Header)
	outReq.ContentLength = r.ContentLength
	outReq.Body = io.NopCloser(strings.NewReader(string(reqBody)))
	outReq.RequestURI = ""

	resp, err := p.transport.(*http.Transport).RoundTrip(outReq)
	if err != nil {
		flow.Error = err.Error()
		flow.State = model.FlowFailed
		p.finalizeFlow(flow, nil)
		http.Error(w, "upstream request failed", http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		flow.Error = err.Error()
		flow.State = model.FlowFailed
		p.finalizeFlow(flow, nil)
		http.Error(w, "failed to read response body", http.StatusInternalServerError)
		return
	}

	respMsg := &model.Message{
		StatusCode:  resp.StatusCode,
		HTTPVersion: formatHTTPVersion(resp.ProtoMajor, resp.ProtoMinor),
		Headers:     cloneHeaders(resp.Header),
		Body:        respBody,
	}

	flow.Response = respMsg
	flow.State = model.FlowCompleted
	flow.Duration = time.Since(start)
	p.finalizeFlow(flow, nil)

	outHeaders := w.Header()
	copyHeaders(outHeaders, resp.Header)
	removeHopHeaders(outHeaders)
	w.WriteHeader(resp.StatusCode)
	w.Write(respBody)
}

func (p *Proxy) handleConnect(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	flowID := newULID()

	host := r.Host
	if host == "" {
		host = r.URL.Host
	}

	flow := &model.Flow{
		ID:         flowID,
		StartTime:  start,
		ClientAddr: r.RemoteAddr,
		Scheme:     "https",
		Host:       host,
		State:      model.FlowPending,
	}
	p.sendEvent(msg.FlowStarted{Flow: flow})

	u := &url.URL{Scheme: "https", Host: host}
	if p.ca != nil && p.scope.Evaluate(u) {
		p.mitmConnect(w, r, flow, host)
		return
	}

	destConn, err := net.DialTimeout("tcp", host, 10*time.Second)
	if err != nil {
		flow.Error = err.Error()
		flow.State = model.FlowFailed
		p.finalizeFlow(flow, nil)
		http.Error(w, "failed to connect to upstream", http.StatusBadGateway)
		return
	}

	hijacker, ok := w.(http.Hijacker)
	if !ok {
		flow.Error = "hijacking not supported"
		flow.State = model.FlowFailed
		p.finalizeFlow(flow, nil)
		http.Error(w, "hijacking not supported", http.StatusInternalServerError)
		return
	}

	clientConn, _, err := hijacker.Hijack()
	if err != nil {
		destConn.Close()
		flow.Error = err.Error()
		flow.State = model.FlowFailed
		p.finalizeFlow(flow, nil)
		return
	}

	clientConn.Write([]byte("HTTP/1.1 200 Connection Established\r\n\r\n"))

	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		io.Copy(destConn, clientConn)
	}()
	go func() {
		defer wg.Done()
		io.Copy(clientConn, destConn)
	}()
	wg.Wait()

	clientConn.Close()
	destConn.Close()

	flow.State = model.FlowCompleted
	flow.Duration = time.Since(start)
	p.finalizeFlow(flow, nil)
}

// HandleInterceptCommand processes a forward/drop command from the TUI.
func (p *Proxy) HandleInterceptCommand(cmd msg.ForwardInterceptedFlow) {
	if v, ok := p.interceptCh.Load(cmd.FlowID); ok {
		ch := v.(chan InterceptResult)
		ch <- InterceptResult{Action: "forward"}
	}
}

func (p *Proxy) HandleInterceptCommandDrop(cmd msg.DropInterceptedFlow) {
	if v, ok := p.interceptCh.Load(cmd.FlowID); ok {
		ch := v.(chan InterceptResult)
		ch <- InterceptResult{Action: "drop"}
	}
}

func (p *Proxy) finalizeFlow(flow *model.Flow, resp *model.Message) {
	if resp != nil {
		flow.Response = resp
	}
	if flow.State == "" {
		flow.State = model.FlowCompleted
	}
	if flow.Duration == 0 {
		flow.Duration = time.Since(flow.StartTime)
	}
	_ = p.store.Save(context.Background(), flow)
	p.sendEvent(msg.FlowCompleted{Flow: flow})
}

func (p *Proxy) sendEvent(e tea.Msg) {
	if p.program != nil {
		p.program.Send(e)
	}
}

func newULID() string {
	now := time.Now()
	return ulid.MustNew(ulid.Timestamp(now), rand.Reader).String()
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

func copyHeaders(dst, src http.Header) {
	for k, v := range src {
		dst[k] = v
	}
}

func removeHopHeaders(h http.Header) {
	for _, name := range hopHeaders {
		h.Del(name)
	}
	if h.Get("Upgrade") != "" {
		return
	}
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
