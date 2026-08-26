package alter_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/wimpysworld/tailor/internal/alter"
	"github.com/wimpysworld/tailor/internal/config"
	"github.com/wimpysworld/tailor/internal/gh"
	"github.com/wimpysworld/tailor/internal/ghfake"
	"github.com/wimpysworld/tailor/internal/model"
	"github.com/wimpysworld/tailor/internal/testutil"
)

// repoJSON mirrors the repoResponse struct in the gh package for building
// mock GET /repos/{owner}/{repo} responses.
type repoJSON struct {
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

// settingsServer creates an httptest server that responds to repo settings
// GET, PATCH, and PUT requests. patchCalled is incremented when PATCH or PUT
// is received.
func settingsServer(repo repoJSON, patchCalled *atomic.Int32) *httptest.Server {
	repo.Permissions.Admin = true
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path

		switch {
		case r.Method == http.MethodGet && path == "/repos/testowner/testrepo":
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(repo)

		case r.Method == http.MethodGet && path == "/repos/testowner/testrepo/actions/permissions/workflow":
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprint(w, `{"default_workflow_permissions":"read","can_approve_pull_request_reviews":false}`)

		case r.Method == http.MethodGet && path == "/repos/testowner/testrepo/private-vulnerability-reporting":
			fmt.Fprint(w, `{"enabled":false}`)

		case r.Method == http.MethodGet && path == "/repos/testowner/testrepo/automated-security-fixes":
			fmt.Fprint(w, `{"enabled":false,"paused":false}`)

		case r.Method == http.MethodPatch && path == "/repos/testowner/testrepo":
			if patchCalled != nil {
				patchCalled.Add(1)
			}
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			fmt.Fprint(w, `{}`)

		case r.Method == http.MethodPut && path == "/repos/testowner/testrepo/actions/permissions/workflow":
			if patchCalled != nil {
				patchCalled.Add(1)
			}
			w.WriteHeader(http.StatusNoContent)

		default:
			w.WriteHeader(http.StatusNotFound)
			fmt.Fprintf(w, `{"message":"Not Found: %s %s"}`, r.Method, path) //nolint:gosec // test HTTP handler, not exposed to user input
		}
	}))
}

func failingSettingsServer() *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprint(w, `{"message":"Internal Server Error"}`)
	}))
}

func TestProcessRepoSettingsNilRepository(t *testing.T) {
	cfg := &config.Config{}
	results, err := alter.ProcessRepoSettings(cfg, alter.DryRun, repoTarget(nil, "", "", false))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if results != nil {
		t.Errorf("expected nil results, got %v", results)
	}
}

func TestProcessRepoSettingsNoRepoContext(t *testing.T) {
	ghfake.FakeNoRepo(t)

	cfg := &config.Config{
		Repository: &model.RepositorySettings{
			HasWiki: new(false),
		},
	}

	var stderr strings.Builder
	target := repoTarget(nil, "", "", false)
	target.Stderr = &stderr
	results, err := alter.ProcessRepoSettings(cfg, alter.DryRun, target)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if results != nil {
		t.Errorf("expected nil results, got %v", results)
	}

	want := "No GitHub repository context found."
	if !strings.Contains(stderr.String(), want) {
		t.Errorf("stderr = %q, want substring %q", stderr.String(), want)
	}
}

func TestProcessRepoSettingsWouldSetWhenDiffer(t *testing.T) {
	ghfake.FakeRepo(t, "testowner", "testrepo")

	live := repoJSON{HasWiki: true, HasIssues: true}
	server := settingsServer(live, nil)
	t.Cleanup(server.Close)
	client := testutil.NewTestClient(t, server)

	cfg := &config.Config{
		Repository: &model.RepositorySettings{
			HasWiki: new(false),
		},
	}

	results, err := alter.ProcessRepoSettings(cfg, alter.DryRun, repoTarget(client, "testowner", "testrepo", true))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("got %d results, want 1", len(results))
	}
	if results[0].Category != alter.WouldSet {
		t.Errorf("category = %q, want %q", results[0].Category, alter.WouldSet)
	}
	if results[0].Field != "has_wiki" {
		t.Errorf("field = %q, want %q", results[0].Field, "has_wiki")
	}
	if results[0].Value != "false" {
		t.Errorf("value = %q, want %q", results[0].Value, "false")
	}
}

func TestProcessRepoSettingsNoChangeWhenMatch(t *testing.T) {
	ghfake.FakeRepo(t, "testowner", "testrepo")

	live := repoJSON{HasWiki: false, HasIssues: true}
	server := settingsServer(live, nil)
	t.Cleanup(server.Close)
	client := testutil.NewTestClient(t, server)

	cfg := &config.Config{
		Repository: &model.RepositorySettings{
			HasWiki: new(false),
		},
	}

	results, err := alter.ProcessRepoSettings(cfg, alter.DryRun, repoTarget(client, "testowner", "testrepo", true))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("got %d results, want 1", len(results))
	}
	if results[0].Category != alter.RepoNoChange {
		t.Errorf("category = %q, want %q", results[0].Category, alter.RepoNoChange)
	}
	if results[0].Field != "has_wiki" {
		t.Errorf("field = %q, want %q", results[0].Field, "has_wiki")
	}
	if results[0].Value != "false" {
		t.Errorf("value = %q, want %q", results[0].Value, "false")
	}
}

func TestProcessRepoSettingsApplyCallsAPI(t *testing.T) {
	ghfake.FakeRepo(t, "testowner", "testrepo")

	var patchCalled atomic.Int32
	live := repoJSON{HasWiki: true}
	server := settingsServer(live, &patchCalled)
	t.Cleanup(server.Close)
	client := testutil.NewTestClient(t, server)

	cfg := &config.Config{
		Repository: &model.RepositorySettings{
			HasWiki: new(false),
		},
	}

	_, err := alter.ProcessRepoSettings(cfg, alter.Apply, repoTarget(client, "testowner", "testrepo", true))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if patchCalled.Load() == 0 {
		t.Error("expected PATCH call on Apply, but none received")
	}
}

func TestProcessRepoSettingsRecutCallsAPI(t *testing.T) {
	ghfake.FakeRepo(t, "testowner", "testrepo")

	var patchCalled atomic.Int32
	live := repoJSON{HasWiki: true}
	server := settingsServer(live, &patchCalled)
	t.Cleanup(server.Close)
	client := testutil.NewTestClient(t, server)

	cfg := &config.Config{
		Repository: &model.RepositorySettings{
			HasWiki: new(false),
		},
	}

	_, err := alter.ProcessRepoSettings(cfg, alter.Recut, repoTarget(client, "testowner", "testrepo", true))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if patchCalled.Load() == 0 {
		t.Error("expected PATCH call on Recut, but none received")
	}
}

func TestProcessRepoSettingsDryRunDoesNotCallAPI(t *testing.T) {
	ghfake.FakeRepo(t, "testowner", "testrepo")

	var patchCalled atomic.Int32
	live := repoJSON{HasWiki: true}
	server := settingsServer(live, &patchCalled)
	t.Cleanup(server.Close)
	client := testutil.NewTestClient(t, server)

	cfg := &config.Config{
		Repository: &model.RepositorySettings{
			HasWiki: new(false),
		},
	}

	_, err := alter.ProcessRepoSettings(cfg, alter.DryRun, repoTarget(client, "testowner", "testrepo", true))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if patchCalled.Load() != 0 {
		t.Error("DryRun should not PATCH, but PATCH was called")
	}
}

func TestProcessRepoSettingsNoApplyWhenAllMatch(t *testing.T) {
	ghfake.FakeRepo(t, "testowner", "testrepo")

	var patchCalled atomic.Int32
	live := repoJSON{HasWiki: false, HasIssues: true}
	server := settingsServer(live, &patchCalled)
	t.Cleanup(server.Close)
	client := testutil.NewTestClient(t, server)

	cfg := &config.Config{
		Repository: &model.RepositorySettings{
			HasWiki:   new(false),
			HasIssues: new(true),
		},
	}

	_, err := alter.ProcessRepoSettings(cfg, alter.Apply, repoTarget(client, "testowner", "testrepo", true))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if patchCalled.Load() != 0 {
		t.Error("should not PATCH when all settings match, but PATCH was called")
	}
}

func TestProcessRepoSettingsErrorPropagated(t *testing.T) {
	ghfake.FakeRepo(t, "testowner", "testrepo")

	server := failingSettingsServer()
	t.Cleanup(server.Close)
	client := testutil.NewTestClient(t, server)

	cfg := &config.Config{
		Repository: &model.RepositorySettings{
			HasWiki: new(false),
		},
	}

	_, err := alter.ProcessRepoSettings(cfg, alter.DryRun, repoTarget(client, "testowner", "testrepo", true))
	if err == nil {
		t.Fatal("expected error from API failure, got nil")
	}
}

func TestProcessRepoSettingsMixedResults(t *testing.T) {
	ghfake.FakeRepo(t, "testowner", "testrepo")

	live := repoJSON{
		HasWiki:             true,
		HasIssues:           true,
		Description:         "My project",
		DeleteBranchOnMerge: false,
	}
	server := settingsServer(live, nil)
	t.Cleanup(server.Close)
	client := testutil.NewTestClient(t, server)

	cfg := &config.Config{
		Repository: &model.RepositorySettings{
			HasWiki:             new(false), // differs
			HasIssues:           new(true),  // matches
			Description:         new("New"), // differs
			DeleteBranchOnMerge: new(true),  // differs
		},
	}

	results, err := alter.ProcessRepoSettings(cfg, alter.DryRun, repoTarget(client, "testowner", "testrepo", true))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := []alter.RepoSettingResult{
		{Field: "description", Category: alter.WouldSet, Value: "New"},
		{Field: "has_wiki", Category: alter.WouldSet, Value: "false"},
		{Field: "has_issues", Category: alter.RepoNoChange, Value: "true"},
		{Field: "delete_branch_on_merge", Category: alter.WouldSet, Value: "true"},
	}
	if len(results) != len(want) {
		t.Fatalf("got %d results, want %d", len(results), len(want))
	}
	for i := range want {
		if results[i] != want[i] {
			t.Errorf("result %d = %#v, want %#v", i, results[i], want[i])
		}
	}
}

func TestProcessRepoSettingsMixedApplyWritesOnlyChangedFields(t *testing.T) {
	ghfake.FakeRepo(t, "testowner", "testrepo")

	var patchBody map[string]any
	var workflowBody map[string]any
	var unexpectedWrites []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/repos/testowner/testrepo":
			fmt.Fprint(w, `{"description":"old","has_wiki":true,"topics":["go"],"permissions":{"admin":true}}`)
		case r.Method == http.MethodGet && r.URL.Path == "/repos/testowner/testrepo/actions/permissions/workflow":
			fmt.Fprint(w, `{"default_workflow_permissions":"read","can_approve_pull_request_reviews":false}`)
		case r.Method == http.MethodGet && r.URL.Path == "/repos/testowner/testrepo/private-vulnerability-reporting":
			fmt.Fprint(w, `{"enabled":false}`)
		case r.Method == http.MethodGet && r.URL.Path == "/repos/testowner/testrepo/vulnerability-alerts":
			w.WriteHeader(http.StatusNotFound)
		case r.Method == http.MethodGet && r.URL.Path == "/repos/testowner/testrepo/automated-security-fixes":
			fmt.Fprint(w, `{"enabled":false,"paused":false}`)
		case r.Method == http.MethodPatch && r.URL.Path == "/repos/testowner/testrepo":
			if err := json.NewDecoder(r.Body).Decode(&patchBody); err != nil {
				t.Errorf("decode PATCH body: %v", err)
			}
			w.WriteHeader(http.StatusNoContent)
		case r.Method == http.MethodPut && r.URL.Path == "/repos/testowner/testrepo/actions/permissions/workflow":
			if err := json.NewDecoder(r.Body).Decode(&workflowBody); err != nil {
				t.Errorf("decode workflow body: %v", err)
			}
			w.WriteHeader(http.StatusNoContent)
		case r.Method != http.MethodGet:
			unexpectedWrites = append(unexpectedWrites, r.Method+" "+r.URL.Path)
			w.WriteHeader(http.StatusNoContent)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)

	topics := []string{"go"}
	cfg := &config.Config{Repository: &model.RepositorySettings{
		Description:                       new("new"),
		HasWiki:                           new(true),
		Topics:                            &topics,
		DefaultWorkflowPermissions:        new("write"),
		CanApprovePullRequestReviews:      new(false),
		PrivateVulnerabilityReportEnabled: new(false),
		VulnerabilityAlertsEnabled:        new(false),
		AutomatedSecurityFixesEnabled:     new(false),
	}}

	results, err := alter.ProcessRepoSettings(cfg, alter.Apply, repoTarget(testutil.NewTestClient(t, server), "testowner", "testrepo", true))
	if err != nil {
		t.Fatal(err)
	}
	if len(unexpectedWrites) != 0 {
		t.Fatalf("unexpected writes = %v", unexpectedWrites)
	}
	if len(patchBody) != 1 || patchBody["description"] != "new" {
		t.Fatalf("PATCH body = %v, want only changed description", patchBody)
	}
	if len(workflowBody) != 2 || workflowBody["default_workflow_permissions"] != "write" || workflowBody["can_approve_pull_request_reviews"] != false {
		t.Fatalf("workflow body = %v, want changed permission and required current approval value", workflowBody)
	}

	for _, result := range results {
		want := alter.RepoNoChange
		if result.Field == "description" || result.Field == "default_workflow_permissions" {
			want = alter.WouldSet
		}
		if result.Category != want {
			t.Errorf("%s category = %q, want %q", result.Field, result.Category, want)
		}
	}
}

func TestProcessRepoSettingsStringFieldValues(t *testing.T) {
	ghfake.FakeRepo(t, "testowner", "testrepo")

	live := repoJSON{Description: "old", Homepage: "https://old.example.com"}
	server := settingsServer(live, nil)
	t.Cleanup(server.Close)
	client := testutil.NewTestClient(t, server)

	cfg := &config.Config{
		Repository: &model.RepositorySettings{
			Description: new("new description"),
			Homepage:    new("https://old.example.com"), // matches
		},
	}

	results, err := alter.ProcessRepoSettings(cfg, alter.DryRun, repoTarget(client, "testowner", "testrepo", true))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("got %d results, want 2", len(results))
	}

	for _, r := range results {
		switch r.Field {
		case "description":
			if r.Category != alter.WouldSet {
				t.Errorf("description: category = %q, want %q", r.Category, alter.WouldSet)
			}
			if r.Value != "new description" {
				t.Errorf("description: value = %q, want %q", r.Value, "new description")
			}
		case "homepage":
			if r.Category != alter.RepoNoChange {
				t.Errorf("homepage: category = %q, want %q", r.Category, alter.RepoNoChange)
			}
			if r.Value != "https://old.example.com" {
				t.Errorf("homepage: value = %q, want %q", r.Value, "https://old.example.com")
			}
		default:
			t.Errorf("unexpected field %q", r.Field)
		}
	}
}

func TestProcessRepoSettingsTopicsNoChange(t *testing.T) {
	ghfake.FakeRepo(t, "testowner", "testrepo")

	live := repoJSON{Topics: []string{"go", "cli"}}
	server := settingsServer(live, nil)
	t.Cleanup(server.Close)
	client := testutil.NewTestClient(t, server)

	topics := []string{"go", "cli"}
	cfg := &config.Config{
		Repository: &model.RepositorySettings{
			Topics: &topics,
		},
	}

	results, err := alter.ProcessRepoSettings(cfg, alter.DryRun, repoTarget(client, "testowner", "testrepo", true))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("got %d results, want 1", len(results))
	}
	if results[0].Field != "topics" {
		t.Errorf("field = %q, want %q", results[0].Field, "topics")
	}
	if results[0].Category != alter.RepoNoChange {
		t.Errorf("category = %q, want %q", results[0].Category, alter.RepoNoChange)
	}
	if results[0].Value != "go, cli" {
		t.Errorf("value = %q, want %q", results[0].Value, "go, cli")
	}
}

func TestProcessRepoSettingsTopicsOrderIgnored(t *testing.T) {
	ghfake.FakeRepo(t, "testowner", "testrepo")

	live := repoJSON{Topics: []string{"cli", "go"}}
	server := settingsServer(live, nil)
	t.Cleanup(server.Close)
	client := testutil.NewTestClient(t, server)

	topics := []string{"go", "cli"}
	cfg := &config.Config{
		Repository: &model.RepositorySettings{
			Topics: &topics,
		},
	}

	results, err := alter.ProcessRepoSettings(cfg, alter.DryRun, repoTarget(client, "testowner", "testrepo", true))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("got %d results, want 1", len(results))
	}
	if results[0].Category != alter.RepoNoChange {
		t.Errorf("category = %q, want %q", results[0].Category, alter.RepoNoChange)
	}
	if results[0].Value != "go, cli" {
		t.Errorf("value = %q, want declared order %q", results[0].Value, "go, cli")
	}
}

func TestProcessRepoSettingsTopicsWouldSet(t *testing.T) {
	ghfake.FakeRepo(t, "testowner", "testrepo")

	live := repoJSON{Topics: []string{"go", "cli"}}
	server := settingsServer(live, nil)
	t.Cleanup(server.Close)
	client := testutil.NewTestClient(t, server)

	topics := []string{"go", "cli", "github"}
	cfg := &config.Config{
		Repository: &model.RepositorySettings{
			Topics: &topics,
		},
	}

	results, err := alter.ProcessRepoSettings(cfg, alter.DryRun, repoTarget(client, "testowner", "testrepo", true))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("got %d results, want 1", len(results))
	}
	if results[0].Field != "topics" {
		t.Errorf("field = %q, want %q", results[0].Field, "topics")
	}
	if results[0].Category != alter.WouldSet {
		t.Errorf("category = %q, want %q", results[0].Category, alter.WouldSet)
	}
	if results[0].Value != "go, cli, github" {
		t.Errorf("value = %q, want %q", results[0].Value, "go, cli, github")
	}
}

func TestProcessRepoSettingsTopicsEmptyVsNil(t *testing.T) {
	ghfake.FakeRepo(t, "testowner", "testrepo")

	// Live has no topics (nil from JSON unmarshalling)
	live := repoJSON{}
	server := settingsServer(live, nil)
	t.Cleanup(server.Close)
	client := testutil.NewTestClient(t, server)

	// Declared: empty slice (clear all topics)
	// The set comparison treats nil and empty as equal, so this is no change
	topics := []string{}
	cfg := &config.Config{
		Repository: &model.RepositorySettings{
			Topics: &topics,
		},
	}

	results, err := alter.ProcessRepoSettings(cfg, alter.DryRun, repoTarget(client, "testowner", "testrepo", true))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("got %d results, want 1", len(results))
	}
	if results[0].Field != "topics" {
		t.Errorf("field = %q, want %q", results[0].Field, "topics")
	}
	if results[0].Category != alter.RepoNoChange {
		t.Errorf("category = %q, want %q", results[0].Category, alter.RepoNoChange)
	}
	if results[0].Value != "" {
		t.Errorf("value = %q, want %q", results[0].Value, "")
	}
}

func TestProcessRepoSettingsTopicsEmptyMatchesEmpty(t *testing.T) {
	ghfake.FakeRepo(t, "testowner", "testrepo")

	// Live has empty topics from JSON
	live := repoJSON{Topics: []string{}}
	server := settingsServer(live, nil)
	t.Cleanup(server.Close)
	client := testutil.NewTestClient(t, server)

	topics := []string{}
	cfg := &config.Config{
		Repository: &model.RepositorySettings{
			Topics: &topics,
		},
	}

	results, err := alter.ProcessRepoSettings(cfg, alter.DryRun, repoTarget(client, "testowner", "testrepo", true))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("got %d results, want 1", len(results))
	}
	if results[0].Category != alter.RepoNoChange {
		t.Errorf("category = %q, want %q", results[0].Category, alter.RepoNoChange)
	}
}

func TestProcessRepoSettingsDefaultWorkflowPermissionsNoChange(t *testing.T) {
	ghfake.FakeRepo(t, "testowner", "testrepo")

	live := repoJSON{}
	// settingsServer returns {"default_workflow_permissions":"read","can_approve_pull_request_reviews":false}
	server := settingsServer(live, nil)
	t.Cleanup(server.Close)
	client := testutil.NewTestClient(t, server)

	cfg := &config.Config{
		Repository: &model.RepositorySettings{
			DefaultWorkflowPermissions: new("read"),
		},
	}

	results, err := alter.ProcessRepoSettings(cfg, alter.DryRun, repoTarget(client, "testowner", "testrepo", true))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("got %d results, want 1", len(results))
	}
	if results[0].Field != "default_workflow_permissions" {
		t.Errorf("field = %q, want %q", results[0].Field, "default_workflow_permissions")
	}
	if results[0].Category != alter.RepoNoChange {
		t.Errorf("category = %q, want %q", results[0].Category, alter.RepoNoChange)
	}
	if results[0].Value != "read" {
		t.Errorf("value = %q, want %q", results[0].Value, "read")
	}
}

func TestProcessRepoSettingsDefaultWorkflowPermissionsWouldSet(t *testing.T) {
	ghfake.FakeRepo(t, "testowner", "testrepo")

	live := repoJSON{}
	// settingsServer returns {"default_workflow_permissions":"read",...}
	server := settingsServer(live, nil)
	t.Cleanup(server.Close)
	client := testutil.NewTestClient(t, server)

	cfg := &config.Config{
		Repository: &model.RepositorySettings{
			DefaultWorkflowPermissions: new("write"),
		},
	}

	results, err := alter.ProcessRepoSettings(cfg, alter.DryRun, repoTarget(client, "testowner", "testrepo", true))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("got %d results, want 1", len(results))
	}
	if results[0].Field != "default_workflow_permissions" {
		t.Errorf("field = %q, want %q", results[0].Field, "default_workflow_permissions")
	}
	if results[0].Category != alter.WouldSet {
		t.Errorf("category = %q, want %q", results[0].Category, alter.WouldSet)
	}
	if results[0].Value != "write" {
		t.Errorf("value = %q, want %q", results[0].Value, "write")
	}
}

func TestProcessRepoSettingsCanApprovePullRequestReviewsNoChange(t *testing.T) {
	ghfake.FakeRepo(t, "testowner", "testrepo")

	live := repoJSON{}
	// settingsServer returns {"can_approve_pull_request_reviews":false}
	server := settingsServer(live, nil)
	t.Cleanup(server.Close)
	client := testutil.NewTestClient(t, server)

	cfg := &config.Config{
		Repository: &model.RepositorySettings{
			CanApprovePullRequestReviews: new(false),
		},
	}

	results, err := alter.ProcessRepoSettings(cfg, alter.DryRun, repoTarget(client, "testowner", "testrepo", true))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("got %d results, want 1", len(results))
	}
	if results[0].Field != "can_approve_pull_request_reviews" {
		t.Errorf("field = %q, want %q", results[0].Field, "can_approve_pull_request_reviews")
	}
	if results[0].Category != alter.RepoNoChange {
		t.Errorf("category = %q, want %q", results[0].Category, alter.RepoNoChange)
	}
}

func TestProcessRepoSettingsSecurityFeaturesDryRun(t *testing.T) {
	ghfake.FakeRepo(t, "testowner", "testrepo")

	var writes atomic.Int32
	server := settingsServer(repoJSON{}, &writes)
	t.Cleanup(server.Close)
	client := testutil.NewTestClient(t, server)
	cfg := &config.Config{Repository: &model.RepositorySettings{
		PrivateVulnerabilityReportEnabled: new(true),
		VulnerabilityAlertsEnabled:        new(true),
		AutomatedSecurityFixesEnabled:     new(true),
	}}

	results, err := alter.ProcessRepoSettings(cfg, alter.DryRun, repoTarget(client, "testowner", "testrepo", true))
	if err != nil {
		t.Fatalf("ProcessRepoSettings() error: %v", err)
	}
	if writes.Load() != 0 {
		t.Errorf("writes = %d, want 0", writes.Load())
	}
	if len(results) != 3 {
		t.Fatalf("results = %v, want three results", results)
	}
	for _, result := range results {
		if result.Category != alter.WouldSet || result.Value != "true" {
			t.Errorf("result = %+v, want would set true", result)
		}
	}
}

func TestProcessRepoSettingsAppliesOnlyChangedSecurityEndpoints(t *testing.T) {
	ghfake.FakeRepo(t, "testowner", "testrepo")
	var patchWrites atomic.Int32
	var securityWrites atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/repos/testowner/testrepo":
			fmt.Fprint(w, `{"has_wiki":true,"permissions":{"admin":true}}`)
		case r.Method == http.MethodGet && r.URL.Path == "/repos/testowner/testrepo/actions/permissions/workflow":
			fmt.Fprint(w, `{"default_workflow_permissions":"read","can_approve_pull_request_reviews":false}`)
		case r.Method == http.MethodGet && r.URL.Path == "/repos/testowner/testrepo/private-vulnerability-reporting":
			fmt.Fprint(w, `{"enabled":true}`)
		case r.Method == http.MethodGet && r.URL.Path == "/repos/testowner/testrepo/vulnerability-alerts":
			w.WriteHeader(http.StatusNoContent)
		case r.Method == http.MethodGet && r.URL.Path == "/repos/testowner/testrepo/automated-security-fixes":
			fmt.Fprint(w, `{"enabled":true,"paused":false}`)
		case r.Method == http.MethodPatch:
			patchWrites.Add(1)
			w.WriteHeader(http.StatusNoContent)
		case r.Method == http.MethodPut || r.Method == http.MethodDelete:
			securityWrites.Add(1)
			w.WriteHeader(http.StatusNoContent)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)
	cfg := &config.Config{Repository: &model.RepositorySettings{
		HasWiki:                           new(false),
		PrivateVulnerabilityReportEnabled: new(true),
		VulnerabilityAlertsEnabled:        new(true),
		AutomatedSecurityFixesEnabled:     new(true),
	}}
	if _, err := alter.ProcessRepoSettings(cfg, alter.Apply, repoTarget(testutil.NewTestClient(t, server), "testowner", "testrepo", true)); err != nil {
		t.Fatal(err)
	}
	if patchWrites.Load() != 1 || securityWrites.Load() != 0 {
		t.Fatalf("patch writes = %d, security writes = %d, want 1 and 0", patchWrites.Load(), securityWrites.Load())
	}
}

func TestProcessRepoSettingsAutomatedFixesPrerequisiteWarning(t *testing.T) {
	tests := []struct {
		name          string
		alertsLive    bool
		alertsDesired *bool
		wantWarning   bool
	}{
		{name: "alerts remain disabled", wantWarning: true},
		{name: "alerts already enabled", alertsLive: true},
		{name: "same run enables alerts", alertsDesired: new(true)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch r.URL.Path {
				case "/repos/testowner/testrepo":
					fmt.Fprint(w, `{"permissions":{"admin":true}}`)
				case "/repos/testowner/testrepo/actions/permissions/workflow":
					fmt.Fprint(w, `{"default_workflow_permissions":"read","can_approve_pull_request_reviews":false}`)
				case "/repos/testowner/testrepo/private-vulnerability-reporting":
					fmt.Fprint(w, `{"enabled":false}`)
				case "/repos/testowner/testrepo/vulnerability-alerts":
					if tt.alertsLive {
						w.WriteHeader(http.StatusNoContent)
					} else {
						w.WriteHeader(http.StatusNotFound)
						fmt.Fprint(w, `{"message":"Not Found"}`)
					}
				case "/repos/testowner/testrepo/automated-security-fixes":
					fmt.Fprint(w, `{"enabled":false,"paused":false}`)
				default:
					http.NotFound(w, r)
				}
			}))
			t.Cleanup(server.Close)
			cfg := &config.Config{Repository: &model.RepositorySettings{
				VulnerabilityAlertsEnabled:    tt.alertsDesired,
				AutomatedSecurityFixesEnabled: new(true),
			}}
			var stderr strings.Builder
			target := repoTarget(testutil.NewTestClient(t, server), "testowner", "testrepo", true)
			target.Stderr = &stderr
			_, err := alter.ProcessRepoSettings(cfg, alter.DryRun, target)
			if err != nil {
				t.Fatal(err)
			}
			if strings.Contains(stderr.String(), "warning: automated_security_fixes_enabled") != tt.wantWarning {
				t.Fatalf("stderr = %q, want warning %t", stderr.String(), tt.wantWarning)
			}
		})
	}
}

func TestProcessRepoSettingsSecurityReadAccessWarning(t *testing.T) {
	ghfake.FakeRepo(t, "testowner", "testrepo")

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/repos/testowner/testrepo":
			fmt.Fprint(w, `{}`)
		case "/repos/testowner/testrepo/private-vulnerability-reporting":
			w.WriteHeader(http.StatusForbidden)
			fmt.Fprint(w, `{"message":"Forbidden"}`)
		case "/repos/testowner/testrepo/actions/permissions/workflow":
			fmt.Fprint(w, `{"default_workflow_permissions":"read","can_approve_pull_request_reviews":false}`)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)
	client := testutil.NewTestClient(t, server)
	cfg := &config.Config{Repository: &model.RepositorySettings{
		PrivateVulnerabilityReportEnabled: new(true),
	}}

	results, err := alter.ProcessRepoSettings(cfg, alter.DryRun, repoTarget(client, "testowner", "testrepo", true))
	if err != nil {
		t.Fatalf("ProcessRepoSettings() error: %v", err)
	}
	if len(results) != 1 || results[0].Category != alter.WouldSkipScope {
		t.Errorf("results = %+v, want one scope warning", results)
	}
}

func TestProcessRepoSettingsUnknownSecurityPrerequisiteSkipsDependent(t *testing.T) {
	tests := []struct {
		name           string
		alertsResponse func(http.ResponseWriter)
		fixesResponse  func(http.ResponseWriter)
		settings       *model.RepositorySettings
		dependentField string
	}{
		{
			name: "unknown alerts block enabling fixes",
			alertsResponse: func(w http.ResponseWriter) {
				w.WriteHeader(http.StatusForbidden)
			},
			fixesResponse: func(w http.ResponseWriter) {
				fmt.Fprint(w, `{"enabled":false,"paused":false}`)
			},
			settings:       &model.RepositorySettings{AutomatedSecurityFixesEnabled: new(true)},
			dependentField: "automated_security_fixes_enabled",
		},
		{
			name: "unknown fixes block disabling alerts",
			alertsResponse: func(w http.ResponseWriter) {
				w.WriteHeader(http.StatusNoContent)
			},
			fixesResponse: func(w http.ResponseWriter) {
				w.WriteHeader(http.StatusForbidden)
			},
			settings:       &model.RepositorySettings{VulnerabilityAlertsEnabled: new(false)},
			dependentField: "vulnerability_alerts_enabled",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var writes atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodGet {
					writes.Add(1)
					w.WriteHeader(http.StatusNoContent)
					return
				}
				switch r.URL.Path {
				case "/repos/testowner/testrepo":
					fmt.Fprint(w, `{"permissions":{"admin":true}}`)
				case "/repos/testowner/testrepo/actions/permissions/workflow":
					fmt.Fprint(w, `{"default_workflow_permissions":"read","can_approve_pull_request_reviews":false}`)
				case "/repos/testowner/testrepo/private-vulnerability-reporting":
					fmt.Fprint(w, `{"enabled":false}`)
				case "/repos/testowner/testrepo/vulnerability-alerts":
					tt.alertsResponse(w)
				case "/repos/testowner/testrepo/automated-security-fixes":
					tt.fixesResponse(w)
				default:
					http.NotFound(w, r)
				}
			}))
			t.Cleanup(server.Close)

			results, err := alter.ProcessRepoSettings(&config.Config{Repository: tt.settings}, alter.Apply, repoTarget(testutil.NewTestClient(t, server), "testowner", "testrepo", true))
			if err != nil {
				t.Fatal(err)
			}
			if writes.Load() != 0 || len(results) != 1 || results[0].Field != tt.dependentField || results[0].Category != alter.WouldSkipScope {
				t.Fatalf("results = %+v, writes = %d, want one dependent skip", results, writes.Load())
			}
			if output := alter.FormatOutput(results, nil, nil, alter.Apply); strings.Contains(output, "set:") {
				t.Fatalf("output = %q, want no set result", output)
			}
		})
	}
}

func TestProcessRepoSettingsSkippedSecurityWriteSkipsDependentOutput(t *testing.T) {
	tests := []struct {
		name          string
		alertsEnabled bool
		fixesEnabled  bool
		settings      *model.RepositorySettings
		dependent     string
	}{
		{
			name: "alerts write blocks fixes",
			settings: &model.RepositorySettings{
				VulnerabilityAlertsEnabled: new(true), AutomatedSecurityFixesEnabled: new(true),
			},
			dependent: "enable automated security fixes",
		},
		{
			name:          "fixes write blocks alerts",
			alertsEnabled: true,
			fixesEnabled:  true,
			settings: &model.RepositorySettings{
				VulnerabilityAlertsEnabled: new(false), AutomatedSecurityFixesEnabled: new(false),
			},
			dependent: "disable vulnerability alerts",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var writes atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodGet {
					writes.Add(1)
					w.WriteHeader(http.StatusForbidden)
					fmt.Fprint(w, `{"message":"Resource not accessible by integration"}`)
					return
				}
				switch r.URL.Path {
				case "/repos/testowner/testrepo":
					fmt.Fprint(w, `{"permissions":{"admin":true}}`)
				case "/repos/testowner/testrepo/actions/permissions/workflow":
					fmt.Fprint(w, `{"default_workflow_permissions":"read","can_approve_pull_request_reviews":false}`)
				case "/repos/testowner/testrepo/private-vulnerability-reporting":
					fmt.Fprint(w, `{"enabled":false}`)
				case "/repos/testowner/testrepo/vulnerability-alerts":
					if tt.alertsEnabled {
						w.WriteHeader(http.StatusNoContent)
					} else {
						w.WriteHeader(http.StatusNotFound)
					}
				case "/repos/testowner/testrepo/automated-security-fixes":
					fmt.Fprintf(w, `{"enabled":%t,"paused":false}`, tt.fixesEnabled)
				default:
					http.NotFound(w, r)
				}
			}))
			t.Cleanup(server.Close)

			results, err := alter.ProcessRepoSettings(&config.Config{Repository: tt.settings}, alter.Apply, repoTarget(testutil.NewTestClient(t, server), "testowner", "testrepo", true))
			if err != nil {
				t.Fatal(err)
			}
			if writes.Load() != 1 {
				t.Fatalf("writes = %d, want one prerequisite attempt", writes.Load())
			}
			output := alter.FormatOutput(results, nil, nil, alter.Apply)
			if strings.Contains(output, "set:") || !strings.Contains(output, tt.dependent) {
				t.Fatalf("output = %q, want skipped dependent %q", output, tt.dependent)
			}
		})
	}
}

func TestProcessRepoSettingsPatch403ScopeProducesSkipScope(t *testing.T) {
	ghfake.FakeRepo(t, "testowner", "testrepo")

	// Server that returns 403 on PATCH (simulating insufficient scope on the main settings call).
	live := repoJSON{HasWiki: true}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path

		switch {
		case r.Method == http.MethodGet && path == "/repos/testowner/testrepo":
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(live)

		case r.Method == http.MethodGet && path == "/repos/testowner/testrepo/actions/permissions/workflow":
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprint(w, `{"default_workflow_permissions":"read","can_approve_pull_request_reviews":false}`)

		case r.Method == http.MethodPatch && path == "/repos/testowner/testrepo":
			// Return 403 to simulate insufficient scope on PATCH.
			w.Header().Set("Content-Type", "application/json")
			w.Header().Set("X-OAuth-Scopes", "public_repo")
			w.Header().Set("X-Accepted-OAuth-Scopes", "repo")
			w.WriteHeader(http.StatusForbidden)
			fmt.Fprint(w, `{"message":"Resource not accessible by integration"}`)

		default:
			w.WriteHeader(http.StatusNotFound)
			fmt.Fprintf(w, `{"message":"Not Found: %s %s"}`, r.Method, path) //nolint:gosec
		}
	}))
	t.Cleanup(server.Close)
	client := testutil.NewTestClient(t, server)

	cfg := &config.Config{
		Repository: &model.RepositorySettings{
			HasWiki: new(false),
		},
	}

	results, err := alter.ProcessRepoSettings(cfg, alter.Apply, repoTarget(client, "testowner", "testrepo", true))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var foundScopeSkip bool
	for _, r := range results {
		if r.Category == alter.WouldSkipScope && r.Operation == gh.Op(gh.OpPatchRepoSettings) {
			foundScopeSkip = true
		}
	}
	if !foundScopeSkip {
		t.Errorf("expected WouldSkipScope result for patch repo settings, got results: %v", results)
	}
}

// forbiddenReadServer returns a test server where the workflow permissions
// GET endpoint returns 403 to simulate insufficient scope on the read path.
func forbiddenReadServer(forbidWF bool) *httptest.Server {
	repo := repoJSON{}
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path

		switch {
		case r.Method == http.MethodGet && path == "/repos/testowner/testrepo":
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(repo)

		case r.Method == http.MethodGet && path == "/repos/testowner/testrepo/actions/permissions/workflow":
			if forbidWF {
				w.Header().Set("Content-Type", "application/json")
				w.Header().Set("X-OAuth-Scopes", "public_repo")
				w.Header().Set("X-Accepted-OAuth-Scopes", "repo")
				w.WriteHeader(http.StatusForbidden)
				fmt.Fprint(w, `{"message":"Resource not accessible by integration"}`)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprint(w, `{"default_workflow_permissions":"read","can_approve_pull_request_reviews":false}`)

		default:
			w.WriteHeader(http.StatusNotFound)
			fmt.Fprintf(w, `{"message":"Not Found: %s %s"}`, r.Method, path) //nolint:gosec
		}
	}))
}

func TestProcessRepoSettingsReadPath403WorkflowProducesSkipScope(t *testing.T) {
	ghfake.FakeRepo(t, "testowner", "testrepo")

	server := forbiddenReadServer(true)
	t.Cleanup(server.Close)
	client := testutil.NewTestClient(t, server)

	cfg := &config.Config{
		Repository: &model.RepositorySettings{
			DefaultWorkflowPermissions: new("write"),
		},
	}

	results, err := alter.ProcessRepoSettings(cfg, alter.DryRun, repoTarget(client, "testowner", "testrepo", true))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("got %d results, want 1", len(results))
	}
	if results[0].Field != "default_workflow_permissions" {
		t.Errorf("field = %q, want %q", results[0].Field, "default_workflow_permissions")
	}
	if results[0].Category != alter.WouldSkipScope {
		t.Errorf("category = %q, want %q", results[0].Category, alter.WouldSkipScope)
	}
}

func TestProcessRepoSettingsReadPath403DoesNotProduceWouldSet(t *testing.T) {
	ghfake.FakeRepo(t, "testowner", "testrepo")

	// Workflow permissions sub-call returns 403.
	server := forbiddenReadServer(true)
	t.Cleanup(server.Close)
	client := testutil.NewTestClient(t, server)

	cfg := &config.Config{
		Repository: &model.RepositorySettings{
			DefaultWorkflowPermissions:   new("write"),
			CanApprovePullRequestReviews: new(true),
		},
	}

	results, err := alter.ProcessRepoSettings(cfg, alter.DryRun, repoTarget(client, "testowner", "testrepo", true))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	for _, r := range results {
		if r.Category == alter.WouldSet {
			t.Errorf("unexpected WouldSet for field %q; should be a skip category", r.Field)
		}
	}

	// Both fields should have skip results.
	skipCount := 0
	for _, r := range results {
		if r.Category == alter.WouldSkipScope {
			skipCount++
		}
	}
	if skipCount != 2 {
		t.Errorf("got %d skip results, want 2", skipCount)
	}
}

func TestProcessRepoSettingsCanApprovePullRequestReviewsWouldSet(t *testing.T) {
	ghfake.FakeRepo(t, "testowner", "testrepo")

	live := repoJSON{}
	// settingsServer returns {"can_approve_pull_request_reviews":false}
	server := settingsServer(live, nil)
	t.Cleanup(server.Close)
	client := testutil.NewTestClient(t, server)

	cfg := &config.Config{
		Repository: &model.RepositorySettings{
			CanApprovePullRequestReviews: new(true),
		},
	}

	results, err := alter.ProcessRepoSettings(cfg, alter.DryRun, repoTarget(client, "testowner", "testrepo", true))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("got %d results, want 1", len(results))
	}
	if results[0].Field != "can_approve_pull_request_reviews" {
		t.Errorf("field = %q, want %q", results[0].Field, "can_approve_pull_request_reviews")
	}
	if results[0].Category != alter.WouldSet {
		t.Errorf("category = %q, want %q", results[0].Category, alter.WouldSet)
	}
}

func TestProcessRepoSettingsDoesNotRewriteDisabledSecurityFixes(t *testing.T) {
	var paths []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			paths = append(paths, r.URL.Path)
			w.WriteHeader(http.StatusNoContent)
			return
		}
		switch r.URL.Path {
		case "/repos/testowner/testrepo":
			fmt.Fprint(w, `{"permissions":{"admin":true}}`)
		case "/repos/testowner/testrepo/actions/permissions/workflow":
			fmt.Fprint(w, `{"default_workflow_permissions":"read","can_approve_pull_request_reviews":false}`)
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

	settings := &model.RepositorySettings{
		VulnerabilityAlertsEnabled:    new(false),
		AutomatedSecurityFixesEnabled: new(false),
	}
	if _, err := alter.ProcessRepoSettings(&config.Config{Repository: settings}, alter.Apply, repoTarget(testutil.NewTestClient(t, server), "testowner", "testrepo", true)); err != nil {
		t.Fatal(err)
	}
	if len(paths) != 1 || paths[0] != "/repos/testowner/testrepo/vulnerability-alerts" {
		t.Fatalf("write paths = %v, want vulnerability alerts only", paths)
	}
}
