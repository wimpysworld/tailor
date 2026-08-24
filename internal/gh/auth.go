package gh

import (
	"errors"

	"github.com/cli/go-gh/v2/pkg/auth"
)

// tokenForHost wraps auth.TokenForHost for testability.
var tokenForHost = auth.TokenForHost

// CheckAuth checks that a GitHub authentication token is present for
// github.com. It does not validate the token against the API.
func CheckAuth() error {
	token, _ := tokenForHost("github.com")
	if token == "" {
		return errors.New("tailor requires GitHub authentication. Set the GH_TOKEN or GITHUB_TOKEN environment variable, or run 'gh auth login'")
	}
	return nil
}
