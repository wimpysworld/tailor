package gh

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"

	"github.com/cli/go-gh/v2/pkg/api"
)

type PagesEnvironmentState struct {
	Missing   bool
	AddBranch bool
	Branch    string
	verified  bool
	access    bool
}

type pagesBranchPolicy struct {
	ID   int64  `json:"id"`
	Name string `json:"name"`
	Type string `json:"type"`
}

func pagesEnvironmentPath(owner, name string) string {
	return fmt.Sprintf("repos/%s/%s/environments/github-pages", owner, name)
}

// ReadPagesEnvironment checks only the fixed Pages environment. accessProven
// must come from PagesRepositoryState.AdminAccess, not a public repository read.
func ReadPagesEnvironment(client *api.RESTClient, owner, name, branch string, accessProven bool) (*PagesEnvironmentState, error) {
	if branch == "" {
		return nil, fmt.Errorf("checking github-pages environment: branch is empty")
	}
	var response struct {
		Name   string          `json:"name"`
		Policy json.RawMessage `json:"deployment_branch_policy"`
	}
	err := boundedHTTPError(client.Get(pagesEnvironmentPath(owner, name), &response))
	var httpErr *api.HTTPError
	if errors.As(err, &httpErr) && httpErr.StatusCode == http.StatusNotFound && accessProven {
		return &PagesEnvironmentState{Missing: true, AddBranch: true, Branch: branch, verified: true, access: true}, nil
	}
	if err != nil {
		return nil, classifyHTTPError(err, Op(OpGetPagesEnvironment))
	}
	if response.Name != "github-pages" || len(response.Policy) == 0 {
		return nil, fmt.Errorf("checking github-pages environment: incomplete response")
	}
	state := &PagesEnvironmentState{Branch: branch, verified: true, access: accessProven}
	if string(response.Policy) == "null" {
		return state, nil
	}
	var policy struct {
		Protected *bool `json:"protected_branches"`
		Custom    *bool `json:"custom_branch_policies"`
	}
	if err := json.Unmarshal(response.Policy, &policy); err != nil {
		return nil, fmt.Errorf("decoding github-pages branch policy: %w", err)
	}
	if policy.Protected == nil || policy.Custom == nil || (*policy.Protected && *policy.Custom) {
		return nil, fmt.Errorf("checking github-pages environment: incompatible branch protection")
	}
	if *policy.Protected {
		var metadata struct {
			Protected *bool `json:"protected"`
		}
		path := fmt.Sprintf("repos/%s/%s/branches/%s", owner, name, url.PathEscape(branch))
		if err := boundedHTTPError(client.Get(path, &metadata)); err != nil {
			return nil, classifyHTTPError(err, Op(OpGetPagesBranch))
		}
		if metadata.Protected == nil || !*metadata.Protected {
			return nil, fmt.Errorf("github-pages environment requires a protected branch: %q is not eligible", branch)
		}
		return state, nil
	}
	if !*policy.Custom {
		return state, nil
	}
	policies, err := readPagesEnvironmentPolicies(client, owner, name)
	if err != nil {
		return nil, err
	}
	state.AddBranch = true
	for _, entry := range policies {
		if entry.Name == branch && entry.Type == "branch" {
			state.AddBranch = false
		}
	}
	return state, nil
}

func readPagesEnvironmentPolicies(client *api.RESTClient, owner, name string) ([]pagesBranchPolicy, error) {
	var all []pagesBranchPolicy
	seen := make(map[int64]bool)
	for page := 1; page <= 1000; page++ {
		path := fmt.Sprintf("%s/deployment-branch-policies?per_page=100&page=%d", pagesEnvironmentPath(owner, name), page)
		response, err := client.RequestWithContext(context.Background(), http.MethodGet, path, nil)
		if err != nil {
			return nil, classifyHTTPError(boundedHTTPError(err), Op(OpListPagesEnvironmentPolicies))
		}
		var body struct {
			Total    *int                 `json:"total_count"`
			Policies *[]pagesBranchPolicy `json:"branch_policies"`
		}
		err = json.NewDecoder(response.Body).Decode(&body)
		_ = response.Body.Close()
		if err != nil {
			return nil, fmt.Errorf("decoding github-pages branch policies: %w", err)
		}
		if body.Total == nil || *body.Total < 0 || body.Policies == nil {
			return nil, fmt.Errorf("decoding github-pages branch policies: incomplete response")
		}
		for _, entry := range *body.Policies {
			if entry.ID <= 0 || entry.Name == "" || (entry.Type != "branch" && entry.Type != "tag") || seen[entry.ID] {
				return nil, fmt.Errorf("decoding github-pages branch policies: invalid or duplicate entry")
			}
			seen[entry.ID] = true
			all = append(all, entry)
		}
		if !hasNextPage(response.Header.Get("Link")) && len(all) == *body.Total {
			return all, nil
		}
		if len(*body.Policies) == 0 || len(all) > *body.Total {
			return nil, fmt.Errorf("reading github-pages branch policies: incomplete collection")
		}
	}
	return nil, fmt.Errorf("reading github-pages branch policies: pagination limit exceeded")
}

// ApplyPagesEnvironment preserves existing settings and returns confirmed writes
// even when a later request fails. It accepts only a successful preflight read.
func ApplyPagesEnvironment(client *api.RESTClient, owner, name string, state *PagesEnvironmentState) (*ApplyResult, error) {
	result := &ApplyResult{}
	if state == nil || !state.verified {
		return result, fmt.Errorf("applying github-pages environment: preflight is unavailable")
	}
	if !state.Missing && !state.AddBranch {
		return result, nil
	}
	// Recheck before writes so an environment created since preflight keeps its policy.
	current, err := ReadPagesEnvironment(client, owner, name, state.Branch, state.access)
	if err != nil {
		return result, err
	}
	path := pagesEnvironmentPath(owner, name)
	if current.Missing {
		body := map[string]any{"deployment_branch_policy": map[string]bool{"protected_branches": false, "custom_branch_policies": true}}
		if err := sendJSON(client, http.MethodPut, path, body); err != nil {
			var httpErr *api.HTTPError
			if !errors.As(err, &httpErr) || (httpErr.StatusCode != http.StatusConflict && httpErr.StatusCode != http.StatusUnprocessableEntity) {
				return result, classifyHTTPError(err, Op(OpPutPagesEnvironment))
			}
			recheck, readErr := ReadPagesEnvironment(client, owner, name, current.Branch, state.access)
			if readErr != nil {
				return result, readErr
			}
			if recheck.Missing {
				return result, classifyHTTPError(err, Op(OpPutPagesEnvironment))
			}
			current = recheck
		} else {
			result.Applied = append(result.Applied, Op(OpPutPagesEnvironment))
		}
	}
	if !current.AddBranch {
		return result, nil
	}
	err = sendJSON(client, http.MethodPost, path+"/deployment-branch-policies", map[string]string{"name": current.Branch, "type": "branch"})
	if err == nil {
		result.Applied = append(result.Applied, Op(OpPostPagesEnvironmentPolicy))
		return result, nil
	}
	var httpErr *api.HTTPError
	if errors.As(err, &httpErr) && (httpErr.StatusCode == http.StatusConflict || httpErr.StatusCode == http.StatusUnprocessableEntity) {
		// A concurrent writer can add the same policy. Confirm the final state once.
		recheck, readErr := ReadPagesEnvironment(client, owner, name, current.Branch, state.access)
		if readErr != nil {
			return result, readErr
		}
		if !recheck.Missing && !recheck.AddBranch {
			return result, nil
		}
	}
	return result, classifyHTTPError(err, Op(OpPostPagesEnvironmentPolicy))
}
