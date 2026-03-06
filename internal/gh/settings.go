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
	"github.com/wimpysworld/tailor/internal/config"
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
// them as a config.RepositorySettings. It makes separate API calls for the
// standard repository fields, private vulnerability reporting, automated
// security fixes, vulnerability alerts, and Actions workflow permissions.
func ReadRepoSettings(client *api.RESTClient, owner, name string) (*config.RepositorySettings, error) {
	var repo repoResponse
	if err := client.Get(fmt.Sprintf("repos/%s/%s", owner, name), &repo); err != nil {
		return nil, fmt.Errorf("fetching repo settings: %w", err)
	}

	var pvr vulnerabilityReportingResponse
	if err := client.Get(fmt.Sprintf("repos/%s/%s/private-vulnerability-reporting", owner, name), &pvr); err != nil {
		return nil, fmt.Errorf("fetching private vulnerability reporting: %w", err)
	}

	var asf vulnerabilityReportingResponse
	if err := client.Get(fmt.Sprintf("repos/%s/%s/automated-security-fixes", owner, name), &asf); err != nil {
		return nil, fmt.Errorf("fetching automated security fixes: %w", err)
	}

	vulnerabilityAlerts, err := readVulnerabilityAlerts(client, owner, name)
	if err != nil {
		return nil, err
	}

	var wfPerms workflowPermissionsResponse
	if err := client.Get(fmt.Sprintf("repos/%s/%s/actions/permissions/workflow", owner, name), &wfPerms); err != nil {
		return nil, fmt.Errorf("fetching workflow permissions: %w", err)
	}

	s := &config.RepositorySettings{
		Description:                       ptr.String(repo.Description),
		Homepage:                          ptr.String(repo.Homepage),
		HasWiki:                           ptr.Bool(repo.HasWiki),
		HasDiscussions:                    ptr.Bool(repo.HasDiscussions),
		HasProjects:                       ptr.Bool(repo.HasProjects),
		HasIssues:                         ptr.Bool(repo.HasIssues),
		AllowMergeCommit:                  ptr.Bool(repo.AllowMergeCommit),
		AllowSquashMerge:                  ptr.Bool(repo.AllowSquashMerge),
		AllowRebaseMerge:                  ptr.Bool(repo.AllowRebaseMerge),
		SquashMergeCommitTitle:            ptr.String(repo.SquashMergeCommitTitle),
		SquashMergeCommitMessage:          ptr.String(repo.SquashMergeCommitMessage),
		MergeCommitTitle:                  ptr.String(repo.MergeCommitTitle),
		MergeCommitMessage:                ptr.String(repo.MergeCommitMessage),
		DeleteBranchOnMerge:               ptr.Bool(repo.DeleteBranchOnMerge),
		AllowUpdateBranch:                 ptr.Bool(repo.AllowUpdateBranch),
		AllowAutoMerge:                    ptr.Bool(repo.AllowAutoMerge),
		Topics:                            &repo.Topics,
		WebCommitSignoffRequired:          ptr.Bool(repo.WebCommitSignoffRequired),
		PrivateVulnerabilityReportEnabled: ptr.Bool(pvr.Enabled),
		AutomatedSecurityFixesEnabled:     ptr.Bool(asf.Enabled),
		VulnerabilityAlertsEnabled:        ptr.Bool(vulnerabilityAlerts),
		DefaultWorkflowPermissions:        ptr.String(wfPerms.DefaultWorkflowPermissions),
		CanApprovePullRequestReviews:      ptr.Bool(wfPerms.CanApprovePullRequestReviews),
	}

	return s, nil
}

// readVulnerabilityAlerts checks whether vulnerability alerts are enabled for a
// repository. The GET /repos/{owner}/{repo}/vulnerability-alerts endpoint
// returns 204 when enabled and 404 when disabled, with no JSON body.
func readVulnerabilityAlerts(client *api.RESTClient, owner, name string) (bool, error) {
	err := client.Get(fmt.Sprintf("repos/%s/%s/vulnerability-alerts", owner, name), nil)
	if err == nil {
		return true, nil
	}

	var httpErr *api.HTTPError
	if errors.As(err, &httpErr) && httpErr.StatusCode == http.StatusNotFound {
		return false, nil
	}

	return false, fmt.Errorf("fetching vulnerability alerts: %w", err)
}

// ApplyRepoSettings sends a PATCH /repos/{owner}/{repo} with the declared
// settings. It also handles fields that require separate API endpoints:
// private vulnerability reporting, vulnerability alerts, automated security
// fixes, topics, and Actions workflow permissions. Returns an error if any
// API call fails.
func ApplyRepoSettings(client *api.RESTClient, owner, name string, settings *config.RepositorySettings) error {
	p := buildSettingsPayload(settings)

	if len(p.Body) > 0 {
		payload, err := json.Marshal(p.Body)
		if err != nil {
			return fmt.Errorf("marshalling repo settings: %w", err)
		}
		if err := client.Patch(fmt.Sprintf("repos/%s/%s", owner, name), bytes.NewReader(payload), nil); err != nil {
			return fmt.Errorf("patching repo settings: %w", err)
		}
	}

	if p.PrivateVulnerabilityReporting != nil {
		pvrPath := fmt.Sprintf("repos/%s/%s/private-vulnerability-reporting", owner, name)
		if *p.PrivateVulnerabilityReporting {
			if err := client.Put(pvrPath, bytes.NewReader([]byte("{}")), nil); err != nil {
				return fmt.Errorf("enabling private vulnerability reporting: %w", err)
			}
		} else {
			if err := client.Delete(pvrPath, nil); err != nil {
				return fmt.Errorf("disabling private vulnerability reporting: %w", err)
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
			return fmt.Errorf("disabling automated security fixes: %w", err)
		}
	}

	// Enable or disable vulnerability alerts.
	if enableVA {
		if err := client.Put(vaPath, bytes.NewReader([]byte("{}")), nil); err != nil {
			return fmt.Errorf("enabling vulnerability alerts: %w", err)
		}
	} else if disableVA {
		if err := client.Delete(vaPath, nil); err != nil {
			return fmt.Errorf("disabling vulnerability alerts: %w", err)
		}
	}

	// Enable security fixes after enabling alerts.
	if enableASF {
		if err := client.Put(asfPath, bytes.NewReader([]byte("{}")), nil); err != nil {
			return fmt.Errorf("enabling automated security fixes: %w", err)
		}
	}

	if p.DefaultWorkflowPermissions != nil || p.CanApprovePullRequestReviews != nil {
		if err := applyWorkflowPermissions(client, owner, name, p); err != nil {
			return err
		}
	}

	if p.Topics != nil {
		topicsBody := struct {
			Names []string `json:"names"`
		}{Names: *p.Topics}
		payload, err := json.Marshal(topicsBody)
		if err != nil {
			return fmt.Errorf("marshalling topics: %w", err)
		}
		if err := client.Put(fmt.Sprintf("repos/%s/%s/topics", owner, name), bytes.NewReader(payload), nil); err != nil {
			return fmt.Errorf("setting topics: %w", err)
		}
	}

	return nil
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
func buildSettingsPayload(settings *config.RepositorySettings) settingsPayload {
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
