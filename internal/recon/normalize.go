package recon

import (
	"net/url"
	"sort"
	"strings"
)

// ClassifyEndpoint determines the category and heuristic score of an endpoint
// based on its path.
func ClassifyEndpoint(rawURL string) (EndpointCategory, int) {
	path := strings.ToLower(extractPath(rawURL))
	if path == "" {
		path = strings.ToLower(rawURL)
	}

	switch {
	case strings.Contains(path, "/admin"), strings.Contains(path, "/wp-admin"),
		strings.Contains(path, "/dashboard"), strings.Contains(path, "/manage"):
		return CatAdmin, categoryScore(CatAdmin)
	case strings.Contains(path, "/login"), strings.Contains(path, "/signin"),
		strings.Contains(path, "/auth"), strings.Contains(path, "/oauth"),
		strings.Contains(path, "/token"), strings.Contains(path, "/register"),
		strings.Contains(path, "/signup"), strings.Contains(path, "/password"):
		return CatAuth, categoryScore(CatAuth)
	case strings.Contains(path, "/graphql"), strings.HasSuffix(path, "/graphql"):
		return CatGraphQL, categoryScore(CatGraphQL)
	case strings.Contains(path, "/upload"), strings.Contains(path, "/uploads"):
		return CatUpload, categoryScore(CatUpload)
	case strings.Contains(path, "/swagger"), strings.Contains(path, "/openapi"),
		strings.Contains(path, "/api-docs"), strings.Contains(path, "/redoc"),
		strings.HasSuffix(path, "/openapi.json"), strings.HasSuffix(path, "/swagger.json"):
		return CatSwagger, categoryScore(CatSwagger)
	case strings.Contains(path, "/debug"), strings.Contains(path, "/actuator"):
		return CatDebug, categoryScore(CatDebug)
	case strings.Contains(path, "/.env"), strings.Contains(path, "/config"),
		strings.Contains(path, "/settings"), strings.Contains(path, "/.git"):
		return CatConfig, categoryScore(CatConfig)
	case strings.Contains(path, "/api/"), strings.HasPrefix(path, "/api"),
		strings.Contains(path, "/rest/"), strings.Contains(path, "/v1/"),
		strings.Contains(path, "/v2/"):
		return CatAPI, categoryScore(CatAPI)
	case strings.Contains(path, "/webhook"), strings.Contains(path, "/callback"):
		return CatWebhook, categoryScore(CatWebhook)
	case strings.Contains(path, "/.well-known/security"),
		strings.Contains(path, "/security.txt"), strings.Contains(path, "/csp-report"):
		return CatSecurity, categoryScore(CatSecurity)
	case strings.Contains(path, "/robots.txt"), strings.Contains(path, "/sitemap"),
		strings.HasSuffix(path, "/docs"), strings.Contains(path, "/documentation"),
		strings.Contains(path, "/readme"):
		return CatDocs, categoryScore(CatDocs)
	case strings.Contains(path, "/health"), strings.Contains(path, "/healthz"),
		strings.Contains(path, "/status"), strings.Contains(path, "/ping"),
		strings.Contains(path, "/alive"):
		return CatHealth, categoryScore(CatHealth)
	default:
		return CatGeneric, 0
	}
}

// NormalizeURL normalizes a URL by lowercasing the scheme and host,
// removing default ports, stripping fragments, and removing trailing slashes.
func NormalizeURL(rawURL string) string {
	rawURL = strings.TrimSpace(rawURL)
	if rawURL == "" {
		return ""
	}

	// Add scheme if missing.
	if !strings.Contains(rawURL, "://") {
		rawURL = "http://" + rawURL
	}

	u, err := url.Parse(rawURL)
	if err != nil || u.Host == "" {
		return ""
	}

	u.Scheme = strings.ToLower(u.Scheme)
	u.Host = strings.ToLower(u.Host)

	// Remove default ports.
	if u.Scheme == "http" && strings.HasSuffix(u.Host, ":80") {
		u.Host = strings.TrimSuffix(u.Host, ":80")
	}
	if u.Scheme == "https" && strings.HasSuffix(u.Host, ":443") {
		u.Host = strings.TrimSuffix(u.Host, ":443")
	}

	// Remove fragment.
	u.Fragment = ""

	// Remove trailing slash from path unless root.
	if u.Path != "/" {
		u.Path = strings.TrimSuffix(u.Path, "/")
	}

	// Sort query params for stable dedup.
	if u.RawQuery != "" {
		values := u.Query()
		keys := make([]string, 0, len(values))
		for k := range values {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		var parts []string
		for _, k := range keys {
			vs := values[k]
			sort.Strings(vs)
			for _, v := range vs {
				parts = append(parts, k+"="+v)
			}
		}
		u.RawQuery = strings.Join(parts, "&")
	}

	return u.String()
}

// DeduplicateHosts merges hosts with the same hostname, combining sources.
func DeduplicateHosts(hosts []Host) []Host {
	byHost := make(map[string]*Host)
	for _, h := range hosts {
		if existing, ok := byHost[h.Hostname]; ok {
			existing.Sources = mergeSources(existing.Sources, h.Sources)
			if h.IP != "" && existing.IP == "" {
				existing.IP = h.IP
			}
		} else {
			cp := h
			cp.Sources = uniqueSources(h.Sources)
			byHost[h.Hostname] = &cp
		}
	}

	result := make([]Host, 0, len(byHost))
	for _, h := range byHost {
		sort.Slice(h.Sources, func(i, j int) bool { return h.Sources[i] < h.Sources[j] })
		result = append(result, *h)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Hostname < result[j].Hostname })
	return result
}

// DeduplicateEndpoints merges endpoints with the same normalized URL,
// combining sources and keeping the highest score.
func DeduplicateEndpoints(endpoints []Endpoint) []Endpoint {
	byURL := make(map[string]*Endpoint)
	for _, e := range endpoints {
		if existing, ok := byURL[e.URL]; ok {
			existing.Sources = mergeSources(existing.Sources, e.Sources)
			if e.Score > existing.Score {
				existing.Score = e.Score
				existing.Category = e.Category
			}
		} else {
			cp := e
			cp.Sources = uniqueSources(e.Sources)
			byURL[e.URL] = &cp
		}
	}

	result := make([]Endpoint, 0, len(byURL))
	for _, e := range byURL {
		sort.Slice(e.Sources, func(i, j int) bool { return e.Sources[i] < e.Sources[j] })
		result = append(result, *e)
	}
	sort.SliceStable(result, func(i, j int) bool { return result[i].Score > result[j].Score })
	return result
}

// DeduplicateTechnologies merges identical tech detections.
func DeduplicateTechnologies(techs []Technology) []Technology {
	type key struct {
		name, version, host string
	}
	seen := make(map[key]Technology)
	for _, t := range techs {
		k := key{t.Name, t.Version, t.Host}
		if existing, ok := seen[k]; ok {
			// Keep higher confidence.
			if t.Confidence > existing.Confidence {
				seen[k] = t
			}
		} else {
			seen[k] = t
		}
	}
	result := make([]Technology, 0, len(seen))
	for _, t := range seen {
		result = append(result, t)
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].Name != result[j].Name {
			return result[i].Name < result[j].Name
		}
		return result[i].Host < result[j].Host
	})
	return result
}

// DeduplicateVulnerabilities merges identical vulnerability entries.
func DeduplicateVulns(vulns []Vulnerability) []Vulnerability {
	type key struct {
		cve, title string
	}
	seen := make(map[key]Vulnerability)
	for _, v := range vulns {
		k := key{v.CVE, v.Title}
		if _, ok := seen[k]; !ok {
			seen[k] = v
		}
	}
	result := make([]Vulnerability, 0, len(seen))
	for _, v := range seen {
		result = append(result, v)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Title < result[j].Title })
	return result
}

func mergeSources(a, b []Source) []Source {
	set := make(map[Source]bool)
	for _, s := range a {
		set[s] = true
	}
	for _, s := range b {
		set[s] = true
	}
	result := make([]Source, 0, len(set))
	for s := range set {
		result = append(result, s)
	}
	return result
}

func uniqueSources(srcs []Source) []Source {
	set := make(map[Source]bool)
	for _, s := range srcs {
		set[s] = true
	}
	result := make([]Source, 0, len(set))
	for s := range set {
		result = append(result, s)
	}
	return result
}
