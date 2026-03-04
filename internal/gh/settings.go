package gh

import (
	"fmt"

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

	s.Description = ptr.String(repo.Description)
	s.Homepage = ptr.String(repo.Homepage)

	return s, nil
}
