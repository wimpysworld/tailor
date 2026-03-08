package gh

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"reflect"
	"strings"

	"github.com/cli/go-gh/v2/pkg/api"
	"github.com/wimpysworld/tailor/internal/model"
	"github.com/wimpysworld/tailor/internal/ptr"
)

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

// vulnerabilityReportingResponse holds the private vulnerability reporting status.
type vulnerabilityReportingResponse struct {
	Enabled bool `json:"enabled"`
}

// workflowPermissionsResponse holds the Actions workflow permission settings.
type workflowPermissionsResponse struct {
	DefaultWorkflowPermissions   string `json:"default_workflow_permissions"`
	CanApprovePullRequestReviews bool   `json:"can_approve_pull_request_reviews"`
}

// ReadRepoSettings fetches repository settings from the GitHub API and returns
// them as a model.RepositorySettings. It makes separate API calls for the
// standard repository fields, private vulnerability reporting, automated
// security fixes, vulnerability alerts, and Actions workflow permissions.
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

	// Each sub-call below uses classifyHTTPError to detect 403 responses.
	// On scope/role errors the corresponding field stays nil (unknown),
	// and the classified error is appended to warnings for the caller.
	var warnings []error

	pvrEnabled, pvrKnown, pvrErr := readSecurityFeatureEnabled(client, fmt.Sprintf("repos/%s/%s/private-vulnerability-reporting", owner, name))
	if pvrKnown {
		s.PrivateVulnerabilityReportEnabled = ptr.Ptr(pvrEnabled)
	} else if pvrErr != nil {
		classified := classifyHTTPError(pvrErr, "fetch private vulnerability reporting")
		if isAccessError(classified) {
			warnings = append(warnings, classified)
		} else {
			return nil, nil, fmt.Errorf("fetching private vulnerability reporting: %w", pvrErr)
		}
	}

	asfEnabled, asfKnown, asfErr := readSecurityFeatureEnabled(client, fmt.Sprintf("repos/%s/%s/automated-security-fixes", owner, name))
	if asfKnown {
		s.AutomatedSecurityFixesEnabled = ptr.Ptr(asfEnabled)
	} else if asfErr != nil {
		classified := classifyHTTPError(asfErr, "fetch automated security fixes")
		if isAccessError(classified) {
			warnings = append(warnings, classified)
		} else {
			return nil, nil, fmt.Errorf("fetching automated security fixes: %w", asfErr)
		}
	}

	vaEnabled, vaKnown, vaErr := readSecurityFeatureStatus(client, fmt.Sprintf("repos/%s/%s/vulnerability-alerts", owner, name))
	if vaKnown {
		s.VulnerabilityAlertsEnabled = ptr.Ptr(vaEnabled)
	} else if vaErr != nil {
		classified := classifyHTTPError(vaErr, "fetch vulnerability alerts")
		if isAccessError(classified) {
			warnings = append(warnings, classified)
		} else {
			return nil, nil, fmt.Errorf("fetching vulnerability alerts: %w", vaErr)
		}
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

// isHTTP404 returns true when err wraps an *api.HTTPError with status 404.
func isHTTP404(err error) bool {
	var httpErr *api.HTTPError
	return errors.As(err, &httpErr) && httpErr.StatusCode == http.StatusNotFound
}

// readSecurityFeatureEnabled reads a security feature GET endpoint that returns
// {"enabled": bool} on success and 404 when the feature is disabled. It returns
// (value, true, nil) on success, (false, true, nil) on 404, and
// (false, false, err) for any other error. The second return value indicates
// whether the result is known (true) or the call failed for a non-404 reason
// and the caller should classify the error.
func readSecurityFeatureEnabled(client *api.RESTClient, path string) (enabled bool, known bool, err error) {
	var resp vulnerabilityReportingResponse
	if err := client.Get(path, &resp); err != nil {
		if isHTTP404(err) {
			return false, true, nil
		}
		return false, false, err
	}
	return resp.Enabled, true, nil
}

// readSecurityFeatureStatus reads a security feature GET endpoint that returns
// 204 when enabled and 404 when disabled, with no JSON body (e.g. vulnerability
// alerts). Returns (true, true, nil) on 204, (false, true, nil) on 404, and
// (false, false, err) for any other error.
func readSecurityFeatureStatus(client *api.RESTClient, path string) (enabled bool, known bool, err error) {
	if err := client.Get(path, nil); err != nil {
		if isHTTP404(err) {
			return false, true, nil
		}
		return false, false, err
	}
	return true, true, nil
}

// SkippedOperation records a sub-operation that was skipped due to
// insufficient token scope or repository role.
type SkippedOperation struct {
	Operation string // e.g. "enabling private vulnerability reporting"
	Err       error  // *ErrInsufficientScope or *ErrInsufficientRole
}

// ApplyResult collects the outcome of ApplyRepoSettings. Skipped lists
// operations that failed with access errors and were gracefully skipped.
type ApplyResult struct {
	Skipped []SkippedOperation
}

// ApplyRepoSettings sends a PATCH /repos/{owner}/{repo} with the declared
// settings. It also handles fields that require separate API endpoints:
// private vulnerability reporting, vulnerability alerts, automated security
// fixes, topics, and Actions workflow permissions. Access errors (insufficient
// scope or role) are collected in the returned ApplyResult rather than aborting.
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

	if p.PrivateVulnerabilityReporting != nil {
		pvrPath := fmt.Sprintf("repos/%s/%s/private-vulnerability-reporting", owner, name)
		var opName string
		var pvrErr error
		if *p.PrivateVulnerabilityReporting {
			opName = "enable private vulnerability reporting"
			pvrErr = client.Put(pvrPath, bytes.NewReader([]byte("{}")), nil)
		} else {
			opName = "disable private vulnerability reporting"
			pvrErr = client.Delete(pvrPath, nil)
		}
		if pvrErr != nil {
			classified := classifyHTTPError(pvrErr, opName)
			if isAccessError(classified) {
				result.Skipped = append(result.Skipped, SkippedOperation{Operation: opName, Err: classified})
			} else {
				return nil, fmt.Errorf("%s: %w", opName, pvrErr)
			}
		}
	}

	// Vulnerability alerts and automated security fixes have ordering
	// constraints: automated_security_fixes requires vulnerability_alerts to
	// be active. When enabling both, enable alerts first. When disabling
	// both, disable security fixes first.
	vaPath := fmt.Sprintf("repos/%s/%s/vulnerability-alerts", owner, name)
	asfPath := fmt.Sprintf("repos/%s/%s/automated-security-fixes", owner, name)

	enableVA := p.VulnerabilityAlerts != nil && *p.VulnerabilityAlerts
	enableASF := p.AutomatedSecurityFixes != nil && *p.AutomatedSecurityFixes
	disableVA := p.VulnerabilityAlerts != nil && !*p.VulnerabilityAlerts
	disableASF := p.AutomatedSecurityFixes != nil && !*p.AutomatedSecurityFixes

	// Disable security fixes before disabling alerts.
	if disableASF {
		if err := client.Delete(asfPath, nil); err != nil {
			classified := classifyHTTPError(err, "disable automated security fixes")
			if isAccessError(classified) {
				result.Skipped = append(result.Skipped, SkippedOperation{Operation: "disable automated security fixes", Err: classified})
			} else {
				return nil, fmt.Errorf("disabling automated security fixes: %w", err)
			}
		}
	}

	// Enable or disable vulnerability alerts.
	if enableVA {
		if err := client.Put(vaPath, bytes.NewReader([]byte("{}")), nil); err != nil {
			classified := classifyHTTPError(err, "enable vulnerability alerts")
			if isAccessError(classified) {
				result.Skipped = append(result.Skipped, SkippedOperation{Operation: "enable vulnerability alerts", Err: classified})
			} else {
				return nil, fmt.Errorf("enabling vulnerability alerts: %w", err)
			}
		}
	} else if disableVA {
		if err := client.Delete(vaPath, nil); err != nil {
			classified := classifyHTTPError(err, "disable vulnerability alerts")
			if isAccessError(classified) {
				result.Skipped = append(result.Skipped, SkippedOperation{Operation: "disable vulnerability alerts", Err: classified})
			} else {
				return nil, fmt.Errorf("disabling vulnerability alerts: %w", err)
			}
		}
	}

	// Enable security fixes after enabling alerts.
	if enableASF {
		if err := client.Put(asfPath, bytes.NewReader([]byte("{}")), nil); err != nil {
			classified := classifyHTTPError(err, "enable automated security fixes")
			if isAccessError(classified) {
				result.Skipped = append(result.Skipped, SkippedOperation{Operation: "enable automated security fixes", Err: classified})
			} else {
				return nil, fmt.Errorf("enabling automated security fixes: %w", err)
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
	// PrivateVulnerabilityReporting is non-nil when the field is declared.
	PrivateVulnerabilityReporting *bool
	// VulnerabilityAlerts is non-nil when the field is declared.
	VulnerabilityAlerts *bool
	// AutomatedSecurityFixes is non-nil when the field is declared.
	AutomatedSecurityFixes *bool
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

		if nonPatchFields[key] {
			switch key {
			case "private_vulnerability_reporting_enabled":
				b := fv.Elem().Bool()
				p.PrivateVulnerabilityReporting = &b
			case "vulnerability_alerts_enabled":
				b := fv.Elem().Bool()
				p.VulnerabilityAlerts = &b
			case "automated_security_fixes_enabled":
				b := fv.Elem().Bool()
				p.AutomatedSecurityFixes = &b
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
