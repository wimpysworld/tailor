package config

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"

	"github.com/wimpysworld/tailor/internal/model"
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
  can_approve_pull_request_reviews: false
  secret_scanning: enabled
  secret_scanning_push_protection: enabled
  secret_scanning_non_provider_patterns: enabled

actions:
  enabled: true
  allowed_actions: selected
  sha_pinning_required: false
  github_owned_allowed: true
  verified_allowed: true
  patterns_allowed:
    - "freerangebytes/setup-actionlint@*"
    - "golang/govulncheck-action@*"
    - "golangci/golangci-lint-action@*"
    - "nick-fields/retry@*"
    - "robherley/go-test-action@*"
    - "softprops/action-gh-release@*"

code_scanning:
  state: configured
  query_suite: default
  threat_model: remote
  # An empty list means GitHub detects the languages. Valid values:
  # actions, c-cpp, csharp, go, java-kotlin, javascript-typescript, python, ruby, swift
  languages: []

code_quality:
  state: not-configured
  # An empty list means GitHub detects the languages. Valid values:
  # csharp, go, java-kotlin, javascript-typescript, python, ruby
  languages: []

ruleset:
  # Tailor manages one ruleset named "Tailor" and owns it entirely.
  # active enforces the rules. disabled keeps the ruleset on GitHub but
  # GitHub ignores it, so a hand-made ruleset can govern instead.
  enforcement: active
  bypass_actors:
    # actor_type: RepositoryRole, Team, User, Integration, DeployKey
    # RepositoryRole actor_id: 2 maintain, 4 write, 5 admin
    # bypass_mode: always, pull_request, exempt
    - actor_id: 5
      actor_type: RepositoryRole
      bypass_mode: always
  conditions:
    ref_name:
      # Branch names or fnmatch patterns in refs/heads/<name> form.
      # include also accepts ~DEFAULT_BRANCH and ~ALL.
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
      enabled: true
      parameters:
        required_approving_review_count: 1
        dismiss_stale_reviews_on_push: true
        require_code_owner_review: false
        require_last_push_approval: false
        required_review_thread_resolution: true
        require_extra_approval_for_unattributed_changes: true
        # Any combination of merge, squash, rebase. At least one.
        allowed_merge_methods:
          - squash
          - rebase
    required_status_checks:
      enabled: false
      parameters:
        # Require branches to be up to date before merging.
        strict_required_status_checks_policy: false
        # Do not require status checks on creation.
        do_not_enforce_on_create: false
        # context is the check name as shown on a pull request. For a GitHub
        # Actions job that is the job's name. integration_id is optional and
        # restricts the check to one app; 15368 is GitHub Actions.
        required_status_checks: []
    code_scanning:
      enabled: false
      parameters:
        # tool is the tool name as GitHub shows it, for example CodeQL.
        # alerts_threshold: none, errors, errors_and_warnings, all
        # security_alerts_threshold: none, critical, high_or_higher, medium_or_higher, all
        code_scanning_tools:
          - tool: CodeQL
            alerts_threshold: errors
            security_alerts_threshold: high_or_higher

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
    color: "179299"
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
    alteration: never

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

  - path: cubic.yaml
    alteration: first-fit

  - path: .tailor.yml
    alteration: always
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
			HasWiki: new(false),
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
	if got, want := info.Mode().Perm(), os.FileMode(0o644); got != want {
		t.Errorf(".tailor.yml permissions = %v, want %v", got, want)
	}
}

func TestWriteReplacesExistingConfig(t *testing.T) {
	dir := t.TempDir()
	configFile := filepath.Join(dir, ".tailor.yml")
	if err := os.WriteFile(configFile, []byte("old config\n"), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	cfg := &Config{
		License: "MIT",
		Swatches: []SwatchEntry{
			{Path: "justfile", Alteration: swatch.FirstFit},
		},
	}
	if err := Write(dir, cfg, "2026-08-25", "Refitted"); err != nil {
		t.Fatalf("Write: %v", err)
	}

	written, err := os.ReadFile(configFile)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if strings.Contains(string(written), "old config") {
		t.Errorf(".tailor.yml still contains old content:\n%s", written)
	}
	if !strings.Contains(string(written), "# Refitted by tailor on 2026-08-25") {
		t.Errorf(".tailor.yml does not contain replacement content:\n%s", written)
	}

	info, err := os.Stat(configFile)
	if err != nil {
		t.Fatalf("Stat: %v", err)
	}
	if got, want := info.Mode().Perm(), os.FileMode(0o600); got != want {
		t.Errorf(".tailor.yml permissions = %v, want preserved %v", got, want)
	}
}

func TestWriteCleansUpTemporaryConfigAfterRenameFailure(t *testing.T) {
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, ".tailor.yml"), 0o755); err != nil {
		t.Fatalf("Mkdir: %v", err)
	}

	cfg := &Config{
		License: "MIT",
		Swatches: []SwatchEntry{
			{Path: "justfile", Alteration: swatch.FirstFit},
		},
	}
	if err := Write(dir, cfg, "2026-08-25", "Refitted"); err == nil {
		t.Fatal("Write() error = nil, want rename error")
	}

	temps, err := filepath.Glob(filepath.Join(dir, ".tailor.yml.tmp-*"))
	if err != nil {
		t.Fatalf("Glob: %v", err)
	}
	if len(temps) != 0 {
		t.Errorf("temporary config files remain after rename failure: %v", temps)
	}
}

func TestWriteRejectsConfigSymlinkOutsideProjectRoot(t *testing.T) {
	dir := t.TempDir()
	outside := filepath.Join(t.TempDir(), "outside.yml")
	wantOutside := []byte("outside must stay unchanged\n")
	if err := os.WriteFile(outside, wantOutside, 0o644); err != nil {
		t.Fatalf("WriteFile(%q): %v", outside, err)
	}
	if err := os.Symlink(outside, filepath.Join(dir, ".tailor.yml")); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}

	cfg := &Config{
		License: "MIT",
		Swatches: []SwatchEntry{
			{Path: "justfile", Alteration: swatch.FirstFit},
		},
	}
	if err := Write(dir, cfg, "2026-08-23", "Refitted"); err == nil {
		t.Fatal("Write() error = nil, want root-confinement error")
	}

	gotOutside, err := os.ReadFile(outside)
	if err != nil {
		t.Fatalf("ReadFile(%q): %v", outside, err)
	}
	if string(gotOutside) != string(wantOutside) {
		t.Errorf("outside config = %q, want %q", gotOutside, wantOutside)
	}
	info, err := os.Lstat(filepath.Join(dir, ".tailor.yml"))
	if err != nil {
		t.Fatalf("Lstat(.tailor.yml): %v", err)
	}
	if info.Mode()&os.ModeSymlink == 0 {
		t.Errorf(".tailor.yml mode = %v, want symlink", info.Mode())
	}
}

func TestWriteOptionalFieldsPresent(t *testing.T) {
	cfg := &Config{
		License: "Apache-2.0",
		Repository: &model.RepositorySettings{
			Description:                       new("My project"),
			Homepage:                          new("https://example.com"),
			HasWiki:                           new(true),
			HasDiscussions:                    new(false),
			HasProjects:                       new(false),
			HasIssues:                         new(true),
			AllowMergeCommit:                  new(true),
			AllowSquashMerge:                  new(true),
			AllowRebaseMerge:                  new(false),
			SquashMergeCommitTitle:            new("PR_TITLE"),
			SquashMergeCommitMessage:          new("COMMIT_MESSAGES"),
			MergeCommitTitle:                  new("PR_TITLE"),
			MergeCommitMessage:                new("PR_BODY"),
			DeleteBranchOnMerge:               new(true),
			AllowUpdateBranch:                 new(true),
			AllowAutoMerge:                    new(false),
			WebCommitSignoffRequired:          new(true),
			PrivateVulnerabilityReportEnabled: new(true),
			VulnerabilityAlertsEnabled:        new(false),
			AutomatedSecurityFixesEnabled:     new(true),
			DefaultWorkflowPermissions:        new("write"),
			CanApprovePullRequestReviews:      new(true),
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
  vulnerability_alerts_enabled: false
  automated_security_fixes_enabled: true
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
			HasWiki:                      new(false),
			HasDiscussions:               new(false),
			HasProjects:                  new(false),
			HasIssues:                    new(true),
			AllowMergeCommit:             new(false),
			AllowSquashMerge:             new(true),
			AllowRebaseMerge:             new(true),
			SquashMergeCommitTitle:       new("PR_TITLE"),
			SquashMergeCommitMessage:     new("PR_BODY"),
			DeleteBranchOnMerge:          new(true),
			AllowUpdateBranch:            new(true),
			AllowAutoMerge:               new(true),
			WebCommitSignoffRequired:     new(false),
			DefaultWorkflowPermissions:   new("read"),
			CanApprovePullRequestReviews: new(false),
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
			HasWiki:          new(false),
			AllowSquashMerge: new(true),
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

func TestWriteDynamicScalarsRoundTrip(t *testing.T) {
	topics := []string{"? topic"}
	patterns := []string{"- actions pattern"}
	cfg := &Config{
		License: "- licence",
		Repository: &model.RepositorySettings{
			Description: new("- repository description"),
			Homepage:    new("? repository homepage"),
			Topics:      &topics,
		},
		Actions: &model.ActionsSettings{
			AllowedActions:  new("? actions policy"),
			PatternsAllowed: &patterns,
		},
		Labels: []model.LabelEntry{
			{
				Name:        "- label name",
				Color:       "? label colour",
				Description: "- label description",
			},
		},
		Swatches: []SwatchEntry{
			{Path: "justfile", Alteration: swatch.FirstFit},
		},
	}

	dir := t.TempDir()
	if err := Write(dir, cfg, "2026-08-25", "Refitted"); err != nil {
		t.Fatalf("Write: %v", err)
	}

	written, err := os.ReadFile(filepath.Join(dir, ".tailor.yml"))
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}

	var parsed Config
	if err := yaml.Unmarshal(written, &parsed); err != nil {
		t.Fatalf("output is not valid YAML: %v\n--- output ---\n%s", err, written)
	}
	if !reflect.DeepEqual(parsed, *cfg) {
		t.Errorf("round-tripped config = %#v, want %#v", parsed, *cfg)
	}
}

func TestWriteHomepageRoundTrips(t *testing.T) {
	tests := []struct {
		name     string
		homepage string
	}{
		{"plain URL", "https://example.com"},
		{"contains space", "https://example.com/my page"},
		{"contains space hash", "https://example.com #fragment"},
		{"contains colon tab", "https://example.com/a:\tb"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cfg := &Config{
				License: "MIT",
				Repository: &model.RepositorySettings{
					Homepage: &tc.homepage,
					HasWiki:  new(false),
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

			var parsed Config
			if err := yaml.Unmarshal(got, &parsed); err != nil {
				t.Fatalf("output is not valid YAML: %v\n--- output ---\n%s", err, got)
			}

			if parsed.Repository == nil || parsed.Repository.Homepage == nil {
				t.Fatal("parsed Repository.Homepage is nil")
			}
			if *parsed.Repository.Homepage != tc.homepage {
				t.Errorf("round-tripped Homepage = %q, want %q", *parsed.Repository.Homepage, tc.homepage)
			}
		})
	}
}

func TestWriteTopicsPreserved(t *testing.T) {
	topics := []string{"go", "cli", "template"}
	cfg := &Config{
		License: "MIT",
		Repository: &model.RepositorySettings{
			HasWiki: new(false),
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
			HasWiki: new(false),
			// Nil topics are omitted from output.
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
			HasWiki: new(false),
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
