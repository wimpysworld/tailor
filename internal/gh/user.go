package gh

import (
	"github.com/cli/go-gh/v2/pkg/api"
)

// userResponse holds the subset of fields returned by GET /user.
type userResponse struct {
	Login string `json:"login"`
}

// FetchUsername returns the authenticated user's login via GET /user.
func FetchUsername(client *api.RESTClient) (string, error) {
	var resp userResponse
	if err := client.Get("user", &resp); err != nil {
		return "", err
	}
	return resp.Login, nil
}
