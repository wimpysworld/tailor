package alter_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/wimpysworld/tailor/internal/alter"
	"github.com/wimpysworld/tailor/internal/config"
	"github.com/wimpysworld/tailor/internal/ghfake"
	"github.com/wimpysworld/tailor/internal/model"
	"github.com/wimpysworld/tailor/internal/ptr"
	"github.com/wimpysworld/tailor/internal/testutil"
)

// repoSettingsJSON returns a JSON string matching the repoResponse struct
// from the gh package.
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
	WebCommitSignoffRequired bool     `json:"web_commit_signoff_required"`
	Topics                   []string `json:"topics"`
}

// settingsServer creates an httptest server that responds to repo settings
// GET, PATCH, and PUT requests, plus a GET /user endpoint (returning 200 by
// default to simulate a PAT). patchCalled is incremented when PATCH or PUT
// is received.
func settingsServer(repo repoJSON, patchCalled *atomic.Int32) *httptest.Server {
	return settingsServerWithTokenType(repo, patchCalled, false)
}

func settingsServerWithTokenType(repo repoJSON, patchCalled *atomic.Int32, _ bool) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path

		switch {
		case r.Method == http.MethodGet && path == "/user":
			fmt.Fprint(w, `{"login":"testuser"}`)

		case r.Method == http.MethodGet && path == "/repos/testowner/testrepo":
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(repo)

		case r.Method == http.MethodPatch && path == "/repos/testowner/testrepo":
			if patchCalled != nil {
				patchCalled.Add(1)
			}
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			fmt.Fprint(w, `{}`)

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
	results, err := alter.ProcessRepoSettings(cfg, alter.DryRun, nil, "", "", false)
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
			HasWiki: ptr.Ptr(false),
		},
	}

	var results []alter.RepoSettingResult
	var err error

	output := captureStderr(t, func() {
		results, err = alter.ProcessRepoSettings(cfg, alter.DryRun, nil, "", "", false)
	})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if results != nil {
		t.Errorf("expected nil results, got %v", results)
	}

	want := "No GitHub repository context found."
	if !bytes.Contains([]byte(output), []byte(want)) {
		t.Errorf("stderr = %q, want substring %q", output, want)
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
			HasWiki: ptr.Ptr(false),
		},
	}

	results, err := alter.ProcessRepoSettings(cfg, alter.DryRun, client, "testowner", "testrepo", true)
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
			HasWiki: ptr.Ptr(false),
		},
	}

	results, err := alter.ProcessRepoSettings(cfg, alter.DryRun, client, "testowner", "testrepo", true)
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
			HasWiki: ptr.Ptr(false),
		},
	}

	_, err := alter.ProcessRepoSettings(cfg, alter.Apply, client, "testowner", "testrepo", true)
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
			HasWiki: ptr.Ptr(false),
		},
	}

	_, err := alter.ProcessRepoSettings(cfg, alter.Recut, client, "testowner", "testrepo", true)
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
			HasWiki: ptr.Ptr(false),
		},
	}

	_, err := alter.ProcessRepoSettings(cfg, alter.DryRun, client, "testowner", "testrepo", true)
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
			HasWiki:   ptr.Ptr(false),
			HasIssues: ptr.Ptr(true),
		},
	}

	_, err := alter.ProcessRepoSettings(cfg, alter.Apply, client, "testowner", "testrepo", true)
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
			HasWiki: ptr.Ptr(false),
		},
	}

	_, err := alter.ProcessRepoSettings(cfg, alter.DryRun, client, "testowner", "testrepo", true)
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
			HasWiki:             ptr.Ptr(false), // differs
			HasIssues:           ptr.Ptr(true),  // matches
			Description:         ptr.Ptr("New"), // differs
			DeleteBranchOnMerge: ptr.Ptr(true),  // differs
		},
	}

	results, err := alter.ProcessRepoSettings(cfg, alter.DryRun, client, "testowner", "testrepo", true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 4 {
		t.Fatalf("got %d results, want 4", len(results))
	}

	counts := map[alter.RepoSettingCategory]int{}
	for _, r := range results {
		counts[r.Category]++
	}
	if counts[alter.WouldSet] != 3 {
		t.Errorf("WouldSet count = %d, want 3", counts[alter.WouldSet])
	}
	if counts[alter.RepoNoChange] != 1 {
		t.Errorf("RepoNoChange count = %d, want 1", counts[alter.RepoNoChange])
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
			Description: ptr.Ptr("new description"),
			Homepage:    ptr.Ptr("https://old.example.com"), // matches
		},
	}

	results, err := alter.ProcessRepoSettings(cfg, alter.DryRun, client, "testowner", "testrepo", true)
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

	results, err := alter.ProcessRepoSettings(cfg, alter.DryRun, client, "testowner", "testrepo", true)
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

	results, err := alter.ProcessRepoSettings(cfg, alter.DryRun, client, "testowner", "testrepo", true)
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
	// slices.Equal treats nil and empty as equal, so this is no change
	topics := []string{}
	cfg := &config.Config{
		Repository: &model.RepositorySettings{
			Topics: &topics,
		},
	}

	results, err := alter.ProcessRepoSettings(cfg, alter.DryRun, client, "testowner", "testrepo", true)
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

	results, err := alter.ProcessRepoSettings(cfg, alter.DryRun, client, "testowner", "testrepo", true)
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
			HasWiki: ptr.Ptr(false),
		},
	}

	results, err := alter.ProcessRepoSettings(cfg, alter.Apply, client, "testowner", "testrepo", true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var foundScopeSkip bool
	for _, r := range results {
		if r.Category == alter.WouldSkipScope && r.Field == "patch repo settings" {
			foundScopeSkip = true
		}
	}
	if !foundScopeSkip {
		t.Errorf("expected WouldSkipScope result for patch repo settings, got results: %v", results)
	}
}
