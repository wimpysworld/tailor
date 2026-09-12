package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/cli/go-gh/v2/pkg/api"
	"github.com/wimpysworld/tailor/internal/alter"
	"github.com/wimpysworld/tailor/internal/config"
	"github.com/wimpysworld/tailor/internal/gh"
	"github.com/wimpysworld/tailor/internal/ghfake"
	"github.com/wimpysworld/tailor/internal/output"
	"github.com/wimpysworld/tailor/internal/swatch"
	"github.com/wimpysworld/tailor/internal/testutil"
)

// fakeNoRepoAuth installs a valid token, a user API that returns octocat,
// and no repository context.
func fakeNoRepoAuth(t *testing.T) {
	t.Helper()

	ghfake.FakeAuth(t, "gho_test")
	ghfake.FakeUserAPI(t, http.StatusOK, "octocat")
	ghfake.FakeNoRepo(t)
}

func TestFitNewDirectoryDefaultConfig(t *testing.T) {
	fakeNoRepoAuth(t)

	dir := filepath.Join(t.TempDir(), "new-project")

	cmd := FitCmd{Path: dir, License: "BlueOak-1.0.0"}
	if err := cmd.Run(); err != nil {
		t.Fatalf("Run() error: %v", err)
	}

	configPath := filepath.Join(dir, ".tailor.yml")
	data, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	content := string(data)

	if !strings.Contains(content, "license: BlueOak-1.0.0") {
		t.Error("config missing 'license: BlueOak-1.0.0'")
	}

	// Go swatches stay out of the default config until Go is explicitly enabled.
	if count := strings.Count(content, "- path:"); count != 25 {
		t.Errorf("swatch count = %d, want 25", count)
	}

	wantSettings := []string{
		"has_wiki:",
		"has_discussions:",
		"has_projects:",
		"has_issues:",
		"allow_merge_commit:",
		"allow_squash_merge:",
		"allow_rebase_merge:",
		"squash_merge_commit_title:",
		"squash_merge_commit_message:",
		"delete_branch_on_merge:",
		"allow_update_branch:",
		"allow_auto_merge:",
		"web_commit_signoff_required:",
	}
	for _, s := range wantSettings {
		if !strings.Contains(content, s) {
			t.Errorf("config missing %q", s)
		}
	}
}

func TestFitPlainOutputPreservesPathControls(t *testing.T) {
	fakeNoRepoAuth(t)

	dir := filepath.Join(t.TempDir(), "safe\ninjected")
	var stdout, stderr strings.Builder
	cmd := FitCmd{
		Path: dir, License: "BlueOak-1.0.0",
		output: output.New(&stdout, &stderr, output.Plain, output.WithTTY(true)),
	}
	if err := cmd.Run(); err != nil {
		t.Fatalf("Run() error: %v", err)
	}
	want := fmt.Sprintf("Fitted %s with .tailor.yml\n", dir)
	if got := stdout.String(); got != want {
		t.Fatalf("plain fit output = %q, want exact legacy output %q", got, want)
	}
}

func TestFitRichOutputEscapesPathControls(t *testing.T) {
	fakeNoRepoAuth(t)
	t.Setenv("TERM", "xterm-256color")
	t.Setenv("NO_COLOR", "1")

	dir := filepath.Join(t.TempDir(), "safe\ninjected")
	var stdout, stderr strings.Builder
	cmd := FitCmd{
		Path: dir, License: "BlueOak-1.0.0",
		output: output.New(&stdout, &stderr, output.Auto, output.WithTTY(true)),
	}
	if err := cmd.Run(); err != nil {
		t.Fatalf("Run() error: %v", err)
	}
	got := stdout.String()
	if strings.Contains(got, "safe\ninjected") || !strings.Contains(got, `\x0a`) {
		t.Fatalf("rich fit output did not escape the path newline:\n%q", got)
	}
}

func TestFitExistingDirectoryWithoutConfig(t *testing.T) {
	fakeNoRepoAuth(t)

	dir := t.TempDir()

	cmd := FitCmd{Path: dir, License: "BlueOak-1.0.0"}
	if err := cmd.Run(); err != nil {
		t.Fatalf("Run() error: %v", err)
	}

	configPath := filepath.Join(dir, ".tailor.yml")
	if _, err := os.Stat(configPath); err != nil {
		t.Fatalf("config file not created: %v", err)
	}
}

func TestFitDoesNotReadVariables(t *testing.T) {
	ghfake.FakeAuth(t, "gho_test")
	ghfake.FakeRepo(t, "octocat", "my-project")
	variableCalls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case strings.Contains(r.URL.Path, "/actions/variables"):
			variableCalls++
			fmt.Fprint(w, `{"total_count":1,"variables":[{"name":"DEPLOY_REGION","value":"eu-west-2"}]}`)
		case strings.HasSuffix(r.URL.Path, "/user"):
			fmt.Fprint(w, `{"login":"octocat"}`)
		case strings.HasSuffix(r.URL.Path, "/repos/octocat/my-project"):
			fmt.Fprint(w, `{"squash_merge_commit_title":"PR_TITLE","squash_merge_commit_message":"PR_BODY","merge_commit_title":"PR_TITLE","merge_commit_message":"PR_BODY"}`)
		default:
			w.WriteHeader(http.StatusForbidden)
			fmt.Fprint(w, `{"message":"Resource not accessible by personal access token"}`)
		}
	}))
	t.Cleanup(srv.Close)
	restore := gh.SetNewRESTClientFunc(func(string) (*api.RESTClient, error) {
		return testutil.NewTestClient(t, srv), nil
	})
	t.Cleanup(restore)
	dir := t.TempDir()
	var stdout, stderr strings.Builder
	if code := run([]string{"fit", dir}, &stdout, &stderr); code != 0 {
		t.Fatalf("fit = %d, stderr: %s", code, stderr.String())
	}
	if variableCalls != 0 {
		t.Fatalf("variable requests = %d, want 0", variableCalls)
	}
	data, err := os.ReadFile(filepath.Join(dir, ".tailor.yml"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "\nvariables:") || !strings.Contains(string(data), "# variables:") {
		t.Fatal("fit must write only the commented variables example")
	}
}

func TestRunPlainFormatPreservesFitOutput(t *testing.T) {
	fakeNoRepoAuth(t)
	dir := filepath.Join(t.TempDir(), "plain-project")
	var stdout, stderr strings.Builder
	if code := run([]string{"--format=plain", "fit", dir}, &stdout, &stderr); code != 0 {
		t.Fatalf("run() = %d, want 0; stderr: %s", code, stderr.String())
	}
	want := fmt.Sprintf("Fitted %s with .tailor.yml\n", dir)
	if got := stdout.String(); got != want {
		t.Fatalf("stdout = %q, want %q", got, want)
	}
}

func TestFitExistingDirectoryWithConfigError(t *testing.T) {
	fakeNoRepoAuth(t)

	dir := t.TempDir()

	testutil.WriteConfig(t, dir, "license: BlueOak-1.0.0\n")

	cmd := FitCmd{Path: dir, License: "BlueOak-1.0.0"}
	err := cmd.Run()
	if err == nil {
		t.Fatal("Run() expected error, got nil")
	}

	wantMsg := ".tailor.yml already exists at " + dir
	if !strings.Contains(err.Error(), wantMsg) {
		t.Errorf("error = %q, want substring %q", err.Error(), wantMsg)
	}
	if !strings.Contains(err.Error(), "edit it directly to change swatch configuration") {
		t.Errorf("error missing edit guidance: %q", err.Error())
	}
}

func TestFitLicenseNone(t *testing.T) {
	fakeNoRepoAuth(t)

	dir := filepath.Join(t.TempDir(), "license-none")

	cmd := FitCmd{Path: dir, License: "none"}
	if err := cmd.Run(); err != nil {
		t.Fatalf("Run() error: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(dir, ".tailor.yml"))
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}

	if !strings.Contains(string(data), "license: none") {
		t.Errorf("config does not contain 'license: none':\n%s", data)
	}
}

func TestFitDescriptionNoRepoContext(t *testing.T) {
	for _, description := range []string{"My project description", ""} {
		t.Run(description, func(t *testing.T) {
			fakeNoRepoAuth(t)
			dir := filepath.Join(t.TempDir(), "with-desc")
			var stdout, stderr strings.Builder
			if code := run([]string{"fit", dir, "--description=" + description}, &stdout, &stderr); code != 0 {
				t.Fatalf("fit = %d, stderr: %s", code, stderr.String())
			}
			cfg, err := config.Load(dir)
			if err != nil {
				t.Fatal(err)
			}
			testutil.AssertPtrEqual(t, cfg.Repository.Description, new(description), "description")
		})
	}
}

func TestFitNoRepoContextUsesDefaults(t *testing.T) {
	fakeNoRepoAuth(t)

	dir := filepath.Join(t.TempDir(), "defaults")

	cmd := FitCmd{Path: dir, License: "BlueOak-1.0.0"}
	if err := cmd.Run(); err != nil {
		t.Fatalf("Run() error: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(dir, ".tailor.yml"))
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	content := string(data)

	if !strings.Contains(content, "repository:") {
		t.Error("config missing repository section")
	}

	// The embedded defaults leave both merge_commit fields nil, so neither is written.
	// The leading newline and spaces exclude squash_merge_commit_title matches.
	if strings.Contains(content, "\n  merge_commit_title:") {
		t.Error("default config should not contain merge_commit_title")
	}
	if strings.Contains(content, "\n  merge_commit_message:") {
		t.Error("default config should not contain merge_commit_message")
	}

	// Repository description defaults to the target directory name.
	// Use leading whitespace to match only the repository-level field,
	// not the description key inside label entries.
	if !strings.Contains(content, "\n  description: defaults") {
		t.Error("default config should contain the directory name as description")
	}

	// No repository context means no URL to default the homepage to.
	if strings.Contains(content, "\n  homepage:") {
		t.Errorf("config should not contain homepage without repo context:\n%s", content)
	}
}

// setupSwatchCommandTest fakes the auth and API paths that runAlter needs,
// writes a three-swatch config with pre-existing first-fit and never files
// into a fresh project directory, and changes into that directory.
func setupSwatchCommandTest(t *testing.T) string {
	t.Helper()

	fakeNoRepoAuth(t)

	dir := t.TempDir()
	cfg := `license: none
swatches:
  - path: .envrc
    alteration: first-fit
  - path: .github/pull_request_template.md
    alteration: never
  - path: SUPPORT.md
    alteration: always
`
	testutil.WriteConfig(t, dir, cfg)
	if err := os.MkdirAll(filepath.Join(dir, ".github"), 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	testutil.WriteFile(t, dir, ".envrc", "custom envrc\n")
	testutil.WriteFile(t, dir, ".github/pull_request_template.md", "custom pull request template\n")
	t.Chdir(dir)
	return dir
}

func requireFileContent(t *testing.T, path, want string) {
	t.Helper()
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile(%q): %v", path, err)
	}
	if string(got) != want {
		t.Errorf("ReadFile(%q) = %q, want %q", path, got, want)
	}
}

func requireSwatchContent(t *testing.T, dir, path string) {
	t.Helper()
	want, err := swatch.Content(path)
	if err != nil {
		t.Fatalf("swatch.Content(%q): %v", path, err)
	}
	requireFileContent(t, filepath.Join(dir, path), string(want))
}

func TestRunBaste(t *testing.T) {
	dir := setupSwatchCommandTest(t)

	var stdout, stderr strings.Builder
	code := run([]string{"baste"}, &stdout, &stderr)

	if code != 0 {
		t.Fatalf("run() = %d, want 0; stderr: %s", code, stderr.String())
	}
	want := "would copy:                          SUPPORT.md\n" +
		"skipped:                             .envrc (first-fit, exists)\n" +
		"skipped:                             .github/pull_request_template.md (mode never)\n"
	if stdout.String() != want {
		t.Errorf("stdout =\n%s\nwant:\n%s", stdout.String(), want)
	}
	requireFileContent(t, filepath.Join(dir, ".envrc"), "custom envrc\n")
	requireFileContent(t, filepath.Join(dir, ".github/pull_request_template.md"), "custom pull request template\n")
	if _, err := os.Stat(filepath.Join(dir, "SUPPORT.md")); !os.IsNotExist(err) {
		t.Errorf("Stat(SUPPORT.md) error = %v, want file not to exist", err)
	}
}

func TestRunAlter(t *testing.T) {
	dir := setupSwatchCommandTest(t)

	var stdout, stderr strings.Builder
	code := run([]string{"alter"}, &stdout, &stderr)

	if code != 0 {
		t.Fatalf("run() = %d, want 0; stderr: %s", code, stderr.String())
	}
	want := "copied:                              SUPPORT.md\n" +
		"skipped:                             .envrc (first-fit, exists)\n" +
		"skipped:                             .github/pull_request_template.md (mode never)\n"
	if stdout.String() != want {
		t.Errorf("stdout =\n%s\nwant:\n%s", stdout.String(), want)
	}
	requireFileContent(t, filepath.Join(dir, ".envrc"), "custom envrc\n")
	requireFileContent(t, filepath.Join(dir, ".github/pull_request_template.md"), "custom pull request template\n")
	requireSwatchContent(t, dir, "SUPPORT.md")
}

func TestRunAlterRecut(t *testing.T) {
	dir := setupSwatchCommandTest(t)

	var stdout, stderr strings.Builder
	code := run([]string{"alter", "--recut"}, &stdout, &stderr)

	if code != 0 {
		t.Fatalf("run() = %d, want 0; stderr: %s", code, stderr.String())
	}
	want := "copied:                              SUPPORT.md\n" +
		"overwritten:                         .envrc\n" +
		"skipped:                             .github/pull_request_template.md (mode never)\n"
	if stdout.String() != want {
		t.Errorf("stdout =\n%s\nwant:\n%s", stdout.String(), want)
	}
	requireSwatchContent(t, dir, ".envrc")
	requireFileContent(t, filepath.Join(dir, ".github/pull_request_template.md"), "custom pull request template\n")
	requireSwatchContent(t, dir, "SUPPORT.md")
}

func TestRunAlterMalformedConfigError(t *testing.T) {
	ghfake.FakeAuth(t, "gho_test")
	ghfake.FakeNoRepo(t)

	dir := t.TempDir()
	testutil.WriteConfig(t, dir, "unknown: true\n")
	t.Chdir(dir)

	err := runAlter(alter.DryRun, io.Discard, io.Discard)
	if err == nil {
		t.Fatal("runAlter() expected error, got nil")
	}
	if !strings.Contains(err.Error(), `unrecognised top-level setting "unknown"`) {
		t.Errorf("error = %q, want underlying config error", err.Error())
	}
	if !strings.Contains(err.Error(), "Run 'tailor fit <path>' to create a valid configuration") {
		t.Errorf("error = %q, want fit guidance", err.Error())
	}
}

func TestMeasureCmdRejectsInvalidConfigPath(t *testing.T) {
	tests := []struct {
		name   string
		create func(t *testing.T, path string)
	}{
		{
			name: "directory",
			create: func(t *testing.T, path string) {
				t.Helper()
				if err := os.Mkdir(path, 0o755); err != nil {
					t.Fatalf("Mkdir: %v", err)
				}
			},
		},
		{
			name: "broken symlink",
			create: func(t *testing.T, path string) {
				t.Helper()
				if err := os.Symlink(filepath.Join(filepath.Dir(path), "missing.yml"), path); err != nil {
					t.Skipf("Symlink unavailable: %v", err)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			tt.create(t, filepath.Join(dir, ".tailor.yml"))
			t.Chdir(dir)

			err := (&MeasureCmd{}).Run()
			want := "loading config: reading config: .tailor.yml is not a regular file"
			if err == nil || err.Error() != want {
				t.Fatalf("MeasureCmd.Run() error = %v, want %q", err, want)
			}
		})
	}
}

func TestDocketAuthenticated(t *testing.T) {
	ghfake.FakeAuth(t, "gho_test")
	ghfake.FakeRepo(t, "octocat", "my-project")

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/user"):
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]string{"login": "octocat"})
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(srv.Close)

	cmd := DocketCmd{client: testutil.NewTestClient(t, srv)}
	if err := cmd.Run(); err != nil {
		t.Fatalf("DocketCmd.Run() error: %v", err)
	}
}

func TestDocketNotAuthenticated(t *testing.T) {
	ghfake.FakeAuth(t, "")
	ghfake.FakeNoRepo(t)

	cmd := DocketCmd{}
	if err := cmd.Run(); err != nil {
		t.Fatalf("DocketCmd.Run() error: %v", err)
	}
}

func TestRunNoArguments(t *testing.T) {
	var stdout, stderr strings.Builder

	code := run(nil, &stdout, &stderr)

	if code != 80 {
		t.Errorf("run() = %d, want 80", code)
	}
	if !strings.Contains(stderr.String(), "expected one of") {
		t.Errorf("stderr = %q, want missing-command error", stderr.String())
	}
	if !strings.Contains(stdout.String(), "Usage: tailor") {
		t.Errorf("stdout = %q, want usage output", stdout.String())
	}
}

func TestRunHelp(t *testing.T) {
	var stdout, stderr strings.Builder

	code := run([]string{"--help"}, &stdout, &stderr)

	if code != 0 {
		t.Errorf("run() = %d, want 0", code)
	}
	if !strings.Contains(stdout.String(), "Usage: tailor") {
		t.Errorf("stdout = %q, want usage output", stdout.String())
	}
	if !strings.Contains(stdout.String(), "Bespoke project templates for GitHub repositories.") {
		t.Errorf("stdout = %q, want description", stdout.String())
	}
	for _, command := range []string{"fit", "alter", "baste", "measure", "docket"} {
		if !strings.Contains(stdout.String(), command) {
			t.Errorf("stdout missing command %q", command)
		}
	}
	if stderr.Len() != 0 {
		t.Errorf("stderr = %q, want empty", stderr.String())
	}
}

func TestRunVersion(t *testing.T) {
	var stdout, stderr strings.Builder

	code := run([]string{"--version"}, &stdout, &stderr)

	if code != 0 {
		t.Errorf("run() = %d, want 0", code)
	}
	if stdout.String() != version+"\n" {
		t.Errorf("stdout = %q, want %q", stdout.String(), version+"\n")
	}
	if stderr.Len() != 0 {
		t.Errorf("stderr = %q, want empty", stderr.String())
	}
}

func TestRunUnknownFlag(t *testing.T) {
	var stdout, stderr strings.Builder

	code := run([]string{"--bogus"}, &stdout, &stderr)

	if code != 80 {
		t.Errorf("run() = %d, want 80", code)
	}
	if !strings.Contains(stderr.String(), "unknown flag --bogus") {
		t.Errorf("stderr = %q, want unknown flag error", stderr.String())
	}
}

func TestRunBasteUsesCommandWriters(t *testing.T) {
	ghfake.FakeAuth(t, "gho_test")
	ghfake.FakeNoRepo(t)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/user") {
			fmt.Fprint(w, `{"login":"octocat"}`)
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	t.Cleanup(srv.Close)
	restore := gh.SetNewRESTClientFunc(func(string) (*api.RESTClient, error) {
		return testutil.NewTestClient(t, srv), nil
	})
	t.Cleanup(restore)

	dir := t.TempDir()
	configYAML := "license: none\nswatches:\n  - path: .gitignore\n    alteration: first-fit\n"
	testutil.WriteConfig(t, dir, configYAML)
	t.Chdir(dir)

	var stdout, stderr strings.Builder
	code := run([]string{"baste"}, &stdout, &stderr)

	if code != 0 {
		t.Fatalf("run() = %d, want 0; stderr: %s", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "would copy:                          .gitignore\n") {
		t.Errorf("stdout = %q, want .gitignore preview", stdout.String())
	}
	if !strings.HasPrefix(stderr.String(), "No licence file found and no licence configured.") {
		t.Errorf("stderr = %q, want missing licence warning", stderr.String())
	}
}

func TestFitAuthFailure(t *testing.T) {
	ghfake.FakeAuth(t, "")

	dir := filepath.Join(t.TempDir(), "auth-fail")

	cmd := FitCmd{Path: dir, License: "BlueOak-1.0.0"}
	err := cmd.Run()
	if err == nil {
		t.Fatal("Run() expected error, got nil")
	}

	wantMsg := "tailor requires GitHub authentication. Set the GH_TOKEN or GITHUB_TOKEN environment variable, or run 'gh auth login'"
	if !strings.Contains(err.Error(), wantMsg) {
		t.Errorf("error = %q, want substring %q", err.Error(), wantMsg)
	}

	// Auth failure stops before project directory creation.
	if _, statErr := os.Stat(dir); statErr == nil {
		t.Error("directory was created despite auth failure")
	}
}

func TestFitInvalidTokenFailure(t *testing.T) {
	ghfake.FakeAuth(t, "gho_invalid")
	ghfake.FakeUserAPI(t, http.StatusUnauthorized, "")

	dir := filepath.Join(t.TempDir(), "invalid-token")

	cmd := FitCmd{Path: dir, License: "BlueOak-1.0.0"}
	err := cmd.Run()
	if err == nil {
		t.Fatal("Run() expected error, got nil")
	}
	if !strings.Contains(err.Error(), "verifying GitHub authentication") {
		t.Errorf("error = %q, want substring %q", err.Error(), "verifying GitHub authentication")
	}

	// Token verification failure stops before project directory creation.
	if _, statErr := os.Stat(dir); statErr == nil {
		t.Error("directory was created despite invalid token")
	}
}
