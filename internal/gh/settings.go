package gh

import (
	"bytes"
	"encoding/json"
	"fmt"
	"reflect"
	"strings"

	"github.com/cli/go-gh/v2/pkg/api"
	"github.com/wimpysworld/tailor/internal/config"
	"github.com/wimpysworld/tailor/internal/ptr"
)

// repoResponse holds the subset of GitHub repository fields we read.
type repoResponse struct {
	Description              string `json:"description"`
	Homepage                 string `json:"homepage"`
	HasWiki                  bool   `json:"has_wiki"`
	HasDiscussions           bool   `json:"has_discussions"`
	HasProjects              bool   `json:"has_projects"`
	HasIssues                bool   `json:"has_issues"`
	AllowMergeCommit         bool   `json:"allow_merge_commit"`
	AllowSquashMerge         bool   `json:"allow_squash_merge"`
	AllowRebaseMerge         bool   `json:"allow_rebase_merge"`
	SquashMergeCommitTitle   string `json:"squash_merge_commit_title"`
	SquashMergeCommitMessage string `json:"squash_merge_commit_message"`
	MergeCommitTitle         string `json:"merge_commit_title"`
	MergeCommitMessage       string `json:"merge_commit_message"`
	DeleteBranchOnMerge      bool   `json:"delete_branch_on_merge"`
	AllowUpdateBranch        bool   `json:"allow_update_branch"`
	AllowAutoMerge           bool   `json:"allow_auto_merge"`
	WebCommitSignoffRequired bool   `json:"web_commit_signoff_required"`
}

// vulnerabilityReportingResponse holds the private vulnerability reporting status.
type vulnerabilityReportingResponse struct {
	Enabled bool `json:"enabled"`
}

// ReadRepoSettings fetches repository settings from the GitHub API and returns
// them as a config.RepositorySettings. It makes two API calls: one for the
// standard repository fields and one for private vulnerability reporting.
func ReadRepoSettings(client *api.RESTClient, owner, name string) (*config.RepositorySettings, error) {
	var repo repoResponse
	if err := client.Get(fmt.Sprintf("repos/%s/%s", owner, name), &repo); err != nil {
		return nil, fmt.Errorf("fetching repo settings: %w", err)
	}

	var pvr vulnerabilityReportingResponse
	if err := client.Get(fmt.Sprintf("repos/%s/%s/private-vulnerability-reporting", owner, name), &pvr); err != nil {
		return nil, fmt.Errorf("fetching private vulnerability reporting: %w", err)
	}

	s := &config.RepositorySettings{
		Description:                      ptr.String(repo.Description),
		Homepage:                         ptr.String(repo.Homepage),
		HasWiki:                          ptr.Bool(repo.HasWiki),
		HasDiscussions:                   ptr.Bool(repo.HasDiscussions),
		HasProjects:                      ptr.Bool(repo.HasProjects),
		HasIssues:                        ptr.Bool(repo.HasIssues),
		AllowMergeCommit:                 ptr.Bool(repo.AllowMergeCommit),
		AllowSquashMerge:                 ptr.Bool(repo.AllowSquashMerge),
		AllowRebaseMerge:                 ptr.Bool(repo.AllowRebaseMerge),
		SquashMergeCommitTitle:           ptr.String(repo.SquashMergeCommitTitle),
		SquashMergeCommitMessage:         ptr.String(repo.SquashMergeCommitMessage),
		MergeCommitTitle:                 ptr.String(repo.MergeCommitTitle),
		MergeCommitMessage:               ptr.String(repo.MergeCommitMessage),
		DeleteBranchOnMerge:              ptr.Bool(repo.DeleteBranchOnMerge),
		AllowUpdateBranch:                ptr.Bool(repo.AllowUpdateBranch),
		AllowAutoMerge:                   ptr.Bool(repo.AllowAutoMerge),
		WebCommitSignoffRequired:         ptr.Bool(repo.WebCommitSignoffRequired),
		PrivateVulnerabilityReportEnabled: ptr.Bool(pvr.Enabled),
	}

	return s, nil
}

// ApplyRepoSettings sends a PATCH /repos/{owner}/{repo} with the declared
// settings. It also handles private_vulnerability_reporting_enabled via its
// separate PUT/DELETE endpoint. Returns an error if any API call fails.
func ApplyRepoSettings(client *api.RESTClient, owner, name string, settings *config.RepositorySettings) error {
	body, pvr := buildSettingsPayload(settings)

	if len(body) > 0 {
		payload, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("marshalling repo settings: %w", err)
		}
		if err := client.Patch(fmt.Sprintf("repos/%s/%s", owner, name), bytes.NewReader(payload), nil); err != nil {
			return fmt.Errorf("patching repo settings: %w", err)
		}
	}

	if pvr != nil {
		pvrPath := fmt.Sprintf("repos/%s/%s/private-vulnerability-reporting", owner, name)
		if *pvr {
			if err := client.Put(pvrPath, bytes.NewReader([]byte("{}")), nil); err != nil {
				return fmt.Errorf("enabling private vulnerability reporting: %w", err)
			}
		} else {
			if err := client.Delete(pvrPath, nil); err != nil {
				return fmt.Errorf("disabling private vulnerability reporting: %w", err)
			}
		}
	}

	return nil
}

// buildSettingsPayload uses reflection to build a map of non-nil fields from
// settings, keyed by their yaml tags. It returns the map for the PATCH body
// and a separate *bool for the private vulnerability reporting field.
func buildSettingsPayload(settings *config.RepositorySettings) (map[string]any, *bool) {
	body := make(map[string]any)
	var pvr *bool

	v := reflect.ValueOf(settings).Elem()
	t := v.Type()

	for i := range t.NumField() {
		field := t.Field(i)
		tag := field.Tag.Get("yaml")
		if tag == "" || tag == ",inline" {
			continue
		}
		// Strip ",omitempty" suffix to get the bare key.
		key, _, _ := strings.Cut(tag, ",")

		fv := v.Field(i)
		if fv.Kind() != reflect.Ptr || fv.IsNil() {
			continue
		}

		if key == "private_vulnerability_reporting_enabled" {
			b := fv.Elem().Bool()
			pvr = &b
			continue
		}

		body[key] = fv.Elem().Interface()
	}

	return body, pvr
}
