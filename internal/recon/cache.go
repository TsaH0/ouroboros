package recon

import (
	"sync"
	"time"
)

// Cache stores recon results during the session to avoid re-running providers.
type Cache struct {
	mu      sync.RWMutex
	results map[string]*ReconSummary // keyed by target
}

// NewCache creates a session-scoped recon cache.
func NewCache() *Cache {
	return &Cache{
		results: make(map[string]*ReconSummary),
	}
}

// Get retrieves a cached result for a target.
func (c *Cache) Get(target string) (*ReconSummary, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	s, ok := c.results[target]
	return s, ok
}

// Set stores a recon summary for a target.
func (c *Cache) Set(target string, summary *ReconSummary) {
	c.mu.Lock()
	defer c.mu.Unlock()
	summary.CreatedAt = time.Now()
	c.results[target] = summary
}

// Clear removes all cached results.
func (c *Cache) Clear() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.results = make(map[string]*ReconSummary)
}

// Size returns the number of cached targets.
func (c *Cache) Size() int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return len(c.results)
}
