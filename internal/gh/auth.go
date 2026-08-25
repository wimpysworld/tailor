package gh

import (
	"errors"

	"github.com/cli/go-gh/v2/pkg/api"
	"github.com/cli/go-gh/v2/pkg/auth"
)

// tokenForHost wraps auth.TokenForHost for testability.
var tokenForHost = auth.TokenForHost

// CheckAuth checks that a GitHub authentication token is present for the
// given host. An empty host falls back to github.com. It does not validate
// the token against the API.
func CheckAuth(host string) error {
	if host == "" {
		host = "github.com"
	}
	token, _ := tokenForHost(host)
	if token == "" {
		return errors.New("tailor requires GitHub authentication. Set the GH_TOKEN or GITHUB_TOKEN environment variable, or run 'gh auth login'")
	}
	return nil
}

// NewRESTClient creates a GitHub REST client bound to the given host.
// An empty host uses the go-gh default host resolution.
func NewRESTClient(host string) (*api.RESTClient, error) {
	return api.NewRESTClient(api.ClientOptions{Host: host})
}
