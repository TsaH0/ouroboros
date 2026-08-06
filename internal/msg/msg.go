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
type ForwardInterceptedFlow struct {
	FlowID string
}

// DropInterceptedFlow instructs the proxy to drop a paused flow.
type DropInterceptedFlow struct {
	FlowID string
}
