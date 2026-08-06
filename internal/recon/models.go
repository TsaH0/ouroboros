package recon

import (
	"net/url"
	"sort"
	"time"
)

// Source identifies which provider produced a finding.
type Source string

const (
	SourceSubfinder    Source = "subfinder"
	SourceGAU          Source = "gau"
	SourceWayback      Source = "waybackurls"
	SourceWhatWeb      Source = "whatweb"
	SourceSearchSploit Source = "searchsploit"
)

// Host is a discovered host/subdomain.
type Host struct {
	Hostname string   `json:"hostname"`
	IP       string   `json:"ip,omitempty"`
	Sources  []Source `json:"sources"`
}

// EndpointCategory classifies an endpoint by function.
type EndpointCategory string

const (
	CatAuth     EndpointCategory = "Authentication"
	CatAdmin    EndpointCategory = "Admin"
	CatGraphQL  EndpointCategory = "GraphQL"
	CatSwagger  EndpointCategory = "Swagger/OpenAPI"
	CatUpload   EndpointCategory = "Upload"
	CatWebhook  EndpointCategory = "Webhook"
	CatAPI      EndpointCategory = "API"
	CatDebug    EndpointCategory = "Debug"
	CatHealth   EndpointCategory = "Health"
	CatConfig   EndpointCategory = "Configuration"
	CatDocs     EndpointCategory = "Documentation"
	CatSecurity EndpointCategory = "Security"
	CatGeneric  EndpointCategory = "Generic"
)

// Endpoint is a discovered URL/endpoint.
type Endpoint struct {
	URL      string           `json:"url"`
	Host     string           `json:"host"`
	Path     string           `json:"path"`
	Method   string           `json:"method,omitempty"`
	Category EndpointCategory `json:"category"`
	Score    int              `json:"score"`
	Sources  []Source         `json:"sources"`
}

// Technology is a detected technology on a host.
type Technology struct {
	Name       string `json:"name"`
	Version    string `json:"version,omitempty"`
	Confidence string `json:"confidence,omitempty"` // e.g. "100", "high"
	Host       string `json:"host"`
	Source     Source `json:"source"`
}

// Vulnerability is a potential vulnerability from SearchSploit.
type Vulnerability struct {
	CVE        string `json:"cve,omitempty"`
	Title      string `json:"title"`
	Severity   string `json:"severity,omitempty"`
	ExploitRef string `json:"exploit_ref,omitempty"`
	Source     Source `json:"source"`
}

// ProviderStatus records the outcome of one provider execution.
type ProviderStatus struct {
	Name     string       `json:"name"`
	Role     ProviderRole `json:"role"`
	Status   string       `json:"status"`
	Findings int          `json:"findings"`
	Error    string       `json:"error,omitempty"`
}

// ReconSummary is the unified output of the recon pipeline.
type ReconSummary struct {
	Target          string           `json:"target"`
	Hosts           []Host           `json:"hosts"`
	Endpoints       []Endpoint       `json:"endpoints"`
	Technologies    []Technology     `json:"technologies"`
	Vulnerabilities []Vulnerability  `json:"vulnerabilities"`
	Providers       []ProviderStatus `json:"providers"`
	CreatedAt       time.Time        `json:"created_at"`
}

// SortedEndpoints returns endpoints sorted by descending score.
func (s *ReconSummary) SortedEndpoints() []Endpoint {
	sorted := make([]Endpoint, len(s.Endpoints))
	copy(sorted, s.Endpoints)
	sort.SliceStable(sorted, func(i, j int) bool {
		return sorted[i].Score > sorted[j].Score
	})
	return sorted
}

// InterestingEndpoints returns endpoints with score > 0.
func (s *ReconSummary) InterestingEndpoints() []Endpoint {
	var result []Endpoint
	for _, e := range s.Endpoints {
		if e.Score > 0 {
			result = append(result, e)
		}
	}
	return result
}

// HostCount returns the number of unique hosts.
func (s *ReconSummary) HostCount() int { return len(s.Hosts) }

// EndpointCount returns the number of unique endpoints.
func (s *ReconSummary) EndpointCount() int { return len(s.Endpoints) }

// TechCount returns the number of detected technologies.
func (s *ReconSummary) TechCount() int { return len(s.Technologies) }

// VulnCount returns the number of potential vulnerabilities.
func (s *ReconSummary) VulnCount() int { return len(s.Vulnerabilities) }

// Levels groups HTTP-like categories for scoring summaries.
func categoryScore(c EndpointCategory) int {
	switch c {
	case CatAdmin:
		return 100
	case CatAuth:
		return 90
	case CatGraphQL:
		return 80
	case CatUpload:
		return 75
	case CatSwagger:
		return 70
	case CatDebug:
		return 65
	case CatConfig:
		return 60
	case CatAPI:
		return 50
	case CatWebhook:
		return 45
	case CatSecurity:
		return 40
	case CatDocs:
		return 20
	case CatHealth:
		return 10
	default:
		return 0
	}
}

// extractHost extracts the hostname from a URL string.
func extractHost(rawURL string) string {
	u, err := url.Parse(rawURL)
	if err != nil || u.Host == "" {
		// might be a bare hostname
		return rawURL
	}
	return u.Hostname()
}

// extractPath extracts the path from a URL string.
func extractPath(rawURL string) string {
	u, err := url.Parse(rawURL)
	if err != nil {
		return ""
	}
	return u.Path
}
