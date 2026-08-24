package gh

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"

	"github.com/cli/go-gh/v2/pkg/api"
	"github.com/wimpysworld/tailor/internal/model"
)

// repoResponse holds the subset of GitHub repository fields read from the API.
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
	Permissions              struct {
		Admin bool `json:"admin"`
	} `json:"permissions"`
}

type securityFeatureResponse struct {
	Enabled *bool `json:"enabled"`
}

// workflowPermissionsResponse holds the Actions workflow permission settings.
type workflowPermissionsResponse struct {
	DefaultWorkflowPermissions   string `json:"default_workflow_permissions"`
	CanApprovePullRequestReviews bool   `json:"can_approve_pull_request_reviews"`
}

// ReadRepoSettings fetches repository settings from the GitHub API and returns
// them as a model.RepositorySettings. It makes separate API calls for the
// standard repository fields, security features, and Actions workflow permissions.
//
// The returned warnings slice contains classified access errors
// (ErrInsufficientScope) for sub-calls that returned 403 or an ambiguous 404.
// The corresponding fields in the returned settings are left nil. Callers can
// log these warnings or ignore them.
func ReadRepoSettings(client *api.RESTClient, owner, name string) (*model.RepositorySettings, []error, error) {
	var repo repoResponse
	if err := client.Get(fmt.Sprintf("repos/%s/%s", owner, name), &repo); err != nil {
		return nil, nil, fmt.Errorf("fetching repo settings: %w", err)
	}

	s := &model.RepositorySettings{
		Description:              new(repo.Description),
		Homepage:                 new(repo.Homepage),
		HasWiki:                  new(repo.HasWiki),
		HasDiscussions:           new(repo.HasDiscussions),
		HasProjects:              new(repo.HasProjects),
		HasIssues:                new(repo.HasIssues),
		AllowMergeCommit:         new(repo.AllowMergeCommit),
		AllowSquashMerge:         new(repo.AllowSquashMerge),
		AllowRebaseMerge:         new(repo.AllowRebaseMerge),
		SquashMergeCommitTitle:   new(repo.SquashMergeCommitTitle),
		SquashMergeCommitMessage: new(repo.SquashMergeCommitMessage),
		MergeCommitTitle:         new(repo.MergeCommitTitle),
		MergeCommitMessage:       new(repo.MergeCommitMessage),
		DeleteBranchOnMerge:      new(repo.DeleteBranchOnMerge),
		AllowUpdateBranch:        new(repo.AllowUpdateBranch),
		AllowAutoMerge:           new(repo.AllowAutoMerge),
		Topics:                   &repo.Topics,
		WebCommitSignoffRequired: new(repo.WebCommitSignoffRequired),
	}

	var warnings []error
	adminRead := false
	var wfPerms workflowPermissionsResponse
	if err := client.Get(fmt.Sprintf("repos/%s/%s/actions/permissions/workflow", owner, name), &wfPerms); err != nil {
		classified := classifyHTTPError(err, "fetch workflow permissions")
		if isAccessError(classified) {
			warnings = append(warnings, classified)
		} else {
			return nil, nil, fmt.Errorf("fetching workflow permissions: %w", err)
		}
	} else {
		adminRead = true
		s.DefaultWorkflowPermissions = new(wfPerms.DefaultWorkflowPermissions)
		s.CanApprovePullRequestReviews = new(wfPerms.CanApprovePullRequestReviews)
	}

	securityFeatures := []struct {
		path             string
		operation        string
		statusOnly       bool
		allow404Disabled bool
		set              func(bool)
	}{
		{
			path:      fmt.Sprintf("repos/%s/%s/private-vulnerability-reporting", owner, name),
			operation: "fetch private vulnerability reporting",
			set:       func(enabled bool) { s.PrivateVulnerabilityReportEnabled = new(enabled) },
		},
		{
			path:             fmt.Sprintf("repos/%s/%s/vulnerability-alerts", owner, name),
			operation:        "fetch vulnerability alerts",
			statusOnly:       true,
			allow404Disabled: adminRead && repo.Permissions.Admin,
			set:              func(enabled bool) { s.VulnerabilityAlertsEnabled = new(enabled) },
		},
		{
			path:             fmt.Sprintf("repos/%s/%s/automated-security-fixes", owner, name),
			operation:        "fetch automated security fixes",
			allow404Disabled: adminRead && repo.Permissions.Admin,
			set:              func(enabled bool) { s.AutomatedSecurityFixesEnabled = new(enabled) },
		},
	}
	for _, feature := range securityFeatures {
		enabled, known, err := readSecurityFeature(client, feature.path, feature.statusOnly, feature.allow404Disabled)
		if known {
			feature.set(enabled)
			continue
		}
		classified := classifyHTTPError(err, feature.operation)
		if isAccessError(classified) {
			warnings = append(warnings, classified)
			continue
		}
		return nil, nil, fmt.Errorf("%s: %w", feature.operation, err)
	}

	return s, warnings, nil
}

func readSecurityFeature(client *api.RESTClient, path string, statusOnly, allow404Disabled bool) (enabled bool, known bool, err error) {
	var response any
	var feature securityFeatureResponse
	if !statusOnly {
		response = &feature
	}
	if err := client.Get(path, response); err != nil {
		var httpErr *api.HTTPError
		if errors.As(err, &httpErr) && httpErr.StatusCode == http.StatusNotFound && allow404Disabled {
			return false, true, nil
		}
		return false, false, err
	}
	if statusOnly {
		return true, true, nil
	}
	if feature.Enabled == nil {
		return false, false, fmt.Errorf("security feature response is missing enabled")
	}
	return *feature.Enabled, true, nil
}

// SkippedOperation records a sub-operation that was skipped due to
// insufficient token scope.
type SkippedOperation struct {
	Operation string // e.g. "set workflow permissions"
	Err       error  // *ErrInsufficientScope
}

// ApplyResult collects the outcome of ApplyRepoSettings. Skipped lists
// operations that failed with access errors and were gracefully skipped.
type ApplyResult struct {
	Skipped []SkippedOperation
}

func recordAccessError(result *ApplyResult, operation string, err error) bool {
	classified := classifyHTTPError(err, operation)
	if !isAccessError(classified) {
		return false
	}
	result.Skipped = append(result.Skipped, SkippedOperation{Operation: operation, Err: classified})
	return true
}

// ApplyRepoSettings sends a PATCH /repos/{owner}/{repo} with the declared
// settings. It also handles fields that require separate API endpoints:
// security features, topics, and Actions workflow permissions. Access errors
// are collected in the returned ApplyResult rather than aborting.
// Hard errors still return as the error value.
func ApplyRepoSettings(client *api.RESTClient, owner, name string, settings *model.RepositorySettings) (*ApplyResult, error) {
	return applyRepoSettings(client, owner, name, settings, nil)
}

// ApplyRepoSettingsWithCurrent applies settings with the live state available to avoid redundant security feature writes.
func ApplyRepoSettingsWithCurrent(client *api.RESTClient, owner, name string, settings, current *model.RepositorySettings) (*ApplyResult, error) {
	return applyRepoSettings(client, owner, name, settings, current)
}

func applyRepoSettings(client *api.RESTClient, owner, name string, settings, current *model.RepositorySettings) (*ApplyResult, error) {
	p := buildSettingsPayload(settings)
	result := &ApplyResult{}

	if len(p.Body) > 0 {
		payload, err := json.Marshal(p.Body)
		if err != nil {
			return nil, fmt.Errorf("marshalling repo settings: %w", err)
		}
		if err := client.Patch(fmt.Sprintf("repos/%s/%s", owner, name), bytes.NewReader(payload), nil); err != nil {
			if !recordAccessError(result, "patch repo settings", err) {
				return nil, fmt.Errorf("patching repo settings: %w", err)
			}
		}
	}

	if p.PrivateVulnerabilityReporting != nil {
		path := fmt.Sprintf("repos/%s/%s/private-vulnerability-reporting", owner, name)
		if _, err := applySecuritySetting(client, path, *p.PrivateVulnerabilityReporting, "private vulnerability reporting", result); err != nil {
			return nil, err
		}
	}

	alertsPath := fmt.Sprintf("repos/%s/%s/vulnerability-alerts", owner, name)
	fixesPath := fmt.Sprintf("repos/%s/%s/automated-security-fixes", owner, name)
	fixesDisabled := current != nil && current.AutomatedSecurityFixesEnabled != nil && !*current.AutomatedSecurityFixesEnabled
	if p.AutomatedSecurityFixes != nil && !*p.AutomatedSecurityFixes && !fixesDisabled {
		var err error
		fixesDisabled, err = applySecuritySetting(client, fixesPath, false, "automated security fixes", result)
		if err != nil {
			return nil, err
		}
	}
	if !fixesDisabled && p.VulnerabilityAlerts != nil && !*p.VulnerabilityAlerts {
		if p.AutomatedSecurityFixes == nil {
			return nil, fmt.Errorf("cannot disable vulnerability alerts while automated security fixes are unmanaged")
		}
		appendSkippedDependency(result, "disable vulnerability alerts")
	}
	alertsApplied := true
	if p.VulnerabilityAlerts != nil && (*p.VulnerabilityAlerts || fixesDisabled) {
		var err error
		alertsApplied, err = applySecuritySetting(client, alertsPath, *p.VulnerabilityAlerts, "vulnerability alerts", result)
		if err != nil {
			return nil, err
		}
		if !alertsApplied && p.AutomatedSecurityFixes != nil && *p.AutomatedSecurityFixes {
			appendSkippedDependency(result, "enable automated security fixes")
		}
	}
	if p.AutomatedSecurityFixes != nil && *p.AutomatedSecurityFixes && alertsApplied {
		if _, err := applySecuritySetting(client, fixesPath, true, "automated security fixes", result); err != nil {
			return nil, err
		}
	}

	if p.DefaultWorkflowPermissions != nil || p.CanApprovePullRequestReviews != nil {
		if err := applyWorkflowPermissions(client, owner, name, p); err != nil {
			if !recordAccessError(result, "set workflow permissions", err) {
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
			if !recordAccessError(result, "set topics", err) {
				return nil, fmt.Errorf("setting topics: %w", err)
			}
		}
	}

	return result, nil
}

func applySecuritySetting(client *api.RESTClient, path string, enabled bool, feature string, result *ApplyResult) (bool, error) {
	action := "disable"
	var err error
	if enabled {
		action = "enable"
		err = client.Put(path, bytes.NewReader([]byte("{}")), nil)
	} else {
		err = client.Delete(path, nil)
	}
	if err == nil {
		return true, nil
	}
	operation := action + " " + feature
	if recordAccessError(result, operation, err) {
		return false, nil
	}
	return false, fmt.Errorf("%s: %w", operation, err)
}

func appendSkippedDependency(result *ApplyResult, operation string) {
	if len(result.Skipped) == 0 {
		return
	}
	result.Skipped = append(result.Skipped, SkippedOperation{
		Operation: operation,
		Err:       result.Skipped[len(result.Skipped)-1].Err,
	})
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
	Body                          map[string]any
	PrivateVulnerabilityReporting *bool
	VulnerabilityAlerts           *bool
	AutomatedSecurityFixes        *bool
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
	"private_vulnerability_reporting_enabled": true,
	"vulnerability_alerts_enabled":            true,
	"automated_security_fixes_enabled":        true,
	"topics":                                  true,
	"default_workflow_permissions":            true,
	"can_approve_pull_request_reviews":        true,
}

// buildSettingsPayload uses reflection to build a map of non-nil fields from
// settings, keyed by their yaml tags. Fields that require separate API
// endpoints are extracted into the returned settingsPayload struct and never
// appear in the PATCH body.
func buildSettingsPayload(settings *model.RepositorySettings) settingsPayload {
	p := settingsPayload{Body: make(map[string]any)}
	if settings == nil {
		return p
	}

	p.PrivateVulnerabilityReporting = settings.PrivateVulnerabilityReportEnabled
	p.VulnerabilityAlerts = settings.VulnerabilityAlertsEnabled
	p.AutomatedSecurityFixes = settings.AutomatedSecurityFixesEnabled
	p.Topics = settings.Topics
	p.DefaultWorkflowPermissions = settings.DefaultWorkflowPermissions
	p.CanApprovePullRequestReviews = settings.CanApprovePullRequestReviews

	for _, field := range model.RepositorySettingFields(settings) {
		if !field.Set || nonPatchFields[field.YAMLKey] {
			continue
		}

		fv := field.Value
		p.Body[field.YAMLKey] = fv.Elem().Interface()
	}

	return p
}
