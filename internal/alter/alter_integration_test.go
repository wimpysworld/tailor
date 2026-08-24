package alter_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/cli/go-gh/v2/pkg/api"
	"github.com/wimpysworld/tailor/internal/alter"
	"github.com/wimpysworld/tailor/internal/config"
	"github.com/wimpysworld/tailor/internal/ghfake"
	"github.com/wimpysworld/tailor/internal/model"
	"github.com/wimpysworld/tailor/internal/swatch"
	"github.com/wimpysworld/tailor/internal/testutil"
)

var approvedDefaultActionPatterns = []string{
	"freerangebytes/setup-actionlint@*",
	"golang/govulncheck-action@*",
	"golangci/golangci-lint-action@*",
	"nick-fields/retry@*",
	"robherley/go-test-action@*",
	"softprops/action-gh-release@*",
}

// apiCall records a single API request made to the mock server.
type apiCall struct {
	Method string
	Path   string
	Body   string
}

// alterTestContext holds everything needed for an integration test of alter.Run.
type alterTestContext struct {
	Dir    string
	Client *api.RESTClient
	Server *httptest.Server

	mu    sync.Mutex
	calls []apiCall
}

// Calls returns a copy of the recorded API calls.
func (c *alterTestContext) Calls() []apiCall {
	c.mu.Lock()
	defer c.mu.Unlock()
	out := make([]apiCall, len(c.calls))
	copy(out, c.calls)
	return out
}

// MutatingCalls returns only PATCH/PUT/DELETE calls.
func (c *alterTestContext) MutatingCalls() []apiCall {
	c.mu.Lock()
	defer c.mu.Unlock()
	var out []apiCall
	for _, call := range c.calls {
		if call.Method == http.MethodPatch || call.Method == http.MethodPut || call.Method == http.MethodDelete {
			out = append(out, call)
		}
	}
	return out
}

// recordCall appends a call to the log.
func (c *alterTestContext) recordCall(method, path, body string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.calls = append(c.calls, apiCall{Method: method, Path: path, Body: body})
}

// testOption configures the mock server behaviour.
type testOption func(*alterServerConfig)

// alterServerConfig holds the mock server's response data.
type alterServerConfig struct {
	username          string
	owner             string
	repo              string
	repoJSON          repoJSON
	licenceID         string
	licenceBody       string
	labels            []model.LabelEntry // labels returned by GET /repos/{owner}/{repo}/labels
	noRepo            bool               // stub RepoContext to return false
	userError         int                // non-zero: return this HTTP status for GET /user
	licenceError      int                // non-zero: return this HTTP status for GET /licenses/*
	patchError        int                // non-zero: return this HTTP status for PATCH /repos/*
	securityEndpoints bool
	alertPutError     int
}

// WithUsername sets the mock username for GET /user.
func WithUsername(u string) testOption {
	return func(c *alterServerConfig) { c.username = u }
}

// WithRepo sets the owner/repo for mock routing.
func WithRepo(owner, repo string) testOption {
	return func(c *alterServerConfig) {
		c.owner = owner
		c.repo = repo
	}
}

// WithRepoSettings sets the live repo settings returned by GET /repos/{owner}/{repo}.
func WithRepoSettings(r repoJSON) testOption {
	return func(c *alterServerConfig) { c.repoJSON = r }
}

// WithLicence sets the licence ID and body for GET /licenses/{id}.
func WithLicence(id, body string) testOption {
	return func(c *alterServerConfig) {
		c.licenceID = id
		c.licenceBody = body
	}
}

// WithLabels sets the labels returned by GET /repos/{owner}/{repo}/labels.
func WithLabels(labels []model.LabelEntry) testOption {
	return func(c *alterServerConfig) { c.labels = labels }
}

// WithNoRepo stubs the repo context to return false (no GitHub remote).
func WithNoRepo() testOption {
	return func(c *alterServerConfig) { c.noRepo = true }
}

// WithUserError makes GET /user return the given HTTP status code.
func WithUserError(statusCode int) testOption {
	return func(c *alterServerConfig) { c.userError = statusCode }
}

// WithLicenceError makes GET /licenses/* return the given HTTP status code.
func WithLicenceError(statusCode int) testOption {
	return func(c *alterServerConfig) { c.licenceError = statusCode }
}

// WithPatchError makes PATCH /repos/* return the given HTTP status code.
func WithPatchError(statusCode int) testOption {
	return func(c *alterServerConfig) { c.patchError = statusCode }
}

func withSecurityEndpoints(alertPutError int) testOption {
	return func(c *alterServerConfig) {
		c.securityEndpoints = true
		c.alertPutError = alertPutError
		c.repoJSON.Permissions.Admin = true
	}
}

// setupAlterTest creates a temp dir, writes .tailor.yml from the provided
// YAML string, sets up a mock HTTP server, stubs the repo context, and
// returns an alterTestContext ready for use with alter.Run.
func setupAlterTest(t *testing.T, configYAML string, opts ...testOption) *alterTestContext {
	t.Helper()

	sc := &alterServerConfig{
		username:    "testuser",
		owner:       "testowner",
		repo:        "testrepo",
		licenceID:   "mit",
		licenceBody: "MIT License text\n\nCopyright (c) [year] [fullname]",
	}
	for _, o := range opts {
		o(sc)
	}

	dir := t.TempDir()

	if err := os.WriteFile(filepath.Join(dir, ".tailor.yml"), []byte(configYAML), 0o644); err != nil {
		t.Fatalf("writing .tailor.yml: %v", err)
	}

	ctx := &alterTestContext{Dir: dir}

	repoPath := fmt.Sprintf("/repos/%s/%s", sc.owner, sc.repo)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path

		// Capture mutating request bodies for later assertions.
		var body string
		if r.Body != nil && (r.Method == http.MethodPatch || r.Method == http.MethodPut || r.Method == http.MethodPost) {
			data, _ := io.ReadAll(r.Body)
			body = string(data)
		}

		ctx.recordCall(r.Method, path, body)

		switch {
		case r.Method == http.MethodGet && path == "/user":
			if sc.userError != 0 {
				w.WriteHeader(sc.userError)
				fmt.Fprintf(w, `{"message":"error"}`)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprintf(w, `{"login":%q}`, sc.username)

		case r.Method == http.MethodGet && path == repoPath:
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(sc.repoJSON)

		case sc.securityEndpoints && r.Method == http.MethodGet && path == repoPath+"/vulnerability-alerts":
			w.WriteHeader(http.StatusNotFound)

		case sc.securityEndpoints && r.Method == http.MethodGet && path == repoPath+"/automated-security-fixes":
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprint(w, `{"enabled":false,"paused":false}`)

		case sc.securityEndpoints && r.Method == http.MethodPut && path == repoPath+"/vulnerability-alerts":
			if sc.alertPutError != 0 {
				w.WriteHeader(sc.alertPutError)
				fmt.Fprint(w, `{"message":"error"}`)
				return
			}
			w.WriteHeader(http.StatusNoContent)

		case sc.securityEndpoints && r.Method == http.MethodPut && path == repoPath+"/automated-security-fixes":
			w.WriteHeader(http.StatusNoContent)

		case r.Method == http.MethodGet && path == repoPath+"/actions/permissions/workflow":
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprint(w, `{"default_workflow_permissions":"read","can_approve_pull_request_reviews":false}`)

		case r.Method == http.MethodGet && path == repoPath+"/actions/permissions":
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprint(w, `{"enabled":true,"allowed_actions":"selected","sha_pinning_required":true}`)

		case r.Method == http.MethodGet && path == repoPath+"/actions/permissions/selected-actions":
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprint(w, `{"github_owned_allowed":true,"verified_allowed":false,"patterns_allowed":["z/*","a/*"]}`)

		case r.Method == http.MethodGet && strings.HasPrefix(path, "/licenses/"):
			if sc.licenceError != 0 {
				w.WriteHeader(sc.licenceError)
				fmt.Fprintf(w, `{"message":"Not Found"}`)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprintf(w, `{"key":%q,"name":"MIT License","body":%q}`, sc.licenceID, sc.licenceBody)

		case r.Method == http.MethodPatch && path == repoPath:
			if sc.patchError != 0 {
				w.WriteHeader(sc.patchError)
				fmt.Fprintf(w, `{"message":"Forbidden"}`)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			fmt.Fprint(w, `{}`)

		case r.Method == http.MethodPut && path == repoPath+"/actions/permissions/workflow":
			w.WriteHeader(http.StatusNoContent)

		case r.Method == http.MethodPut && (path == repoPath+"/actions/permissions" || path == repoPath+"/actions/permissions/selected-actions"):
			w.WriteHeader(http.StatusNoContent)

		case r.Method == http.MethodGet && path == repoPath+"/labels":
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(sc.labels)

		case r.Method == http.MethodPost && path == repoPath+"/labels":
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusCreated)
			fmt.Fprint(w, body)

		case (r.Method == http.MethodPatch || r.Method == http.MethodDelete) && strings.HasPrefix(path, repoPath+"/labels/"):
			w.WriteHeader(http.StatusOK)
			fmt.Fprint(w, `{}`)

		default:
			w.WriteHeader(http.StatusNotFound)
			fmt.Fprintf(w, `{"message":"Not Found: %s %s"}`, r.Method, path) //nolint:gosec // test HTTP handler, not exposed to user input
		}
	}))
	t.Cleanup(server.Close)

	ctx.Server = server
	ctx.Client = testutil.NewTestClient(t, server)

	if sc.noRepo {
		ghfake.FakeNoRepo(t)
	} else {
		ghfake.FakeRepo(t, sc.owner, sc.repo)
	}

	return ctx
}

// loadTestConfig loads .tailor.yml from dir through the config package,
// matching the real alter.Run code path.
func loadTestConfig(t *testing.T, dir string) *config.Config {
	t.Helper()
	cfg, err := config.Load(dir)
	if err != nil {
		t.Fatalf("config.Load(%q): %v", dir, err)
	}
	return cfg
}

// captureAlterRun runs alter.Run in the given mode, capturing stdout and
// suppressing stderr. Returns the stdout output.
func captureAlterRun(t *testing.T, cfg *config.Config, dir string, mode alter.ApplyMode, client *api.RESTClient) string {
	t.Helper()

	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	// Suppress stderr (licence warnings).
	oldStderr := os.Stderr
	_, wErr, _ := os.Pipe()
	os.Stderr = wErr

	err := alter.Run(cfg, dir, mode, client)

	w.Close()
	wErr.Close()
	os.Stdout = oldStdout
	os.Stderr = oldStderr

	if err != nil {
		t.Fatalf("alter.Run() error: %v", err)
	}

	var buf bytes.Buffer
	_, _ = buf.ReadFrom(r)
	return buf.String()
}

// runAlterExpectError runs alter.Run in Apply mode and returns the error.
// Stdout and stderr are suppressed. Fails if no error is returned.
func runAlterExpectError(t *testing.T, cfg *config.Config, dir string, client *api.RESTClient) error {
	t.Helper()

	oldStdout := os.Stdout
	_, wOut, _ := os.Pipe()
	os.Stdout = wOut

	oldStderr := os.Stderr
	_, wErr, _ := os.Pipe()
	os.Stderr = wErr

	err := alter.Run(cfg, dir, alter.Apply, client)

	wOut.Close()
	wErr.Close()
	os.Stdout = oldStdout
	os.Stderr = oldStderr

	if err == nil {
		t.Fatal("expected alter.Run() to return an error, got nil")
	}
	return err
}

// captureAlterRunWithStderr runs alter.Run capturing both stdout and stderr.
// Returns stdout, stderr, and the error (which may be nil).
//
//nolint:unparam // stdout return kept for test symmetry with captureAlterRun
func captureAlterRunWithStderr(t *testing.T, cfg *config.Config, dir string, mode alter.ApplyMode, client *api.RESTClient) (string, string, error) {
	t.Helper()

	oldStdout := os.Stdout
	rOut, wOut, _ := os.Pipe()
	os.Stdout = wOut

	oldStderr := os.Stderr
	rErr, wErr, _ := os.Pipe()
	os.Stderr = wErr

	err := alter.Run(cfg, dir, mode, client)

	wOut.Close()
	wErr.Close()
	os.Stdout = oldStdout
	os.Stderr = oldStderr

	var bufOut, bufErr bytes.Buffer
	_, _ = bufOut.ReadFrom(rOut)
	_, _ = bufErr.ReadFrom(rErr)

	return bufOut.String(), bufErr.String(), err
}

// requireContains fails if output does not contain substr.
func requireContains(t *testing.T, output, substr string) {
	t.Helper()
	if !strings.Contains(output, substr) {
		t.Errorf("expected output to contain %q, got:\n%s", substr, output)
	}
}

func TestAlterRunNormalisesSecurityPrerequisite(t *testing.T) {
	const configYAML = `license: none
repository:
  vulnerability_alerts_enabled: false
  automated_security_fixes_enabled: true
swatches:
  - path: .tailor.yml
    alteration: never
`
	const warning = "warning: set vulnerability_alerts_enabled to true because automated_security_fixes_enabled requires vulnerability alerts\n"

	tests := []struct {
		name      string
		mode      alter.ApplyMode
		wantWrite bool
	}{
		{name: "alter", mode: alter.Apply, wantWrite: true},
		{name: "recut", mode: alter.Recut, wantWrite: true},
		{name: "baste", mode: alter.DryRun},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tc := setupAlterTest(t, configYAML, withSecurityEndpoints(0))
			writeOnDisk(t, tc.Dir, "LICENSE", []byte("existing"))
			before, err := os.ReadFile(filepath.Join(tc.Dir, ".tailor.yml"))
			if err != nil {
				t.Fatalf("reading initial config: %v", err)
			}

			cfg := loadTestConfig(t, tc.Dir)
			stdout, stderr, err := captureAlterRunWithStderr(t, cfg, tc.Dir, tt.mode, tc.Client)
			if err != nil {
				t.Fatalf("alter.Run() error: %v", err)
			}
			if stderr != warning {
				t.Errorf("stderr = %q, want %q", stderr, warning)
			}
			if tt.wantWrite {
				requireContains(t, stdout, "updated:")
			} else {
				requireContains(t, stdout, "would update:")
			}
			requireContains(t, stdout, ".tailor.yml")

			after, err := os.ReadFile(filepath.Join(tc.Dir, ".tailor.yml"))
			if err != nil {
				t.Fatalf("reading final config: %v", err)
			}
			if tt.wantWrite {
				persisted := loadTestConfig(t, tc.Dir)
				testutil.AssertBoolPtr(t, persisted.Repository.VulnerabilityAlertsEnabled, false, true, "vulnerability_alerts_enabled")
				if bytes.Equal(after, before) {
					t.Error(".tailor.yml bytes did not change")
				}
			} else {
				if !bytes.Equal(after, before) {
					t.Error("baste changed .tailor.yml")
				}
				if calls := tc.MutatingCalls(); len(calls) != 0 {
					t.Errorf("baste made mutating API calls: %v", calls)
				}
				return
			}

			var securityWrites []string
			for _, call := range tc.MutatingCalls() {
				if strings.Contains(call.Path, "vulnerability-alerts") || strings.Contains(call.Path, "automated-security-fixes") {
					securityWrites = append(securityWrites, call.Path)
				}
			}
			wantOrder := []string{"/repos/testowner/testrepo/vulnerability-alerts", "/repos/testowner/testrepo/automated-security-fixes"}
			if !slices.Equal(securityWrites, wantOrder) {
				t.Errorf("security API order = %v, want %v", securityWrites, wantOrder)
			}
		})
	}
}

func TestAlterRunAlertFailureBlocksAutomatedSecurityFixes(t *testing.T) {
	const configYAML = `license: none
repository:
  vulnerability_alerts_enabled: false
  automated_security_fixes_enabled: true
swatches:
  - path: .tailor.yml
    alteration: never
`
	tc := setupAlterTest(t, configYAML, withSecurityEndpoints(http.StatusInternalServerError))
	writeOnDisk(t, tc.Dir, "LICENSE", []byte("existing"))

	cfg := loadTestConfig(t, tc.Dir)
	_, stderr, err := captureAlterRunWithStderr(t, cfg, tc.Dir, alter.Apply, tc.Client)
	if err == nil {
		t.Fatal("alter.Run() returned nil, want alert API error")
	}
	requireContains(t, stderr, "warning: set vulnerability_alerts_enabled to true")

	persisted := loadTestConfig(t, tc.Dir)
	testutil.AssertBoolPtr(t, persisted.Repository.VulnerabilityAlertsEnabled, false, true, "vulnerability_alerts_enabled")
	for _, call := range tc.MutatingCalls() {
		if strings.Contains(call.Path, "automated-security-fixes") {
			t.Fatalf("automated security fixes API called after alert failure: %v", tc.MutatingCalls())
		}
	}
}

// requireNotContains fails if output contains substr.
func requireNotContains(t *testing.T, output, substr string) {
	t.Helper()
	if strings.Contains(output, substr) {
		t.Errorf("expected output NOT to contain %q, got:\n%s", substr, output)
	}
}

// TestAlterRunDryRunSmokeTest verifies the integration test infrastructure with
// a single swatch entry. Dry-run reports expected output and writes no files.
func TestAlterRunDryRunSmokeTest(t *testing.T) {
	configYAML := `license: mit
swatches:
  - path: .gitignore
    alteration: first-fit
`
	tc := setupAlterTest(t, configYAML)
	cfg := loadTestConfig(t, tc.Dir)

	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	err := alter.Run(cfg, tc.Dir, alter.DryRun, tc.Client)

	w.Close()
	os.Stdout = oldStdout

	if err != nil {
		t.Fatalf("alter.Run() error: %v", err)
	}

	var buf bytes.Buffer
	_, _ = buf.ReadFrom(r)
	output := buf.String()

	// Dry-run output reports the swatch and licence copies.
	if !strings.Contains(output, "would copy:") {
		t.Errorf("expected output to contain 'would copy:', got:\n%s", output)
	}
	if !strings.Contains(output, ".gitignore") {
		t.Errorf("expected output to contain '.gitignore', got:\n%s", output)
	}
	if !strings.Contains(output, "LICENSE") {
		t.Errorf("expected output to contain 'LICENSE', got:\n%s", output)
	}

	// Dry-run leaves the filesystem unchanged.
	if _, err := os.Stat(filepath.Join(tc.Dir, ".gitignore")); err == nil {
		t.Error("dry run wrote .gitignore to disk")
	}
	if _, err := os.Stat(filepath.Join(tc.Dir, "LICENSE")); err == nil {
		t.Error("dry run wrote LICENSE to disk")
	}

	// Dry-run must not make mutating API calls.
	if mc := tc.MutatingCalls(); len(mc) != 0 {
		t.Errorf("dry run made %d mutating API calls: %v", len(mc), mc)
	}
	for _, call := range tc.Calls() {
		if call.Path == "/repos/testowner/testrepo/actions/permissions" || strings.HasSuffix(call.Path, "/actions/permissions/selected-actions") {
			t.Errorf("config without actions section made Actions policy call: %v", call)
		}
	}
}

func TestAlterRunActionsPolicy(t *testing.T) {
	configYAML := `license: none
actions:
  enabled: false
  allowed_actions: selected
  sha_pinning_required: true
  github_owned_allowed: true
  verified_allowed: false
  patterns_allowed:
    - a/*
    - z/*
swatches: []
`
	tc := setupAlterTest(t, configYAML)
	cfg := loadTestConfig(t, tc.Dir)

	output := captureAlterRun(t, cfg, tc.Dir, alter.DryRun, tc.Client)
	requireContains(t, output, "actions.enabled = false")
	requireContains(t, output, "actions.patterns_allowed (already a/*, z/*)")
	if calls := tc.MutatingCalls(); len(calls) != 0 {
		t.Fatalf("dry run made mutating calls: %v", calls)
	}

	output = captureAlterRun(t, cfg, tc.Dir, alter.Apply, tc.Client)
	requireContains(t, output, "actions.enabled = false")
	calls := tc.MutatingCalls()
	if len(calls) != 1 || calls[0].Path != "/repos/testowner/testrepo/actions/permissions" {
		t.Fatalf("mutating calls = %v, want one Actions permissions PUT", calls)
	}
}

func TestAlterRunOutputByMode(t *testing.T) {
	configYAML := `license: mit
repository:
  has_wiki: false
labels:
  - name: bug
    color: d73a4a
    description: A problem
swatches:
  - path: .gitignore
    alteration: first-fit
`
	tests := []struct {
		name string
		mode alter.ApplyMode
		want string
	}{
		{
			name: "dry run",
			mode: alter.DryRun,
			want: "would set:                           repository.has_wiki = false\n" +
				"would create:                        label.bug = #d73a4a \"A problem\"\n" +
				"would copy:                          .gitignore\n" +
				"would copy:                          LICENSE\n",
		},
		{
			name: "apply",
			mode: alter.Apply,
			want: "set:                                 repository.has_wiki = false\n" +
				"created:                             label.bug = #d73a4a \"A problem\"\n" +
				"copied:                              .gitignore\n" +
				"copied:                              LICENSE\n",
		},
		{
			name: "recut",
			mode: alter.Recut,
			want: "set:                                 repository.has_wiki = false\n" +
				"created:                             label.bug = #d73a4a \"A problem\"\n" +
				"copied:                              .gitignore\n" +
				"copied:                              LICENSE\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tc := setupAlterTest(t, configYAML,
				WithRepoSettings(repoJSON{HasWiki: true}),
			)
			cfg := loadTestConfig(t, tc.Dir)
			got := captureAlterRun(t, cfg, tc.Dir, tt.mode, tc.Client)
			if got != tt.want {
				t.Errorf("alter.Run() output =\n%s\nwant:\n%s", got, tt.want)
			}
		})
	}
}

// TestAlterRunDryRunWithRepoSettings verifies that repo settings appear in
// dry-run output when they differ from live settings.
func TestAlterRunDryRunWithRepoSettings(t *testing.T) {
	configYAML := `license: none
repository:
  has_wiki: false
swatches:
  - path: .gitignore
    alteration: first-fit
`
	tc := setupAlterTest(t, configYAML,
		WithRepoSettings(repoJSON{HasWiki: true}),
	)

	// Write a LICENSE so no licence warning is emitted.
	writeOnDisk(t, tc.Dir, "LICENSE", []byte("existing"))

	cfg := loadTestConfig(t, tc.Dir)

	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	err := alter.Run(cfg, tc.Dir, alter.DryRun, tc.Client)

	w.Close()
	os.Stdout = oldStdout

	if err != nil {
		t.Fatalf("alter.Run() error: %v", err)
	}

	var buf bytes.Buffer
	_, _ = buf.ReadFrom(r)
	output := buf.String()

	if !strings.Contains(output, "would set:") {
		t.Errorf("expected output to contain 'would set:', got:\n%s", output)
	}
	if !strings.Contains(output, "repository.has_wiki") {
		t.Errorf("expected output to contain 'repository.has_wiki', got:\n%s", output)
	}

	// Dry-run must not make mutating API calls.
	if mc := tc.MutatingCalls(); len(mc) != 0 {
		t.Errorf("dry run made %d mutating API calls: %v", len(mc), mc)
	}
}

// TestAlterRunApplyWritesFiles verifies that Apply mode writes swatch files
// and calls the GitHub API.
func TestAlterRunApplyWritesFiles(t *testing.T) {
	configYAML := `license: mit
swatches:
  - path: .gitignore
    alteration: first-fit
`
	tc := setupAlterTest(t, configYAML)
	cfg := loadTestConfig(t, tc.Dir)

	oldStdout := os.Stdout
	_, w, _ := os.Pipe()
	os.Stdout = w

	err := alter.Run(cfg, tc.Dir, alter.Apply, tc.Client)

	w.Close()
	os.Stdout = oldStdout

	if err != nil {
		t.Fatalf("alter.Run() error: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(tc.Dir, ".gitignore"))
	if err != nil {
		t.Fatalf(".gitignore not written: %v", err)
	}
	want, err := swatch.Content(".gitignore")
	if err != nil {
		t.Fatalf("swatch.Content(.gitignore): %v", err)
	}
	if string(data) != string(want) {
		t.Error(".gitignore content does not match embedded swatch")
	}

	if _, err := os.Stat(filepath.Join(tc.Dir, "LICENSE")); err != nil {
		t.Fatalf("LICENSE not written: %v", err)
	}
}

// TestAlterRunDryRunNoFilesOnDisk verifies that when no swatch files exist on
// disk, all swatches report "would copy", the licence reports "would copy",
// and repo settings that differ report "would set".
func TestAlterRunDryRunNoFilesOnDisk(t *testing.T) {
	configYAML := `license: mit
repository:
  has_wiki: false
  delete_branch_on_merge: true
swatches:
  - path: .gitignore
    alteration: first-fit
  - path: CODE_OF_CONDUCT.md
    alteration: always
  - path: SECURITY.md
    alteration: always
`
	tc := setupAlterTest(t, configYAML,
		WithRepoSettings(repoJSON{HasWiki: true, DeleteBranchOnMerge: false}),
	)
	cfg := loadTestConfig(t, tc.Dir)
	output := captureAlterRun(t, cfg, tc.Dir, alter.DryRun, tc.Client)

	// Missing swatches and licence are all reported as copies.
	requireContains(t, output, "would copy:")
	requireContains(t, output, ".gitignore")
	requireContains(t, output, "CODE_OF_CONDUCT.md")
	requireContains(t, output, "SECURITY.md")
	requireContains(t, output, "LICENSE")

	// Repo settings that differ are reported as settings changes.
	requireContains(t, output, "would set:")
	requireContains(t, output, "repository.has_wiki")
	requireContains(t, output, "repository.delete_branch_on_merge")

	// Dry-run must not make mutating API calls.
	if mc := tc.MutatingCalls(); len(mc) != 0 {
		t.Errorf("dry run made %d mutating API calls: %v", len(mc), mc)
	}
}

// TestAlterRunDryRunAllFilesPresent verifies the output when all swatch files
// and licence already exist on disk with matching content. Non-substituted
// "always" swatches show "no change", "first-fit" swatches show "skipped",
// and substituted "always" swatches show "would overwrite".
func TestAlterRunDryRunAllFilesPresent(t *testing.T) {
	configYAML := `license: mit
repository:
  has_wiki: false
swatches:
  - path: .gitignore
    alteration: first-fit
  - path: CODE_OF_CONDUCT.md
    alteration: always
  - path: SECURITY.md
    alteration: always
`
	tc := setupAlterTest(t, configYAML,
		WithRepoSettings(repoJSON{HasWiki: false}),
	)

	// Existing files use matching embedded content.
	for _, src := range []string{".gitignore", "CODE_OF_CONDUCT.md", "SECURITY.md"} {
		content := mustContent(t, src)
		writeOnDisk(t, tc.Dir, src, content)
	}
	// Existing licence exercises first-fit licence behaviour.
	writeOnDisk(t, tc.Dir, "LICENSE", []byte("existing licence"))

	cfg := loadTestConfig(t, tc.Dir)
	output := captureAlterRun(t, cfg, tc.Dir, alter.DryRun, tc.Client)

	// Existing first-fit destinations report the path before the reason.
	requireContains(t, output, "skipped:                             .gitignore (first-fit, exists)")

	// Non-substituted always CODE_OF_CONDUCT.md with matching content: "no change".
	requireContains(t, output, "no change:")
	requireContains(t, output, "CODE_OF_CONDUCT.md")

	// SECURITY.md contains unresolved template content on disk, so resolved
	// substituted content reports "would overwrite".
	requireContains(t, output, "would overwrite:")
	requireContains(t, output, "SECURITY.md")

	// The existing licence uses the same first-fit skip format.
	requireContains(t, output, "skipped:                             LICENSE (first-fit, exists)")

	// Repo setting matches: "no change".
	requireContains(t, output, "repository.has_wiki")

	// No "would copy" should appear since all files exist.
	requireNotContains(t, output, "would copy:")

	// Dry-run must not make mutating API calls.
	if mc := tc.MutatingCalls(); len(mc) != 0 {
		t.Errorf("dry run made %d mutating API calls: %v", len(mc), mc)
	}
}

// TestAlterRunDryRunMixedFiles verifies output when some files exist and others
// are absent, producing a mix of "would copy", "skipped", and "no change".
func TestAlterRunDryRunMixedFiles(t *testing.T) {
	configYAML := `license: mit
swatches:
  - path: .gitignore
    alteration: first-fit
  - path: CODE_OF_CONDUCT.md
    alteration: always
  - path: CONTRIBUTING.md
    alteration: always
`
	tc := setupAlterTest(t, configYAML)

	// Existing .gitignore exercises first-fit skip. Matching
	// CODE_OF_CONDUCT.md content exercises always/no-change.
	writeOnDisk(t, tc.Dir, ".gitignore", mustContent(t, ".gitignore"))
	writeOnDisk(t, tc.Dir, "CODE_OF_CONDUCT.md", mustContent(t, "CODE_OF_CONDUCT.md"))
	// CONTRIBUTING.md absent: will be "would copy".
	// LICENSE absent: will be "would copy".

	cfg := loadTestConfig(t, tc.Dir)
	output := captureAlterRun(t, cfg, tc.Dir, alter.DryRun, tc.Client)

	// Existing first-fit swatches are skipped.
	requireContains(t, output, "skipped:                             .gitignore (first-fit, exists)")

	// Existing always swatches with matching content report no change.
	requireContains(t, output, "no change:")

	// Missing swatches report copies.
	requireContains(t, output, "would copy:")
	requireContains(t, output, "CONTRIBUTING.md")

	// Missing licences share the same copy status.
	requireContains(t, output, "LICENSE")
}

func TestAlterRunSkippedOutputAllModes(t *testing.T) {
	const configYAML = `license: mit
swatches:
  - path: .envrc
    alteration: first-fit
  - path: .github/pull_request_template.md
    alteration: never
`

	for _, mode := range []alter.ApplyMode{alter.DryRun, alter.Apply, alter.Recut} {
		t.Run(fmt.Sprint(mode), func(t *testing.T) {
			tc := setupAlterTest(t, configYAML, WithLicence("mit", "unused"))
			writeOnDisk(t, tc.Dir, ".envrc", []byte("existing"))
			writeOnDisk(t, tc.Dir, "LICENSE", []byte("existing"))

			cfg := loadTestConfig(t, tc.Dir)
			output := captureAlterRun(t, cfg, tc.Dir, mode, tc.Client)
			if mode == alter.Recut {
				requireContains(t, output, "skipped:                             LICENSE (first-fit, exists)")
			} else {
				requireContains(t, output, "skipped:                             .envrc (first-fit, exists)")
			}
			requireContains(t, output, "skipped:                             .github/pull_request_template.md (mode never)")
			requireNotContains(t, output, "would skip")
		})
	}
}

// TestAlterRunDryRunAlwaysSwatchDiffersContent verifies that a non-substituted
// "always" swatch whose on-disk content differs from embedded shows "would overwrite".
func TestAlterRunDryRunAlwaysSwatchDiffersContent(t *testing.T) {
	configYAML := `license: none
swatches:
  - path: CODE_OF_CONDUCT.md
    alteration: always
`
	tc := setupAlterTest(t, configYAML)

	// Existing stale content reports an overwrite.
	writeOnDisk(t, tc.Dir, "CODE_OF_CONDUCT.md", []byte("outdated conduct document"))
	// Existing licence suppresses the missing-licence warning.
	writeOnDisk(t, tc.Dir, "LICENSE", []byte("existing"))

	cfg := loadTestConfig(t, tc.Dir)
	output := captureAlterRun(t, cfg, tc.Dir, alter.DryRun, tc.Client)

	requireContains(t, output, "would overwrite:")
	requireContains(t, output, "CODE_OF_CONDUCT.md")

	// Dry-run does not change the stale file.
	data, err := os.ReadFile(filepath.Join(tc.Dir, "CODE_OF_CONDUCT.md"))
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "outdated conduct document" {
		t.Error("dry run modified CODE_OF_CONDUCT.md on disk")
	}
}

// TestAlterRunDryRunSubstitutedSwatchAlwaysOverwrites verifies that substituted
// "always" swatches (SECURITY.md, .github/FUNDING.yml, .github/ISSUE_TEMPLATE/config.yml)
// always show "would overwrite" even when on-disk content matches the embedded template.
func TestAlterRunDryRunSubstitutedSwatchAlwaysOverwrites(t *testing.T) {
	configYAML := `license: none
swatches:
  - path: SECURITY.md
    alteration: always
`
	tc := setupAlterTest(t, configYAML)

	// Write SECURITY.md with the exact embedded content (before substitution).
	writeOnDisk(t, tc.Dir, "SECURITY.md", mustContent(t, "SECURITY.md"))
	// Existing licence suppresses the missing-licence warning.
	writeOnDisk(t, tc.Dir, "LICENSE", []byte("existing"))

	cfg := loadTestConfig(t, tc.Dir)
	output := captureAlterRun(t, cfg, tc.Dir, alter.DryRun, tc.Client)

	requireContains(t, output, "would overwrite:")
	requireContains(t, output, "SECURITY.md")

	// Must not show "no change" for SECURITY.md.
	for line := range strings.SplitSeq(output, "\n") {
		if strings.Contains(line, "SECURITY.md") && strings.Contains(line, "no change") {
			t.Errorf("substituted swatch SECURITY.md should not show 'no change', got: %s", line)
		}
	}
}

// TestAlterRunDryRunNoFilesWritten verifies that a comprehensive dry-run
// creates no new files and modifies no existing files.
func TestAlterRunDryRunNoFilesWritten(t *testing.T) {
	configYAML := `license: mit
repository:
  has_wiki: false
swatches:
  - path: .gitignore
    alteration: first-fit
  - path: CODE_OF_CONDUCT.md
    alteration: always
  - path: SECURITY.md
    alteration: always
  - path: CONTRIBUTING.md
    alteration: always
`
	tc := setupAlterTest(t, configYAML,
		WithRepoSettings(repoJSON{HasWiki: true}),
	)

	// Existing known content proves dry-run does not modify files.
	existingContent := []byte("original content")
	writeOnDisk(t, tc.Dir, "CODE_OF_CONDUCT.md", existingContent)

	// Capture filesystem state before dry-run.
	dirEntries := func() map[string]int64 {
		entries := make(map[string]int64)
		_ = filepath.Walk(tc.Dir, func(path string, info os.FileInfo, _ error) error {
			if !info.IsDir() {
				rel, _ := filepath.Rel(tc.Dir, path)
				entries[rel] = info.Size()
			}
			return nil
		})
		return entries
	}

	before := dirEntries()

	cfg := loadTestConfig(t, tc.Dir)
	_ = captureAlterRun(t, cfg, tc.Dir, alter.DryRun, tc.Client)

	after := dirEntries()

	// Dry-run creates no new files.
	for path := range after {
		if _, existed := before[path]; !existed {
			t.Errorf("dry run created new file: %s", path)
		}
	}

	// Dry-run preserves existing file content.
	data, err := os.ReadFile(filepath.Join(tc.Dir, "CODE_OF_CONDUCT.md"))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(data, existingContent) {
		t.Error("dry run modified CODE_OF_CONDUCT.md")
	}

	// Absent swatches remain absent after dry-run.
	for _, absent := range []string{".gitignore", "SECURITY.md", "CONTRIBUTING.md", "LICENSE"} {
		if _, err := os.Stat(filepath.Join(tc.Dir, absent)); err == nil {
			t.Errorf("dry run wrote %s to disk", absent)
		}
	}

	// Dry-run must not make mutating API calls.
	if mc := tc.MutatingCalls(); len(mc) != 0 {
		t.Errorf("dry run made %d mutating API calls: %v", len(mc), mc)
	}
}

// TestAlterRunDryRunOutputOrder verifies that actionable items ("would set",
// "would copy", "would overwrite") appear before informational items
// ("no change", "skipped"), and that repo settings appear before swatches.
func TestAlterRunDryRunOutputOrder(t *testing.T) {
	configYAML := `license: mit
repository:
  has_wiki: false
  has_issues: true
swatches:
  - path: .gitignore
    alteration: first-fit
  - path: CODE_OF_CONDUCT.md
    alteration: always
  - path: CONTRIBUTING.md
    alteration: always
`
	tc := setupAlterTest(t, configYAML,
		WithRepoSettings(repoJSON{HasWiki: true, HasIssues: true}),
	)

	// .gitignore present (first-fit, skipped), CODE_OF_CONDUCT.md matching (no change),
	// CONTRIBUTING.md absent (would copy), LICENSE absent (would copy).
	writeOnDisk(t, tc.Dir, ".gitignore", mustContent(t, ".gitignore"))
	writeOnDisk(t, tc.Dir, "CODE_OF_CONDUCT.md", mustContent(t, "CODE_OF_CONDUCT.md"))

	cfg := loadTestConfig(t, tc.Dir)
	output := captureAlterRun(t, cfg, tc.Dir, alter.DryRun, tc.Client)

	lines := strings.Split(strings.TrimSpace(output), "\n")
	if len(lines) == 0 {
		t.Fatal("expected non-empty output")
	}

	// Classify each line as actionable or informational.
	actionableLabels := []string{"would set:", "would copy:", "would overwrite:"}
	informationalLabels := []string{"no change:", "skipped:"}

	isActionable := func(line string) bool {
		for _, label := range actionableLabels {
			if strings.Contains(line, label) {
				return true
			}
		}
		return false
	}
	isInformational := func(line string) bool {
		for _, label := range informationalLabels {
			if strings.Contains(line, label) {
				return true
			}
		}
		return false
	}

	// Repo settings lines must precede swatch lines.
	lastRepoLine := -1
	firstSwatchLine := -1
	for i, line := range lines {
		if strings.Contains(line, "repository.") {
			lastRepoLine = i
		} else if firstSwatchLine == -1 {
			firstSwatchLine = i
		}
	}
	if lastRepoLine >= 0 && firstSwatchLine >= 0 && lastRepoLine > firstSwatchLine {
		t.Errorf("repo settings line at index %d appears after swatch line at index %d", lastRepoLine, firstSwatchLine)
	}

	// Within each section (repo settings, swatches), actionable precedes informational.
	// Check swatch lines only (after repo settings).
	swatchStart := 0
	if lastRepoLine >= 0 {
		swatchStart = lastRepoLine + 1
	}
	swatchLines := lines[swatchStart:]

	seenInformational := false
	for _, line := range swatchLines {
		if isInformational(line) {
			seenInformational = true
		}
		if isActionable(line) && seenInformational {
			t.Errorf("actionable line %q appears after informational line in swatch section", line)
		}
	}
}

// TestAlterRunDryRunColumnWidth verifies that all category labels in the output
// use the default 37-character column width.
func TestAlterRunDryRunColumnWidth(t *testing.T) {
	configYAML := `license: mit
repository:
  has_wiki: false
swatches:
  - path: .gitignore
    alteration: first-fit
  - path: CODE_OF_CONDUCT.md
    alteration: always
  - path: SECURITY.md
    alteration: always
`
	tc := setupAlterTest(t, configYAML,
		WithRepoSettings(repoJSON{HasWiki: true}),
	)

	// Create some files to trigger different categories.
	writeOnDisk(t, tc.Dir, ".gitignore", mustContent(t, ".gitignore"))
	writeOnDisk(t, tc.Dir, "CODE_OF_CONDUCT.md", mustContent(t, "CODE_OF_CONDUCT.md"))
	writeOnDisk(t, tc.Dir, "SECURITY.md", mustContent(t, "SECURITY.md"))

	cfg := loadTestConfig(t, tc.Dir)
	output := captureAlterRun(t, cfg, tc.Dir, alter.DryRun, tc.Client)

	lines := strings.Split(strings.TrimSpace(output), "\n")
	if len(lines) == 0 {
		t.Fatal("expected non-empty output")
	}

	// Each line has the label portion padded to 37 characters.
	knownLabels := []string{
		"would copy:",
		"would overwrite:",
		"no change:",
		"skipped:",
		"would set:",
	}

	const expectedWidth = 37
	for _, line := range lines {
		if len(line) < expectedWidth {
			t.Errorf("line too short to contain label + content: %q", line)
			continue
		}

		labelPart := line[:expectedWidth]
		trimmed := strings.TrimRight(labelPart, " ")

		if !slices.Contains(knownLabels, trimmed) {
			t.Errorf("label portion %q does not match any known label", trimmed)
		}
	}
}

// TestAlterRunApplyEmptyProject verifies that apply mode on an empty project
// writes all swatch files and the licence to disk, and calls PATCH on repo settings.
func TestAlterRunApplyEmptyProject(t *testing.T) {
	configYAML := `license: mit
repository:
  has_wiki: false
swatches:
  - path: .gitignore
    alteration: first-fit
  - path: CODE_OF_CONDUCT.md
    alteration: always
  - path: SECURITY.md
    alteration: always
`
	tc := setupAlterTest(t, configYAML,
		WithLicence("mit", "MIT License text here"),
		WithRepoSettings(repoJSON{HasWiki: true}),
	)
	cfg := loadTestConfig(t, tc.Dir)
	_ = captureAlterRun(t, cfg, tc.Dir, alter.Apply, tc.Client)

	// All swatch files must exist.
	for _, dest := range []string{".gitignore", "CODE_OF_CONDUCT.md", "SECURITY.md"} {
		if _, err := os.Stat(filepath.Join(tc.Dir, dest)); err != nil {
			t.Errorf("expected %s to exist after apply: %v", dest, err)
		}
	}

	// Licence must exist.
	if _, err := os.Stat(filepath.Join(tc.Dir, "LICENSE")); err != nil {
		t.Errorf("expected LICENSE to exist after apply: %v", err)
	}

	// Apply sends the repo settings PATCH.
	found := false
	for _, call := range tc.MutatingCalls() {
		if call.Method == http.MethodPatch && strings.Contains(call.Path, "/repos/") {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected PATCH to repos/{owner}/{repo} in mutating calls")
	}
}

// TestAlterRunApplyFileContentMatchesEmbedded verifies that apply writes
// non-substituted swatches with the exact embedded content.
func TestAlterRunApplyFileContentMatchesEmbedded(t *testing.T) {
	configYAML := `license: none
swatches:
  - path: .gitignore
    alteration: first-fit
  - path: CODE_OF_CONDUCT.md
    alteration: always
  - path: CONTRIBUTING.md
    alteration: always
  - path: SUPPORT.md
    alteration: always
`
	tc := setupAlterTest(t, configYAML)
	// Existing licence suppresses the missing-licence warning.
	writeOnDisk(t, tc.Dir, "LICENSE", []byte("existing"))

	cfg := loadTestConfig(t, tc.Dir)
	_ = captureAlterRun(t, cfg, tc.Dir, alter.Apply, tc.Client)

	for _, src := range []string{".gitignore", "CODE_OF_CONDUCT.md", "CONTRIBUTING.md", "SUPPORT.md"} {
		got, err := os.ReadFile(filepath.Join(tc.Dir, src))
		if err != nil {
			t.Fatalf("reading %s after apply: %v", src, err)
		}
		want := mustContent(t, src)
		if !bytes.Equal(got, want) {
			t.Errorf("%s content does not match embedded swatch (got %d bytes, want %d bytes)", src, len(got), len(want))
		}
	}
}

// TestAlterRunApplyFundingYmlSubstituted verifies that apply writes FUNDING.yml
// with the mock username, not the raw {{GITHUB_USERNAME}} token.
func TestAlterRunApplyFundingYmlSubstituted(t *testing.T) {
	configYAML := `license: none
swatches:
  - path: .github/FUNDING.yml
    alteration: first-fit
`
	tc := setupAlterTest(t, configYAML,
		WithUsername("octocat"),
	)
	writeOnDisk(t, tc.Dir, "LICENSE", []byte("existing"))

	cfg := loadTestConfig(t, tc.Dir)
	_ = captureAlterRun(t, cfg, tc.Dir, alter.Apply, tc.Client)

	data, err := os.ReadFile(filepath.Join(tc.Dir, ".github/FUNDING.yml"))
	if err != nil {
		t.Fatalf("FUNDING.yml not written: %v", err)
	}
	if strings.Contains(string(data), "{{GITHUB_USERNAME}}") {
		t.Error("FUNDING.yml still contains raw {{GITHUB_USERNAME}} token")
	}
	if !strings.Contains(string(data), "octocat") {
		t.Error("FUNDING.yml does not contain substituted username 'octocat'")
	}
}

// TestAlterRunApplySecurityMdSubstituted verifies that apply writes SECURITY.md
// with the constructed advisory URL, not the raw {{ADVISORY_URL}} token.
func TestAlterRunApplySecurityMdSubstituted(t *testing.T) {
	configYAML := `license: none
swatches:
  - path: SECURITY.md
    alteration: always
`
	tc := setupAlterTest(t, configYAML,
		WithRepo("myorg", "myproject"),
	)
	writeOnDisk(t, tc.Dir, "LICENSE", []byte("existing"))

	cfg := loadTestConfig(t, tc.Dir)
	_ = captureAlterRun(t, cfg, tc.Dir, alter.Apply, tc.Client)

	data, err := os.ReadFile(filepath.Join(tc.Dir, "SECURITY.md"))
	if err != nil {
		t.Fatalf("SECURITY.md not written: %v", err)
	}
	if strings.Contains(string(data), "{{ADVISORY_URL}}") {
		t.Error("SECURITY.md still contains raw {{ADVISORY_URL}} token")
	}
	expectedURL := "https://github.com/myorg/myproject/security/advisories/new"
	if !strings.Contains(string(data), expectedURL) {
		t.Errorf("SECURITY.md does not contain expected advisory URL %q", expectedURL)
	}
}

// TestAlterRunApplyCreatesIntermediateDirectories verifies that apply mode
// creates nested directories for swatches like .github/ISSUE_TEMPLATE/bug_report.yml.
func TestAlterRunApplyCreatesIntermediateDirectories(t *testing.T) {
	configYAML := `license: none
swatches:
  - path: .github/ISSUE_TEMPLATE/bug_report.yml
    alteration: always
`
	tc := setupAlterTest(t, configYAML)
	writeOnDisk(t, tc.Dir, "LICENSE", []byte("existing"))

	cfg := loadTestConfig(t, tc.Dir)
	_ = captureAlterRun(t, cfg, tc.Dir, alter.Apply, tc.Client)

	filePath := filepath.Join(tc.Dir, ".github/ISSUE_TEMPLATE/bug_report.yml")
	info, err := os.Stat(filePath)
	if err != nil {
		t.Fatalf("nested file not created: %v", err)
	}
	if info.IsDir() {
		t.Error("expected file, got directory")
	}

	// Parent directories are created with the nested swatch.
	dirPath := filepath.Join(tc.Dir, ".github/ISSUE_TEMPLATE")
	info, err = os.Stat(dirPath)
	if err != nil {
		t.Fatalf("intermediate directory not created: %v", err)
	}
	if !info.IsDir() {
		t.Error("expected directory, got file")
	}
}

// TestAlterRunApplyFirstFitPreservesExisting verifies that first-fit swatch
// files with pre-existing custom content are not overwritten during apply.
func TestAlterRunApplyFirstFitPreservesExisting(t *testing.T) {
	configYAML := `license: none
swatches:
  - path: .gitignore
    alteration: first-fit
  - path: .github/FUNDING.yml
    alteration: first-fit
  - path: CODE_OF_CONDUCT.md
    alteration: always
`
	tc := setupAlterTest(t, configYAML)
	writeOnDisk(t, tc.Dir, "LICENSE", []byte("existing"))

	// Existing first-fit files keep their custom content in apply mode.
	writeOnDisk(t, tc.Dir, ".gitignore", []byte("my custom gitignore"))
	writeOnDisk(t, tc.Dir, ".github/FUNDING.yml", []byte("my custom funding"))

	cfg := loadTestConfig(t, tc.Dir)
	_ = captureAlterRun(t, cfg, tc.Dir, alter.Apply, tc.Client)

	// First-fit files retain their original content.
	data, err := os.ReadFile(filepath.Join(tc.Dir, ".gitignore"))
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "my custom gitignore" {
		t.Errorf(".gitignore was overwritten; got %q", string(data))
	}

	data, err = os.ReadFile(filepath.Join(tc.Dir, ".github/FUNDING.yml"))
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "my custom funding" {
		t.Errorf("FUNDING.yml was overwritten; got %q", string(data))
	}

	// Missing always swatches are written in apply mode.
	if _, err := os.Stat(filepath.Join(tc.Dir, "CODE_OF_CONDUCT.md")); err != nil {
		t.Errorf("CODE_OF_CONDUCT.md should have been written: %v", err)
	}
}

// Non-substituted always swatches use SHA-256 content comparison and are left
// alone when content matches the embedded swatch.
func TestAlterRunApplyAlwaysSwatchNoWriteOnSHA256Match(t *testing.T) {
	configYAML := `license: none
swatches:
  - path: CODE_OF_CONDUCT.md
    alteration: always
`
	tc := setupAlterTest(t, configYAML)
	writeOnDisk(t, tc.Dir, "LICENSE", []byte("existing"))

	// Existing content matches the embedded swatch exactly.
	original := mustContent(t, "CODE_OF_CONDUCT.md")
	writeOnDisk(t, tc.Dir, "CODE_OF_CONDUCT.md", original)

	// Capture modification time before apply.
	infoBefore, err := os.Stat(filepath.Join(tc.Dir, "CODE_OF_CONDUCT.md"))
	if err != nil {
		t.Fatal(err)
	}
	modTimeBefore := infoBefore.ModTime()

	cfg := loadTestConfig(t, tc.Dir)
	_ = captureAlterRun(t, cfg, tc.Dir, alter.Apply, tc.Client)

	// Matching content is preserved.
	data, err := os.ReadFile(filepath.Join(tc.Dir, "CODE_OF_CONDUCT.md"))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(data, original) {
		t.Error("CODE_OF_CONDUCT.md content changed despite SHA-256 match")
	}

	// Matching content is not rewritten.
	infoAfter, err := os.Stat(filepath.Join(tc.Dir, "CODE_OF_CONDUCT.md"))
	if err != nil {
		t.Fatal(err)
	}
	if infoAfter.ModTime() != modTimeBefore {
		t.Error("CODE_OF_CONDUCT.md was re-written despite content matching (modtime changed)")
	}
}

// TestAlterRunApplyLicencePreservesExisting verifies that a pre-existing
// LICENSE file is not overwritten during apply (licence is first-fit).
func TestAlterRunApplyLicencePreservesExisting(t *testing.T) {
	configYAML := `license: mit
swatches:
  - path: .gitignore
    alteration: first-fit
`
	tc := setupAlterTest(t, configYAML,
		WithLicence("mit", "Fresh MIT text"),
	)

	// Existing custom licence is exempt from apply.
	writeOnDisk(t, tc.Dir, "LICENSE", []byte("My Custom Licence"))

	cfg := loadTestConfig(t, tc.Dir)
	_ = captureAlterRun(t, cfg, tc.Dir, alter.Apply, tc.Client)

	data, err := os.ReadFile(filepath.Join(tc.Dir, "LICENSE"))
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "My Custom Licence" {
		t.Errorf("LICENSE was overwritten; got %q", string(data))
	}
}

// TestAlterRunApplyRepoSettingsPatchBody verifies that MutatingCalls() includes
// a PATCH to repos/{owner}/{repo} with the expected JSON body containing the
// settings to change.
func TestAlterRunApplyRepoSettingsPatchBody(t *testing.T) {
	configYAML := `license: none
repository:
  has_wiki: false
  delete_branch_on_merge: true
swatches:
  - path: .gitignore
    alteration: first-fit
`
	tc := setupAlterTest(t, configYAML,
		WithRepoSettings(repoJSON{HasWiki: true, DeleteBranchOnMerge: false}),
	)
	writeOnDisk(t, tc.Dir, "LICENSE", []byte("existing"))

	cfg := loadTestConfig(t, tc.Dir)
	_ = captureAlterRun(t, cfg, tc.Dir, alter.Apply, tc.Client)

	// The PATCH call carries the declared repository settings.
	var patchCall *apiCall
	for _, call := range tc.MutatingCalls() {
		if call.Method == http.MethodPatch && strings.Contains(call.Path, "/repos/testowner/testrepo") {
			patchCall = &call
			break
		}
	}
	if patchCall == nil {
		t.Fatal("no PATCH call to repos/{owner}/{repo} found")
	}

	var body map[string]any
	if err := json.Unmarshal([]byte(patchCall.Body), &body); err != nil {
		t.Fatalf("failed to parse PATCH body as JSON: %v", err)
	}
	if val, ok := body["has_wiki"]; !ok {
		t.Error("PATCH body missing has_wiki field")
	} else if val != false {
		t.Errorf("has_wiki = %v, want false", val)
	}
	if val, ok := body["delete_branch_on_merge"]; !ok {
		t.Error("PATCH body missing delete_branch_on_merge field")
	} else if val != true {
		t.Errorf("delete_branch_on_merge = %v, want true", val)
	}
}

// TestAlterRunRecutOverwritesFirstFitSwatches verifies that recut
// overwrites pre-existing first-fit swatch files with embedded content.
func TestAlterRunRecutOverwritesFirstFitSwatches(t *testing.T) {
	configYAML := `license: none
swatches:
  - path: .gitignore
    alteration: first-fit
  - path: CODE_OF_CONDUCT.md
    alteration: first-fit
`
	tc := setupAlterTest(t, configYAML)
	writeOnDisk(t, tc.Dir, "LICENSE", []byte("existing"))

	// Existing first-fit files are overwritten by recut.
	writeOnDisk(t, tc.Dir, ".gitignore", []byte("custom gitignore"))
	writeOnDisk(t, tc.Dir, "CODE_OF_CONDUCT.md", []byte("custom conduct"))

	cfg := loadTestConfig(t, tc.Dir)
	_ = captureAlterRun(t, cfg, tc.Dir, alter.Recut, tc.Client)

	// Both files contain embedded swatch content, not custom content.
	for _, src := range []string{".gitignore", "CODE_OF_CONDUCT.md"} {
		got, err := os.ReadFile(filepath.Join(tc.Dir, src))
		if err != nil {
			t.Fatalf("reading %s: %v", src, err)
		}
		want := mustContent(t, src)
		if !bytes.Equal(got, want) {
			t.Errorf("%s still contains custom content after recut (got %d bytes, want %d bytes)", src, len(got), len(want))
		}
	}
}

// TestAlterRunRecutDoesNotOverwriteLicence verifies that recut
// does not overwrite an existing LICENSE file (licence is exempt).
func TestAlterRunRecutDoesNotOverwriteLicence(t *testing.T) {
	configYAML := `license: mit
swatches:
  - path: .gitignore
    alteration: first-fit
`
	tc := setupAlterTest(t, configYAML,
		WithLicence("mit", "Fresh MIT text"),
	)

	writeOnDisk(t, tc.Dir, "LICENSE", []byte("My Original Licence"))

	cfg := loadTestConfig(t, tc.Dir)
	_ = captureAlterRun(t, cfg, tc.Dir, alter.Recut, tc.Client)

	data, err := os.ReadFile(filepath.Join(tc.Dir, "LICENSE"))
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "My Original Licence" {
		t.Errorf("recut overwrote LICENSE; got %q", string(data))
	}
}

// TestAlterRunRecutMergesConfigAndOverwrites verifies that recut with a
// first-fit .tailor.yml entry merges missing default swatches, then rewrites
// .tailor.yml through the config write step.
func TestAlterRunRecutMergesConfigAndOverwrites(t *testing.T) {
	configYAML := `license: none
swatches:
  - path: .tailor.yml
    alteration: first-fit
`
	tc := setupAlterTest(t, configYAML)
	writeOnDisk(t, tc.Dir, "LICENSE", []byte("existing"))

	cfg := loadTestConfig(t, tc.Dir)
	_ = captureAlterRun(t, cfg, tc.Dir, alter.Recut, tc.Client)

	data, err := os.ReadFile(filepath.Join(tc.Dir, ".tailor.yml"))
	if err != nil {
		t.Fatal(err)
	}
	// After recut, the merge step rewrites .tailor.yml.
	if len(data) == 0 {
		t.Error(".tailor.yml is empty after recut")
	}
}

// TestAlterRunRecutResolvesTokens verifies that recut runs full
// token resolution on substituted swatches. Pre-writes FUNDING.yml with stale
// content, then checks it contains the freshly resolved username.
func TestAlterRunRecutResolvesTokens(t *testing.T) {
	configYAML := `license: none
swatches:
  - path: .github/FUNDING.yml
    alteration: first-fit
`
	tc := setupAlterTest(t, configYAML,
		WithUsername("freshuser"),
	)
	writeOnDisk(t, tc.Dir, "LICENSE", []byte("existing"))

	// Existing stale funding content is replaced with freshly resolved content.
	writeOnDisk(t, tc.Dir, ".github/FUNDING.yml", []byte("github: staleuser"))

	cfg := loadTestConfig(t, tc.Dir)
	_ = captureAlterRun(t, cfg, tc.Dir, alter.Recut, tc.Client)

	data, err := os.ReadFile(filepath.Join(tc.Dir, ".github/FUNDING.yml"))
	if err != nil {
		t.Fatalf("FUNDING.yml not found: %v", err)
	}
	if strings.Contains(string(data), "staleuser") {
		t.Error("FUNDING.yml still contains stale username after recut")
	}
	if !strings.Contains(string(data), "freshuser") {
		t.Error("FUNDING.yml does not contain freshly resolved username 'freshuser'")
	}
	if strings.Contains(string(data), "{{GITHUB_USERNAME}}") {
		t.Error("FUNDING.yml contains raw {{GITHUB_USERNAME}} token")
	}
}

// TestAlterRunErrorUnrecognisedSwatchSource verifies that a config with an
// unknown swatch source produces an error mentioning valid sources.
func TestAlterRunErrorUnrecognisedSwatchSource(t *testing.T) {
	configYAML := `license: none
swatches:
  - path: nonexistent.txt
    alteration: first-fit
`
	tc := setupAlterTest(t, configYAML)
	writeOnDisk(t, tc.Dir, "LICENSE", []byte("existing"))

	cfg := loadTestConfig(t, tc.Dir)
	err := runAlterExpectError(t, cfg, tc.Dir, tc.Client)

	if !strings.Contains(err.Error(), "unrecognised swatch path") {
		t.Errorf("error = %q, want substring 'unrecognised swatch path'", err)
	}
	if !strings.Contains(err.Error(), ".gitignore") {
		t.Errorf("error should list valid paths, got: %q", err)
	}
}

// TestAlterRunErrorDuplicateDestination verifies that two swatches targeting
// the same destination produce an error.
func TestAlterRunErrorDuplicateDestination(t *testing.T) {
	configYAML := `license: none
swatches:
  - path: .gitignore
    alteration: first-fit
  - path: .gitignore
    alteration: always
`
	tc := setupAlterTest(t, configYAML)
	writeOnDisk(t, tc.Dir, "LICENSE", []byte("existing"))

	cfg := loadTestConfig(t, tc.Dir)
	err := runAlterExpectError(t, cfg, tc.Dir, tc.Client)

	if !strings.Contains(err.Error(), "duplicate swatch path") {
		t.Errorf("error = %q, want substring 'duplicate swatch path'", err)
	}
}

// TestAlterRunErrorUnrecognisedRepoSetting verifies that an unknown key in the
// repository section produces an error listing valid settings.
func TestAlterRunErrorUnrecognisedRepoSetting(t *testing.T) {
	configYAML := `license: none
repository:
  unknown_setting: true
swatches:
  - path: .gitignore
    alteration: first-fit
`
	tc := setupAlterTest(t, configYAML)
	writeOnDisk(t, tc.Dir, "LICENSE", []byte("existing"))

	_, err := config.Load(tc.Dir)
	if err == nil {
		t.Fatal("expected error from config.Load with unrecognised repo setting, got nil")
	}
	if !strings.Contains(err.Error(), "unrecognised repository setting") {
		t.Errorf("error = %q, want substring 'unrecognised repository setting'", err)
	}
}

// TestAlterRunErrorMissingConfigFile verifies that config.Load returns an
// error when .tailor.yml does not exist. This error is caught by the
// CLI layer (cmd/tailor), not by alter.Run which receives a pre-loaded config.
func TestAlterRunErrorMissingConfigFile(t *testing.T) {
	dir := t.TempDir()
	_, err := config.Load(dir)
	if err == nil {
		t.Fatal("expected error from config.Load with missing config, got nil")
	}
	if !strings.Contains(err.Error(), "reading config") {
		t.Errorf("error = %q, want substring 'reading config'", err)
	}
}

// TestAlterRunErrorLicenceFetchFailure verifies that a 404 from the licence
// API propagates as an error from alter.Run.
func TestAlterRunErrorLicenceFetchFailure(t *testing.T) {
	configYAML := `license: bad-id
swatches:
  - path: .gitignore
    alteration: first-fit
`
	tc := setupAlterTest(t, configYAML,
		WithLicenceError(http.StatusNotFound),
	)

	cfg := loadTestConfig(t, tc.Dir)
	err := runAlterExpectError(t, cfg, tc.Dir, tc.Client)

	if !strings.Contains(err.Error(), "licence") && !strings.Contains(err.Error(), "license") {
		t.Errorf("error = %q, want substring containing 'licence' or 'license'", err)
	}
}

// TestAlterRunErrorGetUserFailure verifies that a 401 from GET /user
// propagates as an error. No files are written and no PATCH calls are made.
func TestAlterRunErrorGetUserFailure(t *testing.T) {
	configYAML := `license: none
swatches:
  - path: .gitignore
    alteration: first-fit
`
	tc := setupAlterTest(t, configYAML,
		WithUserError(http.StatusUnauthorized),
	)
	writeOnDisk(t, tc.Dir, "LICENSE", []byte("existing"))

	cfg := loadTestConfig(t, tc.Dir)
	err := runAlterExpectError(t, cfg, tc.Dir, tc.Client)

	if !strings.Contains(err.Error(), "username") && !strings.Contains(err.Error(), "user") {
		t.Errorf("error = %q, want substring containing 'username' or 'user'", err)
	}

	// No swatch files are written.
	if _, statErr := os.Stat(filepath.Join(tc.Dir, ".gitignore")); statErr == nil {
		t.Error(".gitignore was written despite GET /user failure")
	}

	// No mutating API calls are made.
	if mc := tc.MutatingCalls(); len(mc) != 0 {
		t.Errorf("expected no mutating calls, got %d: %v", len(mc), mc)
	}
}

// TestAlterRunErrorPatchFailure verifies that a 500 from PATCH on repo
// settings propagates as an error. No swatch files are written because
// repo settings are processed before swatches.
func TestAlterRunErrorPatchFailure(t *testing.T) {
	configYAML := `license: none
repository:
  has_wiki: false
swatches:
  - path: .gitignore
    alteration: first-fit
`
	tc := setupAlterTest(t, configYAML,
		WithRepoSettings(repoJSON{HasWiki: true}),
		WithPatchError(http.StatusInternalServerError),
	)
	writeOnDisk(t, tc.Dir, "LICENSE", []byte("existing"))

	cfg := loadTestConfig(t, tc.Dir)
	err := runAlterExpectError(t, cfg, tc.Dir, tc.Client)

	if err == nil {
		t.Fatal("expected error from PATCH failure")
	}

	// No swatch files are written because repo settings fail first.
	if _, statErr := os.Stat(filepath.Join(tc.Dir, ".gitignore")); statErr == nil {
		t.Error(".gitignore was written despite PATCH failure")
	}
}

// TestAlterRunPatch403GracefulDegradation verifies that a 403 from PATCH on
// repo settings is gracefully degraded (skipped), and swatches still proceed.
func TestAlterRunPatch403GracefulDegradation(t *testing.T) {
	configYAML := `license: none
repository:
  has_wiki: false
swatches:
  - path: .gitignore
    alteration: first-fit
`
	tc := setupAlterTest(t, configYAML,
		WithRepoSettings(repoJSON{HasWiki: true}),
		WithPatchError(http.StatusForbidden),
	)
	writeOnDisk(t, tc.Dir, "LICENSE", []byte("existing"))

	cfg := loadTestConfig(t, tc.Dir)
	_ = captureAlterRun(t, cfg, tc.Dir, alter.Apply, tc.Client)

	// Swatch files are written despite the PATCH 403.
	if _, statErr := os.Stat(filepath.Join(tc.Dir, ".gitignore")); statErr != nil {
		t.Error(".gitignore was not written despite graceful PATCH degradation")
	}
}

// TestAlterRunNoRepoContextWarning verifies that when no repo context exists
// and the config has a repository section, a warning is emitted on stderr but
// swatches are still processed.
func TestAlterRunNoRepoContextWarning(t *testing.T) {
	configYAML := `license: none
repository:
  has_wiki: false
swatches:
  - path: .gitignore
    alteration: first-fit
`
	tc := setupAlterTest(t, configYAML, WithNoRepo())
	writeOnDisk(t, tc.Dir, "LICENSE", []byte("existing"))

	cfg := loadTestConfig(t, tc.Dir)
	_, stderr, err := captureAlterRunWithStderr(t, cfg, tc.Dir, alter.Apply, tc.Client)
	if err != nil {
		t.Fatalf("alter.Run() returned unexpected error: %v", err)
	}

	// Stderr contains the repo context warning.
	if !strings.Contains(stderr, "No GitHub repository context found") {
		t.Errorf("expected stderr warning about no repo context, got: %q", stderr)
	}

	// Swatches still run, so .gitignore is written.
	if _, statErr := os.Stat(filepath.Join(tc.Dir, ".gitignore")); statErr != nil {
		t.Errorf(".gitignore should have been written despite no repo context: %v", statErr)
	}
}

// TestAlterRunNoRepoContextLeavesTokensUnsubstituted verifies that without
// repo context, tokens depending on owner/repo remain as raw placeholders.
func TestAlterRunNoRepoContextLeavesTokensUnsubstituted(t *testing.T) {
	configYAML := `license: none
swatches:
  - path: SECURITY.md
    alteration: always
  - path: .github/ISSUE_TEMPLATE/config.yml
    alteration: always
`
	tc := setupAlterTest(t, configYAML, WithNoRepo())
	writeOnDisk(t, tc.Dir, "LICENSE", []byte("existing"))

	cfg := loadTestConfig(t, tc.Dir)
	_, _, err := captureAlterRunWithStderr(t, cfg, tc.Dir, alter.Apply, tc.Client)
	if err != nil {
		t.Fatalf("alter.Run() error: %v", err)
	}

	// SECURITY.md should contain the raw {{ADVISORY_URL}} token.
	secData, err := os.ReadFile(filepath.Join(tc.Dir, "SECURITY.md"))
	if err != nil {
		t.Fatalf("SECURITY.md not written: %v", err)
	}
	if !strings.Contains(string(secData), "{{ADVISORY_URL}}") {
		t.Error("SECURITY.md does not contain raw {{ADVISORY_URL}} token; expected unsubstituted")
	}

	// .github/ISSUE_TEMPLATE/config.yml should contain the raw {{SUPPORT_URL}} token.
	issueData, err := os.ReadFile(filepath.Join(tc.Dir, ".github/ISSUE_TEMPLATE/config.yml"))
	if err != nil {
		t.Fatalf(".github/ISSUE_TEMPLATE/config.yml not written: %v", err)
	}
	if !strings.Contains(string(issueData), "{{SUPPORT_URL}}") {
		t.Error(".github/ISSUE_TEMPLATE/config.yml does not contain raw {{SUPPORT_URL}} token; expected unsubstituted")
	}
}

// allNonConfigSwatchesYAML returns a YAML swatches block containing every
// registered swatch except .tailor.yml, using each swatch's default
// alteration mode.
func allNonConfigSwatchesYAML() string {
	var sb strings.Builder
	for _, s := range swatch.All() {
		if s.Path == ".tailor.yml" {
			continue
		}
		fmt.Fprintf(&sb, "  - path: %s\n    alteration: %s\n", s.Path, s.DefaultAlteration)
	}
	return sb.String()
}

// allDefaultRepoSettingsYAML returns YAML for all default repo settings fields
// (excluding description, homepage, topics which are skipped during merge).
func allDefaultRepoSettingsYAML(t *testing.T) string {
	t.Helper()
	defaults, err := config.DefaultConfig("none")
	if err != nil {
		t.Fatalf("loading default config: %v", err)
	}

	var sb strings.Builder
	sb.WriteString("repository:\n")
	r := defaults.Repository
	if r.HasWiki != nil {
		fmt.Fprintf(&sb, "  has_wiki: %t\n", *r.HasWiki)
	}
	if r.HasDiscussions != nil {
		fmt.Fprintf(&sb, "  has_discussions: %t\n", *r.HasDiscussions)
	}
	if r.HasProjects != nil {
		fmt.Fprintf(&sb, "  has_projects: %t\n", *r.HasProjects)
	}
	if r.HasIssues != nil {
		fmt.Fprintf(&sb, "  has_issues: %t\n", *r.HasIssues)
	}
	if r.AllowMergeCommit != nil {
		fmt.Fprintf(&sb, "  allow_merge_commit: %t\n", *r.AllowMergeCommit)
	}
	if r.AllowSquashMerge != nil {
		fmt.Fprintf(&sb, "  allow_squash_merge: %t\n", *r.AllowSquashMerge)
	}
	if r.AllowRebaseMerge != nil {
		fmt.Fprintf(&sb, "  allow_rebase_merge: %t\n", *r.AllowRebaseMerge)
	}
	if r.SquashMergeCommitTitle != nil {
		fmt.Fprintf(&sb, "  squash_merge_commit_title: %s\n", *r.SquashMergeCommitTitle)
	}
	if r.SquashMergeCommitMessage != nil {
		fmt.Fprintf(&sb, "  squash_merge_commit_message: %s\n", *r.SquashMergeCommitMessage)
	}
	if r.MergeCommitTitle != nil {
		fmt.Fprintf(&sb, "  merge_commit_title: %s\n", *r.MergeCommitTitle)
	}
	if r.MergeCommitMessage != nil {
		fmt.Fprintf(&sb, "  merge_commit_message: %s\n", *r.MergeCommitMessage)
	}
	if r.DeleteBranchOnMerge != nil {
		fmt.Fprintf(&sb, "  delete_branch_on_merge: %t\n", *r.DeleteBranchOnMerge)
	}
	if r.AllowUpdateBranch != nil {
		fmt.Fprintf(&sb, "  allow_update_branch: %t\n", *r.AllowUpdateBranch)
	}
	if r.AllowAutoMerge != nil {
		fmt.Fprintf(&sb, "  allow_auto_merge: %t\n", *r.AllowAutoMerge)
	}
	if r.WebCommitSignoffRequired != nil {
		fmt.Fprintf(&sb, "  web_commit_signoff_required: %t\n", *r.WebCommitSignoffRequired)
	}
	if r.PrivateVulnerabilityReportEnabled != nil {
		fmt.Fprintf(&sb, "  private_vulnerability_reporting_enabled: %t\n", *r.PrivateVulnerabilityReportEnabled)
	}
	if r.VulnerabilityAlertsEnabled != nil {
		fmt.Fprintf(&sb, "  vulnerability_alerts_enabled: %t\n", *r.VulnerabilityAlertsEnabled)
	}
	if r.AutomatedSecurityFixesEnabled != nil {
		fmt.Fprintf(&sb, "  automated_security_fixes_enabled: %t\n", *r.AutomatedSecurityFixesEnabled)
	}
	if r.DefaultWorkflowPermissions != nil {
		fmt.Fprintf(&sb, "  default_workflow_permissions: %s\n", *r.DefaultWorkflowPermissions)
	}
	if r.CanApprovePullRequestReviews != nil {
		fmt.Fprintf(&sb, "  can_approve_pull_request_reviews: %t\n", *r.CanApprovePullRequestReviews)
	}
	return sb.String()
}

func allDefaultActionsYAML(t *testing.T) string {
	t.Helper()
	defaults, err := config.DefaultConfig("none")
	if err != nil {
		t.Fatalf("loading default config: %v", err)
	}
	a := defaults.Actions
	if a == nil {
		t.Fatal("default Actions policy is nil")
	}

	var sb strings.Builder
	sb.WriteString("actions:\n")
	fmt.Fprintf(&sb, "  enabled: %t\n", *a.Enabled)
	fmt.Fprintf(&sb, "  allowed_actions: %s\n", *a.AllowedActions)
	fmt.Fprintf(&sb, "  sha_pinning_required: %t\n", *a.SHAPinningRequired)
	fmt.Fprintf(&sb, "  github_owned_allowed: %t\n", *a.GitHubOwnedAllowed)
	fmt.Fprintf(&sb, "  verified_allowed: %t\n", *a.VerifiedAllowed)
	writePatternsAllowedYAML(&sb, *a.PatternsAllowed)
	return sb.String()
}

func writePatternsAllowedYAML(sb *strings.Builder, patterns []string) {
	sb.WriteString("  patterns_allowed:")
	if len(patterns) == 0 {
		sb.WriteString(" []\n")
		return
	}
	sb.WriteByte('\n')
	for _, pattern := range patterns {
		fmt.Fprintf(sb, "    - %q\n", pattern)
	}
}

// allDefaultLabelsYAML returns YAML for all default labels.
func allDefaultLabelsYAML(t *testing.T) string {
	t.Helper()
	defaults, err := config.DefaultConfig("none")
	if err != nil {
		t.Fatalf("loading default config: %v", err)
	}

	var sb strings.Builder
	sb.WriteString("labels:\n")
	for _, l := range defaults.Labels {
		fmt.Fprintf(&sb, "  - name: %s\n    color: %q\n    description: %q\n", l.Name, l.Color, l.Description)
	}
	return sb.String()
}

// TestConfigMergeMissingSwatchesApply verifies that Apply mode with a config
// missing two swatches merges them into cfg.Swatches so they are processed.
// The merge step rewrites the config file on disk with a "Refitted" header
// (swatch processing skips the .tailor.yml entry). The net observable effect:
// the two previously missing swatch files appear on disk.
func TestConfigMergeMissingSwatchesApply(t *testing.T) {
	// Config includes the .tailor.yml swatch (always) but omits SUPPORT.md and justfile.
	configYAML := "license: none\nswatches:\n" +
		"  - path: .tailor.yml\n    alteration: always\n"
	for _, s := range swatch.All() {
		if s.Path == ".tailor.yml" || s.Path == "SUPPORT.md" || s.Path == "justfile" {
			continue
		}
		configYAML += fmt.Sprintf("  - path: %s\n    alteration: %s\n", s.Path, s.DefaultAlteration)
	}

	tc := setupAlterTest(t, configYAML)
	writeOnDisk(t, tc.Dir, "LICENSE", []byte("existing"))

	cfg := loadTestConfig(t, tc.Dir)
	output := captureAlterRun(t, cfg, tc.Dir, alter.Apply, tc.Client)
	requireContains(t, output, "updated:                                                       .tailor.yml\n")

	// MergeDefaultSwatches appends the missing entries before swatch processing.
	for _, dest := range []string{"SUPPORT.md", "justfile"} {
		if _, err := os.Stat(filepath.Join(tc.Dir, dest)); err != nil {
			t.Errorf("expected %s to exist after apply (merged swatch): %v", dest, err)
		}
	}

	// The merge step writes the config file.
	data, err := os.ReadFile(filepath.Join(tc.Dir, ".tailor.yml"))
	if err != nil {
		t.Fatalf(".tailor.yml not found after apply: %v", err)
	}
	if len(data) == 0 {
		t.Error(".tailor.yml is empty after apply")
	}
}

// TestConfigMergeAllPresentApply verifies that Apply mode with all swatches,
// repo settings, and labels already present does not trigger a merge rewrite.
// The merge step finds no missing entries, so config.Write is not called and
// .tailor.yml stays untouched (swatch processing skips the config entry).
func TestConfigMergeAllPresentApply(t *testing.T) {
	configYAML := "license: none\n" +
		allDefaultRepoSettingsYAML(t) +
		allDefaultActionsYAML(t) +
		allDefaultLabelsYAML(t) +
		"swatches:\n" +
		"  - path: .tailor.yml\n    alteration: always\n" +
		allNonConfigSwatchesYAML()

	tc := setupAlterTest(t, configYAML)
	writeOnDisk(t, tc.Dir, "LICENSE", []byte("existing"))

	cfgPath := filepath.Join(tc.Dir, ".tailor.yml")

	cfg := loadTestConfig(t, tc.Dir)
	captureAlterRun(t, cfg, tc.Dir, alter.Apply, tc.Client)

	afterData, err := os.ReadFile(cfgPath)
	if err != nil {
		t.Fatalf("reading config after apply: %v", err)
	}

	// The merge step added zero entries, so config.Write does not run and
	// swatch processing skips the .tailor.yml entry. The file keeps its
	// original content, without a "Refitted" header.
	if strings.Contains(string(afterData), "Refitted by tailor on") {
		t.Error(".tailor.yml contains 'Refitted' header despite no entries being merged")
	}
}

// TestConfigMergeFirstFitApplySkips verifies that when the .tailor.yml swatch
// entry has alteration: first-fit, Apply mode does not rewrite the config.
func TestConfigMergeFirstFitApplySkips(t *testing.T) {
	// Config with first-fit for .tailor.yml, missing SUPPORT.md.
	configYAML := "license: none\nswatches:\n" +
		"  - path: .tailor.yml\n    alteration: first-fit\n" +
		"  - path: .gitignore\n    alteration: first-fit\n"

	tc := setupAlterTest(t, configYAML)
	writeOnDisk(t, tc.Dir, "LICENSE", []byte("existing"))

	originalData, err := os.ReadFile(filepath.Join(tc.Dir, ".tailor.yml"))
	if err != nil {
		t.Fatalf("reading original config: %v", err)
	}

	cfg := loadTestConfig(t, tc.Dir)
	_ = captureAlterRun(t, cfg, tc.Dir, alter.Apply, tc.Client)

	afterData, err := os.ReadFile(filepath.Join(tc.Dir, ".tailor.yml"))
	if err != nil {
		t.Fatalf("reading config after apply: %v", err)
	}

	if !bytes.Equal(originalData, afterData) {
		t.Error(".tailor.yml was rewritten despite first-fit alteration in Apply mode")
	}

	// First-fit config mode skips the merge, so SUPPORT.md remains absent.
	if strings.Contains(string(afterData), "SUPPORT.md") {
		t.Error(".tailor.yml contains SUPPORT.md despite first-fit skipping merge")
	}
}

// TestConfigMergeFirstFitRecutAppends verifies that Recut mode with
// first-fit .tailor.yml overrides to always semantics, appending missing
// entries. The merged swatch files must appear on disk after processing.
func TestConfigMergeFirstFitRecutAppends(t *testing.T) {
	// A first-fit config swatch is promoted to merge behaviour during recut.
	configYAML := "license: none\nswatches:\n" +
		"  - path: .tailor.yml\n    alteration: first-fit\n" +
		"  - path: .gitignore\n    alteration: first-fit\n"

	tc := setupAlterTest(t, configYAML)
	writeOnDisk(t, tc.Dir, "LICENSE", []byte("existing"))

	cfg := loadTestConfig(t, tc.Dir)
	output := captureAlterRun(t, cfg, tc.Dir, alter.Recut, tc.Client)
	requireContains(t, output, "updated:                                                       .tailor.yml\n")

	// Recut merges defaults, then processes the newly merged swatches.
	for _, dest := range []string{"SUPPORT.md", "CONTRIBUTING.md", "CODE_OF_CONDUCT.md", "SECURITY.md"} {
		if _, err := os.Stat(filepath.Join(tc.Dir, dest)); err != nil {
			t.Errorf("expected %s to exist after recut (merged swatch): %v", dest, err)
		}
	}

	// Recut rewrites .tailor.yml through the config write step.
	data, err := os.ReadFile(filepath.Join(tc.Dir, ".tailor.yml"))
	if err != nil {
		t.Fatalf(".tailor.yml not found after recut: %v", err)
	}
	if len(data) == 0 {
		t.Error(".tailor.yml is empty after recut")
	}
}

// TestConfigMergeDryRunNoRewrite verifies that DryRun mode with missing
// entries does not rewrite the config file on disk.
func TestConfigMergeDryRunNoRewrite(t *testing.T) {
	// Dry-run can detect missing entries without rewriting .tailor.yml.
	configYAML := "license: none\nswatches:\n" +
		"  - path: .tailor.yml\n    alteration: always\n" +
		"  - path: .gitignore\n    alteration: first-fit\n"

	tc := setupAlterTest(t, configYAML)
	writeOnDisk(t, tc.Dir, "LICENSE", []byte("existing"))

	originalData, err := os.ReadFile(filepath.Join(tc.Dir, ".tailor.yml"))
	if err != nil {
		t.Fatalf("reading original config: %v", err)
	}

	cfg := loadTestConfig(t, tc.Dir)
	output := captureAlterRun(t, cfg, tc.Dir, alter.DryRun, tc.Client)
	requireContains(t, output, "would update:                                                  .tailor.yml\n")

	afterData, err := os.ReadFile(filepath.Join(tc.Dir, ".tailor.yml"))
	if err != nil {
		t.Fatalf("reading config after dry-run: %v", err)
	}

	if !bytes.Equal(originalData, afterData) {
		t.Error(".tailor.yml was rewritten during dry-run despite ShouldWrite() being false")
	}
	if calls := tc.MutatingCalls(); len(calls) != 0 {
		t.Fatalf("dry run made mutating calls: %v", calls)
	}
}

// TestAlterRunMergeDefaults verifies that missing settings and sections receive
// defaults, the rewritten config validates, and Actions defaults apply at once.
func TestAlterRunMergeDefaults(t *testing.T) {
	// Config has the .tailor.yml swatch set to always (triggers shouldMerge),
	// a partial repository section (only has_wiki), and no labels.
	configYAML := `license: none
repository:
  has_wiki: false
swatches:
  - path: .tailor.yml
    alteration: always
  - path: .gitignore
    alteration: first-fit
`
	tc := setupAlterTest(t, configYAML)
	writeOnDisk(t, tc.Dir, "LICENSE", []byte("existing"))

	cfg := loadTestConfig(t, tc.Dir)
	_ = captureAlterRun(t, cfg, tc.Dir, alter.Apply, tc.Client)

	// Read the rewritten config file.
	data, err := os.ReadFile(filepath.Join(tc.Dir, ".tailor.yml"))
	if err != nil {
		t.Fatalf("reading config after merge: %v", err)
	}
	content := string(data)

	// Repo settings fields are merged from defaults.
	for _, field := range []string{
		"has_issues:",
		"allow_squash_merge:",
		"delete_branch_on_merge:",
		"allow_auto_merge:",
		"private_vulnerability_reporting_enabled: true",
		"vulnerability_alerts_enabled: true",
		"automated_security_fixes_enabled: true",
		"default_workflow_permissions:",
	} {
		if !strings.Contains(content, field) {
			t.Errorf("merged config missing default repo setting %q", field)
		}
	}

	// Existing has_wiki value is preserved instead of replaced by defaults.
	if !strings.Contains(content, "has_wiki: false") {
		t.Errorf("has_wiki was overwritten; expected 'has_wiki: false' to be preserved")
	}

	// Missing labels are merged from defaults.
	if !strings.Contains(content, "labels:") {
		t.Error("merged config missing labels section")
	}
	for _, label := range []string{"bug", "enhancement", "documentation"} {
		if !strings.Contains(content, label) {
			t.Errorf("merged config missing default label %q", label)
		}
	}

	for _, field := range []string{
		"actions:\n",
		"  enabled: true\n",
		"  allowed_actions: selected\n",
		"  sha_pinning_required: false\n",
		"  github_owned_allowed: true\n",
		"  verified_allowed: true\n",
		"  patterns_allowed:\n",
		"    - \"freerangebytes/setup-actionlint@*\"\n",
		"    - \"golang/govulncheck-action@*\"\n",
		"    - \"golangci/golangci-lint-action@*\"\n",
		"    - \"nick-fields/retry@*\"\n",
		"    - \"robherley/go-test-action@*\"\n",
		"    - \"softprops/action-gh-release@*\"\n",
	} {
		if !strings.Contains(content, field) {
			t.Errorf("merged config missing Actions default %q", field)
		}
	}
	if _, err := config.Load(tc.Dir); err != nil {
		t.Fatalf("loading merged config: %v", err)
	}

	writes := map[string]bool{}
	for _, call := range tc.MutatingCalls() {
		if strings.Contains(call.Path, "/actions/permissions") {
			writes[call.Path] = true
		}
	}
	for _, path := range []string{
		"/repos/testowner/testrepo/actions/permissions",
		"/repos/testowner/testrepo/actions/permissions/selected-actions",
	} {
		if !writes[path] {
			t.Errorf("missing same-run Actions write to %s", path)
		}
	}
}

func TestAlterRunCompletesPartialSelectedActionsBeforeProcessing(t *testing.T) {
	configYAML := `license: none
actions:
  allowed_actions: selected
swatches:
  - path: .tailor.yml
    alteration: always
`
	tc := setupAlterTest(t, configYAML)
	cfg := loadTestConfig(t, tc.Dir)
	_ = captureAlterRun(t, cfg, tc.Dir, alter.Apply, tc.Client)
	if cfg.Actions.GitHubOwnedAllowed == nil || !*cfg.Actions.GitHubOwnedAllowed {
		t.Fatalf("merged github_owned_allowed = %v, want true", cfg.Actions.GitHubOwnedAllowed)
	}

	written, err := os.ReadFile(filepath.Join(tc.Dir, ".tailor.yml"))
	if err != nil {
		t.Fatal(err)
	}
	content := string(written)
	for _, field := range []string{
		"  enabled: true\n",
		"  allowed_actions: selected\n",
		"  sha_pinning_required: false\n",
		"  github_owned_allowed: true\n",
		"  verified_allowed: true\n",
		"  patterns_allowed:\n",
		"    - \"freerangebytes/setup-actionlint@*\"\n",
		"    - \"golang/govulncheck-action@*\"\n",
		"    - \"golangci/golangci-lint-action@*\"\n",
		"    - \"nick-fields/retry@*\"\n",
		"    - \"robherley/go-test-action@*\"\n",
		"    - \"softprops/action-gh-release@*\"\n",
	} {
		if !strings.Contains(content, field) {
			t.Errorf("completed config missing %q", field)
		}
	}
	merged, err := config.Load(tc.Dir)
	if err != nil {
		t.Fatalf("loading completed config: %v", err)
	}
	if err := config.ValidateCompleteActions(merged); err != nil {
		t.Fatalf("completed config validation: %v", err)
	}

	var actionsWrites []apiCall
	for _, call := range tc.MutatingCalls() {
		if call.Path == "/repos/testowner/testrepo/actions/permissions" ||
			call.Path == "/repos/testowner/testrepo/actions/permissions/selected-actions" {
			actionsWrites = append(actionsWrites, call)
		}
	}
	if len(actionsWrites) != 3 {
		t.Fatalf("Actions writes = %v, want disable, selected, and final core writes", actionsWrites)
	}
	if actionsWrites[0].Path != "/repos/testowner/testrepo/actions/permissions" ||
		actionsWrites[1].Path != "/repos/testowner/testrepo/actions/permissions/selected-actions" ||
		actionsWrites[2].Path != "/repos/testowner/testrepo/actions/permissions" {
		t.Fatalf("Actions write order = %v, want disable, selected, then final core", actionsWrites)
	}
	var disableBody map[string]any
	if err := json.Unmarshal([]byte(actionsWrites[0].Body), &disableBody); err != nil {
		t.Fatal(err)
	}
	if len(disableBody) != 1 || disableBody["enabled"] != false {
		t.Fatalf("disable body = %v, want enabled false", disableBody)
	}

	var selectedBody map[string]any
	if err := json.Unmarshal([]byte(actionsWrites[1].Body), &selectedBody); err != nil {
		t.Fatal(err)
	}
	if len(selectedBody) != 3 || selectedBody["github_owned_allowed"] != true ||
		selectedBody["verified_allowed"] != true {
		t.Fatalf("selected body = %v, want complete merged policy", selectedBody)
	}
	patterns, ok := selectedBody["patterns_allowed"].([]any)
	if !ok || len(patterns) != len(approvedDefaultActionPatterns) {
		t.Fatalf("patterns_allowed = %v, want %v", selectedBody["patterns_allowed"], approvedDefaultActionPatterns)
	}
	for i, want := range approvedDefaultActionPatterns {
		if patterns[i] != want {
			t.Fatalf("patterns_allowed = %v, want %v", selectedBody["patterns_allowed"], approvedDefaultActionPatterns)
		}
	}

	var coreBody map[string]any
	if err := json.Unmarshal([]byte(actionsWrites[2].Body), &coreBody); err != nil {
		t.Fatal(err)
	}
	if coreBody["enabled"] != true || coreBody["allowed_actions"] != "selected" ||
		coreBody["sha_pinning_required"] != false {
		t.Fatalf("core body = %v, want merged policy in the same run", coreBody)
	}
}

func TestAlterRunBasteMergedActionsDefaultsIsReadOnly(t *testing.T) {
	configYAML := `license: none
swatches:
  - path: .tailor.yml
    alteration: always
`
	tc := setupAlterTest(t, configYAML)
	writeOnDisk(t, tc.Dir, "LICENSE", []byte("existing"))
	before, err := os.ReadFile(filepath.Join(tc.Dir, ".tailor.yml"))
	if err != nil {
		t.Fatal(err)
	}

	cfg := loadTestConfig(t, tc.Dir)
	output := captureAlterRun(t, cfg, tc.Dir, alter.DryRun, tc.Client)
	after, err := os.ReadFile(filepath.Join(tc.Dir, ".tailor.yml"))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(after, before) {
		t.Fatalf("baste changed .tailor.yml:\n%s", after)
	}
	if calls := tc.MutatingCalls(); len(calls) != 0 {
		t.Fatalf("baste made mutating calls: %v", calls)
	}

	getCounts := map[string]int{}
	for _, call := range tc.Calls() {
		if call.Method == http.MethodGet {
			getCounts[call.Path]++
		}
	}
	for _, path := range []string{
		"/repos/testowner/testrepo/actions/permissions",
		"/repos/testowner/testrepo/actions/permissions/selected-actions",
	} {
		if getCounts[path] != 1 {
			t.Errorf("GET %s count = %d, want 1", path, getCounts[path])
		}
	}
	wantPatterns := "actions.patterns_allowed = " + strings.Join(approvedDefaultActionPatterns, ", ")
	if !strings.Contains(output, wantPatterns) {
		t.Fatalf("output does not show the approved default patterns:\n%s", output)
	}
}

func TestAlterRunMergeNonSelectedActionsPolicy(t *testing.T) {
	configYAML := `license: none
actions:
  enabled: false
  allowed_actions: all
swatches:
  - path: .tailor.yml
    alteration: always
`
	tc := setupAlterTest(t, configYAML)
	writeOnDisk(t, tc.Dir, "LICENSE", []byte("existing"))

	cfg := loadTestConfig(t, tc.Dir)
	_ = captureAlterRun(t, cfg, tc.Dir, alter.Apply, tc.Client)

	written, err := os.ReadFile(filepath.Join(tc.Dir, ".tailor.yml"))
	if err != nil {
		t.Fatal(err)
	}
	content := string(written)
	if !strings.Contains(content, "  enabled: false\n") || !strings.Contains(content, "  allowed_actions: all\n") || !strings.Contains(content, "  sha_pinning_required: false\n") {
		t.Fatalf("merged core Actions policy is incomplete:\n%s", content)
	}
	for _, field := range []string{"github_owned_allowed:", "verified_allowed:", "patterns_allowed:"} {
		if strings.Contains(content, field) {
			t.Errorf("merged non-selected policy contains %s", field)
		}
	}
	if _, err := config.Load(tc.Dir); err != nil {
		t.Fatalf("loading merged non-selected config: %v", err)
	}
	for _, call := range tc.Calls() {
		if strings.HasSuffix(call.Path, "/actions/permissions/selected-actions") {
			t.Errorf("non-selected policy called selected-actions endpoint: %v", call)
		}
	}
}

// TestAlterRunMergeCompleteConfigNotRewritten verifies that a config already
// containing all default repo settings and labels is not rewritten.
func TestAlterRunMergeCompleteConfigNotRewritten(t *testing.T) {
	// Embedded defaults provide the complete config fixture.
	defaults, err := config.DefaultConfig("none")
	if err != nil {
		t.Fatalf("loading default config: %v", err)
	}

	// The generated YAML includes all repo settings and labels from the
	// embedded defaults.
	var sb strings.Builder
	sb.WriteString("license: none\n\nrepository:\n")

	// Description, homepage, and topics are nil in the embedded defaults.
	if defaults.Repository != nil {
		if defaults.Repository.HasWiki != nil {
			fmt.Fprintf(&sb, "  has_wiki: %t\n", *defaults.Repository.HasWiki)
		}
		if defaults.Repository.HasDiscussions != nil {
			fmt.Fprintf(&sb, "  has_discussions: %t\n", *defaults.Repository.HasDiscussions)
		}
		if defaults.Repository.HasProjects != nil {
			fmt.Fprintf(&sb, "  has_projects: %t\n", *defaults.Repository.HasProjects)
		}
		if defaults.Repository.HasIssues != nil {
			fmt.Fprintf(&sb, "  has_issues: %t\n", *defaults.Repository.HasIssues)
		}
		if defaults.Repository.AllowMergeCommit != nil {
			fmt.Fprintf(&sb, "  allow_merge_commit: %t\n", *defaults.Repository.AllowMergeCommit)
		}
		if defaults.Repository.AllowSquashMerge != nil {
			fmt.Fprintf(&sb, "  allow_squash_merge: %t\n", *defaults.Repository.AllowSquashMerge)
		}
		if defaults.Repository.AllowRebaseMerge != nil {
			fmt.Fprintf(&sb, "  allow_rebase_merge: %t\n", *defaults.Repository.AllowRebaseMerge)
		}
		if defaults.Repository.SquashMergeCommitTitle != nil {
			fmt.Fprintf(&sb, "  squash_merge_commit_title: %s\n", *defaults.Repository.SquashMergeCommitTitle)
		}
		if defaults.Repository.SquashMergeCommitMessage != nil {
			fmt.Fprintf(&sb, "  squash_merge_commit_message: %s\n", *defaults.Repository.SquashMergeCommitMessage)
		}
		if defaults.Repository.DeleteBranchOnMerge != nil {
			fmt.Fprintf(&sb, "  delete_branch_on_merge: %t\n", *defaults.Repository.DeleteBranchOnMerge)
		}
		if defaults.Repository.AllowUpdateBranch != nil {
			fmt.Fprintf(&sb, "  allow_update_branch: %t\n", *defaults.Repository.AllowUpdateBranch)
		}
		if defaults.Repository.AllowAutoMerge != nil {
			fmt.Fprintf(&sb, "  allow_auto_merge: %t\n", *defaults.Repository.AllowAutoMerge)
		}
		if defaults.Repository.WebCommitSignoffRequired != nil {
			fmt.Fprintf(&sb, "  web_commit_signoff_required: %t\n", *defaults.Repository.WebCommitSignoffRequired)
		}
		if defaults.Repository.PrivateVulnerabilityReportEnabled != nil {
			fmt.Fprintf(&sb, "  private_vulnerability_reporting_enabled: %t\n", *defaults.Repository.PrivateVulnerabilityReportEnabled)
		}
		if defaults.Repository.VulnerabilityAlertsEnabled != nil {
			fmt.Fprintf(&sb, "  vulnerability_alerts_enabled: %t\n", *defaults.Repository.VulnerabilityAlertsEnabled)
		}
		if defaults.Repository.AutomatedSecurityFixesEnabled != nil {
			fmt.Fprintf(&sb, "  automated_security_fixes_enabled: %t\n", *defaults.Repository.AutomatedSecurityFixesEnabled)
		}
		if defaults.Repository.DefaultWorkflowPermissions != nil {
			fmt.Fprintf(&sb, "  default_workflow_permissions: %s\n", *defaults.Repository.DefaultWorkflowPermissions)
		}
		if defaults.Repository.CanApprovePullRequestReviews != nil {
			fmt.Fprintf(&sb, "  can_approve_pull_request_reviews: %t\n", *defaults.Repository.CanApprovePullRequestReviews)
		}
	}
	if defaults.Actions != nil {
		sb.WriteString("\nactions:\n")
		if defaults.Actions.Enabled != nil {
			fmt.Fprintf(&sb, "  enabled: %t\n", *defaults.Actions.Enabled)
		}
		if defaults.Actions.AllowedActions != nil {
			fmt.Fprintf(&sb, "  allowed_actions: %s\n", *defaults.Actions.AllowedActions)
		}
		if defaults.Actions.SHAPinningRequired != nil {
			fmt.Fprintf(&sb, "  sha_pinning_required: %t\n", *defaults.Actions.SHAPinningRequired)
		}
		if defaults.Actions.GitHubOwnedAllowed != nil {
			fmt.Fprintf(&sb, "  github_owned_allowed: %t\n", *defaults.Actions.GitHubOwnedAllowed)
		}
		if defaults.Actions.VerifiedAllowed != nil {
			fmt.Fprintf(&sb, "  verified_allowed: %t\n", *defaults.Actions.VerifiedAllowed)
		}
		if defaults.Actions.PatternsAllowed != nil {
			writePatternsAllowedYAML(&sb, *defaults.Actions.PatternsAllowed)
		}
	}

	sb.WriteString("\nlabels:\n")
	for _, l := range defaults.Labels {
		fmt.Fprintf(&sb, "  - name: %s\n    color: %q\n    description: %q\n", l.Name, l.Color, l.Description)
	}

	sb.WriteString("\nswatches:\n")
	sb.WriteString("  - path: .tailor.yml\n")
	sb.WriteString("    alteration: always\n")
	for _, s := range defaults.Swatches {
		if s.Path == config.ConfigSwatchPath {
			continue
		}
		fmt.Fprintf(&sb, "  - path: %s\n    alteration: %s\n", s.Path, s.Alteration)
	}

	configYAML := sb.String()
	tc := setupAlterTest(t, configYAML)
	writeOnDisk(t, tc.Dir, "LICENSE", []byte("existing"))

	// Capture original config content.
	originalData, err := os.ReadFile(filepath.Join(tc.Dir, ".tailor.yml"))
	if err != nil {
		t.Fatalf("reading original config: %v", err)
	}

	cfg := loadTestConfig(t, tc.Dir)
	_ = captureAlterRun(t, cfg, tc.Dir, alter.Apply, tc.Client)

	afterData, err := os.ReadFile(filepath.Join(tc.Dir, ".tailor.yml"))
	if err != nil {
		t.Fatalf("reading config after alter: %v", err)
	}

	if !bytes.Equal(originalData, afterData) {
		t.Error("complete config was rewritten despite having all defaults already present")
	}
}

// TestAlterRunMergeRepoSettingsOnly verifies that config rewrite triggers
// when only repo settings are merged (no new swatches, labels already present).
func TestAlterRunMergeRepoSettingsOnly(t *testing.T) {
	configYAML := `license: none
labels:
  - name: custom-label
    color: "000000"
    description: "A custom label"
swatches:
  - path: .tailor.yml
    alteration: always
  - path: .gitignore
    alteration: first-fit
`
	tc := setupAlterTest(t, configYAML)
	writeOnDisk(t, tc.Dir, "LICENSE", []byte("existing"))

	originalData, err := os.ReadFile(filepath.Join(tc.Dir, ".tailor.yml"))
	if err != nil {
		t.Fatalf("reading original config: %v", err)
	}

	cfg := loadTestConfig(t, tc.Dir)
	_ = captureAlterRun(t, cfg, tc.Dir, alter.Apply, tc.Client)

	afterData, err := os.ReadFile(filepath.Join(tc.Dir, ".tailor.yml"))
	if err != nil {
		t.Fatalf("reading config after merge: %v", err)
	}

	// Merging repo settings rewrites the config.
	if bytes.Equal(originalData, afterData) {
		t.Error("config was not rewritten despite missing repo settings fields")
	}

	content := string(afterData)

	// Merged repo settings are present.
	if !strings.Contains(content, "has_issues:") {
		t.Error("merged config missing default repo setting has_issues")
	}

	// Existing labels are preserved.
	if !strings.Contains(content, "custom-label") {
		t.Error("existing custom label was lost during merge")
	}
}

func TestAlterRunRetiredWorkflowMigrationByMode(t *testing.T) {
	configYAML := `license: none
swatches:
  - path: .github/workflows/tailor.yml
    alteration: triggered
  - path: .github/workflows/tailor-automerge.yml
    alteration: triggered
`
	tests := []struct {
		name       string
		mode       alter.ApplyMode
		wantOutput string
		wantWrite  bool
	}{
		{
			name: "dry run",
			mode: alter.DryRun,
			wantOutput: "would update:                        .tailor.yml\n" +
				"would remove:                        .github/workflows/tailor-automerge.yml\n" +
				"would remove:                        .github/workflows/tailor.yml\n",
		},
		{
			name:      "apply",
			mode:      alter.Apply,
			wantWrite: true,
			wantOutput: "updated:                             .tailor.yml\n" +
				"removed:                             .github/workflows/tailor-automerge.yml\n" +
				"removed:                             .github/workflows/tailor.yml\n",
		},
		{
			name:      "recut",
			mode:      alter.Recut,
			wantWrite: true,
			wantOutput: "updated:                             .tailor.yml\n" +
				"removed:                             .github/workflows/tailor-automerge.yml\n" +
				"removed:                             .github/workflows/tailor.yml\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tc := setupAlterTest(t, configYAML)
			writeOnDisk(t, tc.Dir, "LICENSE", []byte("existing"))
			for _, path := range config.RetiredWorkflowPaths() {
				writeOnDisk(t, tc.Dir, path, []byte("retired workflow"))
			}
			beforeConfig, err := os.ReadFile(filepath.Join(tc.Dir, ".tailor.yml"))
			if err != nil {
				t.Fatalf("reading original config: %v", err)
			}

			cfg := loadTestConfig(t, tc.Dir)
			got := captureAlterRun(t, cfg, tc.Dir, tt.mode, tc.Client)
			if got != tt.wantOutput {
				t.Errorf("alter.Run() output =\n%s\nwant:\n%s", got, tt.wantOutput)
			}

			afterConfig, err := os.ReadFile(filepath.Join(tc.Dir, ".tailor.yml"))
			if err != nil {
				t.Fatalf("reading config after alter: %v", err)
			}
			if !tt.wantWrite {
				if !bytes.Equal(afterConfig, beforeConfig) {
					t.Error("dry run changed .tailor.yml on disk")
				}
			} else {
				if bytes.Equal(afterConfig, beforeConfig) {
					t.Error("write mode did not update .tailor.yml")
				}
				afterCfg := loadTestConfig(t, tc.Dir)
				if len(afterCfg.Swatches) != 0 {
					t.Errorf("written swatches = %v, want no retired entries", afterCfg.Swatches)
				}
			}

			for _, path := range config.RetiredWorkflowPaths() {
				_, err := os.Lstat(filepath.Join(tc.Dir, path))
				if !tt.wantWrite && err != nil {
					t.Errorf("dry run removed %s: %v", path, err)
				}
				if tt.wantWrite && !errors.Is(err, os.ErrNotExist) {
					t.Errorf("write mode left %s on disk: %v", path, err)
				}
			}
		})
	}
}

func TestAlterRunCoalescesRetiredCleanupWithDefaultMerge(t *testing.T) {
	configYAML := `license: none
swatches:
  - path: justfile
    alteration: never
  - path: .github/workflows/tailor.yml
    alteration: triggered
  - path: .tailor.yml
    alteration: always
  - path: .github/workflows/tailor-automerge.yml
    alteration: triggered
  - path: .github/workflows/tailor.yml
    alteration: always
`
	tc := setupAlterTest(t, configYAML)
	writeOnDisk(t, tc.Dir, "LICENSE", []byte("existing"))
	for _, path := range config.RetiredWorkflowPaths() {
		writeOnDisk(t, tc.Dir, path, []byte("retired workflow"))
	}

	cfg := loadTestConfig(t, tc.Dir)
	output := captureAlterRun(t, cfg, tc.Dir, alter.Apply, tc.Client)
	if count := strings.Count(output, "updated:                                                       .tailor.yml\n"); count != 1 {
		t.Errorf("config update count = %d, want 1\noutput:\n%s", count, output)
	}

	written := loadTestConfig(t, tc.Dir)
	if config.RemoveRetiredWorkflowEntries(written) {
		t.Errorf("written config still contains a retired workflow: %v", written.Swatches)
	}
	if len(written.Swatches) < 3 {
		t.Fatalf("written config has no merged defaults: %v", written.Swatches)
	}
	wantPrefix := []config.SwatchEntry{
		{Path: "justfile", Alteration: swatch.Never},
		{Path: ".tailor.yml", Alteration: swatch.Always},
	}
	if !slices.Equal(written.Swatches[:2], wantPrefix) {
		t.Errorf("active swatch prefix = %v, want %v", written.Swatches[:2], wantPrefix)
	}
	if !slices.ContainsFunc(written.Swatches, func(entry config.SwatchEntry) bool {
		return entry.Path == "SUPPORT.md"
	}) {
		t.Error("written config does not contain a merged SUPPORT.md entry")
	}
}

func TestAlterRunInvalidRemainingConfigChangesNoDiskOrStdout(t *testing.T) {
	configYAML := `license: none
swatches:
  - path: .github/workflows/tailor-automerge.yml
    alteration: triggered
  - path: unknown-active-swatch.yml
    alteration: always
`
	tc := setupAlterTest(t, configYAML)
	writeOnDisk(t, tc.Dir, "LICENSE", []byte("existing"))
	retiredPath := config.RetiredWorkflowPaths()[0]
	writeOnDisk(t, tc.Dir, retiredPath, []byte("retired workflow"))
	beforeConfig, err := os.ReadFile(filepath.Join(tc.Dir, ".tailor.yml"))
	if err != nil {
		t.Fatalf("reading original config: %v", err)
	}

	cfg := loadTestConfig(t, tc.Dir)
	stdout, _, err := captureAlterRunWithStderr(t, cfg, tc.Dir, alter.Apply, tc.Client)
	if err == nil {
		t.Fatal("alter.Run() error = nil, want invalid path error")
	}
	if !strings.Contains(err.Error(), "unrecognised swatch path") {
		t.Errorf("error = %q, want unrecognised swatch path", err)
	}
	if stdout != "" {
		t.Errorf("stdout = %q, want empty output", stdout)
	}
	afterConfig, readErr := os.ReadFile(filepath.Join(tc.Dir, ".tailor.yml"))
	if readErr != nil {
		t.Fatalf("reading config after failed alter: %v", readErr)
	}
	if !bytes.Equal(afterConfig, beforeConfig) {
		t.Error("failed validation changed .tailor.yml on disk")
	}
	if got, readErr := os.ReadFile(filepath.Join(tc.Dir, retiredPath)); readErr != nil || string(got) != "retired workflow" {
		t.Errorf("failed validation changed retired workflow: content %q, error %v", got, readErr)
	}
	if calls := tc.Calls(); len(calls) != 0 {
		t.Errorf("failed validation made API calls: %v", calls)
	}
}

func TestAlterRunRetiredCleanupRetryIsSafe(t *testing.T) {
	configYAML := `license: none
swatches:
  - path: .github/workflows/tailor-automerge.yml
    alteration: triggered
  - path: .github/workflows/tailor.yml
    alteration: triggered
`
	tc := setupAlterTest(t, configYAML)
	writeOnDisk(t, tc.Dir, "LICENSE", []byte("existing"))
	remaining := config.RetiredWorkflowPaths()[1]
	writeOnDisk(t, tc.Dir, remaining, []byte("remaining"))

	cfg := loadTestConfig(t, tc.Dir)
	first := captureAlterRun(t, cfg, tc.Dir, alter.Apply, tc.Client)
	wantFirst := "updated:                             .tailor.yml\n" +
		"removed:                             .github/workflows/tailor.yml\n"
	if first != wantFirst {
		t.Errorf("first alter.Run() output =\n%s\nwant:\n%s", first, wantFirst)
	}

	second := captureAlterRun(t, cfg, tc.Dir, alter.Apply, tc.Client)
	if second != "" {
		t.Errorf("retry alter.Run() output = %q, want empty output", second)
	}
	if _, err := os.Lstat(filepath.Join(tc.Dir, remaining)); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("retired workflow exists after retry: %v", err)
	}
}
