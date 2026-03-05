package alter_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/cli/go-gh/v2/pkg/repository"
	"github.com/wimpysworld/tailor/internal/alter"
	"github.com/wimpysworld/tailor/internal/config"
	"github.com/wimpysworld/tailor/internal/gh"
	"github.com/wimpysworld/tailor/internal/ptr"
)

// fakeRepo installs a currentRepo stub that returns the given owner and name.
func fakeRepo(t *testing.T, owner, name string) {
	t.Helper()
	restore := gh.SetCurrentRepoFunc(func() (repository.Repository, error) {
		return repository.Repository{Owner: owner, Name: name}, nil
	})
	t.Cleanup(restore)
}

// fakeNoRepoContext installs a currentRepo stub that returns an error.
func fakeNoRepoContext(t *testing.T) {
	t.Helper()
	restore := gh.SetCurrentRepoFunc(func() (repository.Repository, error) {
		return repository.Repository{}, errors.New("not a git repository")
	})
	t.Cleanup(restore)
}

// repoSettingsJSON returns a JSON string matching the repoResponse struct
// from the gh package.
type repoJSON struct {
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

// settingsServer creates an httptest server that responds to repo settings
// GET and PATCH requests. patchCalled is incremented when PATCH is received.
func settingsServer(repo repoJSON, pvrEnabled bool, patchCalled *atomic.Int32) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path

		switch {
		case r.Method == http.MethodGet && path == "/repos/testowner/testrepo":
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(repo)

		case r.Method == http.MethodGet && path == "/repos/testowner/testrepo/private-vulnerability-reporting":
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprintf(w, `{"enabled":%t}`, pvrEnabled)

		case r.Method == http.MethodPatch && path == "/repos/testowner/testrepo":
			if patchCalled != nil {
				patchCalled.Add(1)
			}
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			fmt.Fprint(w, `{}`)

		case r.Method == http.MethodPut && path == "/repos/testowner/testrepo/private-vulnerability-reporting":
			if patchCalled != nil {
				patchCalled.Add(1)
			}
			w.WriteHeader(http.StatusNoContent)

		case r.Method == http.MethodDelete && path == "/repos/testowner/testrepo/private-vulnerability-reporting":
			if patchCalled != nil {
				patchCalled.Add(1)
			}
			w.WriteHeader(http.StatusNoContent)

		default:
			w.WriteHeader(http.StatusNotFound)
			fmt.Fprintf(w, `{"message":"Not Found: %s %s"}`, r.Method, path)
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
	fakeNoRepoContext(t)

	cfg := &config.Config{
		Repository: &config.RepositorySettings{
			HasWiki: ptr.Bool(false),
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
	fakeRepo(t, "testowner", "testrepo")

	live := repoJSON{HasWiki: true, HasIssues: true}
	server := settingsServer(live, false, nil)
	t.Cleanup(server.Close)
	client := newTestClient(t, server)

	cfg := &config.Config{
		Repository: &config.RepositorySettings{
			HasWiki: ptr.Bool(false),
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
	fakeRepo(t, "testowner", "testrepo")

	live := repoJSON{HasWiki: false, HasIssues: true}
	server := settingsServer(live, false, nil)
	t.Cleanup(server.Close)
	client := newTestClient(t, server)

	cfg := &config.Config{
		Repository: &config.RepositorySettings{
			HasWiki: ptr.Bool(false),
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
	fakeRepo(t, "testowner", "testrepo")

	var patchCalled atomic.Int32
	live := repoJSON{HasWiki: true}
	server := settingsServer(live, false, &patchCalled)
	t.Cleanup(server.Close)
	client := newTestClient(t, server)

	cfg := &config.Config{
		Repository: &config.RepositorySettings{
			HasWiki: ptr.Bool(false),
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

func TestProcessRepoSettingsForceApplyCallsAPI(t *testing.T) {
	fakeRepo(t, "testowner", "testrepo")

	var patchCalled atomic.Int32
	live := repoJSON{HasWiki: true}
	server := settingsServer(live, false, &patchCalled)
	t.Cleanup(server.Close)
	client := newTestClient(t, server)

	cfg := &config.Config{
		Repository: &config.RepositorySettings{
			HasWiki: ptr.Bool(false),
		},
	}

	_, err := alter.ProcessRepoSettings(cfg, alter.ForceApply, client, "testowner", "testrepo", true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if patchCalled.Load() == 0 {
		t.Error("expected PATCH call on ForceApply, but none received")
	}
}

func TestProcessRepoSettingsDryRunDoesNotCallAPI(t *testing.T) {
	fakeRepo(t, "testowner", "testrepo")

	var patchCalled atomic.Int32
	live := repoJSON{HasWiki: true}
	server := settingsServer(live, false, &patchCalled)
	t.Cleanup(server.Close)
	client := newTestClient(t, server)

	cfg := &config.Config{
		Repository: &config.RepositorySettings{
			HasWiki: ptr.Bool(false),
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
	fakeRepo(t, "testowner", "testrepo")

	var patchCalled atomic.Int32
	live := repoJSON{HasWiki: false, HasIssues: true}
	server := settingsServer(live, false, &patchCalled)
	t.Cleanup(server.Close)
	client := newTestClient(t, server)

	cfg := &config.Config{
		Repository: &config.RepositorySettings{
			HasWiki:   ptr.Bool(false),
			HasIssues: ptr.Bool(true),
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
	fakeRepo(t, "testowner", "testrepo")

	server := failingSettingsServer()
	t.Cleanup(server.Close)
	client := newTestClient(t, server)

	cfg := &config.Config{
		Repository: &config.RepositorySettings{
			HasWiki: ptr.Bool(false),
		},
	}

	_, err := alter.ProcessRepoSettings(cfg, alter.DryRun, client, "testowner", "testrepo", true)
	if err == nil {
		t.Fatal("expected error from API failure, got nil")
	}
}

func TestProcessRepoSettingsMixedResults(t *testing.T) {
	fakeRepo(t, "testowner", "testrepo")

	live := repoJSON{
		HasWiki:         true,
		HasIssues:       true,
		Description:     "My project",
		DeleteBranchOnMerge: false,
	}
	server := settingsServer(live, true, nil)
	t.Cleanup(server.Close)
	client := newTestClient(t, server)

	cfg := &config.Config{
		Repository: &config.RepositorySettings{
			HasWiki:             ptr.Bool(false),   // differs
			HasIssues:           ptr.Bool(true),    // matches
			Description:         ptr.String("New"), // differs
			DeleteBranchOnMerge: ptr.Bool(true),    // differs
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
	fakeRepo(t, "testowner", "testrepo")

	live := repoJSON{Description: "old", Homepage: "https://old.example.com"}
	server := settingsServer(live, false, nil)
	t.Cleanup(server.Close)
	client := newTestClient(t, server)

	cfg := &config.Config{
		Repository: &config.RepositorySettings{
			Description: ptr.String("new description"),
			Homepage:    ptr.String("https://old.example.com"), // matches
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

func TestProcessRepoSettingsPrivateVulnerabilityReporting(t *testing.T) {
	fakeRepo(t, "testowner", "testrepo")

	live := repoJSON{}
	server := settingsServer(live, false, nil)
	t.Cleanup(server.Close)
	client := newTestClient(t, server)

	cfg := &config.Config{
		Repository: &config.RepositorySettings{
			PrivateVulnerabilityReportEnabled: ptr.Bool(true),
		},
	}

	results, err := alter.ProcessRepoSettings(cfg, alter.DryRun, client, "testowner", "testrepo", true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("got %d results, want 1", len(results))
	}
	if results[0].Field != "private_vulnerability_reporting_enabled" {
		t.Errorf("field = %q, want %q", results[0].Field, "private_vulnerability_reporting_enabled")
	}
	if results[0].Category != alter.WouldSet {
		t.Errorf("category = %q, want %q", results[0].Category, alter.WouldSet)
	}
	if results[0].Value != "true" {
		t.Errorf("value = %q, want %q", results[0].Value, "true")
	}
}
