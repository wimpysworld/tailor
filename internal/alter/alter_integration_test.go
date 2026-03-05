package alter_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/cli/go-gh/v2/pkg/api"
	"github.com/wimpysworld/tailor/internal/alter"
	"github.com/wimpysworld/tailor/internal/config"
	"github.com/wimpysworld/tailor/internal/swatch"
	"github.com/wimpysworld/tailor/internal/ghfake"
	"github.com/wimpysworld/tailor/internal/testutil"
)

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
	username    string
	owner       string
	repo        string
	repoJSON    repoJSON
	pvrEnabled  bool
	licenceID   string
	licenceBody string
	noRepo      bool // stub RepoContext to return false
	userError   int  // non-zero: return this HTTP status for GET /user
	licenceError int // non-zero: return this HTTP status for GET /licenses/*
	patchError  int  // non-zero: return this HTTP status for PATCH /repos/*
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

// WithPVR sets the private vulnerability reporting status.
func WithPVR(enabled bool) testOption {
	return func(c *alterServerConfig) { c.pvrEnabled = enabled }
}

// WithLicence sets the licence ID and body for GET /licenses/{id}.
func WithLicence(id, body string) testOption {
	return func(c *alterServerConfig) {
		c.licenceID = id
		c.licenceBody = body
	}
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

// setupAlterTest creates a temp dir, writes .tailor/config.yml from the
// provided YAML string, sets up a mock HTTP server, stubs the repo context,
// and returns an alterTestContext ready for use with alter.Run.
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

	// Write config file.
	cfgDir := filepath.Join(dir, ".tailor")
	if err := os.MkdirAll(cfgDir, 0o755); err != nil {
		t.Fatalf("creating .tailor dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(cfgDir, "config.yml"), []byte(configYAML), 0o644); err != nil {
		t.Fatalf("writing config.yml: %v", err)
	}

	ctx := &alterTestContext{Dir: dir}

	// Build mock server.
	repoPath := fmt.Sprintf("/repos/%s/%s", sc.owner, sc.repo)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path

		// Read body for mutating requests.
		var body string
		if r.Body != nil && (r.Method == http.MethodPatch || r.Method == http.MethodPut || r.Method == http.MethodPost) {
			data, _ := io.ReadAll(r.Body)
			body = string(data)
		}

		// Record all calls.
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
			json.NewEncoder(w).Encode(sc.repoJSON)

		case r.Method == http.MethodGet && path == repoPath+"/private-vulnerability-reporting":
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprintf(w, `{"enabled":%t}`, sc.pvrEnabled)

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

		case r.Method == http.MethodPut && path == repoPath+"/private-vulnerability-reporting":
			w.WriteHeader(http.StatusNoContent)

		case r.Method == http.MethodDelete && path == repoPath+"/private-vulnerability-reporting":
			w.WriteHeader(http.StatusNoContent)

		default:
			w.WriteHeader(http.StatusNotFound)
			fmt.Fprintf(w, `{"message":"Not Found: %s %s"}`, r.Method, path)
		}
	}))
	t.Cleanup(server.Close)

	ctx.Server = server
	ctx.Client = testutil.NewTestClient(t, server)

	// Stub repo context.
	if sc.noRepo {
		ghfake.FakeNoRepo(t)
	} else {
		ghfake.FakeRepo(t, sc.owner, sc.repo)
	}

	return ctx
}

// loadTestConfig loads .tailor/config.yml from dir through the config package,
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
	buf.ReadFrom(r)
	return buf.String()
}

// runAlterExpectError runs alter.Run and returns the error. Stdout and stderr
// are suppressed. Fails if no error is returned.
func runAlterExpectError(t *testing.T, cfg *config.Config, dir string, mode alter.ApplyMode, client *api.RESTClient) error {
	t.Helper()

	oldStdout := os.Stdout
	_, wOut, _ := os.Pipe()
	os.Stdout = wOut

	oldStderr := os.Stderr
	_, wErr, _ := os.Pipe()
	os.Stderr = wErr

	err := alter.Run(cfg, dir, mode, client)

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
	bufOut.ReadFrom(rOut)
	bufErr.ReadFrom(rErr)

	return bufOut.String(), bufErr.String(), err
}

// requireContains fails if output does not contain substr.
func requireContains(t *testing.T, output, substr string) {
	t.Helper()
	if !strings.Contains(output, substr) {
		t.Errorf("expected output to contain %q, got:\n%s", substr, output)
	}
}

// requireNotContains fails if output contains substr.
func requireNotContains(t *testing.T, output, substr string) {
	t.Helper()
	if strings.Contains(output, substr) {
		t.Errorf("expected output NOT to contain %q, got:\n%s", substr, output)
	}
}

// TestAlterRunDryRunSmokeTest verifies the integration test infrastructure works
// by running alter.Run in DryRun mode with a single swatch entry. It checks that
// the expected output is produced and no files are written.
func TestAlterRunDryRunSmokeTest(t *testing.T) {
	configYAML := `license: mit
swatches:
  - source: .gitignore
    destination: .gitignore
    alteration: first-fit
`
	tc := setupAlterTest(t, configYAML)
	cfg := loadTestConfig(t, tc.Dir)

	// Capture stdout.
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
	buf.ReadFrom(r)
	output := buf.String()

	// Verify output contains expected "would copy" lines.
	if !strings.Contains(output, "would copy:") {
		t.Errorf("expected output to contain 'would copy:', got:\n%s", output)
	}
	if !strings.Contains(output, ".gitignore") {
		t.Errorf("expected output to contain '.gitignore', got:\n%s", output)
	}
	if !strings.Contains(output, "LICENSE") {
		t.Errorf("expected output to contain 'LICENSE', got:\n%s", output)
	}

	// Verify no swatch files were written.
	if _, err := os.Stat(filepath.Join(tc.Dir, ".gitignore")); err == nil {
		t.Error("dry run wrote .gitignore to disk")
	}
	if _, err := os.Stat(filepath.Join(tc.Dir, "LICENSE")); err == nil {
		t.Error("dry run wrote LICENSE to disk")
	}

	// Verify no mutating API calls were made.
	if mc := tc.MutatingCalls(); len(mc) != 0 {
		t.Errorf("dry run made %d mutating API calls: %v", len(mc), mc)
	}
}

// TestAlterRunDryRunWithRepoSettings verifies that repo settings appear in
// dry-run output when they differ from live settings.
func TestAlterRunDryRunWithRepoSettings(t *testing.T) {
	configYAML := `license: none
repository:
  has_wiki: false
swatches:
  - source: .gitignore
    destination: .gitignore
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
	buf.ReadFrom(r)
	output := buf.String()

	if !strings.Contains(output, "would set:") {
		t.Errorf("expected output to contain 'would set:', got:\n%s", output)
	}
	if !strings.Contains(output, "repository.has_wiki") {
		t.Errorf("expected output to contain 'repository.has_wiki', got:\n%s", output)
	}

	// Verify no mutating API calls.
	if mc := tc.MutatingCalls(); len(mc) != 0 {
		t.Errorf("dry run made %d mutating API calls: %v", len(mc), mc)
	}
}

// TestAlterRunApplyWritesFiles verifies that Apply mode writes swatch files
// and calls the GitHub API.
func TestAlterRunApplyWritesFiles(t *testing.T) {
	configYAML := `license: mit
swatches:
  - source: .gitignore
    destination: .gitignore
    alteration: first-fit
`
	tc := setupAlterTest(t, configYAML)
	cfg := loadTestConfig(t, tc.Dir)

	// Suppress stdout.
	oldStdout := os.Stdout
	_, w, _ := os.Pipe()
	os.Stdout = w

	err := alter.Run(cfg, tc.Dir, alter.Apply, tc.Client)

	w.Close()
	os.Stdout = oldStdout

	if err != nil {
		t.Fatalf("alter.Run() error: %v", err)
	}

	// Verify .gitignore was written.
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

	// Verify LICENSE was written.
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
  - source: .gitignore
    destination: .gitignore
    alteration: first-fit
  - source: CODE_OF_CONDUCT.md
    destination: CODE_OF_CONDUCT.md
    alteration: always
  - source: SECURITY.md
    destination: SECURITY.md
    alteration: always
`
	tc := setupAlterTest(t, configYAML,
		WithRepoSettings(repoJSON{HasWiki: true, DeleteBranchOnMerge: false}),
	)
	cfg := loadTestConfig(t, tc.Dir)
	output := captureAlterRun(t, cfg, tc.Dir, alter.DryRun, tc.Client)

	// All swatches absent: "would copy" for each.
	requireContains(t, output, "would copy:")
	requireContains(t, output, ".gitignore")
	requireContains(t, output, "CODE_OF_CONDUCT.md")
	requireContains(t, output, "SECURITY.md")
	requireContains(t, output, "LICENSE")

	// Repo settings that differ: "would set".
	requireContains(t, output, "would set:")
	requireContains(t, output, "repository.has_wiki")
	requireContains(t, output, "repository.delete_branch_on_merge")

	// No mutating API calls.
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
  - source: .gitignore
    destination: .gitignore
    alteration: first-fit
  - source: CODE_OF_CONDUCT.md
    destination: CODE_OF_CONDUCT.md
    alteration: always
  - source: SECURITY.md
    destination: SECURITY.md
    alteration: always
`
	tc := setupAlterTest(t, configYAML,
		WithRepoSettings(repoJSON{HasWiki: false}),
	)

	// Pre-write all swatch files with matching embedded content.
	for _, src := range []string{".gitignore", "CODE_OF_CONDUCT.md", "SECURITY.md"} {
		content := mustContent(t, src)
		writeOnDisk(t, tc.Dir, src, content)
	}
	// Pre-write licence.
	writeOnDisk(t, tc.Dir, "LICENSE", []byte("existing licence"))

	cfg := loadTestConfig(t, tc.Dir)
	output := captureAlterRun(t, cfg, tc.Dir, alter.DryRun, tc.Client)

	// first-fit .gitignore exists: "skipped (first-fit, exists)".
	requireContains(t, output, "skipped (first-fit, exists):")
	requireContains(t, output, ".gitignore")

	// Non-substituted always CODE_OF_CONDUCT.md with matching content: "no change".
	requireContains(t, output, "no change:")
	requireContains(t, output, "CODE_OF_CONDUCT.md")

	// Substituted always SECURITY.md: always "would overwrite" regardless of content match.
	requireContains(t, output, "would overwrite:")
	requireContains(t, output, "SECURITY.md")

	// Licence exists: "skipped (first-fit, exists)".
	requireContains(t, output, "LICENSE")

	// Repo setting matches: "no change".
	requireContains(t, output, "repository.has_wiki")

	// No "would copy" should appear since all files exist.
	requireNotContains(t, output, "would copy:")

	// No mutating API calls.
	if mc := tc.MutatingCalls(); len(mc) != 0 {
		t.Errorf("dry run made %d mutating API calls: %v", len(mc), mc)
	}
}

// TestAlterRunDryRunMixedFiles verifies output when some files exist and others
// are absent, producing a mix of "would copy", "skipped", and "no change".
func TestAlterRunDryRunMixedFiles(t *testing.T) {
	configYAML := `license: mit
swatches:
  - source: .gitignore
    destination: .gitignore
    alteration: first-fit
  - source: CODE_OF_CONDUCT.md
    destination: CODE_OF_CONDUCT.md
    alteration: always
  - source: CONTRIBUTING.md
    destination: CONTRIBUTING.md
    alteration: always
`
	tc := setupAlterTest(t, configYAML)

	// Pre-write .gitignore (first-fit, will be skipped) and CODE_OF_CONDUCT.md
	// with matching content (always, will be no change).
	writeOnDisk(t, tc.Dir, ".gitignore", mustContent(t, ".gitignore"))
	writeOnDisk(t, tc.Dir, "CODE_OF_CONDUCT.md", mustContent(t, "CODE_OF_CONDUCT.md"))
	// CONTRIBUTING.md absent: will be "would copy".
	// LICENSE absent: will be "would copy".

	cfg := loadTestConfig(t, tc.Dir)
	output := captureAlterRun(t, cfg, tc.Dir, alter.DryRun, tc.Client)

	// .gitignore present + first-fit = skipped.
	requireContains(t, output, "skipped (first-fit, exists):")

	// CODE_OF_CONDUCT.md present + always + content matches = no change.
	requireContains(t, output, "no change:")

	// CONTRIBUTING.md absent = would copy.
	requireContains(t, output, "would copy:")
	requireContains(t, output, "CONTRIBUTING.md")

	// LICENSE absent = would copy.
	requireContains(t, output, "LICENSE")
}

// TestAlterRunDryRunAlwaysSwatchDiffersContent verifies that a non-substituted
// "always" swatch whose on-disk content differs from embedded shows "would overwrite".
func TestAlterRunDryRunAlwaysSwatchDiffersContent(t *testing.T) {
	configYAML := `license: none
swatches:
  - source: CODE_OF_CONDUCT.md
    destination: CODE_OF_CONDUCT.md
    alteration: always
`
	tc := setupAlterTest(t, configYAML)

	// Pre-write CODE_OF_CONDUCT.md with different content.
	writeOnDisk(t, tc.Dir, "CODE_OF_CONDUCT.md", []byte("outdated conduct document"))
	// Pre-write LICENSE to suppress warning.
	writeOnDisk(t, tc.Dir, "LICENSE", []byte("existing"))

	cfg := loadTestConfig(t, tc.Dir)
	output := captureAlterRun(t, cfg, tc.Dir, alter.DryRun, tc.Client)

	requireContains(t, output, "would overwrite:")
	requireContains(t, output, "CODE_OF_CONDUCT.md")

	// Verify the file was NOT modified (dry-run).
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
  - source: SECURITY.md
    destination: SECURITY.md
    alteration: always
`
	tc := setupAlterTest(t, configYAML)

	// Write SECURITY.md with the exact embedded content (before substitution).
	writeOnDisk(t, tc.Dir, "SECURITY.md", mustContent(t, "SECURITY.md"))
	// Pre-write LICENSE to suppress warning.
	writeOnDisk(t, tc.Dir, "LICENSE", []byte("existing"))

	cfg := loadTestConfig(t, tc.Dir)
	output := captureAlterRun(t, cfg, tc.Dir, alter.DryRun, tc.Client)

	requireContains(t, output, "would overwrite:")
	requireContains(t, output, "SECURITY.md")

	// Must not show "no change" for SECURITY.md.
	lines := strings.Split(output, "\n")
	for _, line := range lines {
		if strings.Contains(line, "SECURITY.md") && strings.Contains(line, "no change") {
			t.Errorf("substituted swatch SECURITY.md should not show 'no change', got: %s", line)
		}
	}
}

// TestAlterRunDryRunNoFilesWritten verifies that after a comprehensive dry-run,
// no new files are created and no existing files are modified.
func TestAlterRunDryRunNoFilesWritten(t *testing.T) {
	configYAML := `license: mit
repository:
  has_wiki: false
swatches:
  - source: .gitignore
    destination: .gitignore
    alteration: first-fit
  - source: CODE_OF_CONDUCT.md
    destination: CODE_OF_CONDUCT.md
    alteration: always
  - source: SECURITY.md
    destination: SECURITY.md
    alteration: always
  - source: CONTRIBUTING.md
    destination: CONTRIBUTING.md
    alteration: always
`
	tc := setupAlterTest(t, configYAML,
		WithRepoSettings(repoJSON{HasWiki: true}),
	)

	// Pre-write one file with known content to verify it is not modified.
	existingContent := []byte("original content")
	writeOnDisk(t, tc.Dir, "CODE_OF_CONDUCT.md", existingContent)

	// Record filesystem state before dry-run.
	dirEntries := func() map[string]int64 {
		entries := make(map[string]int64)
		filepath.Walk(tc.Dir, func(path string, info os.FileInfo, _ error) error {
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

	// Check no new files were created.
	for path := range after {
		if _, existed := before[path]; !existed {
			t.Errorf("dry run created new file: %s", path)
		}
	}

	// Verify CODE_OF_CONDUCT.md was not modified.
	data, err := os.ReadFile(filepath.Join(tc.Dir, "CODE_OF_CONDUCT.md"))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(data, existingContent) {
		t.Error("dry run modified CODE_OF_CONDUCT.md")
	}

	// Verify no swatch files that were absent were written.
	for _, absent := range []string{".gitignore", "SECURITY.md", "CONTRIBUTING.md", "LICENSE"} {
		if _, err := os.Stat(filepath.Join(tc.Dir, absent)); err == nil {
			t.Errorf("dry run wrote %s to disk", absent)
		}
	}

	// No mutating API calls.
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
  - source: .gitignore
    destination: .gitignore
    alteration: first-fit
  - source: CODE_OF_CONDUCT.md
    destination: CODE_OF_CONDUCT.md
    alteration: always
  - source: CONTRIBUTING.md
    destination: CONTRIBUTING.md
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
	informationalLabels := []string{"no change:", "skipped (first-fit, exists):"}

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
// are padded to exactly 29 characters (the labelWidth constant from format.go).
func TestAlterRunDryRunColumnWidth(t *testing.T) {
	configYAML := `license: mit
repository:
  has_wiki: false
swatches:
  - source: .gitignore
    destination: .gitignore
    alteration: first-fit
  - source: CODE_OF_CONDUCT.md
    destination: CODE_OF_CONDUCT.md
    alteration: always
  - source: SECURITY.md
    destination: SECURITY.md
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

	// Each line should have the label portion padded to 29 characters,
	// meaning position 29 starts the content (no leading space at pos 29
	// unless the label itself is shorter).
	knownLabels := []string{
		"would copy:",
		"would overwrite:",
		"no change:",
		"skipped (first-fit, exists):",
		"would set:",
	}

	const expectedWidth = 29
	for _, line := range lines {
		if len(line) < expectedWidth {
			t.Errorf("line too short to contain label + content: %q", line)
			continue
		}

		labelPart := line[:expectedWidth]
		trimmed := strings.TrimRight(labelPart, " ")

		found := false
		for _, label := range knownLabels {
			if trimmed == label {
				found = true
				break
			}
		}
		if !found {
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
  - source: .gitignore
    destination: .gitignore
    alteration: first-fit
  - source: CODE_OF_CONDUCT.md
    destination: CODE_OF_CONDUCT.md
    alteration: always
  - source: SECURITY.md
    destination: SECURITY.md
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

	// Repo settings PATCH must have been called.
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

// TestAlterRunApplyFileContentMatchesEmbedded verifies that non-substituted
// swatch files written during apply match the embedded swatch content exactly.
func TestAlterRunApplyFileContentMatchesEmbedded(t *testing.T) {
	configYAML := `license: none
swatches:
  - source: .gitignore
    destination: .gitignore
    alteration: first-fit
  - source: CODE_OF_CONDUCT.md
    destination: CODE_OF_CONDUCT.md
    alteration: always
  - source: CONTRIBUTING.md
    destination: CONTRIBUTING.md
    alteration: always
`
	tc := setupAlterTest(t, configYAML)
	// Pre-write LICENSE to suppress warning.
	writeOnDisk(t, tc.Dir, "LICENSE", []byte("existing"))

	cfg := loadTestConfig(t, tc.Dir)
	_ = captureAlterRun(t, cfg, tc.Dir, alter.Apply, tc.Client)

	for _, src := range []string{".gitignore", "CODE_OF_CONDUCT.md", "CONTRIBUTING.md"} {
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

// TestAlterRunApplyFundingYmlSubstituted verifies that FUNDING.yml written
// during apply contains the mock username, not the raw {{GITHUB_USERNAME}} token.
func TestAlterRunApplyFundingYmlSubstituted(t *testing.T) {
	configYAML := `license: none
swatches:
  - source: .github/FUNDING.yml
    destination: .github/FUNDING.yml
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

// TestAlterRunApplySecurityMdSubstituted verifies that SECURITY.md written
// during apply contains the constructed advisory URL, not the raw {{ADVISORY_URL}} token.
func TestAlterRunApplySecurityMdSubstituted(t *testing.T) {
	configYAML := `license: none
swatches:
  - source: SECURITY.md
    destination: SECURITY.md
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
  - source: .github/ISSUE_TEMPLATE/bug_report.yml
    destination: .github/ISSUE_TEMPLATE/bug_report.yml
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

	// Verify parent directories exist.
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
  - source: .gitignore
    destination: .gitignore
    alteration: first-fit
  - source: .github/FUNDING.yml
    destination: .github/FUNDING.yml
    alteration: first-fit
  - source: CODE_OF_CONDUCT.md
    destination: CODE_OF_CONDUCT.md
    alteration: always
`
	tc := setupAlterTest(t, configYAML)
	writeOnDisk(t, tc.Dir, "LICENSE", []byte("existing"))

	// Pre-write first-fit files with custom content.
	writeOnDisk(t, tc.Dir, ".gitignore", []byte("my custom gitignore"))
	writeOnDisk(t, tc.Dir, ".github/FUNDING.yml", []byte("my custom funding"))

	cfg := loadTestConfig(t, tc.Dir)
	_ = captureAlterRun(t, cfg, tc.Dir, alter.Apply, tc.Client)

	// First-fit files must retain their original content.
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

	// Always swatch (CODE_OF_CONDUCT.md) should have been written.
	if _, err := os.Stat(filepath.Join(tc.Dir, "CODE_OF_CONDUCT.md")); err != nil {
		t.Errorf("CODE_OF_CONDUCT.md should have been written: %v", err)
	}
}

// TestAlterRunApplyAlwaysSwatchNoWriteOnMD5Match verifies that a non-substituted
// "always" swatch whose content already matches the embedded swatch is left alone.
func TestAlterRunApplyAlwaysSwatchNoWriteOnMD5Match(t *testing.T) {
	configYAML := `license: none
swatches:
  - source: CODE_OF_CONDUCT.md
    destination: CODE_OF_CONDUCT.md
    alteration: always
`
	tc := setupAlterTest(t, configYAML)
	writeOnDisk(t, tc.Dir, "LICENSE", []byte("existing"))

	// Pre-write CODE_OF_CONDUCT.md with exact embedded content.
	original := mustContent(t, "CODE_OF_CONDUCT.md")
	writeOnDisk(t, tc.Dir, "CODE_OF_CONDUCT.md", original)

	// Record modification time before apply.
	infoBefore, err := os.Stat(filepath.Join(tc.Dir, "CODE_OF_CONDUCT.md"))
	if err != nil {
		t.Fatal(err)
	}
	modTimeBefore := infoBefore.ModTime()

	cfg := loadTestConfig(t, tc.Dir)
	_ = captureAlterRun(t, cfg, tc.Dir, alter.Apply, tc.Client)

	// Content must remain identical.
	data, err := os.ReadFile(filepath.Join(tc.Dir, "CODE_OF_CONDUCT.md"))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(data, original) {
		t.Error("CODE_OF_CONDUCT.md content changed despite MD5 match")
	}

	// Modification time should not have changed (file was not re-written).
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
  - source: .gitignore
    destination: .gitignore
    alteration: first-fit
`
	tc := setupAlterTest(t, configYAML,
		WithLicence("mit", "Fresh MIT text"),
	)

	// Pre-write LICENSE with custom content.
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
  - source: .gitignore
    destination: .gitignore
    alteration: first-fit
`
	tc := setupAlterTest(t, configYAML,
		WithRepoSettings(repoJSON{HasWiki: true, DeleteBranchOnMerge: false}),
	)
	writeOnDisk(t, tc.Dir, "LICENSE", []byte("existing"))

	cfg := loadTestConfig(t, tc.Dir)
	_ = captureAlterRun(t, cfg, tc.Dir, alter.Apply, tc.Client)

	// Find the PATCH call.
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

	// Verify the body contains expected settings.
	var body map[string]interface{}
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

// ---------------------------------------------------------------------------
// Phase 6.4 - Recut integration tests
// ---------------------------------------------------------------------------

// TestAlterRunRecutOverwritesFirstFitSwatches verifies that recut
// overwrites pre-existing first-fit swatch files with embedded content.
func TestAlterRunRecutOverwritesFirstFitSwatches(t *testing.T) {
	configYAML := `license: none
swatches:
  - source: .gitignore
    destination: .gitignore
    alteration: first-fit
  - source: CODE_OF_CONDUCT.md
    destination: CODE_OF_CONDUCT.md
    alteration: first-fit
`
	tc := setupAlterTest(t, configYAML)
	writeOnDisk(t, tc.Dir, "LICENSE", []byte("existing"))

	// Pre-write first-fit files with custom content.
	writeOnDisk(t, tc.Dir, ".gitignore", []byte("custom gitignore"))
	writeOnDisk(t, tc.Dir, "CODE_OF_CONDUCT.md", []byte("custom conduct"))

	cfg := loadTestConfig(t, tc.Dir)
	_ = captureAlterRun(t, cfg, tc.Dir, alter.Recut, tc.Client)

	// Both files must now contain embedded swatch content, not custom content.
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
  - source: .gitignore
    destination: .gitignore
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

// TestAlterRunRecutDoesNotOverwriteConfig verifies that recut
// does not overwrite .tailor/config.yml (config is exempt).
func TestAlterRunRecutDoesNotOverwriteConfig(t *testing.T) {
	configYAML := `license: none
swatches:
  - source: .tailor/config.yml
    destination: .tailor/config.yml
    alteration: first-fit
`
	tc := setupAlterTest(t, configYAML)
	writeOnDisk(t, tc.Dir, "LICENSE", []byte("existing"))

	// The config already exists from setupAlterTest. Read it.
	originalCfg, err := os.ReadFile(filepath.Join(tc.Dir, ".tailor/config.yml"))
	if err != nil {
		t.Fatal(err)
	}

	cfg := loadTestConfig(t, tc.Dir)
	_ = captureAlterRun(t, cfg, tc.Dir, alter.Recut, tc.Client)

	data, err := os.ReadFile(filepath.Join(tc.Dir, ".tailor/config.yml"))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(data, originalCfg) {
		t.Error("recut overwrote .tailor/config.yml")
	}
}

// TestAlterRunRecutResolvesTokens verifies that recut runs full
// token resolution on substituted swatches. Pre-writes FUNDING.yml with stale
// content, then checks it contains the freshly resolved username.
func TestAlterRunRecutResolvesTokens(t *testing.T) {
	configYAML := `license: none
swatches:
  - source: .github/FUNDING.yml
    destination: .github/FUNDING.yml
    alteration: first-fit
`
	tc := setupAlterTest(t, configYAML,
		WithUsername("freshuser"),
	)
	writeOnDisk(t, tc.Dir, "LICENSE", []byte("existing"))

	// Pre-write FUNDING.yml with stale content.
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

// ---------------------------------------------------------------------------
// Phase 6.5 - Error path integration tests
// ---------------------------------------------------------------------------

// TestAlterRunErrorUnrecognisedSwatchSource verifies that a config with an
// unknown swatch source produces an error mentioning valid sources.
func TestAlterRunErrorUnrecognisedSwatchSource(t *testing.T) {
	configYAML := `license: none
swatches:
  - source: nonexistent.txt
    destination: nonexistent.txt
    alteration: first-fit
`
	tc := setupAlterTest(t, configYAML)
	writeOnDisk(t, tc.Dir, "LICENSE", []byte("existing"))

	cfg := loadTestConfig(t, tc.Dir)
	err := runAlterExpectError(t, cfg, tc.Dir, alter.Apply, tc.Client)

	if !strings.Contains(err.Error(), "unrecognised swatch source") {
		t.Errorf("error = %q, want substring 'unrecognised swatch source'", err)
	}
	if !strings.Contains(err.Error(), ".gitignore") {
		t.Errorf("error should list valid sources, got: %q", err)
	}
}

// TestAlterRunErrorDuplicateDestination verifies that two swatches targeting
// the same destination produce an error.
func TestAlterRunErrorDuplicateDestination(t *testing.T) {
	configYAML := `license: none
swatches:
  - source: .gitignore
    destination: .gitignore
    alteration: first-fit
  - source: CODE_OF_CONDUCT.md
    destination: .gitignore
    alteration: always
`
	tc := setupAlterTest(t, configYAML)
	writeOnDisk(t, tc.Dir, "LICENSE", []byte("existing"))

	cfg := loadTestConfig(t, tc.Dir)
	err := runAlterExpectError(t, cfg, tc.Dir, alter.Apply, tc.Client)

	if !strings.Contains(err.Error(), "duplicate destination") {
		t.Errorf("error = %q, want substring 'duplicate destination'", err)
	}
}

// TestAlterRunErrorUnrecognisedRepoSetting verifies that an unknown key in the
// repository section produces an error listing valid settings.
func TestAlterRunErrorUnrecognisedRepoSetting(t *testing.T) {
	configYAML := `license: none
repository:
  unknown_setting: true
swatches:
  - source: .gitignore
    destination: .gitignore
    alteration: first-fit
`
	tc := setupAlterTest(t, configYAML)
	writeOnDisk(t, tc.Dir, "LICENSE", []byte("existing"))

	cfg := loadTestConfig(t, tc.Dir)
	err := runAlterExpectError(t, cfg, tc.Dir, alter.Apply, tc.Client)

	if !strings.Contains(err.Error(), "unrecognised repository setting") {
		t.Errorf("error = %q, want substring 'unrecognised repository setting'", err)
	}
}

// TestAlterRunErrorMissingConfigFile verifies that config.Load returns an
// error when .tailor/config.yml does not exist. This error is caught by the
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
  - source: .gitignore
    destination: .gitignore
    alteration: first-fit
`
	tc := setupAlterTest(t, configYAML,
		WithLicenceError(http.StatusNotFound),
	)

	cfg := loadTestConfig(t, tc.Dir)
	err := runAlterExpectError(t, cfg, tc.Dir, alter.Apply, tc.Client)

	if !strings.Contains(err.Error(), "licence") && !strings.Contains(err.Error(), "license") {
		t.Errorf("error = %q, want substring containing 'licence' or 'license'", err)
	}
}

// TestAlterRunErrorGetUserFailure verifies that a 401 from GET /user
// propagates as an error. No files should be written and no PATCH calls made.
func TestAlterRunErrorGetUserFailure(t *testing.T) {
	configYAML := `license: none
swatches:
  - source: .gitignore
    destination: .gitignore
    alteration: first-fit
`
	tc := setupAlterTest(t, configYAML,
		WithUserError(http.StatusUnauthorized),
	)
	writeOnDisk(t, tc.Dir, "LICENSE", []byte("existing"))

	cfg := loadTestConfig(t, tc.Dir)
	err := runAlterExpectError(t, cfg, tc.Dir, alter.Apply, tc.Client)

	if !strings.Contains(err.Error(), "username") && !strings.Contains(err.Error(), "user") {
		t.Errorf("error = %q, want substring containing 'username' or 'user'", err)
	}

	// No swatch files should have been written.
	if _, statErr := os.Stat(filepath.Join(tc.Dir, ".gitignore")); statErr == nil {
		t.Error(".gitignore was written despite GET /user failure")
	}

	// No mutating API calls.
	if mc := tc.MutatingCalls(); len(mc) != 0 {
		t.Errorf("expected no mutating calls, got %d: %v", len(mc), mc)
	}
}

// TestAlterRunErrorPatchFailure verifies that a 403 from PATCH on repo
// settings propagates as an error. No swatch files should be written because
// repo settings are processed before swatches.
func TestAlterRunErrorPatchFailure(t *testing.T) {
	configYAML := `license: none
repository:
  has_wiki: false
swatches:
  - source: .gitignore
    destination: .gitignore
    alteration: first-fit
`
	tc := setupAlterTest(t, configYAML,
		WithRepoSettings(repoJSON{HasWiki: true}),
		WithPatchError(http.StatusForbidden),
	)
	writeOnDisk(t, tc.Dir, "LICENSE", []byte("existing"))

	cfg := loadTestConfig(t, tc.Dir)
	err := runAlterExpectError(t, cfg, tc.Dir, alter.Apply, tc.Client)

	if err == nil {
		t.Fatal("expected error from PATCH failure")
	}

	// No swatch files should have been written (repo settings fail first).
	if _, statErr := os.Stat(filepath.Join(tc.Dir, ".gitignore")); statErr == nil {
		t.Error(".gitignore was written despite PATCH failure")
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
  - source: .gitignore
    destination: .gitignore
    alteration: first-fit
`
	tc := setupAlterTest(t, configYAML, WithNoRepo())
	writeOnDisk(t, tc.Dir, "LICENSE", []byte("existing"))

	cfg := loadTestConfig(t, tc.Dir)
	stdout, stderr, err := captureAlterRunWithStderr(t, cfg, tc.Dir, alter.Apply, tc.Client)

	if err != nil {
		t.Fatalf("alter.Run() returned unexpected error: %v", err)
	}

	// Stderr should contain the repo context warning.
	if !strings.Contains(stderr, "No GitHub repository context found") {
		t.Errorf("expected stderr warning about no repo context, got: %q", stderr)
	}

	// Swatches should still be processed: .gitignore written.
	if _, statErr := os.Stat(filepath.Join(tc.Dir, ".gitignore")); statErr != nil {
		t.Errorf(".gitignore should have been written despite no repo context: %v", statErr)
	}

	_ = stdout
}

// TestAlterRunNoRepoContextLeavesTokensUnsubstituted verifies that without
// repo context, tokens depending on owner/repo remain as raw placeholders.
func TestAlterRunNoRepoContextLeavesTokensUnsubstituted(t *testing.T) {
	configYAML := `license: none
swatches:
  - source: SECURITY.md
    destination: SECURITY.md
    alteration: always
  - source: .github/ISSUE_TEMPLATE/config.yml
    destination: .github/ISSUE_TEMPLATE/config.yml
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
