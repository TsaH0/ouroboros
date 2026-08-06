package model

import "time"

// Analysis records the result of an LLM analysis on a flow or recon summary.
type Analysis struct {
	ID        string    `json:"id"`
	FlowID    string    `json:"flow_id"`    // empty for bulk/recon analyses
	Kind      string    `json:"kind"`       // "flow", "bulk", "recon"
	Provider  string    `json:"provider"`
	Model     string    `json:"model"`
	Summary   string    `json:"summary"`
	RawJSON   string    `json:"raw_json"`
	CreatedAt time.Time `json:"created_at"`
}
