package alter_test

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/wimpysworld/tailor/internal/alter"
)

func TestAlterRunSecretScanningPrerequisiteWarning(t *testing.T) {
	configYAML := `license: none
repository:
  secret_scanning_push_protection: enabled
swatches: []
`
	tc := setupAlterTest(t, configYAML)
	cfg := loadTestConfig(t, tc.Dir)

	var stdout, stderr strings.Builder
	if err := alter.Run(cfg, tc.Dir, alter.DryRun, tc.Client, &stdout, &stderr); err != nil {
		t.Fatalf("alter.Run() error: %v", err)
	}
	want := "warning: set secret_scanning to enabled because secret_scanning_push_protection requires secret scanning\n"
	if !strings.Contains(stderr.String(), want) {
		t.Errorf("stderr = %q, want %q", stderr.String(), want)
	}
	if cfg.Repository.SecretScanning == nil || *cfg.Repository.SecretScanning != "enabled" {
		t.Errorf("secret_scanning = %v, want enabled", cfg.Repository.SecretScanning)
	}
}

func TestAlterRunWritesNormalisedSecretScanning(t *testing.T) {
	configYAML := `license: none
repository:
  secret_scanning_push_protection: enabled
swatches: []
`
	tc := setupAlterTest(t, configYAML)
	cfg := loadTestConfig(t, tc.Dir)
	_ = captureAlterRun(t, cfg, tc.Dir, alter.Apply, tc.Client)

	data, err := os.ReadFile(filepath.Join(tc.Dir, ".tailor.yml"))
	if err != nil {
		t.Fatal(err)
	}
	content := string(data)
	for _, want := range []string{"Refitted by tailor on", "  secret_scanning: enabled\n  secret_scanning_push_protection: enabled\n"} {
		if !strings.Contains(content, want) {
			t.Errorf("rewritten config missing %q:\n%s", want, content)
		}
	}
}

func TestAlterRunSecretScanningAbsentBlock(t *testing.T) {
	configYAML := `license: none
repository:
  has_wiki: false
  secret_scanning: enabled
  secret_scanning_push_protection: enabled
swatches: []
`
	tc := setupAlterTest(t, configYAML, WithRepoSettings(repoJSON{HasWiki: true}))
	cfg := loadTestConfig(t, tc.Dir)
	output := captureAlterRun(t, cfg, tc.Dir, alter.Apply, tc.Client)

	requireContains(t, output, "set:                                                           repository.has_wiki = false\n")
	requireContains(t, output, "would skip (insufficient scope: token missing required scope): secret_scanning\n")
	requireContains(t, output, "would skip (insufficient scope: token missing required scope): secret_scanning_push_protection\n")
	if strings.Contains(output, "secret_scanning = enabled") {
		t.Errorf("output reports a secret scanning write without admin access:\n%s", output)
	}

	patches := tc.MutatingCalls()
	if len(patches) != 1 || patches[0].Path != "/repos/testowner/testrepo" {
		t.Fatalf("mutating calls = %v, want one repository PATCH", patches)
	}
	if patches[0].Body != `{"has_wiki":false}` {
		t.Errorf("PATCH body = %s, want has_wiki only", patches[0].Body)
	}
}

func TestAlterRunSecretScanningPresentBlock(t *testing.T) {
	configYAML := `license: none
repository:
  secret_scanning: enabled
  secret_scanning_push_protection: enabled
swatches: []
`
	live := &securityAndAnalysisJSON{
		SecretScanning:               securityStatusJSON{Status: "enabled"},
		SecretScanningPushProtection: securityStatusJSON{Status: "disabled"},
	}
	tc := setupAlterTest(t, configYAML, WithRepoSettings(repoJSON{SecurityAndAnalysis: live}))
	cfg := loadTestConfig(t, tc.Dir)
	output := captureAlterRun(t, cfg, tc.Dir, alter.Apply, tc.Client)

	requireContains(t, output, "repository.secret_scanning (already enabled)")
	requireContains(t, output, "repository.secret_scanning_push_protection = enabled")

	patches := tc.MutatingCalls()
	if len(patches) != 1 {
		t.Fatalf("mutating calls = %v, want one repository PATCH", patches)
	}
	want := `{"security_and_analysis":{"secret_scanning_push_protection":{"status":"enabled"}}}`
	if patches[0].Body != want {
		t.Errorf("PATCH body = %s, want %s", patches[0].Body, want)
	}
}

func TestAlterRunStageOrder(t *testing.T) {
	configYAML := `license: none
repository:
  has_wiki: false
actions:
  enabled: true
code_scanning:
  state: configured
code_quality:
  state: configured
ruleset:
  enforcement: active
  bypass_actors: []
  conditions:
    ref_name:
      include:
        - ~DEFAULT_BRANCH
      exclude: []
  rules:
    creation: false
    update: false
    deletion: true
    required_linear_history: false
    required_signatures: false
    non_fast_forward: true
    pull_request:
      enabled: false
    required_status_checks:
      enabled: false
labels:
  - name: bug
    color: d73a4a
    description: A problem
swatches: []
`
	tc := setupAlterTest(t, configYAML)
	cfg := loadTestConfig(t, tc.Dir)
	output := captureAlterRun(t, cfg, tc.Dir, alter.Apply, tc.Client)
	requireContains(t, output, "code_scanning.state = configured")
	requireContains(t, output, "code_quality.state = configured")
	requireContains(t, output, "ruleset.enforcement = active")

	var reads []string
	for _, call := range tc.Calls() {
		if call.Method != http.MethodGet {
			continue
		}
		reads = append(reads, call.Path)
	}
	want := []string{
		"/repos/testowner/testrepo/actions/permissions",
		"/repos/testowner/testrepo/code-scanning/default-setup",
		"/repos/testowner/testrepo/code-quality/setup",
		"/repos/testowner/testrepo/rulesets",
		"/repos/testowner/testrepo/labels",
	}
	if got := ordered(reads, want); !got {
		t.Errorf("GET order = %v, want %v in order", reads, want)
	}

	var writes []string
	for _, call := range tc.MutatingCalls() {
		writes = append(writes, call.Path)
	}
	for _, path := range []string{"/repos/testowner/testrepo/code-scanning/default-setup", "/repos/testowner/testrepo/code-quality/setup"} {
		if !ordered(writes, []string{path}) {
			t.Errorf("mutating calls = %v, missing %s", writes, path)
		}
	}
	var posts []string
	for _, call := range tc.Calls() {
		if call.Method == http.MethodPost {
			posts = append(posts, call.Path)
		}
	}
	if !ordered(posts, []string{"/repos/testowner/testrepo/rulesets", "/repos/testowner/testrepo/labels"}) {
		t.Errorf("POST order = %v, want the ruleset before labels", posts)
	}
}

// ordered reports whether want appears in got in order, ignoring other entries.
func ordered(got, want []string) bool {
	next := 0
	for _, path := range got {
		if next < len(want) && path == want[next] {
			next++
		}
	}
	return next == len(want)
}
