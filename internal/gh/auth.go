package gh

import (
	"errors"

	"github.com/cli/go-gh/v2/pkg/auth"
)

// tokenForHost wraps auth.TokenForHost for testability.
var tokenForHost = auth.TokenForHost

// CheckAuth verifies that a valid GitHub authentication token is available for github.com.
// It returns an error if no valid token is available.
func CheckAuth() error {
	token, _ := tokenForHost("github.com")
	if token == "" {
		return errors.New("tailor requires GitHub authentication. Set the GH_TOKEN or GITHUB_TOKEN environment variable, or run 'gh auth login'")
	}
	return nil
}
