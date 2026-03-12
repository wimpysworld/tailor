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
// In GitHub Actions, it returns GITHUB_REPOSITORY_OWNER without an API call.
func FetchUsername(client *api.RESTClient) (string, error) {
	if os.Getenv("GITHUB_ACTIONS") == "true" {
		if owner := os.Getenv("GITHUB_REPOSITORY_OWNER"); owner != "" {
			return owner, nil
		}
	}

	var resp userResponse
	if err := client.Get("user", &resp); err != nil {
		return "", err
	}
	return resp.Login, nil
}
