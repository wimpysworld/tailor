package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/wimpysworld/tailor/internal/alter"
	"github.com/wimpysworld/tailor/internal/config"
	"github.com/wimpysworld/tailor/internal/ghfake"
	"github.com/wimpysworld/tailor/internal/swatch"
	"github.com/wimpysworld/tailor/internal/testutil"
)

func TestFitNewDirectoryDefaultConfig(t *testing.T) {
	ghfake.FakeAuth(t, "gho_test")
	ghfake.FakeUserAPI(t, http.StatusOK, "octocat")
	ghfake.FakeNoRepo(t)

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

	// Licence uses the requested default.
	if !strings.Contains(content, "license: BlueOak-1.0.0") {
		t.Error("config missing 'license: BlueOak-1.0.0'")
	}

	// The default config includes all registered swatches.
	if count := strings.Count(content, "- path:"); count != 16 {
		t.Errorf("swatch count = %d, want 16", count)
	}

	// The default config includes the default repo settings.
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

	// Default config omits merge_commit_title and merge_commit_message.
	// Use leading newline+spaces to avoid matching squash_merge_commit_title.
	if strings.Contains(content, "\n  merge_commit_title:") {
		t.Error("default config should not contain merge_commit_title")
	}
	if strings.Contains(content, "\n  merge_commit_message:") {
		t.Error("default config should not contain merge_commit_message")
	}
}

func TestFitExistingDirectoryWithoutConfig(t *testing.T) {
	ghfake.FakeAuth(t, "gho_test")
	ghfake.FakeUserAPI(t, http.StatusOK, "octocat")
	ghfake.FakeNoRepo(t)

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

func TestFitExistingDirectoryWithConfigError(t *testing.T) {
	ghfake.FakeAuth(t, "gho_test")
	ghfake.FakeUserAPI(t, http.StatusOK, "octocat")
	ghfake.FakeNoRepo(t)

	dir := t.TempDir()

	// Pre-create .tailor.yml.
	if err := os.WriteFile(filepath.Join(dir, ".tailor.yml"), []byte("license: BlueOak-1.0.0\n"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

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
	ghfake.FakeAuth(t, "gho_test")
	ghfake.FakeUserAPI(t, http.StatusOK, "octocat")
	ghfake.FakeNoRepo(t)

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
	ghfake.FakeAuth(t, "gho_test")
	ghfake.FakeUserAPI(t, http.StatusOK, "octocat")
	ghfake.FakeNoRepo(t)

	dir := filepath.Join(t.TempDir(), "with-desc")

	cmd := FitCmd{Path: dir, License: "BlueOak-1.0.0", Description: "My project description"}
	if err := cmd.Run(); err != nil {
		t.Fatalf("Run() error: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(dir, ".tailor.yml"))
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}

	if !strings.Contains(string(data), "description: My project description") {
		t.Errorf("config does not contain description:\n%s", data)
	}
}

func TestFitNoRepoContextUsesDefaults(t *testing.T) {
	ghfake.FakeAuth(t, "gho_test")
	ghfake.FakeUserAPI(t, http.StatusOK, "octocat")
	ghfake.FakeNoRepo(t)

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

	// Default repo settings should be present.
	if !strings.Contains(content, "repository:") {
		t.Error("config missing repository section")
	}

	// merge_commit_title and merge_commit_message should be absent
	// because they are nil in the default embedded config.
	// Use leading newline+spaces to avoid matching squash_merge_commit_title.
	if strings.Contains(content, "\n  merge_commit_title:") {
		t.Error("default config should not contain merge_commit_title")
	}
	if strings.Contains(content, "\n  merge_commit_message:") {
		t.Error("default config should not contain merge_commit_message")
	}

	// Repository description should be absent when not provided.
	// Use leading whitespace to match only the repository-level field,
	// not the description key inside label entries.
	if strings.Contains(content, "\n  description:") {
		t.Error("default config should not contain repository description when not set")
	}
}

func setupSwatchCommandTest(t *testing.T) (string, *strings.Builder, func(alter.ApplyMode) error) {
	t.Helper()

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
	if err := os.WriteFile(filepath.Join(dir, ".tailor.yml"), []byte(cfg), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	for path, content := range map[string]string{
		".envrc":                           "custom envrc\n",
		".github/pull_request_template.md": "custom pull request template\n",
	} {
		fullPath := filepath.Join(dir, path)
		if err := os.MkdirAll(filepath.Dir(fullPath), 0o755); err != nil {
			t.Fatalf("MkdirAll: %v", err)
		}
		if err := os.WriteFile(fullPath, []byte(content), 0o644); err != nil {
			t.Fatalf("WriteFile: %v", err)
		}
	}

	var output strings.Builder
	run := func(mode alter.ApplyMode) error {
		cfg, err := config.Load(dir)
		if err != nil {
			return err
		}
		results, err := alter.ProcessSwatches(cfg, dir, mode, &alter.TokenContext{})
		if err != nil {
			return err
		}
		output.WriteString(alter.FormatOutput(nil, nil, results, mode))
		return nil
	}
	return dir, &output, run
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

func TestBasteCmdRun(t *testing.T) {
	dir, output, run := setupSwatchCommandTest(t)
	cmd := BasteCmd{run: run}
	if err := cmd.Run(); err != nil {
		t.Fatalf("BasteCmd.Run() error: %v", err)
	}

	want := "would copy:                          SUPPORT.md\n" +
		"skipped:                             .envrc (first-fit, exists)\n" +
		"skipped:                             .github/pull_request_template.md (mode never)\n"
	if output.String() != want {
		t.Errorf("stdout =\n%s\nwant:\n%s", output.String(), want)
	}
	requireFileContent(t, filepath.Join(dir, ".envrc"), "custom envrc\n")
	requireFileContent(t, filepath.Join(dir, ".github/pull_request_template.md"), "custom pull request template\n")
	if _, err := os.Stat(filepath.Join(dir, "SUPPORT.md")); !os.IsNotExist(err) {
		t.Errorf("Stat(SUPPORT.md) error = %v, want file not to exist", err)
	}
}

func TestAlterCmdRun(t *testing.T) {
	dir, output, run := setupSwatchCommandTest(t)
	cmd := AlterCmd{run: run}
	if err := cmd.Run(); err != nil {
		t.Fatalf("AlterCmd.Run() error: %v", err)
	}

	want := "copied:                              SUPPORT.md\n" +
		"skipped:                             .envrc (first-fit, exists)\n" +
		"skipped:                             .github/pull_request_template.md (mode never)\n"
	if output.String() != want {
		t.Errorf("stdout =\n%s\nwant:\n%s", output.String(), want)
	}
	requireFileContent(t, filepath.Join(dir, ".envrc"), "custom envrc\n")
	requireFileContent(t, filepath.Join(dir, ".github/pull_request_template.md"), "custom pull request template\n")
	wantSupport, err := swatch.Content("SUPPORT.md")
	if err != nil {
		t.Fatalf("swatch.Content(): %v", err)
	}
	requireFileContent(t, filepath.Join(dir, "SUPPORT.md"), string(wantSupport))
}

func TestAlterCmdRunRecut(t *testing.T) {
	dir, output, run := setupSwatchCommandTest(t)
	cmd := AlterCmd{Recut: true, run: run}
	if err := cmd.Run(); err != nil {
		t.Fatalf("AlterCmd{Recut: true}.Run() error: %v", err)
	}

	want := "copied:                              SUPPORT.md\n" +
		"overwritten:                         .envrc\n" +
		"skipped:                             .github/pull_request_template.md (mode never)\n"
	if output.String() != want {
		t.Errorf("stdout =\n%s\nwant:\n%s", output.String(), want)
	}
	wantEnvrc, err := swatch.Content(".envrc")
	if err != nil {
		t.Fatalf("swatch.Content(): %v", err)
	}
	requireFileContent(t, filepath.Join(dir, ".envrc"), string(wantEnvrc))
	requireFileContent(t, filepath.Join(dir, ".github/pull_request_template.md"), "custom pull request template\n")
	wantSupport, err := swatch.Content("SUPPORT.md")
	if err != nil {
		t.Fatalf("swatch.Content(): %v", err)
	}
	requireFileContent(t, filepath.Join(dir, "SUPPORT.md"), string(wantSupport))
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
