package proxy

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/TsaH0/ouroboros/internal/scope"
	"github.com/TsaH0/ouroboros/internal/store"
)

func TestHTTPSMITMFramesBufferedResponse(t *testing.T) {
	upstream := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		w.WriteHeader(http.StatusOK)
		w.(http.Flusher).Flush() // Force an upstream response without Content-Length.
		_, _ = io.WriteString(w, "captured body")
	}))
	defer upstream.Close()

	ca, err := generateCA()
	if err != nil {
		t.Fatalf("generate CA: %v", err)
	}

	flowStore := store.NewMemoryStore()
	// Create a scope manager with an allow-all rule.
	scopeMgr := scope.NewManager(nil)
	scopeMgr.AddRule(context.Background(), scope.Rule{
		Kind:      scope.RuleKindHost,
		Pattern:   "*",
		MatchMode: scope.MatchModeWildcard,
		Action:    scope.ActionInclude,
		Enabled:   true,
		Priority:  0,
	})

	proxyHandler := New(flowStore, nil, scopeMgr, nil)
	proxyHandler.SetCA(ca)
	proxyHandler.transport = upstream.Client().Transport

	proxyServer := httptest.NewServer(proxyHandler)
	defer proxyServer.Close()

	proxyURL, err := url.Parse(proxyServer.URL)
	if err != nil {
		t.Fatalf("parse proxy URL: %v", err)
	}
	roots := x509.NewCertPool()
	roots.AddCert(ca.X509)

	client := &http.Client{
		Transport: &http.Transport{
			Proxy:             http.ProxyURL(proxyURL),
			TLSClientConfig:   &tls.Config{RootCAs: roots},
			ForceAttemptHTTP2: false,
		},
		Timeout: 5 * time.Second,
	}

	resp, err := client.Get(upstream.URL + "/through-proxy")
	if err != nil {
		t.Fatalf("GET through HTTPS MITM proxy: %v", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read response: %v", err)
	}
	if got, want := string(body), "captured body"; got != want {
		t.Fatalf("body = %q, want %q", got, want)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusOK)
	}

	flows, err := flowStore.ListFlows(context.Background())
	if err != nil {
		t.Fatalf("list flows: %v", err)
	}
	if len(flows) != 1 || flows[0].Response == nil || flows[0].Response.StatusCode != http.StatusOK {
		t.Fatalf("captured flows = %#v, want one completed HTTP 200 flow", flows)
	}
}
