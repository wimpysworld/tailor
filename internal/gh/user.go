package gh

import (
	"os"

	"github.com/cli/go-gh/v2/pkg/api"
)

// userResponse holds the subset of fields returned by GET /user.
type userResponse struct {
	Login string `json:"login"`
}

// FetchUsername returns the authenticated user's login.
func FetchUsername(client *api.RESTClient) (string, error) {
	if os.Getenv("GITHUB_ACTIONS") == "true" {
		if actor := os.Getenv("GITHUB_ACTOR"); actor != "" {
			return actor, nil
		}
	}

	var resp userResponse
	if err := client.Get("user", &resp); err != nil {
		return "", err
	}
	return resp.Login, nil
}
