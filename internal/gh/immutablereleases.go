package gh

import (
	"errors"
	"fmt"
	"net/http"

	"github.com/cli/go-gh/v2/pkg/api"
)

// ImmutableReleasesState includes the read-only owner enforcement flag.
type ImmutableReleasesState struct {
	Enabled         *bool `json:"enabled"`
	EnforcedByOwner *bool `json:"enforced_by_owner"`
}

// ReadImmutableReleases reads the toggle without changing any release.
func ReadImmutableReleases(client *api.RESTClient, owner, name string) (*ImmutableReleasesState, error) {
	path := fmt.Sprintf("repos/%s/%s", owner, name)
	var state ImmutableReleasesState
	err := boundedHTTPError(client.Get(path+"/immutable-releases", &state))
	if err == nil {
		if state.Enabled == nil || state.EnforcedByOwner == nil {
			return nil, fmt.Errorf("immutable releases response is missing enabled or enforced_by_owner")
		}
		return &state, nil
	}
	var httpErr *api.HTTPError
	if errors.As(err, &httpErr) && httpErr.StatusCode == http.StatusNotFound {
		// GitHub also hides inaccessible endpoints with 404. Confirm both
		// repository admin access and token Administration-read access.
		var repo repoResponse
		if probeErr := boundedHTTPError(client.Get(path, &repo)); probeErr != nil {
			return nil, classifyHTTPError(probeErr, Op(OpFetchImmutableReleases))
		}
		if repo.Permissions.Admin {
			var permissions workflowPermissionsResponse
			if probeErr := boundedHTTPError(client.Get(path+"/actions/permissions/workflow", &permissions)); probeErr != nil {
				return nil, classifyHTTPError(probeErr, Op(OpFetchImmutableReleases))
			}
			return &ImmutableReleasesState{Enabled: new(false), EnforcedByOwner: new(false)}, nil
		}
	}
	return nil, classifyHTTPError(err, Op(OpFetchImmutableReleases))
}

// ApplyImmutableReleases changes the repository toggle, not existing releases.
func ApplyImmutableReleases(client *api.RESTClient, owner, name string, enabled bool) error {
	method := http.MethodDelete
	if enabled {
		method = http.MethodPut
	}
	err := boundedHTTPError(client.Do(method, fmt.Sprintf("repos/%s/%s/immutable-releases", owner, name), nil, nil))
	if err != nil {
		return fmt.Errorf("setting immutable releases: %w", classifyHTTPError(err, SecurityFeatureOp(enabled, OpSetImmutableReleases)))
	}
	return nil
}
