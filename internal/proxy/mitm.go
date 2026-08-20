package proxy

import (
	"bufio"
	"bytes"
	"context"
	"crypto/tls"
	"io"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/TsaH0/ouroboros/internal/model"
	"github.com/TsaH0/ouroboros/internal/msg"
)

// readBufferedConn wraps a net.Conn and a bufio.Reader, reading from the
// buffered reader first before falling through to the raw connection.
type readBufferedConn struct {
	net.Conn
	br *bufio.Reader
}

func (r *readBufferedConn) Read(p []byte) (int, error) {
	if r.br.Buffered() > 0 {
		return r.br.Read(p)
	}
	return r.Conn.Read(p)
}

// mitmConnect handles CONNECT with MITM interception.
func (p *Proxy) mitmConnect(w http.ResponseWriter, r *http.Request, flow *model.Flow, host string) {
	hijacker, ok := w.(http.Hijacker)
	if !ok {
		flow.Error = "hijacking not supported"
		flow.State = model.FlowFailed
		p.finalizeFlow(flow, nil)
		http.Error(w, "hijacking not supported", http.StatusInternalServerError)
		return
	}

	clientConn, bufrw, err := hijacker.Hijack()
	if err != nil {
		flow.Error = err.Error()
		flow.State = model.FlowFailed
		p.finalizeFlow(flow, nil)
		return
	}

	// Wrap with buffered reader so TLS reads any data the HTTP server already buffered.
	bufConn := &readBufferedConn{Conn: clientConn, br: bufrw.Reader}

	// Send 200 before TLS handshake.
	clientConn.Write([]byte("HTTP/1.1 200 Connection Established\r\n\r\n"))

	// TLS config with dynamic cert generation.
	// - MinVersion TLS12: Go defaults include 1.2+ ; explicit for clarity.
	// - NextProtos advertises h2 + http/1.1 so browsers negotiating h2 don't
	//   abort. We still speak http/1.1 on the wire (see resp.Proto rewrite).
	// - GetCertificate normalizes SNI/host (strip port, lower-case) and
	//   surfaces cert-gen errors with host context.
	tlsConfig := &tls.Config{
		MinVersion: tls.VersionTLS12,
		GetCertificate: func(hello *tls.ClientHelloInfo) (*tls.Certificate, error) {
			sni := hello.ServerName
			if sni == "" {
				sni = host
			}
			if h, _, err := net.SplitHostPort(sni); err == nil {
				sni = h
			}
			sni = strings.ToLower(strings.TrimSpace(sni))
			cert, err := SignHost(p.ca, sni)
			if err != nil {
				return nil, err
			}
			return cert, nil
		},
		NextProtos: []string{"http/1.1"},
	}

	tlsConn := tls.Server(bufConn, tlsConfig)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := tlsConn.HandshakeContext(ctx); err != nil {
		errStr := err.Error()
		lower := strings.ToLower(errStr)
		// Classify: browser fault vs proxy fault.
		// Browser: CA not trusted / pinning / HSTS / client abort.
		isBrowserFault := strings.Contains(lower, "unknown authority") ||
			strings.Contains(lower, "bad certificate") ||
			strings.Contains(lower, "certificate signed by unknown") ||
			strings.Contains(lower, "unknown ca") ||
			strings.Contains(lower, "alert") ||
			strings.Contains(lower, "eof") ||
			strings.Contains(lower, "connection reset") ||
			strings.Contains(lower, "broken pipe") ||
			strings.Contains(lower, "no such host")
		isTimeout := strings.Contains(lower, "timeout") || strings.Contains(lower, "deadline")
		hint := "hint: install CA: go run ./cmd/ouroboros --install-ca | sudo tee /usr/local/share/ca-certificates/ouroboros.crt && sudo update-ca-certificates (or import ca.pem in browser); then restart browser"
		fault := "browser"
		if !isBrowserFault && !isTimeout && p.ca == nil {
			fault = "proxy (CA not loaded)"
			hint = "proxy fault: CA failed to load — check ~/.config/ouroboros/ca.pem"
		} else if !isBrowserFault && strings.Contains(lower, "cert") {
			fault = "proxy (cert gen)"
		}
		if isTimeout {
			fault = "browser/timeout"
		}
		// Keep flow.Error short for TUI table; full hint visible in detail pane.
		// Do NOT log to stdout/stderr — it corrupts the Bubble Tea AltScreen.
		// The failure is already persisted via finalizeFlow and visible as a failed flow.
		flow.Error = "TLS handshake failed for " + host + ": " + errStr + " [" + fault + " fault] — " + hint
		flow.State = model.FlowFailed
		p.finalizeFlow(flow, nil)
		clientConn.Close()
		return
	}

	// Read requests in a loop (HTTP/1.1 keep-alive).
	br := bufio.NewReader(tlsConn)
	bw := bufio.NewWriter(tlsConn)
	for {
		tlsConn.SetReadDeadline(time.Now().Add(30 * time.Second))
		req, err := http.ReadRequest(br)
		if err != nil {
			break // EOF or connection closed
		}
		tlsConn.SetReadDeadline(time.Time{})

		start := time.Now()
		subFlowID := newULID()

		// Read request body.
		reqBody, _ := io.ReadAll(req.Body)
		req.Body.Close()

		reqMsg := &model.Message{
			Method:      req.Method,
			URL:         "https://" + host + req.URL.RequestURI(),
			HTTPVersion: formatHTTPVersion(req.ProtoMajor, req.ProtoMinor),
			Headers:     cloneHeaders(req.Header),
			Body:        reqBody,
		}

		subFlow := &model.Flow{
			ID:         subFlowID,
			StartTime:  start,
			ClientAddr: r.RemoteAddr,
			Scheme:     "https",
			Host:       host,
			Request:    reqMsg,
			State:      model.FlowPending,
		}
		p.sendEvent(msg.FlowStarted{Flow: subFlow})

		// Check intercept.
		var edited *msg.EditedRequest
		if p.interceptSvc != nil && p.interceptSvc.Evaluate(subFlow) {
			subFlow.State = model.FlowIntercepted
			_ = p.store.SaveFlow(context.Background(), subFlow)
			p.sendEvent(msg.InterceptionRequired{FlowID: subFlow.ID})

			ch := make(chan InterceptResult, 1)
			p.interceptCh.Store(subFlow.ID, ch)
			select {
			case result := <-ch:
				p.interceptCh.Delete(subFlow.ID)
				if result.Action == "drop" {
					subFlow.State = model.FlowDropped
					p.finalizeFlow(subFlow, nil)
					continue
				}
				edited = result.Edited
			case <-time.After(5 * time.Minute):
				p.interceptCh.Delete(subFlow.ID)
				subFlow.State = model.FlowDropped
				p.finalizeFlow(subFlow, nil)
				continue
			}
			// Apply edits if user modified in TUI.
			if edited != nil {
				if edited.Method != "" {
					req.Method = edited.Method
					subFlow.Request.Method = edited.Method
				}
				if edited.URL != "" {
					subFlow.Request.URL = edited.URL
					if u, err := url.Parse(edited.URL); err == nil {
						req.URL = u
					}
				}
				if edited.Headers != nil {
					req.Header = http.Header(edited.Headers)
					subFlow.Request.Headers = edited.Headers
				}
				if edited.Body != nil {
					reqBody = edited.Body
					subFlow.Request.Body = edited.Body
				}
			}
		}

		// Build upstream URL (use edited URL/path if present).
		targetURL := &url.URL{
			Scheme:   "https",
			Host:     host,
			Path:     req.URL.Path,
			RawQuery: req.URL.RawQuery,
		}
		if edited != nil && edited.URL != "" {
			if u, err := url.Parse(edited.URL); err == nil {
				targetURL.Path = u.Path
				targetURL.RawQuery = u.RawQuery
				if u.Host != "" {
					targetURL.Host = u.Host
					host = u.Host
				}
			}
		}

		// Forward upstream.
		outReq, err := http.NewRequestWithContext(r.Context(), req.Method, targetURL.String(), bytes.NewReader(reqBody))
		if err != nil {
			subFlow.Error = err.Error()
			subFlow.State = model.FlowFailed
			p.finalizeFlow(subFlow, nil)
			continue
		}
		copyHeaders(outReq.Header, req.Header)
		removeHopHeaders(outReq.Header)
		outReq.ContentLength = int64(len(reqBody))
		outReq.RequestURI = ""

		resp, err := p.transport.(*http.Transport).RoundTrip(outReq)
		if err != nil {
			subFlow.Error = err.Error()
			subFlow.State = model.FlowFailed
			p.finalizeFlow(subFlow, nil)
			continue
		}

		respBody, _ := io.ReadAll(resp.Body)
		resp.Body.Close()

		respMsg := &model.Message{
			StatusCode:  resp.StatusCode,
			HTTPVersion: formatHTTPVersion(resp.ProtoMajor, resp.ProtoMinor),
			Headers:     cloneHeaders(resp.Header),
			Body:        respBody,
		}

		subFlow.Response = respMsg
		subFlow.State = model.FlowCompleted
		subFlow.Duration = time.Since(start)
		p.finalizeFlow(subFlow, nil)

		// Normalize the upstream response to the HTTP/1.1 client connection.
		resp.Proto = "HTTP/1.1"
		resp.ProtoMajor = 1
		resp.ProtoMinor = 1
		resp.TransferEncoding = nil
		resp.ContentLength = int64(len(respBody))
		resp.Header.Del("Transfer-Encoding")
		resp.Header.Set("Content-Length", strconv.FormatInt(resp.ContentLength, 10))
		resp.Body = io.NopCloser(bytes.NewReader(respBody))
		if err := resp.Write(bw); err != nil {
			break
		}
		if err := bw.Flush(); err != nil {
			break
		}
	}

	tlsConn.Close()
	clientConn.Close()
}
