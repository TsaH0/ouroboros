package recon

import "context"

// ReconFinding is a raw finding from a provider before normalization.
type ReconFinding struct {
	Type       string `json:"type"` // "host", "url", "technology", "vulnerability"
	Source     Source `json:"source"`
	Value      string `json:"value"`
	Technology string `json:"technology,omitempty"`
	Version    string `json:"version,omitempty"`
	Confidence string `json:"confidence,omitempty"`
	CVE        string `json:"cve,omitempty"`
	Title      string `json:"title,omitempty"`
	ExploitRef string `json:"exploit_ref,omitempty"`
}

// ReconProvider is the provider abstraction for recon sources.
type ReconProvider interface {
	Name() string
	Run(ctx context.Context, target string) ([]ReconFinding, error)
}

// CommandRunner executes external commands with a timeout.
// This avoids hardcoding exec.Command throughout the application.
type CommandRunner interface {
	Run(ctx context.Context, name string, args []string) ([]byte, error)
}

// ProviderRole classifies when in the pipeline a provider runs.
type ProviderRole string

const (
	RoleDiscovery  ProviderRole = "discovery"  // subdomain/URL discovery
	RoleEnrichment ProviderRole = "enrichment" // tech detection, vuln lookup
)

// ProviderMetadata describes a registered provider.
type ProviderMetadata struct {
	Provider ReconProvider
	Role     ProviderRole
	Timeout  int // seconds; 0 = no explicit timeout
}
