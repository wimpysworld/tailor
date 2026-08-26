package gh

import (
	"errors"
	"fmt"
	"time"

	"github.com/cli/go-gh/v2/pkg/api"
	"github.com/cli/go-gh/v2/pkg/auth"
)

const apiRequestTimeout = 30 * time.Second

// tokenForHost wraps auth.TokenForHost for testability.
var tokenForHost = auth.TokenForHost

// newRESTClient wraps api.NewRESTClient for testability.
var newRESTClient = func(host string) (*api.RESTClient, error) {
	return api.NewRESTClient(restClientOptions(host))
}

func restClientOptions(host string) api.ClientOptions {
	return api.ClientOptions{Host: host, Timeout: apiRequestTimeout}
}

// ResolveHost returns the given host, or the go-gh default host when the
// given host is empty. The default host honours GH_HOST and falls back to
// github.com.
func ResolveHost(host string) string {
	if host != "" {
		return host
	}
	defaultHost, _ := auth.DefaultHost()
	return defaultHost
}

// CheckAuth checks that a GitHub authentication token is present for the
// given host. An empty host falls back to the default host (see
// ResolveHost). It does not validate the token against the API.
func CheckAuth(host string) error {
	token, _ := tokenForHost(ResolveHost(host))
	if token == "" {
		return errors.New("tailor requires GitHub authentication. Set the GH_TOKEN or GITHUB_TOKEN environment variable, or run 'gh auth login'")
	}
	return nil
}

// NewRESTClient creates a GitHub REST client bound to the given host.
// Callers must resolve an empty host with ResolveHost first.
func NewRESTClient(host string) (*api.RESTClient, error) {
	return newRESTClient(host)
}

// VerifyAuth checks that a token is present for the given host and verifies
// it against the API with a single GET /user request. It returns the REST
// client and the authenticated username so callers can reuse both without a
// second request. An empty host falls back to the default host (see
// ResolveHost).
func VerifyAuth(host string) (*api.RESTClient, string, error) {
	resolved := ResolveHost(host)
	if err := CheckAuth(resolved); err != nil {
		return nil, "", err
	}
	client, err := NewRESTClient(resolved)
	if err != nil {
		return nil, "", fmt.Errorf("creating GitHub API client: %w", err)
	}
	username, err := FetchUsername(client)
	if err != nil {
		return nil, "", fmt.Errorf("verifying GitHub authentication: %w", err)
	}
	return client, username, nil
}
