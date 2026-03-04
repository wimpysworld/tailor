package gh

import (
	"errors"

	"github.com/cli/go-gh/v2/pkg/auth"
)

// tokenForHost wraps auth.TokenForHost for testability.
var tokenForHost = func(host string) (string, string) {
	return auth.TokenForHost(host)
}

// CheckAuth verifies that the GitHub CLI is authenticated for github.com.
// It returns an error if no valid token is available.
func CheckAuth() error {
	token, _ := tokenForHost("github.com")
	if token == "" {
		return errors.New("tailor requires an authenticated GitHub CLI; run 'gh auth login' to authenticate")
	}
	return nil
}
