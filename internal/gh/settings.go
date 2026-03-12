package gh

import (
	"bytes"
	"encoding/json"
	"fmt"
	"reflect"
	"strings"

	"github.com/cli/go-gh/v2/pkg/api"
	"github.com/wimpysworld/tailor/internal/model"
	"github.com/wimpysworld/tailor/internal/ptr"
)

// installationTokenUnreliableFields lists repo response fields that GitHub
// Actions installation tokens (GITHUB_TOKEN / secrets.GITHUB_TOKEN) return as
// zero values (false / empty string) regardless of the actual repository
// configuration. Comparing these against the user's config produces false
// positives ("would set" when the repo is already correct).
//
// The operation name is used as the key in readWarningOperationFields
// (internal/alter/settings.go) to suppress WouldSet entries for these fields.
const InstallationTokenReadOp = "read repo settings (installation token)" //nolint:gosec // not a credential

var installationTokenUnreliableFields = map[string]bool{
	"allow_auto_merge":            true,
	"allow_rebase_merge":          true,
	"allow_squash_merge":          true,
	"allow_update_branch":         true,
	"delete_branch_on_merge":      true,
	"squash_merge_commit_message": true,
	"squash_merge_commit_title":   true,
}

// repoResponse holds the subset of GitHub repository fields we read.
type repoResponse struct {
	Description              string   `json:"description"`
	Homepage                 string   `json:"homepage"`
	HasWiki                  bool     `json:"has_wiki"`
	HasDiscussions           bool     `json:"has_discussions"`
	HasProjects              bool     `json:"has_projects"`
	HasIssues                bool     `json:"has_issues"`
	AllowMergeCommit         bool     `json:"allow_merge_commit"`
	AllowSquashMerge         bool     `json:"allow_squash_merge"`
	AllowRebaseMerge         bool     `json:"allow_rebase_merge"`
	SquashMergeCommitTitle   string   `json:"squash_merge_commit_title"`
	SquashMergeCommitMessage string   `json:"squash_merge_commit_message"`
	MergeCommitTitle         string   `json:"merge_commit_title"`
	MergeCommitMessage       string   `json:"merge_commit_message"`
	DeleteBranchOnMerge      bool     `json:"delete_branch_on_merge"`
	AllowUpdateBranch        bool     `json:"allow_update_branch"`
	AllowAutoMerge           bool     `json:"allow_auto_merge"`
	WebCommitSignoffRequired bool     `json:"web_commit_signoff_required"`
	Topics                   []string `json:"topics"`
}

// workflowPermissionsResponse holds the Actions workflow permission settings.
type workflowPermissionsResponse struct {
	DefaultWorkflowPermissions   string `json:"default_workflow_permissions"`
	CanApprovePullRequestReviews bool   `json:"can_approve_pull_request_reviews"`
}

// ReadRepoSettings fetches repository settings from the GitHub API and returns
// them as a model.RepositorySettings. It makes separate API calls for the
// standard repository fields and Actions workflow permissions.
//
// The returned warnings slice contains classified access errors
// (ErrInsufficientScope, ErrInsufficientRole) for sub-calls that returned 403.
// The corresponding fields in the returned settings are left nil. Callers can
// log these warnings or ignore them.
func ReadRepoSettings(client *api.RESTClient, owner, name string) (*model.RepositorySettings, []error, error) {
	var repo repoResponse
	if err := client.Get(fmt.Sprintf("repos/%s/%s", owner, name), &repo); err != nil {
		return nil, nil, fmt.Errorf("fetching repo settings: %w", err)
	}

	s := &model.RepositorySettings{
		Description:              ptr.Ptr(repo.Description),
		Homepage:                 ptr.Ptr(repo.Homepage),
		HasWiki:                  ptr.Ptr(repo.HasWiki),
		HasDiscussions:           ptr.Ptr(repo.HasDiscussions),
		HasProjects:              ptr.Ptr(repo.HasProjects),
		HasIssues:                ptr.Ptr(repo.HasIssues),
		AllowMergeCommit:         ptr.Ptr(repo.AllowMergeCommit),
		AllowSquashMerge:         ptr.Ptr(repo.AllowSquashMerge),
		AllowRebaseMerge:         ptr.Ptr(repo.AllowRebaseMerge),
		SquashMergeCommitTitle:   ptr.Ptr(repo.SquashMergeCommitTitle),
		SquashMergeCommitMessage: ptr.Ptr(repo.SquashMergeCommitMessage),
		MergeCommitTitle:         ptr.Ptr(repo.MergeCommitTitle),
		MergeCommitMessage:       ptr.Ptr(repo.MergeCommitMessage),
		DeleteBranchOnMerge:      ptr.Ptr(repo.DeleteBranchOnMerge),
		AllowUpdateBranch:        ptr.Ptr(repo.AllowUpdateBranch),
		AllowAutoMerge:           ptr.Ptr(repo.AllowAutoMerge),
		Topics:                   &repo.Topics,
		WebCommitSignoffRequired: ptr.Ptr(repo.WebCommitSignoffRequired),
	}

	// When using a GitHub Actions installation token, the API returns zero
	// values for certain fields. Nil them out and emit a synthetic warning
	// so the comparison layer skips them instead of producing false diffs.
	// IsInstallationToken probes GET /user to distinguish installation
	// tokens from PATs; the result is cached per process.
	var warnings []error
	if IsInstallationToken(client) {
		nilUnreliableFields(s)
		warnings = append(warnings, &ErrInsufficientScope{
			Operation: InstallationTokenReadOp,
			Message:   "installation token returns unreliable values for merge/branch settings",
		})
	}

	var wfPerms workflowPermissionsResponse
	if err := client.Get(fmt.Sprintf("repos/%s/%s/actions/permissions/workflow", owner, name), &wfPerms); err != nil {
		classified := classifyHTTPError(err, "fetch workflow permissions")
		if isAccessError(classified) {
			warnings = append(warnings, classified)
		} else {
			return nil, nil, fmt.Errorf("fetching workflow permissions: %w", err)
		}
	} else {
		s.DefaultWorkflowPermissions = ptr.Ptr(wfPerms.DefaultWorkflowPermissions)
		s.CanApprovePullRequestReviews = ptr.Ptr(wfPerms.CanApprovePullRequestReviews)
	}

	return s, warnings, nil
}

// SkippedOperation records a sub-operation that was skipped due to
// insufficient token scope or repository role.
type SkippedOperation struct {
	Operation string // e.g. "set workflow permissions"
	Err       error  // *ErrInsufficientScope or *ErrInsufficientRole
}

// ApplyResult collects the outcome of ApplyRepoSettings. Skipped lists
// operations that failed with access errors and were gracefully skipped.
type ApplyResult struct {
	Skipped []SkippedOperation
}

// ApplyRepoSettings sends a PATCH /repos/{owner}/{repo} with the declared
// settings. It also handles fields that require separate API endpoints:
// topics and Actions workflow permissions. Access errors (insufficient scope
// or role) are collected in the returned ApplyResult rather than aborting.
// Hard errors still return as the error value.
func ApplyRepoSettings(client *api.RESTClient, owner, name string, settings *model.RepositorySettings) (*ApplyResult, error) {
	p := buildSettingsPayload(settings)
	result := &ApplyResult{}

	if len(p.Body) > 0 {
		payload, err := json.Marshal(p.Body)
		if err != nil {
			return nil, fmt.Errorf("marshalling repo settings: %w", err)
		}
		if err := client.Patch(fmt.Sprintf("repos/%s/%s", owner, name), bytes.NewReader(payload), nil); err != nil {
			classified := classifyHTTPError(err, "patch repo settings")
			if isAccessError(classified) {
				result.Skipped = append(result.Skipped, SkippedOperation{Operation: "patch repo settings", Err: classified})
			} else {
				return nil, fmt.Errorf("patching repo settings: %w", err)
			}
		}
	}

	if p.DefaultWorkflowPermissions != nil || p.CanApprovePullRequestReviews != nil {
		if err := applyWorkflowPermissions(client, owner, name, p); err != nil {
			classified := classifyHTTPError(err, "set workflow permissions")
			if isAccessError(classified) {
				result.Skipped = append(result.Skipped, SkippedOperation{Operation: "set workflow permissions", Err: classified})
			} else {
				return nil, err
			}
		}
	}

	if p.Topics != nil {
		topicsBody := struct {
			Names []string `json:"names"`
		}{Names: *p.Topics}
		payload, err := json.Marshal(topicsBody)
		if err != nil {
			return nil, fmt.Errorf("marshalling topics: %w", err)
		}
		if err := client.Put(fmt.Sprintf("repos/%s/%s/topics", owner, name), bytes.NewReader(payload), nil); err != nil {
			classified := classifyHTTPError(err, "set topics")
			if isAccessError(classified) {
				result.Skipped = append(result.Skipped, SkippedOperation{Operation: "set topics", Err: classified})
			} else {
				return nil, fmt.Errorf("setting topics: %w", err)
			}
		}
	}

	return result, nil
}

// applyWorkflowPermissions sends a PUT to the Actions workflow permissions
// endpoint. The endpoint replaces both fields atomically, so when only one
// field is declared in the config, the other is fetched from the current
// repository state.
func applyWorkflowPermissions(client *api.RESTClient, owner, name string, p settingsPayload) error {
	wfpPath := fmt.Sprintf("repos/%s/%s/actions/permissions/workflow", owner, name)

	perms := p.DefaultWorkflowPermissions
	approve := p.CanApprovePullRequestReviews

	// When one field is missing, read the current value from the API so the
	// PUT body is always complete.
	if perms == nil || approve == nil {
		var current workflowPermissionsResponse
		if err := client.Get(wfpPath, &current); err != nil {
			return fmt.Errorf("fetching current workflow permissions: %w", err)
		}
		if perms == nil {
			perms = &current.DefaultWorkflowPermissions
		}
		if approve == nil {
			approve = &current.CanApprovePullRequestReviews
		}
	}

	wfpBody := map[string]any{
		"default_workflow_permissions":     *perms,
		"can_approve_pull_request_reviews": *approve,
	}
	payload, err := json.Marshal(wfpBody)
	if err != nil {
		return fmt.Errorf("marshalling workflow permissions: %w", err)
	}
	if err := client.Put(wfpPath, bytes.NewReader(payload), nil); err != nil {
		return fmt.Errorf("setting workflow permissions: %w", err)
	}
	return nil
}

// settingsPayload holds the separated output of buildSettingsPayload. Fields
// that require their own API endpoints are extracted from the PATCH body.
type settingsPayload struct {
	// Body is the map sent as PATCH /repos/{owner}/{repo}.
	Body map[string]any
	// Topics is non-nil when the field is declared.
	Topics *[]string
	// DefaultWorkflowPermissions is non-nil when the field is declared.
	DefaultWorkflowPermissions *string
	// CanApprovePullRequestReviews is non-nil when the field is declared.
	CanApprovePullRequestReviews *bool
}

// nonPatchFields lists yaml keys that must not appear in the PATCH body
// because they are managed by separate API endpoints.
var nonPatchFields = map[string]bool{
	"topics":                           true,
	"default_workflow_permissions":     true,
	"can_approve_pull_request_reviews": true,
}

// buildSettingsPayload uses reflection to build a map of non-nil fields from
// settings, keyed by their yaml tags. Fields that require separate API
// endpoints are extracted into the returned settingsPayload struct and never
// appear in the PATCH body.
func buildSettingsPayload(settings *model.RepositorySettings) settingsPayload {
	p := settingsPayload{Body: make(map[string]any)}

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
		if fv.Kind() != reflect.Pointer || fv.IsNil() {
			continue
		}

		if nonPatchFields[key] {
			switch key {
			case "topics":
				s := fv.Elem().Interface().([]string)
				p.Topics = &s
			case "default_workflow_permissions":
				s := fv.Elem().String()
				p.DefaultWorkflowPermissions = &s
			case "can_approve_pull_request_reviews":
				b := fv.Elem().Bool()
				p.CanApprovePullRequestReviews = &b
			}
			continue
		}

		p.Body[key] = fv.Elem().Interface()
	}

	return p
}

// nilUnreliableFields sets pointer fields in s to nil when their YAML tag
// matches installationTokenUnreliableFields. This prevents false-positive
// diffs when the API returns zero values instead of the actual configuration.
func nilUnreliableFields(s *model.RepositorySettings) {
	rv := reflect.ValueOf(s).Elem()
	rt := rv.Type()
	for i := range rt.NumField() {
		f := rt.Field(i)
		tag := f.Tag.Get("yaml")
		if tag == "" || tag == ",inline" {
			continue
		}
		key, _, _ := strings.Cut(tag, ",")
		if installationTokenUnreliableFields[key] {
			rv.Field(i).Set(reflect.Zero(f.Type))
		}
	}
}
