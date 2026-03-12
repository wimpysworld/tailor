package gh

import (
	"os"

	"github.com/cli/go-gh/v2/pkg/api"
)

// userResponse holds the subset of fields returned by GET /user.
type userResponse struct {
	Login string `json:"login"`
}

// FetchUsername returns the authenticated user's login via GET /user.
// When running in GitHub Actions with an installation token (detected by
// probing GET /user for a 403), it falls back to GITHUB_REPOSITORY_OWNER.
func FetchUsername(client *api.RESTClient) (string, error) {
	var resp userResponse
	if err := client.Get("user", &resp); err != nil {
		// In GitHub Actions with an installation token, GET /user returns 403.
		// Fall back to the environment variable.
		if os.Getenv("GITHUB_ACTIONS") == "true" {
			if owner := os.Getenv("GITHUB_REPOSITORY_OWNER"); owner != "" {
				return owner, nil
			}
		}
		return "", err
	}
	return resp.Login, nil
}
