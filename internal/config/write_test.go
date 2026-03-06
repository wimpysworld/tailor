package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"

	"github.com/wimpysworld/tailor/internal/ptr"
	"github.com/wimpysworld/tailor/internal/swatch"
)

// wantSpecOutput is the exact byte-for-byte expected output from the
// specification when writing DefaultConfig("MIT") with date 2026-03-02.
const wantSpecOutput = `# Initially fitted by tailor on 2026-03-02
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

swatches:
  - source: .github/workflows/tailor.yml
    destination: .github/workflows/tailor.yml
    alteration: always

  - source: .github/dependabot.yml
    destination: .github/dependabot.yml
    alteration: first-fit

  - source: .github/FUNDING.yml
    destination: .github/FUNDING.yml
    alteration: first-fit

  - source: .github/ISSUE_TEMPLATE/bug_report.yml
    destination: .github/ISSUE_TEMPLATE/bug_report.yml
    alteration: always

  - source: .github/ISSUE_TEMPLATE/feature_request.yml
    destination: .github/ISSUE_TEMPLATE/feature_request.yml
    alteration: always

  - source: .github/ISSUE_TEMPLATE/config.yml
    destination: .github/ISSUE_TEMPLATE/config.yml
    alteration: first-fit

  - source: .github/pull_request_template.md
    destination: .github/pull_request_template.md
    alteration: always

  - source: SECURITY.md
    destination: SECURITY.md
    alteration: always

  - source: CODE_OF_CONDUCT.md
    destination: CODE_OF_CONDUCT.md
    alteration: always

  - source: CONTRIBUTING.md
    destination: CONTRIBUTING.md
    alteration: always

  - source: SUPPORT.md
    destination: SUPPORT.md
    alteration: always

  - source: justfile
    destination: justfile
    alteration: first-fit

  - source: flake.nix
    destination: flake.nix
    alteration: first-fit

  - source: .gitignore
    destination: .gitignore
    alteration: first-fit

  - source: .envrc
    destination: .envrc
    alteration: first-fit

  - source: .tailor/config.yml
    destination: .tailor/config.yml
    alteration: always

  - source: .github/workflows/tailor-automerge.yml
    destination: .github/workflows/tailor-automerge.yml
    alteration: triggered
`

func TestWriteDefaultConfigMatchesSpec(t *testing.T) {
	cfg, err := DefaultConfig("MIT")
	if err != nil {
		t.Fatalf("DefaultConfig: %v", err)
	}

	dir := t.TempDir()
	if err := Write(dir, cfg, "2026-03-02", "Initially fitted"); err != nil {
		t.Fatalf("Write: %v", err)
	}

	got, err := os.ReadFile(filepath.Join(dir, ".tailor", "config.yml"))
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}

	if string(got) != wantSpecOutput {
		t.Errorf("output does not match spec\n--- got ---\n%s\n--- want ---\n%s", got, wantSpecOutput)
	}
}

func TestWriteCreatesTailorDirectory(t *testing.T) {
	dir := t.TempDir()
	tailorDir := filepath.Join(dir, ".tailor")

	// Confirm .tailor/ does not exist before Write.
	if _, err := os.Stat(tailorDir); err == nil {
		t.Fatal(".tailor/ already exists before Write")
	}

	cfg := &Config{
		License: "MIT",
		Repository: &RepositorySettings{
			HasWiki: ptr.Bool(false),
		},
		Swatches: []SwatchEntry{
			{Source: "justfile", Destination: "justfile", Alteration: swatch.FirstFit},
		},
	}

	if err := Write(dir, cfg, "2026-01-01", "Initially fitted"); err != nil {
		t.Fatalf("Write: %v", err)
	}

	info, err := os.Stat(tailorDir)
	if err != nil {
		t.Fatalf(".tailor/ not created: %v", err)
	}
	if !info.IsDir() {
		t.Error(".tailor is not a directory")
	}
}

func TestWriteOptionalFieldsPresent(t *testing.T) {
	cfg := &Config{
		License: "Apache-2.0",
		Repository: &RepositorySettings{
			Description:                       ptr.String("My project"),
			Homepage:                          ptr.String("https://example.com"),
			HasWiki:                           ptr.Bool(true),
			HasDiscussions:                    ptr.Bool(false),
			HasProjects:                       ptr.Bool(false),
			HasIssues:                         ptr.Bool(true),
			AllowMergeCommit:                  ptr.Bool(true),
			AllowSquashMerge:                  ptr.Bool(true),
			AllowRebaseMerge:                  ptr.Bool(false),
			SquashMergeCommitTitle:            ptr.String("PR_TITLE"),
			SquashMergeCommitMessage:          ptr.String("COMMIT_MESSAGES"),
			MergeCommitTitle:                  ptr.String("PR_TITLE"),
			MergeCommitMessage:                ptr.String("PR_BODY"),
			DeleteBranchOnMerge:               ptr.Bool(true),
			AllowUpdateBranch:                 ptr.Bool(true),
			AllowAutoMerge:                    ptr.Bool(false),
			WebCommitSignoffRequired:          ptr.Bool(true),
			PrivateVulnerabilityReportEnabled: ptr.Bool(true),
		},
		Swatches: []SwatchEntry{
			{Source: "justfile", Destination: "justfile", Alteration: swatch.FirstFit},
		},
	}

	want := `# Initially fitted by tailor on 2026-03-02
license: Apache-2.0

repository:
  description: My project
  homepage: "https://example.com"
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

swatches:
  - source: justfile
    destination: justfile
    alteration: first-fit
`

	dir := t.TempDir()
	if err := Write(dir, cfg, "2026-03-02", "Initially fitted"); err != nil {
		t.Fatalf("Write: %v", err)
	}

	got, err := os.ReadFile(filepath.Join(dir, ".tailor", "config.yml"))
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
		Repository: &RepositorySettings{
			// Description, Homepage, MergeCommitTitle, MergeCommitMessage are nil.
			HasWiki:                           ptr.Bool(false),
			HasDiscussions:                    ptr.Bool(false),
			HasProjects:                       ptr.Bool(false),
			HasIssues:                         ptr.Bool(true),
			AllowMergeCommit:                  ptr.Bool(false),
			AllowSquashMerge:                  ptr.Bool(true),
			AllowRebaseMerge:                  ptr.Bool(true),
			SquashMergeCommitTitle:            ptr.String("PR_TITLE"),
			SquashMergeCommitMessage:          ptr.String("PR_BODY"),
			DeleteBranchOnMerge:               ptr.Bool(true),
			AllowUpdateBranch:                 ptr.Bool(true),
			AllowAutoMerge:                    ptr.Bool(true),
			WebCommitSignoffRequired:          ptr.Bool(false),
			PrivateVulnerabilityReportEnabled: ptr.Bool(true),
		},
		Swatches: []SwatchEntry{
			{Source: "justfile", Destination: "justfile", Alteration: swatch.FirstFit},
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

swatches:
  - source: justfile
    destination: justfile
    alteration: first-fit
`

	dir := t.TempDir()
	if err := Write(dir, cfg, "2026-03-02", "Initially fitted"); err != nil {
		t.Fatalf("Write: %v", err)
	}

	got, err := os.ReadFile(filepath.Join(dir, ".tailor", "config.yml"))
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
		Repository: &RepositorySettings{
			Description:      &desc,
			HasWiki:          ptr.Bool(false),
			AllowSquashMerge: ptr.Bool(true),
		},
		Swatches: []SwatchEntry{
			{Source: "justfile", Destination: "justfile", Alteration: swatch.FirstFit},
		},
	}

	dir := t.TempDir()
	if err := Write(dir, cfg, "2026-03-04", "Initially fitted"); err != nil {
		t.Fatalf("Write: %v", err)
	}

	got, err := os.ReadFile(filepath.Join(dir, ".tailor", "config.yml"))
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

func TestWriteNilRepositoryOmitted(t *testing.T) {
	cfg := &Config{
		License: "MIT",
		Swatches: []SwatchEntry{
			{Source: "justfile", Destination: "justfile", Alteration: swatch.FirstFit},
		},
	}

	dir := t.TempDir()
	if err := Write(dir, cfg, "2026-03-04", "Initially fitted"); err != nil {
		t.Fatalf("Write: %v", err)
	}

	got, err := os.ReadFile(filepath.Join(dir, ".tailor", "config.yml"))
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
