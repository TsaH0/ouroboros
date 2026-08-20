package scope

import (
	"net/url"

	"github.com/TsaH0/ouroboros/internal/model"
)

// Service is the public interface for scope evaluation.
// The proxy uses this interface for MITM decisions.
type Service interface {
	// Evaluate returns true if the URL is explicitly in scope.
	Evaluate(u *url.URL) bool
	// Status returns the tri-state scope status for a URL.
	Status(u *url.URL) model.ScopeStatus
}
