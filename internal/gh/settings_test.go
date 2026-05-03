package gh

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/cli/go-gh/v2/pkg/api"
	"github.com/wimpysworld/tailor/internal/model"
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
	"web_commit_signoff_required": false,
	"topics": ["go", "cli-tool"]
}`

func TestReadRepoSettings(t *testing.T) {
	tests := []struct {
		name     string
		repoJSON string
		// expected field checks
		wantDesc    string
		wantDescNil bool
		wantHome    string
		wantHomeNil bool
		wantWiki    bool
		wantDisc    bool
		wantProj    bool
		wantIssues  bool
		wantMerge   bool
		wantSquash  bool
		wantRebase  bool
		wantSqTitle string
		wantSqMsg   string
		wantMcTitle string
		wantMcMsg   string
		wantDelete  bool
		wantUpdate  bool
		wantSignoff bool
		wantTopics  []string
	}{
		{
			name:        "all fields populated",
			repoJSON:    fullRepoJSON,
			wantDesc:    "A tailor for your repos",
			wantHome:    "https://tailor.dev",
			wantWiki:    false,
			wantDisc:    true,
			wantProj:    false,
			wantIssues:  true,
			wantMerge:   false,
			wantSquash:  true,
			wantRebase:  true,
			wantSqTitle: "PR_TITLE",
			wantSqMsg:   "PR_BODY",
			wantMcTitle: "PR_TITLE",
			wantMcMsg:   "PR_BODY",
			wantDelete:  true,
			wantUpdate:  true,
			wantSignoff: false,
			wantTopics:  []string{"go", "cli-tool"},
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
				"web_commit_signoff_required": true
			}`,
			wantDesc:    "",
			wantHome:    "",
			wantWiki:    true,
			wantDisc:    false,
			wantProj:    true,
			wantIssues:  false,
			wantMerge:   true,
			wantSquash:  false,
			wantRebase:  false,
			wantSqTitle: "COMMIT_OR_PR_TITLE",
			wantSqMsg:   "COMMIT_MESSAGES",
			wantMcTitle: "MERGE_MESSAGE",
			wantMcMsg:   "PR_TITLE",
			wantDelete:  false,
			wantUpdate:  false,
			wantSignoff: true,
			wantTopics:  nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch r.URL.Path {
				case "/repos/testowner/testrepo":
					fmt.Fprint(w, tt.repoJSON)
				default:
					http.NotFound(w, r)
				}
			}))
			t.Cleanup(server.Close)

			client := newTestClient(t, server)
			settings, _, err := ReadRepoSettings(client, "testowner", "testrepo")
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
			testutil.AssertBoolPtr(t, settings.WebCommitSignoffRequired, false, tt.wantSignoff, "web_commit_signoff_required")

			// string fields (always non-nil)
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
	_, _, err := ReadRepoSettings(client, "testowner", "testrepo")
	if err == nil {
		t.Fatal("ReadRepoSettings() expected error, got nil")
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
	settings := &model.RepositorySettings{
		Description: ptr.Ptr("new desc"),
		HasWiki:     ptr.Ptr(true),
	}

	_, err := ApplyRepoSettings(client, "testowner", "testrepo", settings)
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
	// Verify nil fields excluded.
	if _, ok := gotBody["homepage"]; ok {
		t.Error("homepage should not be in PATCH body when nil")
	}

	// Verify topics are excluded from the PATCH body.
	for _, key := range []string{
		"topics",
	} {
		if _, ok := gotBody[key]; ok {
			t.Errorf("%s should not be in PATCH body", key)
		}
	}
}

func TestBuildSettingsPayloadExtractsTopics(t *testing.T) {
	topics := []string{"go", "cli"}
	settings := &model.RepositorySettings{
		Description: ptr.Ptr("desc"),
		HasWiki:     ptr.Ptr(true),
		Topics:      &topics,
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
		"topics",
	} {
		if _, ok := p.Body[key]; ok {
			t.Errorf("%s should not be in PATCH body", key)
		}
	}

	// Verify extracted fields.
	if p.Topics == nil {
		t.Fatal("Topics is nil, want non-nil")
	}
	if len(*p.Topics) != 2 || (*p.Topics)[0] != "go" || (*p.Topics)[1] != "cli" {
		t.Errorf("Topics = %v, want [go cli]", *p.Topics)
	}
}

func TestBuildSettingsPayloadNilFieldsStayNil(t *testing.T) {
	settings := &model.RepositorySettings{
		HasWiki: ptr.Ptr(true),
	}

	p := buildSettingsPayload(settings)

	if p.Topics != nil {
		t.Errorf("Topics = %v, want nil", p.Topics)
	}
	if _, ok := p.Body["has_wiki"]; !ok {
		t.Error("has_wiki missing from PATCH body")
	}
}

func TestBuildSettingsPayloadEmptyTopics(t *testing.T) {
	topics := []string{}
	settings := &model.RepositorySettings{
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

func TestApplyRepoSettingsPatchError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprint(w, `{"message": "Internal Server Error"}`)
	}))
	t.Cleanup(server.Close)

	client := newTestClient(t, server)
	settings := &model.RepositorySettings{
		HasWiki: ptr.Ptr(true),
	}

	_, err := ApplyRepoSettings(client, "testowner", "testrepo", settings)
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

	client := newTestClient(t, server)
	settings := &model.RepositorySettings{
		HasWiki: ptr.Ptr(true),
	}

	result, err := ApplyRepoSettings(client, "testowner", "testrepo", settings)
	if err != nil {
		t.Fatalf("ApplyRepoSettings() unexpected hard error: %v", err)
	}
	if len(result.Skipped) != 1 {
		t.Fatalf("expected 1 skipped operation, got %d", len(result.Skipped))
	}
	if result.Skipped[0].Operation != "patch repo settings" {
		t.Errorf("skipped operation = %q, want %q", result.Skipped[0].Operation, "patch repo settings")
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
	settings := &model.RepositorySettings{
		Topics: &topics,
	}

	_, err := ApplyRepoSettings(client, "testowner", "testrepo", settings)
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
	settings := &model.RepositorySettings{
		Topics: &topics,
	}

	_, err := ApplyRepoSettings(client, "testowner", "testrepo", settings)
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
	settings := &model.RepositorySettings{
		HasWiki: ptr.Ptr(true),
	}

	_, err := ApplyRepoSettings(client, "testowner", "testrepo", settings)
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
	settings := &model.RepositorySettings{
		Topics: &topics,
	}

	_, err := ApplyRepoSettings(client, "testowner", "testrepo", settings)
	if err == nil {
		t.Fatal("ApplyRepoSettings() expected error from topics PUT, got nil")
	}
}

// --- Task 2.2: partial application tests ---

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

	client := newTestClient(t, server)
	topics := []string{"go"}
	settings := &model.RepositorySettings{
		HasWiki: ptr.Ptr(true),
		Topics:  &topics,
	}

	result, err := ApplyRepoSettings(client, "testowner", "testrepo", settings)
	if err != nil {
		t.Fatalf("unexpected hard error: %v", err)
	}
	if len(result.Skipped) != 1 || result.Skipped[0].Operation != "set topics" {
		t.Errorf("Skipped = %v, want [{set topics ...}]", result.Skipped)
	}
}

func TestApplyRepoSettingsAllSkipped(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		fmt.Fprint(w, `{"message": "Resource not accessible by integration"}`)
	}))
	t.Cleanup(server.Close)

	client := newTestClient(t, server)
	settings := &model.RepositorySettings{
		HasWiki: ptr.Ptr(true),
	}

	result, err := ApplyRepoSettings(client, "testowner", "testrepo", settings)
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

	client := newTestClient(t, server)
	settings := &model.RepositorySettings{
		HasWiki: ptr.Ptr(true),
	}

	result, err := ApplyRepoSettings(client, "testowner", "testrepo", settings)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Skipped) != 0 {
		t.Errorf("Skipped = %v, want empty", result.Skipped)
	}
}
