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
	if err := boundedHTTPError(client.Get(fmt.Sprintf("repos/%s/%s", owner, name), &repo)); err != nil {
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
	if err := boundedHTTPError(client.Get(fmt.Sprintf("repos/%s/%s/actions/permissions/workflow", owner, name), &wfPerms)); err != nil {
		classified := classifyHTTPError(err, Op(OpFetchWorkflowPermissions))
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
		operation        Operation
		statusOnly       bool
		allow404Disabled bool
		set              func(bool)
	}{
		{
			path:      fmt.Sprintf("repos/%s/%s/private-vulnerability-reporting", owner, name),
			operation: Op(OpFetchPrivateVulnerabilityReporting),
			set:       func(enabled bool) { s.PrivateVulnerabilityReportEnabled = new(enabled) },
		},
		{
			path:             fmt.Sprintf("repos/%s/%s/vulnerability-alerts", owner, name),
			operation:        Op(OpFetchVulnerabilityAlerts),
			statusOnly:       true,
			allow404Disabled: adminRead && repo.Permissions.Admin,
			set:              func(enabled bool) { s.VulnerabilityAlertsEnabled = new(enabled) },
		},
		{
			path:             fmt.Sprintf("repos/%s/%s/automated-security-fixes", owner, name),
			operation:        Op(OpFetchAutomatedSecurityFixes),
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

// readSecurityFeature reads one security feature endpoint. statusOnly is for
// endpoints that answer with a bare status code (204 enabled, 404 disabled)
// instead of a JSON body. allow404Disabled treats 404 as a confirmed disabled
// state; that reading is only safe with confirmed admin access, because 404
// also means the endpoint is not visible to the token.
func readSecurityFeature(client *api.RESTClient, path string, statusOnly, allow404Disabled bool) (enabled bool, known bool, err error) {
	var response any
	var feature securityFeatureResponse
	if !statusOnly {
		response = &feature
	}
	if err := boundedHTTPError(client.Get(path, response)); err != nil {
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
	Operation Operation
	Err       error // *ErrInsufficientScope
}

// ApplyResult collects the outcome of ApplyRepoSettings. Skipped lists
// operations that failed with access errors and were gracefully skipped.
type ApplyResult struct {
	Skipped []SkippedOperation
}

// recordAccessError appends the operation to result.Skipped when err is an
// access error, and reports whether it did. Other errors are left to the
// caller to return as hard failures.
func recordAccessError(result *ApplyResult, operation Operation, err error) bool {
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
		if err := boundedHTTPError(client.Patch(fmt.Sprintf("repos/%s/%s", owner, name), bytes.NewReader(payload), nil)); err != nil {
			if !recordAccessError(result, Op(OpPatchRepoSettings), err) {
				return nil, fmt.Errorf("patching repo settings: %w", err)
			}
		}
	}

	if err := applyPrivateVulnerabilityReporting(client, owner, name, p, result); err != nil {
		return nil, err
	}

	if err := applyVulnerabilityAlertsAndFixes(client, owner, name, p, current, result); err != nil {
		return nil, err
	}

	if err := applyWorkflowPermissions(client, owner, name, p, result); err != nil {
		return nil, err
	}

	if err := applyTopics(client, owner, name, p, result); err != nil {
		return nil, err
	}

	return result, nil
}

// applyPrivateVulnerabilityReporting toggles the private vulnerability
// reporting endpoint when the field is declared.
func applyPrivateVulnerabilityReporting(client *api.RESTClient, owner, name string, p settingsPayload, result *ApplyResult) error {
	if p.PrivateVulnerabilityReporting == nil {
		return nil
	}
	path := fmt.Sprintf("repos/%s/%s/private-vulnerability-reporting", owner, name)
	_, err := applySecuritySetting(client, path, *p.PrivateVulnerabilityReporting, OpSetPrivateVulnerabilityReporting, result)
	return err
}

// applyVulnerabilityAlertsAndFixes sequences the vulnerability-alerts and
// automated-security-fixes endpoints. GitHub couples the two features, so the
// order is fixed: disable automated security fixes before disabling
// vulnerability alerts, and enable vulnerability alerts before enabling
// automated security fixes. current, when non-nil, supplies the live state so
// an already disabled fixes feature is not disabled again.
func applyVulnerabilityAlertsAndFixes(client *api.RESTClient, owner, name string, p settingsPayload, current *model.RepositorySettings, result *ApplyResult) error {
	alertsPath := fmt.Sprintf("repos/%s/%s/vulnerability-alerts", owner, name)
	fixesPath := fmt.Sprintf("repos/%s/%s/automated-security-fixes", owner, name)
	fixesDisabled := current != nil && isFalse(current.AutomatedSecurityFixesEnabled)
	if isFalse(p.AutomatedSecurityFixes) && !fixesDisabled {
		var err error
		fixesDisabled, err = applySecuritySetting(client, fixesPath, false, OpSetAutomatedSecurityFixes, result)
		if err != nil {
			return err
		}
	}
	if !fixesDisabled && isFalse(p.VulnerabilityAlerts) {
		if p.AutomatedSecurityFixes == nil {
			return fmt.Errorf("cannot disable vulnerability alerts while automated security fixes are unmanaged")
		}
		appendSkippedDependency(result, SecurityFeatureOp(false, OpSetVulnerabilityAlerts))
	}
	alertsApplied := true
	if p.VulnerabilityAlerts != nil && (*p.VulnerabilityAlerts || fixesDisabled) {
		var err error
		alertsApplied, err = applySecuritySetting(client, alertsPath, *p.VulnerabilityAlerts, OpSetVulnerabilityAlerts, result)
		if err != nil {
			return err
		}
		if !alertsApplied && isTrue(p.AutomatedSecurityFixes) {
			appendSkippedDependency(result, SecurityFeatureOp(true, OpSetAutomatedSecurityFixes))
		}
	}
	if isTrue(p.AutomatedSecurityFixes) && alertsApplied {
		if _, err := applySecuritySetting(client, fixesPath, true, OpSetAutomatedSecurityFixes, result); err != nil {
			return err
		}
	}
	return nil
}

// applyTopics replaces the repository topics when the field is declared.
func applyTopics(client *api.RESTClient, owner, name string, p settingsPayload, result *ApplyResult) error {
	if p.Topics == nil {
		return nil
	}
	topicsBody := struct {
		Names []string `json:"names"`
	}{Names: *p.Topics}
	payload, err := json.Marshal(topicsBody)
	if err != nil {
		return fmt.Errorf("marshalling topics: %w", err)
	}
	if err := boundedHTTPError(client.Put(fmt.Sprintf("repos/%s/%s/topics", owner, name), bytes.NewReader(payload), nil)); err != nil {
		if !recordAccessError(result, Op(OpSetTopics), err) {
			return fmt.Errorf("setting topics: %w", err)
		}
	}
	return nil
}

func applySecuritySetting(client *api.RESTClient, path string, enabled bool, kind OperationKind, result *ApplyResult) (bool, error) {
	var err error
	if enabled {
		err = boundedHTTPError(client.Put(path, bytes.NewReader([]byte("{}")), nil))
	} else {
		err = boundedHTTPError(client.Delete(path, nil))
	}
	if err == nil {
		return true, nil
	}
	operation := SecurityFeatureOp(enabled, kind)
	if recordAccessError(result, operation, err) {
		return false, nil
	}
	return false, fmt.Errorf("%s: %w", operation, err)
}

// appendSkippedDependency records an operation that was not attempted because
// a prerequisite operation was skipped, reusing the error from the most
// recent skip. It does nothing when nothing was skipped.
func appendSkippedDependency(result *ApplyResult, operation Operation) {
	if len(result.Skipped) == 0 {
		return
	}
	result.Skipped = append(result.Skipped, SkippedOperation{
		Operation: operation,
		Err:       result.Skipped[len(result.Skipped)-1].Err,
	})
}

// applyWorkflowPermissions sends a PUT to the Actions workflow permissions
// endpoint when either field is declared. The endpoint replaces both fields
// atomically, so when only one field is declared in the config, the other is
// fetched from the current repository state. Access errors are recorded in
// result rather than returned.
func applyWorkflowPermissions(client *api.RESTClient, owner, name string, p settingsPayload, result *ApplyResult) error {
	if p.DefaultWorkflowPermissions == nil && p.CanApprovePullRequestReviews == nil {
		return nil
	}
	if err := putWorkflowPermissions(client, owner, name, p); err != nil {
		if !recordAccessError(result, Op(OpSetWorkflowPermissions), err) {
			return err
		}
	}
	return nil
}

// putWorkflowPermissions builds the complete PUT body and sends it.
func putWorkflowPermissions(client *api.RESTClient, owner, name string, p settingsPayload) error {
	wfpPath := fmt.Sprintf("repos/%s/%s/actions/permissions/workflow", owner, name)

	perms := p.DefaultWorkflowPermissions
	approve := p.CanApprovePullRequestReviews

	// When one field is missing, read the current value from the API so the
	// PUT body is always complete.
	if perms == nil || approve == nil {
		var current workflowPermissionsResponse
		if err := boundedHTTPError(client.Get(wfpPath, &current)); err != nil {
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
	if err := boundedHTTPError(client.Put(wfpPath, bytes.NewReader(payload), nil)); err != nil {
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
