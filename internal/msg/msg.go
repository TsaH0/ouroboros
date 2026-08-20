package msg

import "ouroboros/internal/model"

// --- Events (dispatched from proxy/backend to TUI) ---

// FlowStarted is emitted when a new flow begins.
type FlowStarted struct {
	Flow *model.Flow
}

// RequestReceived is emitted when the proxy receives the request headers/body.
type RequestReceived struct {
	FlowID string
	Req    *model.Message
}

// InterceptionRequired is emitted when a flow matches intercept rules and is paused.
type InterceptionRequired struct {
	FlowID string
}

// ResponseReceived is emitted when the proxy receives the upstream response.
type ResponseReceived struct {
	FlowID string
	Resp   *model.Message
}

// FlowCompleted is emitted when a flow finishes (success, drop, or error).
type FlowCompleted struct {
	Flow *model.Flow
}

// --- Commands (dispatched from TUI to proxy/backend) ---

// ForwardInterceptedFlow instructs the proxy to release a paused flow.
// If Edited is non-nil, the proxy forwards the edited request instead of the original.
type ForwardInterceptedFlow struct {
	FlowID string
	Edited *EditedRequest
}

// EditedRequest carries an edited request for intercept forward.
type EditedRequest struct {
	Method  string
	URL     string
	Headers map[string][]string
	Body    []byte
}

// DropInterceptedFlow instructs the proxy to drop a paused flow.
type DropInterceptedFlow struct {
	FlowID string
}

// ScopePresetChangedMsg is emitted when the active scope preset changes.
// History panes react by re-filtering their display.
type ScopePresetChangedMsg struct {
	PresetID   string // new active preset ID (empty = global)
	PresetName string // human-readable name for status bar
}
