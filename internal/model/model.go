package model

import "time"

// ScopeStatus indicates whether a flow's target is within the authorized scope.
type ScopeStatus string

const (
	ScopeInScope    ScopeStatus = "in_scope"
	ScopeOutOfScope ScopeStatus = "out_of_scope"
	ScopeUnknown    ScopeStatus = "unknown"
)

// FlowState represents the lifecycle state of an intercepted flow.
type FlowState string

const (
	FlowPending     FlowState = "pending"
	FlowIntercepted FlowState = "intercepted"
	FlowCompleted   FlowState = "completed"
	FlowDropped     FlowState = "dropped"
	FlowFailed      FlowState = "failed"
)

// Message represents a single HTTP request or response.
type Message struct {
	Method      string              `json:"method"`
	URL         string              `json:"url"`
	HTTPVersion string              `json:"http_version"`
	StatusCode  int                 `json:"status_code"`
	Headers     map[string][]string `json:"headers"`
	Body        []byte              `json:"body"`
}

// Flow represents a single intercepted HTTP flow (request + response pair).
type Flow struct {
	ID          string        `json:"id"`
	ProjectID   string        `json:"project_id"`
	StartTime   time.Time     `json:"start_time"`
	Duration    time.Duration `json:"duration"`
	ClientAddr  string        `json:"client_addr"`
	ServerAddr  string        `json:"server_addr"`
	Scheme      string        `json:"scheme"`
	Host        string        `json:"host"`
	Port        int           `json:"port"`
	Request     *Message      `json:"request"`
	Response    *Message      `json:"response"`
	Error       string        `json:"error"`
	ScopeStatus ScopeStatus   `json:"scope_status"`
	State       FlowState     `json:"state"`
	Tags        []string      `json:"tags"`
	Notes       string        `json:"notes"`
}
