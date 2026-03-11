package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"

	"github.com/wimpysworld/tailor/internal/model"
	"github.com/wimpysworld/tailor/internal/ptr"
	"github.com/wimpysworld/tailor/internal/swatch"
)

// wantSpecOutput is the exact byte-for-byte expected output from the
// specification when writing DefaultConfig("BlueOak-1.0.0") with date 2026-03-02.
const wantSpecOutput = `# Initially fitted by tailor on 2026-03-02
license: BlueOak-1.0.0

repository:
  has_wiki: false
  has_discussions: false
  has_projects: false
  has_issues: true
  allow_merge_commit: false
  allow_squash_merge: true
  allow_rebase_merge: true
  squash_merge_commit_title: PR_TITLE
  squash_merge_commit_message: PR_BODY
  delete_branch_on_merge: true
  allow_update_branch: true
  allow_auto_merge: true
  web_commit_signoff_required: false
  private_vulnerability_reporting_enabled: true
  vulnerability_alerts_enabled: true
  automated_security_fixes_enabled: true
  default_workflow_permissions: read
  can_approve_pull_request_reviews: true

labels:
  - name: bug
    color: d20f39
    description: "Something isn't working"

  - name: documentation
    color: 04a5e5
    description: Documentation improvement

  - name: duplicate
    color: 8839ef
    description: Already exists

  - name: enhancement
    color: 1e66f5
    description: New feature request

  - name: good first issue
    color: 40a02b
    description: Good for newcomers

  - name: help wanted
    color: 179299
    description: Extra attention needed

  - name: invalid
    color: e64553
    description: Not valid or relevant

  - name: question
    color: 7287fd
    description: Needs more information

  - name: wontfix
    color: dc8a78
    description: Will not be worked on

  - name: dependencies
    color: fe640b
    description: Dependency update

  - name: github_actions
    color: ea76cb
    description: GitHub Actions update

  - name: hacktoberfest-accepted
    color: df8e1d
    description: Hacktoberfest contribution

swatches:
  - path: .github/workflows/tailor.yml
    alteration: always

  - path: .github/dependabot.yml
    alteration: first-fit

  - path: .github/FUNDING.yml
    alteration: first-fit

  - path: .github/ISSUE_TEMPLATE/bug_report.yml
    alteration: always

  - path: .github/ISSUE_TEMPLATE/feature_request.yml
    alteration: always

  - path: .github/ISSUE_TEMPLATE/config.yml
    alteration: first-fit

  - path: .github/pull_request_template.md
    alteration: always

  - path: SECURITY.md
    alteration: always

  - path: CODE_OF_CONDUCT.md
    alteration: always

  - path: CONTRIBUTING.md
    alteration: always

  - path: SUPPORT.md
    alteration: always

  - path: justfile
    alteration: first-fit

  - path: flake.nix
    alteration: first-fit

  - path: .gitignore
    alteration: first-fit

  - path: .envrc
    alteration: first-fit

  - path: .tailor.yml
    alteration: always

  - path: .github/workflows/tailor-automerge.yml
    alteration: triggered
`

func TestWriteDefaultConfigMatchesSpec(t *testing.T) {
	cfg, err := DefaultConfig("BlueOak-1.0.0")
	if err != nil {
		t.Fatalf("DefaultConfig: %v", err)
	}

	dir := t.TempDir()
	if err := Write(dir, cfg, "2026-03-02", "Initially fitted"); err != nil {
		t.Fatalf("Write: %v", err)
	}

	got, err := os.ReadFile(filepath.Join(dir, ".tailor.yml"))
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}

	if string(got) != wantSpecOutput {
		t.Errorf("output does not match spec\n--- got ---\n%s\n--- want ---\n%s", got, wantSpecOutput)
	}
}

func TestWriteCreatesFile(t *testing.T) {
	dir := t.TempDir()
	configFile := filepath.Join(dir, ".tailor.yml")

	// Confirm .tailor.yml does not exist before Write.
	if _, err := os.Stat(configFile); err == nil {
		t.Fatal(".tailor.yml already exists before Write")
	}

	cfg := &Config{
		License: "MIT",
		Repository: &model.RepositorySettings{
			HasWiki: ptr.Ptr(false),
		},
		Swatches: []SwatchEntry{
			{Path: "justfile", Alteration: swatch.FirstFit},
		},
	}

	if err := Write(dir, cfg, "2026-01-01", "Initially fitted"); err != nil {
		t.Fatalf("Write: %v", err)
	}

	info, err := os.Stat(configFile)
	if err != nil {
		t.Fatalf(".tailor.yml not created: %v", err)
	}
	if info.IsDir() {
		t.Error(".tailor.yml is a directory, want file")
	}
}

func TestWriteOptionalFieldsPresent(t *testing.T) {
	cfg := &Config{
		License: "Apache-2.0",
		Repository: &model.RepositorySettings{
			Description:                       ptr.Ptr("My project"),
			Homepage:                          ptr.Ptr("https://example.com"),
			HasWiki:                           ptr.Ptr(true),
			HasDiscussions:                    ptr.Ptr(false),
			HasProjects:                       ptr.Ptr(false),
			HasIssues:                         ptr.Ptr(true),
			AllowMergeCommit:                  ptr.Ptr(true),
			AllowSquashMerge:                  ptr.Ptr(true),
			AllowRebaseMerge:                  ptr.Ptr(false),
			SquashMergeCommitTitle:            ptr.Ptr("PR_TITLE"),
			SquashMergeCommitMessage:          ptr.Ptr("COMMIT_MESSAGES"),
			MergeCommitTitle:                  ptr.Ptr("PR_TITLE"),
			MergeCommitMessage:                ptr.Ptr("PR_BODY"),
			DeleteBranchOnMerge:               ptr.Ptr(true),
			AllowUpdateBranch:                 ptr.Ptr(true),
			AllowAutoMerge:                    ptr.Ptr(false),
			WebCommitSignoffRequired:          ptr.Ptr(true),
			PrivateVulnerabilityReportEnabled: ptr.Ptr(true),
			VulnerabilityAlertsEnabled:        ptr.Ptr(true),
			AutomatedSecurityFixesEnabled:     ptr.Ptr(false),
			DefaultWorkflowPermissions:        ptr.Ptr("write"),
			CanApprovePullRequestReviews:      ptr.Ptr(true),
		},
		Swatches: []SwatchEntry{
			{Path: "justfile", Alteration: swatch.FirstFit},
		},
	}

	want := `# Initially fitted by tailor on 2026-03-02
license: Apache-2.0

repository:
  description: My project
  homepage: https://example.com
  has_wiki: true
  has_discussions: false
  has_projects: false
  has_issues: true
  allow_merge_commit: true
  allow_squash_merge: true
  allow_rebase_merge: false
  squash_merge_commit_title: PR_TITLE
  squash_merge_commit_message: COMMIT_MESSAGES
  merge_commit_title: PR_TITLE
  merge_commit_message: PR_BODY
  delete_branch_on_merge: true
  allow_update_branch: true
  allow_auto_merge: false
  web_commit_signoff_required: true
  private_vulnerability_reporting_enabled: true
  vulnerability_alerts_enabled: true
  automated_security_fixes_enabled: false
  default_workflow_permissions: write
  can_approve_pull_request_reviews: true

swatches:
  - path: justfile
    alteration: first-fit
`

	dir := t.TempDir()
	if err := Write(dir, cfg, "2026-03-02", "Initially fitted"); err != nil {
		t.Fatalf("Write: %v", err)
	}

	got, err := os.ReadFile(filepath.Join(dir, ".tailor.yml"))
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}

	if string(got) != want {
		t.Errorf("output mismatch with optional fields present\n--- got ---\n%s\n--- want ---\n%s", got, want)
	}
}

func TestWriteOptionalFieldsOmitted(t *testing.T) {
	cfg := &Config{
		License: "MIT",
		Repository: &model.RepositorySettings{
			// Description, Homepage, MergeCommitTitle, MergeCommitMessage are nil.
			HasWiki:                           ptr.Ptr(false),
			HasDiscussions:                    ptr.Ptr(false),
			HasProjects:                       ptr.Ptr(false),
			HasIssues:                         ptr.Ptr(true),
			AllowMergeCommit:                  ptr.Ptr(false),
			AllowSquashMerge:                  ptr.Ptr(true),
			AllowRebaseMerge:                  ptr.Ptr(true),
			SquashMergeCommitTitle:            ptr.Ptr("PR_TITLE"),
			SquashMergeCommitMessage:          ptr.Ptr("PR_BODY"),
			DeleteBranchOnMerge:               ptr.Ptr(true),
			AllowUpdateBranch:                 ptr.Ptr(true),
			AllowAutoMerge:                    ptr.Ptr(true),
			WebCommitSignoffRequired:          ptr.Ptr(false),
			PrivateVulnerabilityReportEnabled: ptr.Ptr(true),
			VulnerabilityAlertsEnabled:        ptr.Ptr(true),
			AutomatedSecurityFixesEnabled:     ptr.Ptr(true),
			DefaultWorkflowPermissions:        ptr.Ptr("read"),
			CanApprovePullRequestReviews:      ptr.Ptr(false),
		},
		Swatches: []SwatchEntry{
			{Path: "justfile", Alteration: swatch.FirstFit},
		},
	}

	want := `# Initially fitted by tailor on 2026-03-02
license: MIT

repository:
  has_wiki: false
  has_discussions: false
  has_projects: false
  has_issues: true
  allow_merge_commit: false
  allow_squash_merge: true
  allow_rebase_merge: true
  squash_merge_commit_title: PR_TITLE
  squash_merge_commit_message: PR_BODY
  delete_branch_on_merge: true
  allow_update_branch: true
  allow_auto_merge: true
  web_commit_signoff_required: false
  private_vulnerability_reporting_enabled: true
  vulnerability_alerts_enabled: true
  automated_security_fixes_enabled: true
  default_workflow_permissions: read
  can_approve_pull_request_reviews: false

swatches:
  - path: justfile
    alteration: first-fit
`

	dir := t.TempDir()
	if err := Write(dir, cfg, "2026-03-02", "Initially fitted"); err != nil {
		t.Fatalf("Write: %v", err)
	}

	got, err := os.ReadFile(filepath.Join(dir, ".tailor.yml"))
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}

	if string(got) != want {
		t.Errorf("output mismatch with optional fields omitted\n--- got ---\n%s\n--- want ---\n%s", got, want)
	}
}

func TestWriteYAMLSpecialCharactersQuoted(t *testing.T) {
	desc := `My project: a tool for #things`
	cfg := &Config{
		License: "MIT",
		Repository: &model.RepositorySettings{
			Description:      &desc,
			HasWiki:          ptr.Ptr(false),
			AllowSquashMerge: ptr.Ptr(true),
		},
		Swatches: []SwatchEntry{
			{Path: "justfile", Alteration: swatch.FirstFit},
		},
	}

	dir := t.TempDir()
	if err := Write(dir, cfg, "2026-03-04", "Initially fitted"); err != nil {
		t.Fatalf("Write: %v", err)
	}

	got, err := os.ReadFile(filepath.Join(dir, ".tailor.yml"))
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}

	// The output must be valid YAML that round-trips through yaml.Unmarshal.
	var parsed Config
	if err := yaml.Unmarshal(got, &parsed); err != nil {
		t.Fatalf("output is not valid YAML: %v\n--- output ---\n%s", err, got)
	}

	if parsed.Repository == nil || parsed.Repository.Description == nil {
		t.Fatal("parsed Repository.Description is nil")
	}
	if *parsed.Repository.Description != desc {
		t.Errorf("round-tripped Description = %q, want %q", *parsed.Repository.Description, desc)
	}
}

func TestWriteTopicsPreserved(t *testing.T) {
	topics := []string{"go", "cli", "template"}
	cfg := &Config{
		License: "MIT",
		Repository: &model.RepositorySettings{
			HasWiki: ptr.Ptr(false),
			Topics:  &topics,
		},
		Swatches: []SwatchEntry{
			{Path: "justfile", Alteration: swatch.FirstFit},
		},
	}

	dir := t.TempDir()
	if err := Write(dir, cfg, "2026-03-10", "Refitted"); err != nil {
		t.Fatalf("Write: %v", err)
	}

	got, err := os.ReadFile(filepath.Join(dir, ".tailor.yml"))
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}

	output := string(got)
	if !strings.Contains(output, "topics:") {
		t.Fatalf("output missing topics:\n%s", output)
	}

	// Round-trip through YAML to confirm topics survive.
	var parsed Config
	if err := yaml.Unmarshal(got, &parsed); err != nil {
		t.Fatalf("output is not valid YAML: %v\n--- output ---\n%s", err, got)
	}

	if parsed.Repository == nil || parsed.Repository.Topics == nil {
		t.Fatal("parsed Repository.Topics is nil")
	}
	if len(*parsed.Repository.Topics) != 3 {
		t.Fatalf("topics length = %d, want 3", len(*parsed.Repository.Topics))
	}
	for i, want := range topics {
		if (*parsed.Repository.Topics)[i] != want {
			t.Errorf("topic[%d] = %q, want %q", i, (*parsed.Repository.Topics)[i], want)
		}
	}
}

func TestWriteTopicsOmittedWhenNil(t *testing.T) {
	cfg := &Config{
		License: "MIT",
		Repository: &model.RepositorySettings{
			HasWiki: ptr.Ptr(false),
			// Topics is nil - should be omitted from output.
		},
		Swatches: []SwatchEntry{
			{Path: "justfile", Alteration: swatch.FirstFit},
		},
	}

	dir := t.TempDir()
	if err := Write(dir, cfg, "2026-03-10", "Refitted"); err != nil {
		t.Fatalf("Write: %v", err)
	}

	got, err := os.ReadFile(filepath.Join(dir, ".tailor.yml"))
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}

	if strings.Contains(string(got), "topics:") {
		t.Errorf("output contains 'topics:' when Topics is nil:\n%s", got)
	}
}

func TestWriteEmptyTopicsRoundTrip(t *testing.T) {
	topics := []string{}
	cfg := &Config{
		License: "MIT",
		Repository: &model.RepositorySettings{
			HasWiki: ptr.Ptr(false),
			Topics:  &topics,
		},
		Swatches: []SwatchEntry{
			{Path: "justfile", Alteration: swatch.FirstFit},
		},
	}

	dir := t.TempDir()
	if err := Write(dir, cfg, "2026-03-10", "Refitted"); err != nil {
		t.Fatalf("Write: %v", err)
	}

	got, err := os.ReadFile(filepath.Join(dir, ".tailor.yml"))
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}

	output := string(got)
	if !strings.Contains(output, "topics: []") {
		t.Fatalf("expected 'topics: []' in output, got:\n%s", output)
	}

	// Round-trip: parse back and verify Topics is non-nil empty slice.
	var parsed Config
	if err := yaml.Unmarshal(got, &parsed); err != nil {
		t.Fatalf("output is not valid YAML: %v\n--- output ---\n%s", err, got)
	}

	if parsed.Repository == nil || parsed.Repository.Topics == nil {
		t.Fatal("round-tripped Topics is nil, want non-nil empty slice")
	}
	if len(*parsed.Repository.Topics) != 0 {
		t.Errorf("round-tripped Topics length = %d, want 0", len(*parsed.Repository.Topics))
	}
}

func TestWriteNilRepositoryOmitted(t *testing.T) {
	cfg := &Config{
		License: "MIT",
		Swatches: []SwatchEntry{
			{Path: "justfile", Alteration: swatch.FirstFit},
		},
	}

	dir := t.TempDir()
	if err := Write(dir, cfg, "2026-03-04", "Initially fitted"); err != nil {
		t.Fatalf("Write: %v", err)
	}

	got, err := os.ReadFile(filepath.Join(dir, ".tailor.yml"))
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}

	output := string(got)
	if strings.Contains(output, "repository:") {
		t.Errorf("output contains 'repository:' when Repository is nil:\n%s", output)
	}

	// Must still be valid YAML.
	var parsed Config
	if err := yaml.Unmarshal(got, &parsed); err != nil {
		t.Fatalf("output is not valid YAML: %v\n--- output ---\n%s", err, got)
	}
}
