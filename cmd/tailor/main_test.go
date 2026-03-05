package main

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/cli/go-gh/v2/pkg/repository"
	"github.com/wimpysworld/tailor/internal/gh"
)

// fakeAuth installs a tokenForHost stub that returns the given token.
// It registers cleanup via t.Cleanup.
func fakeAuth(t *testing.T, token string) {
	t.Helper()
	restore := gh.SetTokenForHostFunc(func(string) (string, string) {
		return token, "oauth_token"
	})
	t.Cleanup(restore)
}

// fakeNoRepo installs a currentRepo stub that returns an error,
// simulating no GitHub remote.
func fakeNoRepo(t *testing.T) {
	t.Helper()
	restore := gh.SetCurrentRepoFunc(func() (repository.Repository, error) {
		return repository.Repository{}, errors.New("not a git repository")
	})
	t.Cleanup(restore)
}

func TestFitNewDirectoryDefaultConfig(t *testing.T) {
	fakeAuth(t, "gho_test")
	fakeNoRepo(t)

	dir := filepath.Join(t.TempDir(), "new-project")

	cmd := FitCmd{Path: dir, License: "MIT"}
	if err := cmd.Run(); err != nil {
		t.Fatalf("Run() error: %v", err)
	}

	configPath := filepath.Join(dir, ".tailor", "config.yml")
	data, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	content := string(data)

	// Verify license.
	if !strings.Contains(content, "license: MIT") {
		t.Error("config missing 'license: MIT'")
	}

	// Verify 16 swatches are present (count "- source:" occurrences).
	if count := strings.Count(content, "- source:"); count != 16 {
		t.Errorf("swatch count = %d, want 16", count)
	}

	// Verify the 14 default repo settings are present.
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
		"private_vulnerability_reporting_enabled:",
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
	fakeAuth(t, "gho_test")
	fakeNoRepo(t)

	dir := t.TempDir()

	cmd := FitCmd{Path: dir, License: "MIT"}
	if err := cmd.Run(); err != nil {
		t.Fatalf("Run() error: %v", err)
	}

	configPath := filepath.Join(dir, ".tailor", "config.yml")
	if _, err := os.Stat(configPath); err != nil {
		t.Fatalf("config file not created: %v", err)
	}
}

func TestFitExistingDirectoryWithConfigError(t *testing.T) {
	fakeAuth(t, "gho_test")
	fakeNoRepo(t)

	dir := t.TempDir()

	// Pre-create .tailor/config.yml.
	tailorDir := filepath.Join(dir, ".tailor")
	if err := os.MkdirAll(tailorDir, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(filepath.Join(tailorDir, "config.yml"), []byte("license: MIT\n"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	cmd := FitCmd{Path: dir, License: "MIT"}
	err := cmd.Run()
	if err == nil {
		t.Fatal("Run() expected error, got nil")
	}

	wantMsg := ".tailor/config.yml already exists at " + dir
	if !strings.Contains(err.Error(), wantMsg) {
		t.Errorf("error = %q, want substring %q", err.Error(), wantMsg)
	}
	if !strings.Contains(err.Error(), "edit it directly to change swatch configuration") {
		t.Errorf("error missing edit guidance: %q", err.Error())
	}
}

func TestFitLicenseNone(t *testing.T) {
	fakeAuth(t, "gho_test")
	fakeNoRepo(t)

	dir := filepath.Join(t.TempDir(), "license-none")

	cmd := FitCmd{Path: dir, License: "none"}
	if err := cmd.Run(); err != nil {
		t.Fatalf("Run() error: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(dir, ".tailor", "config.yml"))
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}

	if !strings.Contains(string(data), "license: none") {
		t.Errorf("config does not contain 'license: none':\n%s", data)
	}
}

func TestFitDescriptionNoRepoContext(t *testing.T) {
	fakeAuth(t, "gho_test")
	fakeNoRepo(t)

	dir := filepath.Join(t.TempDir(), "with-desc")

	cmd := FitCmd{Path: dir, License: "MIT", Description: "My project description"}
	if err := cmd.Run(); err != nil {
		t.Fatalf("Run() error: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(dir, ".tailor", "config.yml"))
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}

	if !strings.Contains(string(data), "description: My project description") {
		t.Errorf("config does not contain description:\n%s", data)
	}
}

func TestFitNoRepoContextUsesDefaults(t *testing.T) {
	fakeAuth(t, "gho_test")
	fakeNoRepo(t)

	dir := filepath.Join(t.TempDir(), "defaults")

	cmd := FitCmd{Path: dir, License: "MIT"}
	if err := cmd.Run(); err != nil {
		t.Fatalf("Run() error: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(dir, ".tailor", "config.yml"))
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

	// Description should be absent when not provided.
	if strings.Contains(content, "description:") {
		t.Error("default config should not contain description when not set")
	}
}

// setupAlterTest creates a temp directory with a minimal .tailor/config.yml,
// starts an httptest server that handles the API calls alter.Run makes,
// sets GH_TOKEN so go-gh creates a client, redirects http.DefaultTransport
// to the test server, and chdir to the temp directory.
func setupAlterTest(t *testing.T) string {
	t.Helper()
	fakeAuth(t, "gho_test")
	fakeNoRepo(t)

	dir := t.TempDir()
	tailorDir := filepath.Join(dir, ".tailor")
	if err := os.MkdirAll(tailorDir, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	cfg := "license: none\nswatches: []\n"
	if err := os.WriteFile(filepath.Join(tailorDir, "config.yml"), []byte(cfg), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/user"):
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]string{"login": "testuser"})
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(srv.Close)

	oldTransport := http.DefaultTransport
	http.DefaultTransport = &redirectTransport{
		target:   srv.URL,
		delegate: oldTransport,
	}
	t.Cleanup(func() { http.DefaultTransport = oldTransport })

	t.Setenv("GH_TOKEN", "gho_test")

	oldDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd: %v", err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("Chdir: %v", err)
	}
	t.Cleanup(func() { os.Chdir(oldDir) })

	return dir
}

// redirectTransport sends all requests to the test server, preserving the
// original path and query but rewriting scheme and host.
type redirectTransport struct {
	target   string // test server URL, e.g. "http://127.0.0.1:PORT"
	delegate http.RoundTripper
}

func (rt *redirectTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	req = req.Clone(req.Context())
	req.URL.Scheme = "http"
	req.URL.Host = strings.TrimPrefix(rt.target, "http://")
	return rt.delegate.RoundTrip(req)
}

func TestBasteCmdRun(t *testing.T) {
	setupAlterTest(t)
	cmd := BasteCmd{}
	if err := cmd.Run(); err != nil {
		t.Fatalf("BasteCmd.Run() error: %v", err)
	}
}

func TestAlterCmdRunRecut(t *testing.T) {
	setupAlterTest(t)
	cmd := AlterCmd{Recut: true}
	if err := cmd.Run(); err != nil {
		t.Fatalf("AlterCmd{Recut: true}.Run() error: %v", err)
	}
}

func TestDocketAuthenticated(t *testing.T) {
	fakeAuth(t, "gho_test")

	restore := gh.SetCurrentRepoFunc(func() (repository.Repository, error) {
		return repository.Parse("octocat/my-project")
	})
	t.Cleanup(restore)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/user"):
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]string{"login": "octocat"})
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(srv.Close)

	oldTransport := http.DefaultTransport
	http.DefaultTransport = &redirectTransport{
		target:   srv.URL,
		delegate: oldTransport,
	}
	t.Cleanup(func() { http.DefaultTransport = oldTransport })

	t.Setenv("GH_TOKEN", "gho_test")

	cmd := DocketCmd{}
	if err := cmd.Run(); err != nil {
		t.Fatalf("DocketCmd.Run() error: %v", err)
	}
}

func TestDocketNotAuthenticated(t *testing.T) {
	fakeAuth(t, "")
	fakeNoRepo(t)

	cmd := DocketCmd{}
	if err := cmd.Run(); err != nil {
		t.Fatalf("DocketCmd.Run() error: %v", err)
	}
}

func TestFitAuthFailure(t *testing.T) {
	fakeAuth(t, "")

	dir := filepath.Join(t.TempDir(), "auth-fail")

	cmd := FitCmd{Path: dir, License: "MIT"}
	err := cmd.Run()
	if err == nil {
		t.Fatal("Run() expected error, got nil")
	}

	wantMsg := "tailor requires GitHub authentication. Set the GH_TOKEN or GITHUB_TOKEN environment variable, or run 'gh auth login'"
	if !strings.Contains(err.Error(), wantMsg) {
		t.Errorf("error = %q, want substring %q", err.Error(), wantMsg)
	}

	// Directory should not have been created.
	if _, statErr := os.Stat(dir); statErr == nil {
		t.Error("directory was created despite auth failure")
	}
}
