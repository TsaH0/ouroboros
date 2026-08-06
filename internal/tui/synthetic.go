package tui

import (
	"crypto/rand"
	crand "math/rand/v2"
	"time"

	"charm.land/bubbletea/v2"
	"github.com/oklog/ulid/v2"

	"ouroboros/internal/model"
	"ouroboros/internal/msg"
)

// GenerateSyntheticFlow creates a randomized FlowCompleted event for demo purposes.
func GenerateSyntheticFlow() tea.Msg {
	now := time.Now()
	id := ulid.MustNew(ulid.Timestamp(now), rand.Reader).String()

	methods := []string{"GET", "POST", "PUT", "DELETE", "PATCH"}
	hosts := []string{"api.example.com", "app.example.com", "auth.example.com"}
	paths := []string{"/api/users", "/api/login", "/api/items", "/api/orders", "/api/config"}
	statuses := []int{200, 201, 204, 301, 400, 401, 403, 404, 500}

	method := methods[crand.IntN(len(methods))]
	host := hosts[crand.IntN(len(hosts))]
	path := paths[crand.IntN(len(paths))]
	status := statuses[crand.IntN(len(statuses))]
	scheme := "https"
	port := 443

	req := &model.Message{
		Method:      method,
		URL:         scheme + "://" + host + path,
		HTTPVersion: "HTTP/1.1",
		Headers:     map[string][]string{"Host": {host}, "User-Agent": {"Ouroboros/0.1"}},
		Body:        nil,
	}

	resp := &model.Message{
		HTTPVersion: "HTTP/1.1",
		StatusCode:  status,
		Headers:     map[string][]string{"Content-Type": {"application/json"}, "Server": {"nginx"}},
		Body:        []byte(`{"ok":true}`),
	}

	flow := &model.Flow{
		ID:          id,
		StartTime:   now.Add(-time.Duration(crand.IntN(5000)) * time.Millisecond),
		Duration:    time.Duration(crand.IntN(500)) * time.Millisecond,
		Scheme:      scheme,
		Host:        host,
		Port:        port,
		Request:     req,
		Response:    resp,
		ScopeStatus: model.ScopeInScope,
		State:       model.FlowCompleted,
	}

	return msg.FlowCompleted{Flow: flow}
}

// waitForSyntheticFlow returns a Cmd that waits 2 seconds then generates a flow.
func waitForSyntheticFlow() tea.Cmd {
	return func() tea.Msg {
		time.Sleep(2 * time.Second)
		return GenerateSyntheticFlow()
	}
}
