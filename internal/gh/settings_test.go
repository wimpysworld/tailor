package gh

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/wimpysworld/tailor/internal/model"
	"github.com/wimpysworld/tailor/internal/testutil"
)

var newTestClient = testutil.NewTestClient

const fullRepoJSON = `{
	"description": "A tailor for your repos",
	"homepage": "https://tailor.dev",
	"has_wiki": false,
	"has_discussions": true,
	"has_projects": false,
	"has_issues": true,
	"allow_merge_commit": false,
	"allow_squash_merge": true,
	"allow_rebase_merge": true,
	"squash_merge_commit_title": "PR_TITLE",
	"squash_merge_commit_message": "PR_BODY",
	"merge_commit_title": "PR_TITLE",
	"merge_commit_message": "PR_BODY",
	"delete_branch_on_merge": true,
	"allow_update_branch": true,
	"allow_auto_merge": true,
	"web_commit_signoff_required": false,
	"topics": ["go", "cli-tool"],
	"permissions": {"admin": true},
	"security_and_analysis": {
		"secret_scanning": {"status": "enabled"},
		"secret_scanning_push_protection": {"status": "disabled"}
	}
}`

const (
	wfPermsReadJSON  = `{"default_workflow_permissions": "read", "can_approve_pull_request_reviews": false}`
	wfPermsWriteJSON = `{"default_workflow_permissions": "write", "can_approve_pull_request_reviews": true}`
)

func TestReadRepoSettings(t *testing.T) {
	tests := []struct {
		name        string
		repoJSON    string
		wfPermsJSON string
		// expected field checks
		wantDesc       string
		wantDescNil    bool
		wantHome       string
		wantHomeNil    bool
		wantWiki       bool
		wantDisc       bool
		wantProj       bool
		wantIssues     bool
		wantMerge      bool
		wantSquash     bool
		wantRebase     bool
		wantSqTitle    string
		wantSqMsg      string
		wantMcTitle    string
		wantMcMsg      string
		wantDelete     bool
		wantUpdate     bool
		wantAuto       bool
		wantSignoff    bool
		wantTopics     []string
		wantWfPerms    string
		wantCanApprove bool
	}{
		{
			name:           "all fields populated",
			repoJSON:       fullRepoJSON,
			wfPermsJSON:    wfPermsWriteJSON,
			wantDesc:       "A tailor for your repos",
			wantHome:       "https://tailor.dev",
			wantWiki:       false,
			wantDisc:       true,
			wantProj:       false,
			wantIssues:     true,
			wantMerge:      false,
			wantSquash:     true,
			wantRebase:     true,
			wantSqTitle:    "PR_TITLE",
			wantSqMsg:      "PR_BODY",
			wantMcTitle:    "PR_TITLE",
			wantMcMsg:      "PR_BODY",
			wantDelete:     true,
			wantUpdate:     true,
			wantAuto:       true,
			wantSignoff:    false,
			wantTopics:     []string{"go", "cli-tool"},
			wantWfPerms:    "write",
			wantCanApprove: true,
		},
		{
			name: "empty description and homepage pass through",
			repoJSON: `{
				"description": "",
				"homepage": "",
				"has_wiki": true,
				"has_discussions": false,
				"has_projects": true,
				"has_issues": false,
				"allow_merge_commit": true,
				"allow_squash_merge": false,
				"allow_rebase_merge": false,
				"squash_merge_commit_title": "COMMIT_OR_PR_TITLE",
				"squash_merge_commit_message": "COMMIT_MESSAGES",
				"merge_commit_title": "MERGE_MESSAGE",
				"merge_commit_message": "PR_TITLE",
				"delete_branch_on_merge": false,
				"allow_update_branch": false,
				"allow_auto_merge": false,
				"web_commit_signoff_required": true
			}`,
			wfPermsJSON:    wfPermsReadJSON,
			wantDesc:       "",
			wantHome:       "",
			wantWiki:       true,
			wantDisc:       false,
			wantProj:       true,
			wantIssues:     false,
			wantMerge:      true,
			wantSquash:     false,
			wantRebase:     false,
			wantSqTitle:    "COMMIT_OR_PR_TITLE",
			wantSqMsg:      "COMMIT_MESSAGES",
			wantMcTitle:    "MERGE_MESSAGE",
			wantMcMsg:      "PR_TITLE",
			wantDelete:     false,
			wantUpdate:     false,
			wantAuto:       false,
			wantSignoff:    true,
			wantTopics:     nil,
			wantWfPerms:    "read",
			wantCanApprove: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch r.URL.Path {
				case "/repos/testowner/testrepo":
					fmt.Fprint(w, tt.repoJSON)
				case "/repos/testowner/testrepo/actions/permissions/workflow":
					fmt.Fprint(w, tt.wfPermsJSON)
				default:
					http.NotFound(w, r)
				}
			}))
			t.Cleanup(server.Close)

			client := testutil.NewTestClient(t, server)
			settings, _, err := ReadRepoSettings(client, "testowner", "testrepo")
			if err != nil {
				t.Fatalf("ReadRepoSettings() error: %v", err)
			}

			// description and homepage
			testutil.AssertPtr(t, settings.Description, tt.wantDescNil, tt.wantDesc, "description")
			testutil.AssertPtr(t, settings.Homepage, tt.wantHomeNil, tt.wantHome, "homepage")

			// bool fields
			testutil.AssertPtr(t, settings.HasWiki, false, tt.wantWiki, "has_wiki")
			testutil.AssertPtr(t, settings.HasDiscussions, false, tt.wantDisc, "has_discussions")
			testutil.AssertPtr(t, settings.HasProjects, false, tt.wantProj, "has_projects")
			testutil.AssertPtr(t, settings.HasIssues, false, tt.wantIssues, "has_issues")
			testutil.AssertPtr(t, settings.AllowMergeCommit, false, tt.wantMerge, "allow_merge_commit")
			testutil.AssertPtr(t, settings.AllowSquashMerge, false, tt.wantSquash, "allow_squash_merge")
			testutil.AssertPtr(t, settings.AllowRebaseMerge, false, tt.wantRebase, "allow_rebase_merge")
			testutil.AssertPtr(t, settings.DeleteBranchOnMerge, false, tt.wantDelete, "delete_branch_on_merge")
			testutil.AssertPtr(t, settings.AllowUpdateBranch, false, tt.wantUpdate, "allow_update_branch")
			testutil.AssertPtr(t, settings.AllowAutoMerge, false, tt.wantAuto, "allow_auto_merge")
			testutil.AssertPtr(t, settings.WebCommitSignoffRequired, false, tt.wantSignoff, "web_commit_signoff_required")
			testutil.AssertPtr(t, settings.CanApprovePullRequestReviews, false, tt.wantCanApprove, "can_approve_pull_request_reviews")

			// string fields (always non-nil)
			testutil.AssertPtr(t, settings.DefaultWorkflowPermissions, false, tt.wantWfPerms, "default_workflow_permissions")
			testutil.AssertPtr(t, settings.SquashMergeCommitTitle, false, tt.wantSqTitle, "squash_merge_commit_title")
			testutil.AssertPtr(t, settings.SquashMergeCommitMessage, false, tt.wantSqMsg, "squash_merge_commit_message")
			testutil.AssertPtr(t, settings.MergeCommitTitle, false, tt.wantMcTitle, "merge_commit_title")
			testutil.AssertPtr(t, settings.MergeCommitMessage, false, tt.wantMcMsg, "merge_commit_message")

			// topics
			if tt.wantTopics == nil {
				if settings.Topics != nil && *settings.Topics != nil {
					t.Errorf("topics = %v, want nil", *settings.Topics)
				}
			} else {
				if settings.Topics == nil {
					t.Fatal("topics is nil, want non-nil")
				}
				got := *settings.Topics
				if len(got) != len(tt.wantTopics) {
					t.Fatalf("topics length = %d, want %d", len(got), len(tt.wantTopics))
				}
				for i, v := range got {
					if v != tt.wantTopics[i] {
						t.Errorf("topics[%d] = %q, want %q", i, v, tt.wantTopics[i])
					}
				}
			}
		})
	}
}

func TestReadRepoSettingsIgnoresGitHubActionsEnvironment(t *testing.T) {
	t.Setenv("GITHUB_ACTIONS", "true")
	t.Setenv("GITHUB_REPOSITORY_OWNER", "actions-owner")

	var userRequests atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/user":
			userRequests.Add(1)
			w.WriteHeader(http.StatusForbidden)
			fmt.Fprint(w, `{"message": "Resource not accessible by integration"}`)
		case "/repos/testowner/testrepo":
			fmt.Fprint(w, `{"permissions":{"admin":true},"security_and_analysis":{"secret_scanning":{"status":"enabled"}}}`)
		case "/repos/testowner/testrepo/actions/permissions/workflow":
			fmt.Fprint(w, wfPermsReadJSON)
		case "/repos/testowner/testrepo/private-vulnerability-reporting":
			fmt.Fprint(w, `{"enabled":false}`)
		case "/repos/testowner/testrepo/vulnerability-alerts":
			w.WriteHeader(http.StatusNoContent)
		case "/repos/testowner/testrepo/automated-security-fixes":
			fmt.Fprint(w, `{"enabled":false,"paused":false}`)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)

	client := testutil.NewTestClient(t, server)
	settings, warnings, err := ReadRepoSettings(client, "testowner", "testrepo")
	if err != nil {
		t.Fatalf("ReadRepoSettings() error: %v", err)
	}
	if got := userRequests.Load(); got != 0 {
		t.Errorf("ReadRepoSettings() made %d /user requests, want 0", got)
	}
	if len(warnings) != 0 {
		t.Errorf("ReadRepoSettings() warnings = %v, want none", warnings)
	}

	testutil.AssertPtr(t, settings.AllowAutoMerge, false, false, "allow_auto_merge")
	testutil.AssertPtr(t, settings.AllowRebaseMerge, false, false, "allow_rebase_merge")
	testutil.AssertPtr(t, settings.AllowSquashMerge, false, false, "allow_squash_merge")
	testutil.AssertPtr(t, settings.AllowUpdateBranch, false, false, "allow_update_branch")
	testutil.AssertPtr(t, settings.DeleteBranchOnMerge, false, false, "delete_branch_on_merge")
	testutil.AssertPtr(t, settings.SquashMergeCommitMessage, false, "", "squash_merge_commit_message")
	testutil.AssertPtr(t, settings.SquashMergeCommitTitle, false, "", "squash_merge_commit_title")
}

func TestReadRepoSettingsRepoAPIError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		fmt.Fprint(w, `{"message": "Not Found"}`)
	}))
	t.Cleanup(server.Close)

	client := testutil.NewTestClient(t, server)
	_, _, err := ReadRepoSettings(client, "testowner", "testrepo")
	if err == nil {
		t.Fatal("ReadRepoSettings() expected error, got nil")
	}
}

func TestReadRepoSettingsWFPerms403GracefulDegradation(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/repos/testowner/testrepo":
			fmt.Fprint(w, fullRepoJSON)
		case "/repos/testowner/testrepo/actions/permissions/workflow":
			w.WriteHeader(http.StatusForbidden)
			fmt.Fprint(w, `{"message": "Forbidden"}`)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)

	client := testutil.NewTestClient(t, server)
	settings, _, err := ReadRepoSettings(client, "testowner", "testrepo")
	if err != nil {
		t.Fatalf("ReadRepoSettings() unexpected error: %v", err)
	}

	// Workflow permissions should be nil (inaccessible).
	if settings.DefaultWorkflowPermissions != nil {
		t.Errorf("DefaultWorkflowPermissions = %v, want nil", *settings.DefaultWorkflowPermissions)
	}
	if settings.CanApprovePullRequestReviews != nil {
		t.Errorf("CanApprovePullRequestReviews = %v, want nil", *settings.CanApprovePullRequestReviews)
	}
	// Other fields should be populated.
	testutil.AssertPtr(t, settings.Description, false, "A tailor for your repos", "description")
}

func TestReadRepoSettingsAll403GracefulDegradation(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/repos/testowner/testrepo":
			fmt.Fprint(w, fullRepoJSON)
		default:
			w.WriteHeader(http.StatusForbidden)
			fmt.Fprint(w, `{"message": "Forbidden"}`)
		}
	}))
	t.Cleanup(server.Close)

	client := testutil.NewTestClient(t, server)
	settings, warnings, err := ReadRepoSettings(client, "testowner", "testrepo")
	if err != nil {
		t.Fatalf("ReadRepoSettings() unexpected error: %v", err)
	}

	// All sub-call fields should be nil.
	if settings.DefaultWorkflowPermissions != nil {
		t.Errorf("DefaultWorkflowPermissions = %v, want nil", *settings.DefaultWorkflowPermissions)
	}
	if settings.CanApprovePullRequestReviews != nil {
		t.Errorf("CanApprovePullRequestReviews = %v, want nil", *settings.CanApprovePullRequestReviews)
	}
	if settings.PrivateVulnerabilityReportEnabled != nil || settings.VulnerabilityAlertsEnabled != nil || settings.AutomatedSecurityFixesEnabled != nil {
		t.Error("security settings are non-nil after access failures")
	}
	// Four warnings: three security settings and workflow permissions.
	if len(warnings) != 4 {
		t.Errorf("expected 4 warnings, got %d", len(warnings))
	}
	// Core repo fields should still be populated.
	testutil.AssertPtr(t, settings.Description, false, "A tailor for your repos", "description")
	testutil.AssertPtr(t, settings.HasWiki, false, false, "has_wiki")
}

func TestReadRepoSettingsNon403StillFails(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/repos/testowner/testrepo":
			fmt.Fprint(w, fullRepoJSON)
		case "/repos/testowner/testrepo/actions/permissions/workflow":
			w.WriteHeader(http.StatusInternalServerError)
			fmt.Fprint(w, `{"message": "Internal Server Error"}`)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)

	client := testutil.NewTestClient(t, server)
	_, _, err := ReadRepoSettings(client, "testowner", "testrepo")
	if err == nil {
		t.Fatal("ReadRepoSettings() expected error for 500, got nil")
	}
}

func TestReadRepoSettingsSecurityFeatures(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/repos/testowner/testrepo":
			fmt.Fprint(w, fullRepoJSON)
		case "/repos/testowner/testrepo/private-vulnerability-reporting":
			fmt.Fprint(w, `{"enabled":true}`)
		case "/repos/testowner/testrepo/vulnerability-alerts":
			w.WriteHeader(http.StatusNoContent)
		case "/repos/testowner/testrepo/automated-security-fixes":
			fmt.Fprint(w, `{"enabled":false,"paused":true}`)
		case "/repos/testowner/testrepo/actions/permissions/workflow":
			fmt.Fprint(w, wfPermsReadJSON)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)

	settings, warnings, err := ReadRepoSettings(testutil.NewTestClient(t, server), "testowner", "testrepo")
	if err != nil {
		t.Fatalf("ReadRepoSettings() error: %v", err)
	}
	if len(warnings) != 0 {
		t.Fatalf("warnings = %v, want none", warnings)
	}
	testutil.AssertPtr(t, settings.PrivateVulnerabilityReportEnabled, false, true, "private_vulnerability_reporting_enabled")
	testutil.AssertPtr(t, settings.VulnerabilityAlertsEnabled, false, true, "vulnerability_alerts_enabled")
	testutil.AssertPtr(t, settings.AutomatedSecurityFixesEnabled, false, false, "automated_security_fixes_enabled")
}

func TestReadRepoSettingsSecurityFeature404HandlingForAdmin(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/repos/testowner/testrepo":
			fmt.Fprint(w, fullRepoJSON)
		case "/repos/testowner/testrepo/actions/permissions/workflow":
			fmt.Fprint(w, wfPermsReadJSON)
		default:
			w.WriteHeader(http.StatusNotFound)
			fmt.Fprint(w, `{"message":"Not Found"}`)
		}
	}))
	t.Cleanup(server.Close)

	settings, warnings, err := ReadRepoSettings(testutil.NewTestClient(t, server), "testowner", "testrepo")
	if err != nil {
		t.Fatalf("ReadRepoSettings() error: %v", err)
	}
	if len(warnings) != 1 {
		t.Fatalf("warnings = %v, want private reporting access warning", warnings)
	}
	testutil.AssertPtr(t, settings.PrivateVulnerabilityReportEnabled, true, false, "private_vulnerability_reporting_enabled")
	testutil.AssertPtr(t, settings.VulnerabilityAlertsEnabled, false, false, "vulnerability_alerts_enabled")
	testutil.AssertPtr(t, settings.AutomatedSecurityFixesEnabled, false, false, "automated_security_fixes_enabled")
}

func TestReadRepoSettingsSecurityFeature404UnknownWithoutConfirmedAdminAccess(t *testing.T) {
	tests := []struct {
		name           string
		repo           string
		workflowStatus int
		wantWarnings   int
	}{
		// The repository response omits security_and_analysis, which adds one
		// secret scanning access warning to each case.
		{name: "non-admin", repo: `{"permissions":{"admin":false}}`, workflowStatus: http.StatusOK, wantWarnings: 4},
		{name: "administration read denied", repo: `{"permissions":{"admin":true}}`, workflowStatus: http.StatusForbidden, wantWarnings: 5},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/repos/testowner/testrepo" {
					fmt.Fprint(w, tt.repo)
					return
				}
				if r.URL.Path == "/repos/testowner/testrepo/actions/permissions/workflow" && tt.workflowStatus != http.StatusOK {
					w.WriteHeader(tt.workflowStatus)
					fmt.Fprint(w, `{"message":"Forbidden"}`)
					return
				}
				if r.URL.Path == "/repos/testowner/testrepo/actions/permissions/workflow" {
					fmt.Fprint(w, wfPermsReadJSON)
					return
				}
				w.WriteHeader(http.StatusNotFound)
				fmt.Fprint(w, `{"message":"Not Found"}`)
			}))
			t.Cleanup(server.Close)

			settings, warnings, err := ReadRepoSettings(testutil.NewTestClient(t, server), "testowner", "testrepo")
			if err != nil {
				t.Fatalf("ReadRepoSettings() error: %v", err)
			}
			if len(warnings) != tt.wantWarnings {
				t.Fatalf("warnings = %v, want %d", warnings, tt.wantWarnings)
			}
			if settings.PrivateVulnerabilityReportEnabled != nil || settings.VulnerabilityAlertsEnabled != nil || settings.AutomatedSecurityFixesEnabled != nil {
				t.Fatalf("security settings = %+v, want unknown", settings)
			}
		})
	}
}

func TestReadRepoSettingsSecurityFeatureHardError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/repos/testowner/testrepo" {
			fmt.Fprint(w, fullRepoJSON)
			return
		}
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprint(w, `{"message":"Internal Server Error"}`)
	}))
	t.Cleanup(server.Close)

	if _, _, err := ReadRepoSettings(testutil.NewTestClient(t, server), "testowner", "testrepo"); err == nil {
		t.Fatal("ReadRepoSettings() error = nil, want hard error")
	}
}

func TestReadSecurityFeatureRejectsEmptyJSONSuccess(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	t.Cleanup(server.Close)

	for _, feature := range []string{"private vulnerability reporting", "automated security fixes"} {
		t.Run(feature, func(t *testing.T) {
			_, known, err := readSecurityFeature(testutil.NewTestClient(t, server), "feature", false, false)
			if err == nil || known {
				t.Fatalf("readSecurityFeature() known = %t, error = %v, want protocol error", known, err)
			}
		})
	}
}

func TestApplyRepoSettingsPatchBody(t *testing.T) {
	var gotMethod string
	var gotPath string
	var gotBody map[string]any

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotPath = r.URL.Path
		body, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(body, &gotBody)
		w.WriteHeader(http.StatusNoContent)
	}))
	t.Cleanup(server.Close)

	client := testutil.NewTestClient(t, server)
	settings := &model.RepositorySettings{
		Description:    new("new desc"),
		HasWiki:        new(true),
		AllowAutoMerge: new(false),
	}

	_, err := ApplyRepoSettings(client, "testowner", "testrepo", settings, nil)
	if err != nil {
		t.Fatalf("ApplyRepoSettings() error: %v", err)
	}

	if gotMethod != http.MethodPatch {
		t.Errorf("method = %s, want PATCH", gotMethod)
	}
	if gotPath != "/repos/testowner/testrepo" {
		t.Errorf("path = %s, want /repos/testowner/testrepo", gotPath)
	}

	// Non-nil fields appear with their declared values.
	if gotBody["description"] != "new desc" {
		t.Errorf("description = %v, want %q", gotBody["description"], "new desc")
	}
	if gotBody["has_wiki"] != true {
		t.Errorf("has_wiki = %v, want true", gotBody["has_wiki"])
	}
	if gotBody["allow_auto_merge"] != false {
		t.Errorf("allow_auto_merge = %v, want false", gotBody["allow_auto_merge"])
	}

	// Nil fields are excluded.
	if _, ok := gotBody["homepage"]; ok {
		t.Error("homepage should not be in PATCH body when nil")
	}

	// Fields managed by separate endpoints are excluded from the PATCH body.
	for _, key := range []string{
		"private_vulnerability_reporting_enabled",
		"vulnerability_alerts_enabled",
		"automated_security_fixes_enabled",
		"topics",
		"default_workflow_permissions",
		"can_approve_pull_request_reviews",
	} {
		if _, ok := gotBody[key]; ok {
			t.Errorf("%s should not be in PATCH body", key)
		}
	}
}

func TestBuildSettingsPayloadExcludesNonPatchFields(t *testing.T) {
	topics := []string{"go", "cli"}
	settings := &model.RepositorySettings{
		Description:                       new("desc"),
		HasWiki:                           new(true),
		PrivateVulnerabilityReportEnabled: new(true),
		VulnerabilityAlertsEnabled:        new(false),
		AutomatedSecurityFixesEnabled:     new(true),
		Topics:                            &topics,
		DefaultWorkflowPermissions:        new("read"),
		CanApprovePullRequestReviews:      new(true),
	}

	body := buildSettingsPayload(settings)

	// PATCH body should contain only the PATCH-eligible fields.
	if _, ok := body["description"]; !ok {
		t.Error("description missing from PATCH body")
	}
	if _, ok := body["has_wiki"]; !ok {
		t.Error("has_wiki missing from PATCH body")
	}
	if len(body) != 2 {
		t.Errorf("body = %v, want only description and has_wiki", body)
	}

	// Non-PATCH fields must not appear in the body.
	for _, key := range []string{
		"private_vulnerability_reporting_enabled",
		"vulnerability_alerts_enabled",
		"automated_security_fixes_enabled",
		"topics",
		"default_workflow_permissions",
		"can_approve_pull_request_reviews",
	} {
		if _, ok := body[key]; ok {
			t.Errorf("%s should not be in PATCH body", key)
		}
	}
}

func TestBuildSettingsPayloadNilSettings(t *testing.T) {
	body := buildSettingsPayload(nil)

	if body == nil || len(body) != 0 {
		t.Errorf("body = %v, want non-nil empty map", body)
	}
}

func TestBuildSettingsPayloadEmptyTopics(t *testing.T) {
	topics := []string{}
	settings := &model.RepositorySettings{
		Topics: &topics,
	}

	body := buildSettingsPayload(settings)

	if _, ok := body["topics"]; ok {
		t.Error("topics should not be in PATCH body")
	}
}

func TestApplyRepoSettingsPatchError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprint(w, `{"message": "Internal Server Error"}`)
	}))
	t.Cleanup(server.Close)

	client := testutil.NewTestClient(t, server)
	settings := &model.RepositorySettings{
		HasWiki: new(true),
	}

	_, err := ApplyRepoSettings(client, "testowner", "testrepo", settings, nil)
	if err == nil {
		t.Fatal("ApplyRepoSettings() expected error from PATCH, got nil")
	}
}

func TestApplyRepoSettingsPatch403Skipped(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		fmt.Fprint(w, `{"message": "Forbidden"}`)
	}))
	t.Cleanup(server.Close)

	client := testutil.NewTestClient(t, server)
	settings := &model.RepositorySettings{
		HasWiki: new(true),
	}

	result, err := ApplyRepoSettings(client, "testowner", "testrepo", settings, nil)
	if err != nil {
		t.Fatalf("ApplyRepoSettings() unexpected hard error: %v", err)
	}
	if len(result.Skipped) != 1 {
		t.Fatalf("expected 1 skipped operation, got %d", len(result.Skipped))
	}
	if result.Skipped[0].Operation.String() != "patch repo settings" {
		t.Errorf("skipped operation = %q, want %q", result.Skipped[0].Operation.String(), "patch repo settings")
	}
}

func TestApplyRepoSettingsWorkflowPermsBothFields(t *testing.T) {
	var gotMethod string
	var gotPath string
	var gotBody map[string]any

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotPath = r.URL.Path
		body, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(body, &gotBody)
		w.WriteHeader(http.StatusNoContent)
	}))
	t.Cleanup(server.Close)

	client := testutil.NewTestClient(t, server)
	settings := &model.RepositorySettings{
		DefaultWorkflowPermissions:   new("read"),
		CanApprovePullRequestReviews: new(false),
	}

	_, err := ApplyRepoSettings(client, "testowner", "testrepo", settings, nil)
	if err != nil {
		t.Fatalf("ApplyRepoSettings() error: %v", err)
	}

	if gotMethod != http.MethodPut {
		t.Errorf("method = %s, want PUT", gotMethod)
	}
	if gotPath != "/repos/testowner/testrepo/actions/permissions/workflow" {
		t.Errorf("path = %s, want /repos/testowner/testrepo/actions/permissions/workflow", gotPath)
	}
	if gotBody["default_workflow_permissions"] != "read" {
		t.Errorf("default_workflow_permissions = %v, want %q", gotBody["default_workflow_permissions"], "read")
	}
	if gotBody["can_approve_pull_request_reviews"] != false {
		t.Errorf("can_approve_pull_request_reviews = %v, want false", gotBody["can_approve_pull_request_reviews"])
	}
}

func TestApplyRepoSettingsWorkflowPermsPartialFetchesCurrent(t *testing.T) {
	var methods []string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		methods = append(methods, r.Method)
		if r.Method == http.MethodGet {
			fmt.Fprint(w, `{"default_workflow_permissions": "write", "can_approve_pull_request_reviews": true}`)
			return
		}
		// The PUT body contains both workflow-permission fields.
		body, _ := io.ReadAll(r.Body)
		var gotBody map[string]any
		_ = json.Unmarshal(body, &gotBody)
		if gotBody["default_workflow_permissions"] != "read" {
			t.Errorf("default_workflow_permissions = %v, want %q", gotBody["default_workflow_permissions"], "read")
		}
		if gotBody["can_approve_pull_request_reviews"] != true {
			t.Errorf("can_approve_pull_request_reviews = %v, want true (from current)", gotBody["can_approve_pull_request_reviews"])
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	t.Cleanup(server.Close)

	client := testutil.NewTestClient(t, server)
	settings := &model.RepositorySettings{
		DefaultWorkflowPermissions: new("read"),
	}

	_, err := ApplyRepoSettings(client, "testowner", "testrepo", settings, nil)
	if err != nil {
		t.Fatalf("ApplyRepoSettings() error: %v", err)
	}

	if len(methods) != 2 {
		t.Fatalf("expected 2 API calls (GET + PUT), got %d: %v", len(methods), methods)
	}
	if methods[0] != http.MethodGet {
		t.Errorf("first call method = %s, want GET", methods[0])
	}
	if methods[1] != http.MethodPut {
		t.Errorf("second call method = %s, want PUT", methods[1])
	}
}

func TestApplyRepoSettingsWorkflowPermsSkippedWhenBothNil(t *testing.T) {
	var methods []string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		methods = append(methods, r.Method)
		w.WriteHeader(http.StatusNoContent)
	}))
	t.Cleanup(server.Close)

	client := testutil.NewTestClient(t, server)
	settings := &model.RepositorySettings{
		HasWiki: new(true),
	}

	_, err := ApplyRepoSettings(client, "testowner", "testrepo", settings, nil)
	if err != nil {
		t.Fatalf("ApplyRepoSettings() error: %v", err)
	}

	// Only PATCH for has_wiki, no workflow permissions call.
	if len(methods) != 1 {
		t.Fatalf("expected 1 API call, got %d: %v", len(methods), methods)
	}
	if methods[0] != http.MethodPatch {
		t.Errorf("method = %s, want PATCH", methods[0])
	}
}

func TestApplyRepoSettingsWorkflowPermsGetError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			w.WriteHeader(http.StatusForbidden)
			fmt.Fprint(w, `{"message": "Forbidden"}`)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	t.Cleanup(server.Close)

	client := testutil.NewTestClient(t, server)
	settings := &model.RepositorySettings{
		CanApprovePullRequestReviews: new(true),
	}

	result, err := ApplyRepoSettings(client, "testowner", "testrepo", settings, nil)
	if err != nil {
		t.Fatalf("ApplyRepoSettings() unexpected hard error: %v", err)
	}
	if len(result.Skipped) != 1 {
		t.Fatalf("expected 1 skipped operation, got %d", len(result.Skipped))
	}
	if result.Skipped[0].Operation.String() != "set workflow permissions" {
		t.Errorf("skipped operation = %q, want %q", result.Skipped[0].Operation.String(), "set workflow permissions")
	}
}

func TestApplyRepoSettingsWorkflowPermsPutError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprint(w, `{"message": "Internal Server Error"}`)
	}))
	t.Cleanup(server.Close)

	client := testutil.NewTestClient(t, server)
	settings := &model.RepositorySettings{
		DefaultWorkflowPermissions:   new("read"),
		CanApprovePullRequestReviews: new(false),
	}

	_, err := ApplyRepoSettings(client, "testowner", "testrepo", settings, nil)
	if err == nil {
		t.Fatal("ApplyRepoSettings() expected error from PUT workflow permissions, got nil")
	}
}

func TestApplyRepoSettingsTopicsPut(t *testing.T) {
	var gotMethod string
	var gotPath string
	var gotBody map[string]any

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotPath = r.URL.Path
		body, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(body, &gotBody)
		w.WriteHeader(http.StatusNoContent)
	}))
	t.Cleanup(server.Close)

	client := testutil.NewTestClient(t, server)
	topics := []string{"go", "cli-tool"}
	settings := &model.RepositorySettings{
		Topics: &topics,
	}

	_, err := ApplyRepoSettings(client, "testowner", "testrepo", settings, nil)
	if err != nil {
		t.Fatalf("ApplyRepoSettings() error: %v", err)
	}

	if gotMethod != http.MethodPut {
		t.Errorf("method = %s, want PUT", gotMethod)
	}
	if gotPath != "/repos/testowner/testrepo/topics" {
		t.Errorf("path = %s, want /repos/testowner/testrepo/topics", gotPath)
	}

	names, ok := gotBody["names"].([]any)
	if !ok {
		t.Fatalf("body missing names key or wrong type: %v", gotBody)
	}
	if len(names) != 2 {
		t.Fatalf("names length = %d, want 2", len(names))
	}
	if names[0] != "go" || names[1] != "cli-tool" {
		t.Errorf("names = %v, want [go cli-tool]", names)
	}
}

func TestApplyRepoSettingsTopicsPutEmpty(t *testing.T) {
	var gotMethod string
	var gotPath string
	var gotBody map[string]any

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotPath = r.URL.Path
		body, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(body, &gotBody)
		w.WriteHeader(http.StatusNoContent)
	}))
	t.Cleanup(server.Close)

	client := testutil.NewTestClient(t, server)
	topics := []string{}
	settings := &model.RepositorySettings{
		Topics: &topics,
	}

	_, err := ApplyRepoSettings(client, "testowner", "testrepo", settings, nil)
	if err != nil {
		t.Fatalf("ApplyRepoSettings() error: %v", err)
	}

	if gotMethod != http.MethodPut {
		t.Errorf("method = %s, want PUT", gotMethod)
	}
	if gotPath != "/repos/testowner/testrepo/topics" {
		t.Errorf("path = %s, want /repos/testowner/testrepo/topics", gotPath)
	}

	names, ok := gotBody["names"].([]any)
	if !ok {
		t.Fatalf("body missing names key or wrong type: %v", gotBody)
	}
	if len(names) != 0 {
		t.Errorf("names length = %d, want 0", len(names))
	}
}

func TestApplyRepoSettingsTopicsSkippedWhenNil(t *testing.T) {
	var methods []string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		methods = append(methods, r.Method)
		w.WriteHeader(http.StatusNoContent)
	}))
	t.Cleanup(server.Close)

	client := testutil.NewTestClient(t, server)
	settings := &model.RepositorySettings{
		HasWiki: new(true),
	}

	_, err := ApplyRepoSettings(client, "testowner", "testrepo", settings, nil)
	if err != nil {
		t.Fatalf("ApplyRepoSettings() error: %v", err)
	}

	// Should only have the PATCH call, no PUT for topics.
	if len(methods) != 1 {
		t.Fatalf("expected 1 API call, got %d: %v", len(methods), methods)
	}
	if methods[0] != http.MethodPatch {
		t.Errorf("method = %s, want PATCH", methods[0])
	}
}

func TestApplyRepoSettingsTopicsError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprint(w, `{"message": "Internal Server Error"}`)
	}))
	t.Cleanup(server.Close)

	client := testutil.NewTestClient(t, server)
	topics := []string{"go"}
	settings := &model.RepositorySettings{
		Topics: &topics,
	}

	_, err := ApplyRepoSettings(client, "testowner", "testrepo", settings, nil)
	if err == nil {
		t.Fatal("ApplyRepoSettings() expected error from topics PUT, got nil")
	}
}

func TestApplyRepoSettingsPartialTopics403(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/repos/testowner/testrepo/topics" {
			w.WriteHeader(http.StatusForbidden)
			fmt.Fprint(w, `{"message": "Resource not accessible by integration"}`)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	t.Cleanup(server.Close)

	client := testutil.NewTestClient(t, server)
	topics := []string{"go"}
	settings := &model.RepositorySettings{
		HasWiki: new(true),
		Topics:  &topics,
	}

	result, err := ApplyRepoSettings(client, "testowner", "testrepo", settings, nil)
	if err != nil {
		t.Fatalf("unexpected hard error: %v", err)
	}
	if len(result.Skipped) != 1 || result.Skipped[0].Operation.String() != "set topics" {
		t.Errorf("Skipped = %v, want [{set topics ...}]", result.Skipped)
	}
}

func TestApplyRepoSettingsAllSkipped(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		fmt.Fprint(w, `{"message": "Resource not accessible by integration"}`)
	}))
	t.Cleanup(server.Close)

	client := testutil.NewTestClient(t, server)
	settings := &model.RepositorySettings{
		HasWiki: new(true),
	}

	result, err := ApplyRepoSettings(client, "testowner", "testrepo", settings, nil)
	if err != nil {
		t.Fatalf("unexpected hard error: %v", err)
	}
	if len(result.Skipped) != 1 {
		t.Errorf("expected 1 skipped, got %d: %v", len(result.Skipped), result.Skipped)
	}
}

func TestApplyRepoSettingsApplyResultPopulatedOnSuccess(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	t.Cleanup(server.Close)

	client := testutil.NewTestClient(t, server)
	settings := &model.RepositorySettings{
		HasWiki: new(true),
	}

	result, err := ApplyRepoSettings(client, "testowner", "testrepo", settings, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Skipped) != 0 {
		t.Errorf("Skipped = %v, want empty", result.Skipped)
	}
}

func TestApplyRepoSettingsSecurityFeatureMethodsAndEnableOrder(t *testing.T) {
	type call struct {
		method string
		path   string
	}
	var calls []call
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls = append(calls, call{method: r.Method, path: r.URL.Path})
		w.WriteHeader(http.StatusNoContent)
	}))
	t.Cleanup(server.Close)

	settings := &model.RepositorySettings{
		PrivateVulnerabilityReportEnabled: new(true),
		VulnerabilityAlertsEnabled:        new(true),
		AutomatedSecurityFixesEnabled:     new(true),
	}
	if _, err := ApplyRepoSettings(testutil.NewTestClient(t, server), "testowner", "testrepo", settings, nil); err != nil {
		t.Fatalf("ApplyRepoSettings() error: %v", err)
	}

	want := []call{
		{http.MethodPut, "/repos/testowner/testrepo/private-vulnerability-reporting"},
		{http.MethodPut, "/repos/testowner/testrepo/vulnerability-alerts"},
		{http.MethodPut, "/repos/testowner/testrepo/automated-security-fixes"},
	}
	if len(calls) != len(want) {
		t.Fatalf("calls = %v, want %v", calls, want)
	}
	for i := range want {
		if calls[i] != want[i] {
			t.Errorf("call[%d] = %v, want %v", i, calls[i], want[i])
		}
	}
}

func TestApplyRepoSettingsSecurityFeatureDisableOrder(t *testing.T) {
	var paths []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete {
			t.Errorf("method = %s, want DELETE", r.Method)
		}
		paths = append(paths, r.URL.Path)
		w.WriteHeader(http.StatusNoContent)
	}))
	t.Cleanup(server.Close)

	settings := &model.RepositorySettings{
		VulnerabilityAlertsEnabled:    new(false),
		AutomatedSecurityFixesEnabled: new(false),
	}
	if _, err := ApplyRepoSettings(testutil.NewTestClient(t, server), "testowner", "testrepo", settings, nil); err != nil {
		t.Fatalf("ApplyRepoSettings() error: %v", err)
	}

	want := []string{
		"/repos/testowner/testrepo/automated-security-fixes",
		"/repos/testowner/testrepo/vulnerability-alerts",
	}
	if fmt.Sprint(paths) != fmt.Sprint(want) {
		t.Errorf("paths = %v, want %v", paths, want)
	}
}

func TestApplyRepoSettingsDoesNotDisableAlertsWhenFixesAreUnmanaged(t *testing.T) {
	var calls int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls++
		w.WriteHeader(http.StatusNoContent)
	}))
	t.Cleanup(server.Close)

	settings := &model.RepositorySettings{VulnerabilityAlertsEnabled: new(false)}
	_, err := ApplyRepoSettings(testutil.NewTestClient(t, server), "testowner", "testrepo", settings, nil)
	if err == nil || err.Error() != "cannot disable vulnerability alerts while automated security fixes are unmanaged" {
		t.Fatalf("ApplyRepoSettings() error = %v, want unmanaged fixes error", err)
	}
	if calls != 0 {
		t.Fatalf("calls = %d, want 0", calls)
	}
}

func TestApplyRepoSettingsSecurityFeatureAccessErrorIsSkipped(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		fmt.Fprint(w, `{"message":"Forbidden"}`)
	}))
	t.Cleanup(server.Close)

	settings := &model.RepositorySettings{VulnerabilityAlertsEnabled: new(true)}
	result, err := ApplyRepoSettings(testutil.NewTestClient(t, server), "testowner", "testrepo", settings, nil)
	if err != nil {
		t.Fatalf("ApplyRepoSettings() error: %v", err)
	}
	if len(result.Skipped) != 1 || result.Skipped[0].Operation.String() != "enable vulnerability alerts" {
		t.Errorf("Skipped = %v, want enable vulnerability alerts", result.Skipped)
	}
}

func TestApplyRepoSettingsSecurityPrerequisiteStopsDependentWrite(t *testing.T) {
	tests := []struct {
		name     string
		settings *model.RepositorySettings
		first    string
	}{
		{
			name: "enable alerts before fixes",
			settings: &model.RepositorySettings{
				VulnerabilityAlertsEnabled:    new(true),
				AutomatedSecurityFixesEnabled: new(true),
			},
			first: "/repos/testowner/testrepo/vulnerability-alerts",
		},
		{
			name: "disable fixes before alerts",
			settings: &model.RepositorySettings{
				VulnerabilityAlertsEnabled:    new(false),
				AutomatedSecurityFixesEnabled: new(false),
			},
			first: "/repos/testowner/testrepo/automated-security-fixes",
		},
	}
	for _, tt := range tests {
		for _, status := range []int{http.StatusForbidden, http.StatusInternalServerError} {
			t.Run(fmt.Sprintf("%s status %d", tt.name, status), func(t *testing.T) {
				var calls []string
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					calls = append(calls, r.URL.Path)
					w.WriteHeader(status)
					fmt.Fprint(w, `{"message":"failed"}`)
				}))
				t.Cleanup(server.Close)

				result, err := ApplyRepoSettings(testutil.NewTestClient(t, server), "testowner", "testrepo", tt.settings, nil)
				if status == http.StatusForbidden {
					if err != nil || len(result.Skipped) != 2 {
						t.Fatalf("ApplyRepoSettings() = %+v, %v, want skipped prerequisite and dependent", result, err)
					}
					if result.Skipped[1].Operation == result.Skipped[0].Operation {
						t.Fatalf("Skipped = %+v, want distinct dependent operation", result.Skipped)
					}
				} else if err == nil {
					t.Fatal("ApplyRepoSettings() error = nil, want prerequisite failure")
				}
				if len(calls) != 1 || calls[0] != tt.first {
					t.Fatalf("calls = %v, want only %s", calls, tt.first)
				}
			})
		}
	}
}

func TestApplyRepoSettingsSecurityFeatureHardErrorStops(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprint(w, `{"message":"Internal Server Error"}`)
	}))
	t.Cleanup(server.Close)

	settings := &model.RepositorySettings{AutomatedSecurityFixesEnabled: new(true)}
	if _, err := ApplyRepoSettings(testutil.NewTestClient(t, server), "testowner", "testrepo", settings, nil); err == nil {
		t.Fatal("ApplyRepoSettings() error = nil, want hard error")
	}
}
