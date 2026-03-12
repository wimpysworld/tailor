package gh

import (
	"errors"
	"net/http"
	"os"
	"sync"

	"github.com/cli/go-gh/v2/pkg/api"
)

// tokenProbeResult caches the outcome of probing GET /user to distinguish
// installation tokens from PATs. Installation tokens (GITHUB_TOKEN in Actions)
// cannot call user-scoped endpoints and return 403.
type tokenProbeResult struct {
	once           sync.Once
	isInstallation bool
}

var tokenProbe tokenProbeResult

// ResetTokenProbe clears the cached probe result. Intended for tests only.
func ResetTokenProbe() {
	tokenProbe = tokenProbeResult{}
}

// IsInstallationToken returns true when the token associated with client
// appears to be a GitHub Actions installation token. Detection works by
// calling GET /user: installation tokens receive 403, PATs succeed.
//
// Outside GitHub Actions (GITHUB_ACTIONS != "true") this always returns false
// without making an API call, preserving local-run behaviour.
//
// The result is cached for the lifetime of the process.
func IsInstallationToken(client *api.RESTClient) bool {
	if os.Getenv("GITHUB_ACTIONS") != "true" {
		return false
	}

	tokenProbe.once.Do(func() {
		var resp userResponse
		if err := client.Get("user", &resp); err != nil {
			var httpErr *api.HTTPError
			if errors.As(err, &httpErr) && httpErr.StatusCode == http.StatusForbidden {
				tokenProbe.isInstallation = true
			}
			// Other errors (network, 401, etc.) leave isInstallation false,
			// which avoids suppressing fields unnecessarily.
		}
	})

	return tokenProbe.isInstallation
}
