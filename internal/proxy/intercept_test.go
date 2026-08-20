package proxy

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"regexp"
	"testing"
	"time"

	"github.com/TsaH0/ouroboros/internal/intercept"
	"github.com/TsaH0/ouroboros/internal/msg"
	"github.com/TsaH0/ouroboros/internal/scope"
	"github.com/TsaH0/ouroboros/internal/store"
)

func TestInterceptForward(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		io.WriteString(w, "forwarded")
	}))
	defer upstream.Close()

	flowStore := store.NewMemoryStore()
	scopeMgr := scope.NewManager(nil)
	scopeMgr.AddRule(context.Background(), scope.Rule{
		Kind: scope.RuleKindHost, Pattern: "*", MatchMode: scope.MatchModeWildcard,
		Action: scope.ActionInclude, Enabled: true, Priority: 0,
	})
	is := intercept.NewMatcher([]intercept.Rule{
		{Allow: true, Host: regexp.MustCompile(`.*`)},
	})

	pxy := New(flowStore, nil, scopeMgr, is)
	pxy.transport = http.DefaultTransport

	proxyServer := httptest.NewServer(pxy)
	defer proxyServer.Close()

	proxyURL, _ := url.Parse(proxyServer.URL)
	client := &http.Client{
		Transport: &http.Transport{Proxy: http.ProxyURL(proxyURL)},
		Timeout:   5 * time.Second,
	}

	// The flow should be intercepted. We need to unblock it.
	go func() {
		time.Sleep(100 * time.Millisecond)
		flows, _ := flowStore.ListFlows(context.Background())
		for _, f := range flows {
			if f.State == "intercepted" {
				pxy.HandleInterceptCommand(msg.ForwardInterceptedFlow{FlowID: f.ID})
				return
			}
		}
	}()

	resp, err := client.Get(upstream.URL + "/test")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if string(body) != "forwarded" {
		t.Fatalf("body = %q, want %q", body, "forwarded")
	}
}

func TestInterceptDrop(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		io.WriteString(w, "should not reach")
	}))
	defer upstream.Close()

	flowStore := store.NewMemoryStore()
	scopeMgr := scope.NewManager(nil)
	scopeMgr.AddRule(context.Background(), scope.Rule{
		Kind: scope.RuleKindHost, Pattern: "*", MatchMode: scope.MatchModeWildcard,
		Action: scope.ActionInclude, Enabled: true, Priority: 0,
	})
	is := intercept.NewMatcher([]intercept.Rule{
		{Allow: true, Host: regexp.MustCompile(`.*`)},
	})

	pxy := New(flowStore, nil, scopeMgr, is)
	pxy.transport = http.DefaultTransport

	proxyServer := httptest.NewServer(pxy)
	defer proxyServer.Close()

	proxyURL, _ := url.Parse(proxyServer.URL)
	client := &http.Client{
		Transport: &http.Transport{Proxy: http.ProxyURL(proxyURL)},
		Timeout:   5 * time.Second,
	}

	go func() {
		time.Sleep(100 * time.Millisecond)
		flows, _ := flowStore.ListFlows(context.Background())
		for _, f := range flows {
			if f.State == "intercepted" {
				pxy.HandleInterceptCommandDrop(msg.DropInterceptedFlow{FlowID: f.ID})
				return
			}
		}
	}()

	resp, err := client.Get(upstream.URL + "/test")
	if err != nil {
		t.Fatalf("unexpected connection error: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("status = %d, want 403 Forbidden for dropped flow", resp.StatusCode)
	}
}
