package gh

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/cli/go-gh/v2/pkg/api"
	"github.com/wimpysworld/tailor/internal/config"
	"github.com/wimpysworld/tailor/internal/ptr"
	"github.com/wimpysworld/tailor/internal/testutil"
)

// testTransport redirects all requests to the test server, preserving the
// original request path so the test handler can route by path.
type testTransport struct {
	server *httptest.Server
}

func (t *testTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	req.URL.Scheme = "http"
	req.URL.Host = t.server.Listener.Addr().String()
	return http.DefaultTransport.RoundTrip(req)
}

// newTestClient creates an api.RESTClient that sends all requests to the
// given test server.
func newTestClient(t *testing.T, server *httptest.Server) *api.RESTClient {
	t.Helper()
	client, err := api.NewRESTClient(api.ClientOptions{
		Host:      "github.com",
		AuthToken: "test-token",
		Transport: &testTransport{server: server},
	})
	if err != nil {
		t.Fatalf("NewRESTClient: %v", err)
	}
	return client
}

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
	"topics": ["go", "cli-tool"]
}`

const (
	pvrEnabledJSON   = `{"enabled": true}`
	asfEnabledJSON   = `{"enabled": true}`
	wfPermsReadJSON  = `{"default_workflow_permissions": "read", "can_approve_pull_request_reviews": false}`
	wfPermsWriteJSON = `{"default_workflow_permissions": "write", "can_approve_pull_request_reviews": true}`
)

func TestReadRepoSettings(t *testing.T) {
	tests := []struct {
		name        string
		repoJSON    string
		pvrJSON     string
		asfJSON     string
		wfPermsJSON string
		vaStatus    int // HTTP status for vulnerability-alerts endpoint
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
		wantPVR        bool
		wantASF        bool
		wantVA         bool
		wantTopics     []string
		wantWfPerms    string
		wantCanApprove bool
	}{
		{
			name:           "all fields populated",
			repoJSON:       fullRepoJSON,
			pvrJSON:        pvrEnabledJSON,
			asfJSON:        asfEnabledJSON,
			wfPermsJSON:    wfPermsWriteJSON,
			vaStatus:       http.StatusNoContent,
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
			wantPVR:        true,
			wantASF:        true,
			wantVA:         true,
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
			pvrJSON:        `{"enabled": false}`,
			asfJSON:        `{"enabled": false}`,
			wfPermsJSON:    wfPermsReadJSON,
			vaStatus:       http.StatusNotFound,
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
			wantPVR:        false,
			wantASF:        false,
			wantVA:         false,
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
				case "/repos/testowner/testrepo/private-vulnerability-reporting":
					fmt.Fprint(w, tt.pvrJSON)
				case "/repos/testowner/testrepo/automated-security-fixes":
					fmt.Fprint(w, tt.asfJSON)
				case "/repos/testowner/testrepo/vulnerability-alerts":
					w.WriteHeader(tt.vaStatus)
				case "/repos/testowner/testrepo/actions/permissions/workflow":
					fmt.Fprint(w, tt.wfPermsJSON)
				default:
					http.NotFound(w, r)
				}
			}))
			t.Cleanup(server.Close)

			client := newTestClient(t, server)
			settings, err := ReadRepoSettings(client, "testowner", "testrepo")
			if err != nil {
				t.Fatalf("ReadRepoSettings() error: %v", err)
			}

			// description and homepage
			testutil.AssertStringPtr(t, settings.Description, tt.wantDescNil, tt.wantDesc, "description")
			testutil.AssertStringPtr(t, settings.Homepage, tt.wantHomeNil, tt.wantHome, "homepage")

			// bool fields
			testutil.AssertBoolPtr(t, settings.HasWiki, false, tt.wantWiki, "has_wiki")
			testutil.AssertBoolPtr(t, settings.HasDiscussions, false, tt.wantDisc, "has_discussions")
			testutil.AssertBoolPtr(t, settings.HasProjects, false, tt.wantProj, "has_projects")
			testutil.AssertBoolPtr(t, settings.HasIssues, false, tt.wantIssues, "has_issues")
			testutil.AssertBoolPtr(t, settings.AllowMergeCommit, false, tt.wantMerge, "allow_merge_commit")
			testutil.AssertBoolPtr(t, settings.AllowSquashMerge, false, tt.wantSquash, "allow_squash_merge")
			testutil.AssertBoolPtr(t, settings.AllowRebaseMerge, false, tt.wantRebase, "allow_rebase_merge")
			testutil.AssertBoolPtr(t, settings.DeleteBranchOnMerge, false, tt.wantDelete, "delete_branch_on_merge")
			testutil.AssertBoolPtr(t, settings.AllowUpdateBranch, false, tt.wantUpdate, "allow_update_branch")
			testutil.AssertBoolPtr(t, settings.AllowAutoMerge, false, tt.wantAuto, "allow_auto_merge")
			testutil.AssertBoolPtr(t, settings.WebCommitSignoffRequired, false, tt.wantSignoff, "web_commit_signoff_required")
			testutil.AssertBoolPtr(t, settings.PrivateVulnerabilityReportEnabled, false, tt.wantPVR, "private_vulnerability_reporting_enabled")
			testutil.AssertBoolPtr(t, settings.AutomatedSecurityFixesEnabled, false, tt.wantASF, "automated_security_fixes_enabled")
			testutil.AssertBoolPtr(t, settings.VulnerabilityAlertsEnabled, false, tt.wantVA, "vulnerability_alerts_enabled")
			testutil.AssertBoolPtr(t, settings.CanApprovePullRequestReviews, false, tt.wantCanApprove, "can_approve_pull_request_reviews")

			// string fields (always non-nil)
			testutil.AssertStringPtr(t, settings.DefaultWorkflowPermissions, false, tt.wantWfPerms, "default_workflow_permissions")
			testutil.AssertStringPtr(t, settings.SquashMergeCommitTitle, false, tt.wantSqTitle, "squash_merge_commit_title")
			testutil.AssertStringPtr(t, settings.SquashMergeCommitMessage, false, tt.wantSqMsg, "squash_merge_commit_message")
			testutil.AssertStringPtr(t, settings.MergeCommitTitle, false, tt.wantMcTitle, "merge_commit_title")
			testutil.AssertStringPtr(t, settings.MergeCommitMessage, false, tt.wantMcMsg, "merge_commit_message")

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

func TestReadRepoSettingsRepoAPIError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		fmt.Fprint(w, `{"message": "Not Found"}`)
	}))
	t.Cleanup(server.Close)

	client := newTestClient(t, server)
	_, err := ReadRepoSettings(client, "testowner", "testrepo")
	if err == nil {
		t.Fatal("ReadRepoSettings() expected error, got nil")
	}
}

func TestReadRepoSettingsPVRAPIError(t *testing.T) {
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

	client := newTestClient(t, server)
	_, err := ReadRepoSettings(client, "testowner", "testrepo")
	if err == nil {
		t.Fatal("ReadRepoSettings() expected error for PVR failure, got nil")
	}
}

func TestReadRepoSettingsVAAPIError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/repos/testowner/testrepo":
			fmt.Fprint(w, fullRepoJSON)
		case "/repos/testowner/testrepo/private-vulnerability-reporting":
			fmt.Fprint(w, pvrEnabledJSON)
		case "/repos/testowner/testrepo/automated-security-fixes":
			fmt.Fprint(w, asfEnabledJSON)
		case "/repos/testowner/testrepo/vulnerability-alerts":
			w.WriteHeader(http.StatusForbidden)
			fmt.Fprint(w, `{"message": "Forbidden"}`)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)

	client := newTestClient(t, server)
	_, err := ReadRepoSettings(client, "testowner", "testrepo")
	if err == nil {
		t.Fatal("ReadRepoSettings() expected error for VA failure, got nil")
	}
}

func TestReadRepoSettingsASFAPIError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/repos/testowner/testrepo":
			fmt.Fprint(w, fullRepoJSON)
		case "/repos/testowner/testrepo/private-vulnerability-reporting":
			fmt.Fprint(w, pvrEnabledJSON)
		case "/repos/testowner/testrepo/automated-security-fixes":
			w.WriteHeader(http.StatusForbidden)
			fmt.Fprint(w, `{"message": "Forbidden"}`)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)

	client := newTestClient(t, server)
	_, err := ReadRepoSettings(client, "testowner", "testrepo")
	if err == nil {
		t.Fatal("ReadRepoSettings() expected error for ASF failure, got nil")
	}
}

func TestReadRepoSettingsWFPermsAPIError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/repos/testowner/testrepo":
			fmt.Fprint(w, fullRepoJSON)
		case "/repos/testowner/testrepo/private-vulnerability-reporting":
			fmt.Fprint(w, pvrEnabledJSON)
		case "/repos/testowner/testrepo/automated-security-fixes":
			fmt.Fprint(w, asfEnabledJSON)
		case "/repos/testowner/testrepo/vulnerability-alerts":
			w.WriteHeader(http.StatusNoContent)
		case "/repos/testowner/testrepo/actions/permissions/workflow":
			w.WriteHeader(http.StatusForbidden)
			fmt.Fprint(w, `{"message": "Forbidden"}`)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)

	client := newTestClient(t, server)
	_, err := ReadRepoSettings(client, "testowner", "testrepo")
	if err == nil {
		t.Fatal("ReadRepoSettings() expected error for workflow permissions failure, got nil")
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

	client := newTestClient(t, server)
	settings := &config.RepositorySettings{
		Description:    ptr.String("new desc"),
		HasWiki:        ptr.Bool(true),
		AllowAutoMerge: ptr.Bool(false),
	}

	err := ApplyRepoSettings(client, "testowner", "testrepo", settings)
	if err != nil {
		t.Fatalf("ApplyRepoSettings() error: %v", err)
	}

	if gotMethod != http.MethodPatch {
		t.Errorf("method = %s, want PATCH", gotMethod)
	}
	if gotPath != "/repos/testowner/testrepo" {
		t.Errorf("path = %s, want /repos/testowner/testrepo", gotPath)
	}

	// Verify non-nil fields present with correct values.
	if gotBody["description"] != "new desc" {
		t.Errorf("description = %v, want %q", gotBody["description"], "new desc")
	}
	if gotBody["has_wiki"] != true {
		t.Errorf("has_wiki = %v, want true", gotBody["has_wiki"])
	}
	if gotBody["allow_auto_merge"] != false {
		t.Errorf("allow_auto_merge = %v, want false", gotBody["allow_auto_merge"])
	}

	// Verify nil fields excluded.
	if _, ok := gotBody["homepage"]; ok {
		t.Error("homepage should not be in PATCH body when nil")
	}

	// Verify all six non-PATCH fields excluded from body.
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

func TestBuildSettingsPayloadExtractsAllNonPatchFields(t *testing.T) {
	topics := []string{"go", "cli"}
	settings := &config.RepositorySettings{
		Description:                       ptr.String("desc"),
		HasWiki:                           ptr.Bool(true),
		PrivateVulnerabilityReportEnabled: ptr.Bool(true),
		VulnerabilityAlertsEnabled:        ptr.Bool(true),
		AutomatedSecurityFixesEnabled:     ptr.Bool(false),
		Topics:                            &topics,
		DefaultWorkflowPermissions:        ptr.String("read"),
		CanApprovePullRequestReviews:      ptr.Bool(true),
	}

	p := buildSettingsPayload(settings)

	// PATCH body should contain only the PATCH-eligible fields.
	if _, ok := p.Body["description"]; !ok {
		t.Error("description missing from PATCH body")
	}
	if _, ok := p.Body["has_wiki"]; !ok {
		t.Error("has_wiki missing from PATCH body")
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
		if _, ok := p.Body[key]; ok {
			t.Errorf("%s should not be in PATCH body", key)
		}
	}

	// Verify extracted fields.
	if p.PrivateVulnerabilityReporting == nil || *p.PrivateVulnerabilityReporting != true {
		t.Errorf("PrivateVulnerabilityReporting = %v, want ptr(true)", p.PrivateVulnerabilityReporting)
	}
	if p.VulnerabilityAlerts == nil || *p.VulnerabilityAlerts != true {
		t.Errorf("VulnerabilityAlerts = %v, want ptr(true)", p.VulnerabilityAlerts)
	}
	if p.AutomatedSecurityFixes == nil || *p.AutomatedSecurityFixes != false {
		t.Errorf("AutomatedSecurityFixes = %v, want ptr(false)", p.AutomatedSecurityFixes)
	}
	if p.Topics == nil {
		t.Fatal("Topics is nil, want non-nil")
	}
	if len(*p.Topics) != 2 || (*p.Topics)[0] != "go" || (*p.Topics)[1] != "cli" {
		t.Errorf("Topics = %v, want [go cli]", *p.Topics)
	}
	if p.DefaultWorkflowPermissions == nil || *p.DefaultWorkflowPermissions != "read" {
		t.Errorf("DefaultWorkflowPermissions = %v, want ptr(read)", p.DefaultWorkflowPermissions)
	}
	if p.CanApprovePullRequestReviews == nil || *p.CanApprovePullRequestReviews != true {
		t.Errorf("CanApprovePullRequestReviews = %v, want ptr(true)", p.CanApprovePullRequestReviews)
	}
}

func TestBuildSettingsPayloadNilFieldsStayNil(t *testing.T) {
	settings := &config.RepositorySettings{
		HasWiki: ptr.Bool(true),
	}

	p := buildSettingsPayload(settings)

	if p.PrivateVulnerabilityReporting != nil {
		t.Errorf("PrivateVulnerabilityReporting = %v, want nil", p.PrivateVulnerabilityReporting)
	}
	if p.VulnerabilityAlerts != nil {
		t.Errorf("VulnerabilityAlerts = %v, want nil", p.VulnerabilityAlerts)
	}
	if p.AutomatedSecurityFixes != nil {
		t.Errorf("AutomatedSecurityFixes = %v, want nil", p.AutomatedSecurityFixes)
	}
	if p.Topics != nil {
		t.Errorf("Topics = %v, want nil", p.Topics)
	}
	if p.DefaultWorkflowPermissions != nil {
		t.Errorf("DefaultWorkflowPermissions = %v, want nil", p.DefaultWorkflowPermissions)
	}
	if p.CanApprovePullRequestReviews != nil {
		t.Errorf("CanApprovePullRequestReviews = %v, want nil", p.CanApprovePullRequestReviews)
	}

	if _, ok := p.Body["has_wiki"]; !ok {
		t.Error("has_wiki missing from PATCH body")
	}
}

func TestBuildSettingsPayloadEmptyTopics(t *testing.T) {
	topics := []string{}
	settings := &config.RepositorySettings{
		Topics: &topics,
	}

	p := buildSettingsPayload(settings)

	if p.Topics == nil {
		t.Fatal("Topics is nil, want non-nil empty slice")
	}
	if len(*p.Topics) != 0 {
		t.Errorf("Topics length = %d, want 0", len(*p.Topics))
	}
	if _, ok := p.Body["topics"]; ok {
		t.Error("topics should not be in PATCH body")
	}
}

func TestApplyRepoSettingsPVRPut(t *testing.T) {
	var gotMethod string
	var gotPath string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotPath = r.URL.Path
		w.WriteHeader(http.StatusNoContent)
	}))
	t.Cleanup(server.Close)

	client := newTestClient(t, server)
	settings := &config.RepositorySettings{
		PrivateVulnerabilityReportEnabled: ptr.Bool(true),
	}

	err := ApplyRepoSettings(client, "testowner", "testrepo", settings)
	if err != nil {
		t.Fatalf("ApplyRepoSettings() error: %v", err)
	}

	if gotMethod != http.MethodPut {
		t.Errorf("method = %s, want PUT", gotMethod)
	}
	if gotPath != "/repos/testowner/testrepo/private-vulnerability-reporting" {
		t.Errorf("path = %s, want /repos/testowner/testrepo/private-vulnerability-reporting", gotPath)
	}
}

func TestApplyRepoSettingsPVRDelete(t *testing.T) {
	var gotMethod string
	var gotPath string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotPath = r.URL.Path
		w.WriteHeader(http.StatusNoContent)
	}))
	t.Cleanup(server.Close)

	client := newTestClient(t, server)
	settings := &config.RepositorySettings{
		PrivateVulnerabilityReportEnabled: ptr.Bool(false),
	}

	err := ApplyRepoSettings(client, "testowner", "testrepo", settings)
	if err != nil {
		t.Fatalf("ApplyRepoSettings() error: %v", err)
	}

	if gotMethod != http.MethodDelete {
		t.Errorf("method = %s, want DELETE", gotMethod)
	}
	if gotPath != "/repos/testowner/testrepo/private-vulnerability-reporting" {
		t.Errorf("path = %s, want /repos/testowner/testrepo/private-vulnerability-reporting", gotPath)
	}
}

func TestApplyRepoSettingsNoPatchWhenOnlyPVR(t *testing.T) {
	var methods []string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		methods = append(methods, r.Method)
		w.WriteHeader(http.StatusNoContent)
	}))
	t.Cleanup(server.Close)

	client := newTestClient(t, server)
	settings := &config.RepositorySettings{
		PrivateVulnerabilityReportEnabled: ptr.Bool(true),
	}

	err := ApplyRepoSettings(client, "testowner", "testrepo", settings)
	if err != nil {
		t.Fatalf("ApplyRepoSettings() error: %v", err)
	}

	if len(methods) != 1 {
		t.Fatalf("expected 1 API call, got %d: %v", len(methods), methods)
	}
	if methods[0] != http.MethodPut {
		t.Errorf("single call method = %s, want PUT (no PATCH)", methods[0])
	}
}

func TestApplyRepoSettingsPatchError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		fmt.Fprint(w, `{"message": "Forbidden"}`)
	}))
	t.Cleanup(server.Close)

	client := newTestClient(t, server)
	settings := &config.RepositorySettings{
		HasWiki: ptr.Bool(true),
	}

	err := ApplyRepoSettings(client, "testowner", "testrepo", settings)
	if err == nil {
		t.Fatal("ApplyRepoSettings() expected error from PATCH, got nil")
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

	client := newTestClient(t, server)
	settings := &config.RepositorySettings{
		DefaultWorkflowPermissions:   ptr.String("read"),
		CanApprovePullRequestReviews: ptr.Bool(false),
	}

	err := ApplyRepoSettings(client, "testowner", "testrepo", settings)
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
		// Verify the PUT body contains both fields.
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

	client := newTestClient(t, server)
	settings := &config.RepositorySettings{
		DefaultWorkflowPermissions: ptr.String("read"),
	}

	err := ApplyRepoSettings(client, "testowner", "testrepo", settings)
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

	client := newTestClient(t, server)
	settings := &config.RepositorySettings{
		HasWiki: ptr.Bool(true),
	}

	err := ApplyRepoSettings(client, "testowner", "testrepo", settings)
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

	client := newTestClient(t, server)
	settings := &config.RepositorySettings{
		CanApprovePullRequestReviews: ptr.Bool(true),
	}

	err := ApplyRepoSettings(client, "testowner", "testrepo", settings)
	if err == nil {
		t.Fatal("ApplyRepoSettings() expected error from GET workflow permissions, got nil")
	}
}

func TestApplyRepoSettingsWorkflowPermsPutError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprint(w, `{"message": "Internal Server Error"}`)
	}))
	t.Cleanup(server.Close)

	client := newTestClient(t, server)
	settings := &config.RepositorySettings{
		DefaultWorkflowPermissions:   ptr.String("read"),
		CanApprovePullRequestReviews: ptr.Bool(false),
	}

	err := ApplyRepoSettings(client, "testowner", "testrepo", settings)
	if err == nil {
		t.Fatal("ApplyRepoSettings() expected error from PUT workflow permissions, got nil")
	}
}

func TestApplyRepoSettingsPVRError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprint(w, `{"message": "Internal Server Error"}`)
	}))
	t.Cleanup(server.Close)

	client := newTestClient(t, server)
	settings := &config.RepositorySettings{
		PrivateVulnerabilityReportEnabled: ptr.Bool(true),
	}

	err := ApplyRepoSettings(client, "testowner", "testrepo", settings)
	if err == nil {
		t.Fatal("ApplyRepoSettings() expected error from PUT, got nil")
	}
}

func TestApplyRepoSettingsVAPut(t *testing.T) {
	var gotMethod string
	var gotPath string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotPath = r.URL.Path
		w.WriteHeader(http.StatusNoContent)
	}))
	t.Cleanup(server.Close)

	client := newTestClient(t, server)
	settings := &config.RepositorySettings{
		VulnerabilityAlertsEnabled: ptr.Bool(true),
	}

	err := ApplyRepoSettings(client, "testowner", "testrepo", settings)
	if err != nil {
		t.Fatalf("ApplyRepoSettings() error: %v", err)
	}

	if gotMethod != http.MethodPut {
		t.Errorf("method = %s, want PUT", gotMethod)
	}
	if gotPath != "/repos/testowner/testrepo/vulnerability-alerts" {
		t.Errorf("path = %s, want /repos/testowner/testrepo/vulnerability-alerts", gotPath)
	}
}

func TestApplyRepoSettingsVADelete(t *testing.T) {
	var gotMethod string
	var gotPath string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotPath = r.URL.Path
		w.WriteHeader(http.StatusNoContent)
	}))
	t.Cleanup(server.Close)

	client := newTestClient(t, server)
	settings := &config.RepositorySettings{
		VulnerabilityAlertsEnabled: ptr.Bool(false),
	}

	err := ApplyRepoSettings(client, "testowner", "testrepo", settings)
	if err != nil {
		t.Fatalf("ApplyRepoSettings() error: %v", err)
	}

	if gotMethod != http.MethodDelete {
		t.Errorf("method = %s, want DELETE", gotMethod)
	}
	if gotPath != "/repos/testowner/testrepo/vulnerability-alerts" {
		t.Errorf("path = %s, want /repos/testowner/testrepo/vulnerability-alerts", gotPath)
	}
}

func TestApplyRepoSettingsASFPut(t *testing.T) {
	var gotMethod string
	var gotPath string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotPath = r.URL.Path
		w.WriteHeader(http.StatusNoContent)
	}))
	t.Cleanup(server.Close)

	client := newTestClient(t, server)
	settings := &config.RepositorySettings{
		AutomatedSecurityFixesEnabled: ptr.Bool(true),
	}

	err := ApplyRepoSettings(client, "testowner", "testrepo", settings)
	if err != nil {
		t.Fatalf("ApplyRepoSettings() error: %v", err)
	}

	if gotMethod != http.MethodPut {
		t.Errorf("method = %s, want PUT", gotMethod)
	}
	if gotPath != "/repos/testowner/testrepo/automated-security-fixes" {
		t.Errorf("path = %s, want /repos/testowner/testrepo/automated-security-fixes", gotPath)
	}
}

func TestApplyRepoSettingsASFDelete(t *testing.T) {
	var gotMethod string
	var gotPath string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotPath = r.URL.Path
		w.WriteHeader(http.StatusNoContent)
	}))
	t.Cleanup(server.Close)

	client := newTestClient(t, server)
	settings := &config.RepositorySettings{
		AutomatedSecurityFixesEnabled: ptr.Bool(false),
	}

	err := ApplyRepoSettings(client, "testowner", "testrepo", settings)
	if err != nil {
		t.Fatalf("ApplyRepoSettings() error: %v", err)
	}

	if gotMethod != http.MethodDelete {
		t.Errorf("method = %s, want DELETE", gotMethod)
	}
	if gotPath != "/repos/testowner/testrepo/automated-security-fixes" {
		t.Errorf("path = %s, want /repos/testowner/testrepo/automated-security-fixes", gotPath)
	}
}

func TestApplyRepoSettingsEnableBothVABeforeASF(t *testing.T) {
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

	client := newTestClient(t, server)
	settings := &config.RepositorySettings{
		VulnerabilityAlertsEnabled:    ptr.Bool(true),
		AutomatedSecurityFixesEnabled: ptr.Bool(true),
	}

	err := ApplyRepoSettings(client, "testowner", "testrepo", settings)
	if err != nil {
		t.Fatalf("ApplyRepoSettings() error: %v", err)
	}

	if len(calls) != 2 {
		t.Fatalf("expected 2 API calls, got %d: %v", len(calls), calls)
	}

	// Alerts must be enabled before security fixes.
	if calls[0].method != http.MethodPut || calls[0].path != "/repos/testowner/testrepo/vulnerability-alerts" {
		t.Errorf("call[0] = %s %s, want PUT /repos/testowner/testrepo/vulnerability-alerts", calls[0].method, calls[0].path)
	}
	if calls[1].method != http.MethodPut || calls[1].path != "/repos/testowner/testrepo/automated-security-fixes" {
		t.Errorf("call[1] = %s %s, want PUT /repos/testowner/testrepo/automated-security-fixes", calls[1].method, calls[1].path)
	}
}

func TestApplyRepoSettingsDisableBothASFBeforeVA(t *testing.T) {
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

	client := newTestClient(t, server)
	settings := &config.RepositorySettings{
		VulnerabilityAlertsEnabled:    ptr.Bool(false),
		AutomatedSecurityFixesEnabled: ptr.Bool(false),
	}

	err := ApplyRepoSettings(client, "testowner", "testrepo", settings)
	if err != nil {
		t.Fatalf("ApplyRepoSettings() error: %v", err)
	}

	if len(calls) != 2 {
		t.Fatalf("expected 2 API calls, got %d: %v", len(calls), calls)
	}

	// Security fixes must be disabled before alerts.
	if calls[0].method != http.MethodDelete || calls[0].path != "/repos/testowner/testrepo/automated-security-fixes" {
		t.Errorf("call[0] = %s %s, want DELETE /repos/testowner/testrepo/automated-security-fixes", calls[0].method, calls[0].path)
	}
	if calls[1].method != http.MethodDelete || calls[1].path != "/repos/testowner/testrepo/vulnerability-alerts" {
		t.Errorf("call[1] = %s %s, want DELETE /repos/testowner/testrepo/vulnerability-alerts", calls[1].method, calls[1].path)
	}
}

func TestApplyRepoSettingsVAError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprint(w, `{"message": "Internal Server Error"}`)
	}))
	t.Cleanup(server.Close)

	client := newTestClient(t, server)
	settings := &config.RepositorySettings{
		VulnerabilityAlertsEnabled: ptr.Bool(true),
	}

	err := ApplyRepoSettings(client, "testowner", "testrepo", settings)
	if err == nil {
		t.Fatal("ApplyRepoSettings() expected error from VA PUT, got nil")
	}
}

func TestApplyRepoSettingsASFError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprint(w, `{"message": "Internal Server Error"}`)
	}))
	t.Cleanup(server.Close)

	client := newTestClient(t, server)
	settings := &config.RepositorySettings{
		AutomatedSecurityFixesEnabled: ptr.Bool(true),
	}

	err := ApplyRepoSettings(client, "testowner", "testrepo", settings)
	if err == nil {
		t.Fatal("ApplyRepoSettings() expected error from ASF PUT, got nil")
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

	client := newTestClient(t, server)
	topics := []string{"go", "cli-tool"}
	settings := &config.RepositorySettings{
		Topics: &topics,
	}

	err := ApplyRepoSettings(client, "testowner", "testrepo", settings)
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

	client := newTestClient(t, server)
	topics := []string{}
	settings := &config.RepositorySettings{
		Topics: &topics,
	}

	err := ApplyRepoSettings(client, "testowner", "testrepo", settings)
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

	client := newTestClient(t, server)
	settings := &config.RepositorySettings{
		HasWiki: ptr.Bool(true),
	}

	err := ApplyRepoSettings(client, "testowner", "testrepo", settings)
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

	client := newTestClient(t, server)
	topics := []string{"go"}
	settings := &config.RepositorySettings{
		Topics: &topics,
	}

	err := ApplyRepoSettings(client, "testowner", "testrepo", settings)
	if err == nil {
		t.Fatal("ApplyRepoSettings() expected error from topics PUT, got nil")
	}
}

func TestApplyRepoSettingsNoPatchWhenOnlyVAAndASF(t *testing.T) {
	var methods []string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		methods = append(methods, r.Method)
		w.WriteHeader(http.StatusNoContent)
	}))
	t.Cleanup(server.Close)

	client := newTestClient(t, server)
	settings := &config.RepositorySettings{
		VulnerabilityAlertsEnabled:    ptr.Bool(true),
		AutomatedSecurityFixesEnabled: ptr.Bool(true),
	}

	err := ApplyRepoSettings(client, "testowner", "testrepo", settings)
	if err != nil {
		t.Fatalf("ApplyRepoSettings() error: %v", err)
	}

	if len(methods) != 2 {
		t.Fatalf("expected 2 API calls, got %d: %v", len(methods), methods)
	}
	for _, m := range methods {
		if m == http.MethodPatch {
			t.Error("should not send PATCH when only VA and ASF are set")
		}
	}
}
