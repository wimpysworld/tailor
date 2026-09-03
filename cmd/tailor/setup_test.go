package main

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/cli/go-gh/v2/pkg/api"
	"github.com/wimpysworld/tailor/internal/config"
	"github.com/wimpysworld/tailor/internal/gh"
	"github.com/wimpysworld/tailor/internal/ghfake"
	"github.com/wimpysworld/tailor/internal/testutil"
)

const (
	liveCodeScanningJSON = `{"state":"configured","languages":["actions","go"],"query_suite":"extended","threat_model":"remote_and_local","runner_type":"standard","runner_label":null}`
	liveCodeQualityJSON  = `{"state":"configured","languages":["go"],"runner_type":"standard","runner_label":null,"ai_findings_option":"disabled"}`
)

// fitSetupServer fakes the endpoints that fit reads. setupStatus and the two
// bodies control the code scanning and Code Quality setup reads.
func fitSetupServer(t *testing.T, setupStatus int, codeScanningJSON, codeQualityJSON string) {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case strings.HasSuffix(r.URL.Path, "/user"):
			fmt.Fprint(w, `{"login":"octocat"}`)
		case strings.HasSuffix(r.URL.Path, "/code-scanning/default-setup"):
			w.WriteHeader(setupStatus)
			fmt.Fprint(w, codeScanningJSON)
		case strings.HasSuffix(r.URL.Path, "/code-quality/setup"):
			w.WriteHeader(setupStatus)
			fmt.Fprint(w, codeQualityJSON)
		case strings.HasSuffix(r.URL.Path, "/actions/permissions/workflow"):
			fmt.Fprint(w, `{"default_workflow_permissions":"read","can_approve_pull_request_reviews":false}`)
		case strings.HasSuffix(r.URL.Path, "/private-vulnerability-reporting"),
			strings.HasSuffix(r.URL.Path, "/automated-security-fixes"):
			fmt.Fprint(w, `{"enabled":true}`)
		case strings.HasSuffix(r.URL.Path, "/vulnerability-alerts"):
			w.WriteHeader(http.StatusNoContent)
		case strings.HasSuffix(r.URL.Path, "/repos/octocat/my-project"):
			fmt.Fprint(w, `{"squash_merge_commit_title":"PR_TITLE","squash_merge_commit_message":"PR_BODY","merge_commit_title":"PR_TITLE","merge_commit_message":"PR_BODY",`+
				`"security_and_analysis":{"secret_scanning":{"status":"enabled"},"secret_scanning_push_protection":{"status":"disabled"}}}`)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(srv.Close)
	restore := gh.SetNewRESTClientFunc(func(string) (*api.RESTClient, error) {
		return testutil.NewTestClient(t, srv), nil
	})
	t.Cleanup(restore)
}

func TestFitWritesLiveSetupWithEmptyLanguages(t *testing.T) {
	ghfake.FakeAuth(t, "gho_test")
	ghfake.FakeRepo(t, "octocat", "my-project")
	fitSetupServer(t, http.StatusOK, liveCodeScanningJSON, liveCodeQualityJSON)

	dir := t.TempDir()
	var stdout, stderr strings.Builder
	if code := run([]string{"fit", dir}, &stdout, &stderr); code != 0 {
		t.Fatalf("run() = %d, want 0; stderr: %s", code, stderr.String())
	}
	if stderr.String() != "" {
		t.Errorf("stderr = %q, want empty", stderr.String())
	}

	data, err := os.ReadFile(filepath.Join(dir, ".tailor.yml"))
	if err != nil {
		t.Fatal(err)
	}
	content := string(data)
	for _, want := range []string{
		"  secret_scanning: enabled\n  secret_scanning_push_protection: disabled\n",
		"\ncode_scanning:\n  state: configured\n  query_suite: extended\n  threat_model: remote_and_local\n" +
			"  # An empty list means GitHub detects the languages. Valid values:\n" +
			"  # actions, c-cpp, csharp, go, java-kotlin, javascript-typescript, python, ruby, swift\n" +
			"  languages: []\n",
		"\ncode_quality:\n  state: configured\n" +
			"  # An empty list means GitHub detects the languages. Valid values:\n" +
			"  # csharp, go, java-kotlin, javascript-typescript, python, ruby\n" +
			"  languages: []\n",
	} {
		if !strings.Contains(content, want) {
			t.Errorf("config missing %q:\n%s", want, content)
		}
	}
}

func TestFitSetupEmptyFieldsKeepBuiltInDefaults(t *testing.T) {
	ghfake.FakeAuth(t, "gho_test")
	ghfake.FakeRepo(t, "octocat", "my-project")
	fitSetupServer(t, http.StatusOK, `{"state":"not-configured","languages":[]}`, `{"state":"not-configured","languages":[]}`)

	dir := t.TempDir()
	var stdout, stderr strings.Builder
	if code := run([]string{"fit", dir}, &stdout, &stderr); code != 0 {
		t.Fatalf("run() = %d, want 0; stderr: %s", code, stderr.String())
	}

	data, err := os.ReadFile(filepath.Join(dir, ".tailor.yml"))
	if err != nil {
		t.Fatal(err)
	}
	content := string(data)
	want := "\ncode_scanning:\n  state: not-configured\n  query_suite: default\n  threat_model: remote\n"
	if !strings.Contains(content, want) {
		t.Errorf("config missing %q:\n%s", want, content)
	}
	for _, empty := range []string{`query_suite: ""`, `threat_model: ""`, "query_suite: \n", "threat_model: \n"} {
		if strings.Contains(content, empty) {
			t.Errorf("config contains empty value %q:\n%s", empty, content)
		}
	}

	// The written config must load and validate on the next alter.
	if _, err := config.Load(dir); err != nil {
		t.Fatalf("config.Load() error: %v", err)
	}
}

func TestFitSetupReadHardErrorStopsCommand(t *testing.T) {
	ghfake.FakeAuth(t, "gho_test")
	ghfake.FakeRepo(t, "octocat", "my-project")
	fitSetupServer(t, http.StatusInternalServerError, `{"message":"boom"}`, `{"message":"boom"}`)

	dir := t.TempDir()
	var stdout, stderr strings.Builder
	if code := run([]string{"fit", dir}, &stdout, &stderr); code == 0 {
		t.Fatal("run() = 0, want failure for a 500 setup read")
	}
	if !strings.Contains(stderr.String(), "fetch code scanning setup") || !strings.Contains(stderr.String(), "boom") {
		t.Errorf("stderr = %q, want the API error", stderr.String())
	}
	if _, err := os.Stat(filepath.Join(dir, ".tailor.yml")); err == nil {
		t.Error("fit wrote .tailor.yml despite the failed setup read")
	}
}

func TestFitSetupNotAvailableUsesBuiltIn(t *testing.T) {
	ghfake.FakeAuth(t, "gho_test")
	ghfake.FakeRepo(t, "octocat", "my-project")
	fitSetupServer(t, http.StatusNotFound, `{"message":"Not Found"}`, `{"message":"Not Found"}`)

	dir := t.TempDir()
	var stdout, stderr strings.Builder
	if code := run([]string{"fit", dir}, &stdout, &stderr); code != 0 {
		t.Fatalf("run() = %d, want 0; stderr: %s", code, stderr.String())
	}
	wantStderr := "warning: fetch code scanning setup: not available (HTTP 404)\n" +
		"warning: fetch code quality setup: not available (HTTP 404)\n"
	if stderr.String() != wantStderr {
		t.Errorf("stderr = %q, want %q", stderr.String(), wantStderr)
	}

	data, err := os.ReadFile(filepath.Join(dir, ".tailor.yml"))
	if err != nil {
		t.Fatal(err)
	}
	content := string(data)
	for _, want := range []string{
		"\ncode_scanning:\n  state: configured\n  query_suite: default\n  threat_model: remote\n",
		"\ncode_quality:\n  state: not-configured\n",
	} {
		if !strings.Contains(content, want) {
			t.Errorf("config missing built-in section %q:\n%s", want, content)
		}
	}
}
