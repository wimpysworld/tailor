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

// fitRulesets controls the fake rulesets endpoints that fit reads.
type fitRulesets struct {
	listStatus int    // zero: 200
	listBody   string // empty: no rulesets
	readBody   string // served for GET /rulesets/1
}

// fitSetupServer fakes the endpoints that fit reads. setupStatus and the two
// bodies control the code scanning and Code Quality setup reads. rulesets
// controls the rulesets reads; a zero value lists no rulesets.
func fitSetupServer(t *testing.T, setupStatus int, codeScanningJSON, codeQualityJSON string, rulesets fitRulesets) {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case strings.HasSuffix(r.URL.Path, "/rulesets"):
			if rulesets.listStatus != 0 {
				w.WriteHeader(rulesets.listStatus)
				fmt.Fprint(w, `{"message":"error"}`)
				return
			}
			if rulesets.listBody == "" {
				fmt.Fprint(w, `[]`)
				return
			}
			fmt.Fprint(w, rulesets.listBody)
		case strings.HasSuffix(r.URL.Path, "/rulesets/1"):
			fmt.Fprint(w, rulesets.readBody)
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
				`"security_and_analysis":{"secret_scanning":{"status":"enabled"},"secret_scanning_push_protection":{"status":"disabled"},"secret_scanning_non_provider_patterns":{"status":"enabled"}}}`)
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
	fitSetupServer(t, http.StatusOK, liveCodeScanningJSON, liveCodeQualityJSON, fitRulesets{})

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
		"  secret_scanning: enabled\n  secret_scanning_push_protection: disabled\n  secret_scanning_non_provider_patterns: enabled\n",
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
	fitSetupServer(t, http.StatusOK, `{"state":"not-configured","languages":[]}`, `{"state":"not-configured","languages":[]}`, fitRulesets{})

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
	fitSetupServer(t, http.StatusInternalServerError, `{"message":"boom"}`, `{"message":"boom"}`, fitRulesets{})

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
	fitSetupServer(t, http.StatusNotFound, `{"message":"Not Found"}`, `{"message":"Not Found"}`, fitRulesets{})

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

const (
	tailorRulesetList = `[{"id":1,"name":"Tailor","target":"branch","enforcement":"disabled","source_type":"Repository"}]`
	liveRulesetJSON   = `{"id":1,"name":"Tailor","target":"branch","enforcement":"disabled",` +
		`"bypass_actors":[{"actor_id":4,"actor_type":"RepositoryRole","bypass_mode":"pull_request"},{"actor_id":null,"actor_type":"DeployKey","bypass_mode":"always"}],` +
		`"conditions":{"ref_name":{"include":["refs/heads/main","release/*"],"exclude":["refs/heads/wip/*"]}},` +
		`"rules":[{"type":"creation"},{"type":"required_status_checks","parameters":{"strict_required_status_checks_policy":true,"do_not_enforce_on_create":false,` +
		`"required_status_checks":[{"context":"lint","integration_id":15368}]}},` +
		`{"type":"code_scanning","parameters":{"code_scanning_tools":[{"tool":"CodeQL","alerts_threshold":"errors_and_warnings","security_alerts_threshold":"critical"}]}}]}`
)

func TestFitWritesLiveRuleset(t *testing.T) {
	ghfake.FakeAuth(t, "gho_test")
	ghfake.FakeRepo(t, "octocat", "my-project")
	fitSetupServer(t, http.StatusOK, liveCodeScanningJSON, liveCodeQualityJSON, fitRulesets{listBody: tailorRulesetList, readBody: liveRulesetJSON})

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
		"\nruleset:\n",
		"  enforcement: disabled\n",
		"    - actor_id: 4\n      actor_type: RepositoryRole\n      bypass_mode: pull_request\n    - actor_type: DeployKey\n      bypass_mode: always\n",
		"      include:\n        - refs/heads/main\n        - \"release/*\"\n      exclude:\n        - \"refs/heads/wip/*\"\n",
		"    creation: true\n    update: false\n    deletion: false\n",
		// The live ruleset has no pull request rule, so the built-in
		// parameters stay for the day the rule is enabled.
		"    pull_request:\n      enabled: false\n      parameters:\n        required_approving_review_count: 1\n",
		"    required_status_checks:\n      enabled: true\n      parameters:\n        # Require branches to be up to date before merging.\n        strict_required_status_checks_policy: true\n",
		"        required_status_checks:\n          - context: lint\n            integration_id: 15368\n",
		"    code_scanning:\n      enabled: true\n      parameters:\n" +
			"        # tool is the tool name as GitHub shows it, for example CodeQL.\n" +
			"        # alerts_threshold: none, errors, errors_and_warnings, all\n" +
			"        # security_alerts_threshold: none, critical, high_or_higher, medium_or_higher, all\n" +
			"        code_scanning_tools:\n          - tool: CodeQL\n            alerts_threshold: errors_and_warnings\n            security_alerts_threshold: critical\n",
	} {
		if !strings.Contains(content, want) {
			t.Errorf("config missing %q:\n%s", want, content)
		}
	}
	if _, err := config.Load(dir); err != nil {
		t.Fatalf("config.Load() error: %v", err)
	}
}

func TestFitRulesetWithoutCodeScanningKeepsBuiltInTools(t *testing.T) {
	ghfake.FakeAuth(t, "gho_test")
	ghfake.FakeRepo(t, "octocat", "my-project")
	withoutRule := strings.Replace(liveRulesetJSON,
		`,{"type":"code_scanning","parameters":{"code_scanning_tools":[{"tool":"CodeQL","alerts_threshold":"errors_and_warnings","security_alerts_threshold":"critical"}]}}`, "", 1)
	if withoutRule == liveRulesetJSON {
		t.Fatal("liveRulesetJSON does not carry the code scanning rule")
	}
	fitSetupServer(t, http.StatusOK, liveCodeScanningJSON, liveCodeQualityJSON, fitRulesets{listBody: tailorRulesetList, readBody: withoutRule})

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
	want := "    code_scanning:\n      enabled: false\n      parameters:\n" +
		"        # tool is the tool name as GitHub shows it, for example CodeQL.\n" +
		"        # alerts_threshold: none, errors, errors_and_warnings, all\n" +
		"        # security_alerts_threshold: none, critical, high_or_higher, medium_or_higher, all\n" +
		"        code_scanning_tools:\n          - tool: CodeQL\n            alerts_threshold: errors\n            security_alerts_threshold: high_or_higher\n"
	if !strings.Contains(content, want) {
		t.Errorf("config missing the built-in code scanning block %q:\n%s", want, content)
	}
	if _, err := config.Load(dir); err != nil {
		t.Fatalf("config.Load() error: %v", err)
	}
}

func TestFitRulesetFallbackUsesBuiltIn(t *testing.T) {
	tests := []struct {
		name       string
		rulesets   fitRulesets
		wantStderr string
	}{
		{name: "absent", rulesets: fitRulesets{}},
		{name: "other rulesets only", rulesets: fitRulesets{listBody: `[{"id":9,"name":"Other"}]`}},
		{name: "forbidden", rulesets: fitRulesets{listStatus: http.StatusForbidden}, wantStderr: "warning: list rulesets: not available (HTTP 403)\n"},
		{
			name:       "bypass actors omitted",
			rulesets:   fitRulesets{listBody: tailorRulesetList, readBody: `{"id":1,"name":"Tailor","enforcement":"active","conditions":{"ref_name":{"include":["~ALL"],"exclude":[]}},"rules":[]}`},
			wantStderr: "warning: fetch ruleset: response omitted bypass_actors; the token cannot manage the ruleset\n",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ghfake.FakeAuth(t, "gho_test")
			ghfake.FakeRepo(t, "octocat", "my-project")
			fitSetupServer(t, http.StatusOK, liveCodeScanningJSON, liveCodeQualityJSON, tt.rulesets)

			dir := t.TempDir()
			var stdout, stderr strings.Builder
			if code := run([]string{"fit", dir}, &stdout, &stderr); code != 0 {
				t.Fatalf("run() = %d, want 0; stderr: %s", code, stderr.String())
			}
			if stderr.String() != tt.wantStderr {
				t.Errorf("stderr = %q, want %q", stderr.String(), tt.wantStderr)
			}
			data, err := os.ReadFile(filepath.Join(dir, ".tailor.yml"))
			if err != nil {
				t.Fatal(err)
			}
			content := string(data)
			for _, want := range []string{
				"\nruleset:\n",
				"  enforcement: active\n",
				"    - actor_id: 5\n      actor_type: RepositoryRole\n      bypass_mode: always\n",
				"      include:\n        - ~DEFAULT_BRANCH\n      exclude: []\n",
				"    pull_request:\n      enabled: true\n",
			} {
				if !strings.Contains(content, want) {
					t.Errorf("config missing built-in ruleset text %q:\n%s", want, content)
				}
			}
		})
	}
}

func TestFitRulesetEvaluateEnforcementKeepsBuiltIn(t *testing.T) {
	ghfake.FakeAuth(t, "gho_test")
	ghfake.FakeRepo(t, "octocat", "my-project")
	evaluateRulesetJSON := strings.Replace(liveRulesetJSON, `"enforcement":"disabled"`, `"enforcement":"evaluate"`, 1)
	fitSetupServer(t, http.StatusOK, liveCodeScanningJSON, liveCodeQualityJSON, fitRulesets{listBody: tailorRulesetList, readBody: evaluateRulesetJSON})

	dir := t.TempDir()
	var stdout, stderr strings.Builder
	if code := run([]string{"fit", dir}, &stdout, &stderr); code != 0 {
		t.Fatalf("run() = %d, want 0; stderr: %s", code, stderr.String())
	}
	want := "warning: the Tailor ruleset enforcement \"evaluate\" is not managed; wrote enforcement: active\n"
	if stderr.String() != want {
		t.Errorf("stderr = %q, want %q", stderr.String(), want)
	}
	data, err := os.ReadFile(filepath.Join(dir, ".tailor.yml"))
	if err != nil {
		t.Fatal(err)
	}
	content := string(data)
	for _, want := range []string{
		"  enforcement: active\n",
		"    creation: true\n",
	} {
		if !strings.Contains(content, want) {
			t.Errorf("config missing %q:\n%s", want, content)
		}
	}
	if _, err := config.Load(dir); err != nil {
		t.Fatalf("config.Load() error: %v", err)
	}
}

func TestFitRulesetHardErrorStopsCommand(t *testing.T) {
	ghfake.FakeAuth(t, "gho_test")
	ghfake.FakeRepo(t, "octocat", "my-project")
	fitSetupServer(t, http.StatusOK, liveCodeScanningJSON, liveCodeQualityJSON, fitRulesets{listStatus: http.StatusInternalServerError})

	dir := t.TempDir()
	var stdout, stderr strings.Builder
	if code := run([]string{"fit", dir}, &stdout, &stderr); code == 0 {
		t.Fatal("run() = 0, want failure for a 500 rulesets read")
	}
	if !strings.Contains(stderr.String(), "list rulesets") {
		t.Errorf("stderr = %q, want the API error", stderr.String())
	}
	if _, err := os.Stat(filepath.Join(dir, ".tailor.yml")); err == nil {
		t.Error("fit wrote .tailor.yml despite the failed rulesets read")
	}
}
