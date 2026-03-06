package config

import (
	"strings"
	"testing"

	"gopkg.in/yaml.v3"

	"github.com/wimpysworld/tailor/internal/ptr"
	"github.com/wimpysworld/tailor/internal/swatch"
	"github.com/wimpysworld/tailor/internal/testutil"
)

// specYAML is the exact config body from the specification (lines 331-415),
// minus the leading comment which is not part of the data model.
const specYAML = `license: MIT

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
    alteration: first-fit

  - source: .github/workflows/tailor-automerge.yml
    destination: .github/workflows/tailor-automerge.yml
    alteration: triggered
`

func TestUnmarshalSpecYAML(t *testing.T) {
	var cfg Config
	if err := yaml.Unmarshal([]byte(specYAML), &cfg); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}

	if cfg.License != "MIT" {
		t.Errorf("License = %q, want %q", cfg.License, "MIT")
	}

	if cfg.Repository == nil {
		t.Fatal("Repository is nil, want non-nil")
	}

	r := cfg.Repository
	testutil.AssertBoolPtr(t, r.HasWiki, false, false, "has_wiki")
	testutil.AssertBoolPtr(t, r.HasDiscussions, false, false, "has_discussions")
	testutil.AssertBoolPtr(t, r.HasProjects, false, false, "has_projects")
	testutil.AssertBoolPtr(t, r.HasIssues, false, true, "has_issues")
	testutil.AssertBoolPtr(t, r.AllowMergeCommit, false, false, "allow_merge_commit")
	testutil.AssertBoolPtr(t, r.AllowSquashMerge, false, true, "allow_squash_merge")
	testutil.AssertBoolPtr(t, r.AllowRebaseMerge, false, true, "allow_rebase_merge")
	testutil.AssertStringPtr(t, r.SquashMergeCommitTitle, false, "PR_TITLE", "squash_merge_commit_title")
	testutil.AssertStringPtr(t, r.SquashMergeCommitMessage, false, "PR_BODY", "squash_merge_commit_message")
	testutil.AssertBoolPtr(t, r.DeleteBranchOnMerge, false, true, "delete_branch_on_merge")
	testutil.AssertBoolPtr(t, r.AllowUpdateBranch, false, true, "allow_update_branch")
	testutil.AssertBoolPtr(t, r.AllowAutoMerge, false, true, "allow_auto_merge")
	testutil.AssertBoolPtr(t, r.WebCommitSignoffRequired, false, false, "web_commit_signoff_required")
	testutil.AssertBoolPtr(t, r.PrivateVulnerabilityReportEnabled, false, true, "private_vulnerability_reporting_enabled")

	if len(cfg.Swatches) != 17 {
		t.Fatalf("Swatches count = %d, want 17", len(cfg.Swatches))
	}

	// Spot-check the first and last swatch entries.
	first := cfg.Swatches[0]
	if first.Source != ".github/workflows/tailor.yml" {
		t.Errorf("first swatch Source = %q", first.Source)
	}
	if first.Destination != ".github/workflows/tailor.yml" {
		t.Errorf("first swatch Destination = %q", first.Destination)
	}
	if first.Alteration != swatch.Always {
		t.Errorf("first swatch Alteration = %q", first.Alteration)
	}

	last := cfg.Swatches[16]
	if last.Source != ".github/workflows/tailor-automerge.yml" {
		t.Errorf("last swatch Source = %q", last.Source)
	}
	if last.Alteration != swatch.Triggered {
		t.Errorf("last swatch Alteration = %q", last.Alteration)
	}
}

func TestMarshalRoundTrip(t *testing.T) {
	var original Config
	if err := yaml.Unmarshal([]byte(specYAML), &original); err != nil {
		t.Fatalf("initial Unmarshal failed: %v", err)
	}

	out, err := yaml.Marshal(&original)
	if err != nil {
		t.Fatalf("Marshal failed: %v", err)
	}

	var roundTripped Config
	if err := yaml.Unmarshal(out, &roundTripped); err != nil {
		t.Fatalf("round-trip Unmarshal failed: %v", err)
	}

	if roundTripped.License != original.License {
		t.Errorf("License = %q, want %q", roundTripped.License, original.License)
	}

	if roundTripped.Repository == nil {
		t.Fatal("round-tripped Repository is nil")
	}

	if len(roundTripped.Swatches) != len(original.Swatches) {
		t.Fatalf("Swatches count = %d, want %d", len(roundTripped.Swatches), len(original.Swatches))
	}

	for i, s := range roundTripped.Swatches {
		o := original.Swatches[i]
		if s.Source != o.Source || s.Destination != o.Destination || s.Alteration != o.Alteration {
			t.Errorf("swatch[%d] mismatch: got {%q, %q, %q}, want {%q, %q, %q}",
				i, s.Source, s.Destination, s.Alteration, o.Source, o.Destination, o.Alteration)
		}
	}
}

func TestRepositoryNilWhenAbsent(t *testing.T) {
	input := `license: MIT
swatches:
  - source: justfile
    destination: justfile
    alteration: first-fit
`
	var cfg Config
	if err := yaml.Unmarshal([]byte(input), &cfg); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}

	if cfg.Repository != nil {
		t.Errorf("Repository = %+v, want nil when section is absent", cfg.Repository)
	}
}

func TestRepositoryOmittedInMarshalWhenNil(t *testing.T) {
	cfg := Config{
		License: "MIT",
		Swatches: []SwatchEntry{
			{Source: "justfile", Destination: "justfile", Alteration: swatch.FirstFit},
		},
	}

	out, err := yaml.Marshal(&cfg)
	if err != nil {
		t.Fatalf("Marshal failed: %v", err)
	}

	if strings.Contains(string(out), "repository") {
		t.Errorf("marshalled output contains 'repository' when Repository is nil:\n%s", out)
	}
}

func TestOptionalRepositoryFieldsOmitted(t *testing.T) {
	cfg := Config{
		License: "MIT",
		Repository: &RepositorySettings{
			HasWiki: ptr.Bool(false),
		},
		Swatches: []SwatchEntry{
			{Source: "justfile", Destination: "justfile", Alteration: swatch.FirstFit},
		},
	}

	out, err := yaml.Marshal(&cfg)
	if err != nil {
		t.Fatalf("Marshal failed: %v", err)
	}

	s := string(out)
	if !strings.Contains(s, "has_wiki") {
		t.Error("expected has_wiki in output")
	}
	if strings.Contains(s, "has_discussions") {
		t.Error("has_discussions should be omitted when nil")
	}
}

func TestRepositoryStringFields(t *testing.T) {
	input := `license: MIT
repository:
  description: My project
  homepage: https://example.com
  merge_commit_title: PR_TITLE
  merge_commit_message: PR_BODY
swatches: []
`
	var cfg Config
	if err := yaml.Unmarshal([]byte(input), &cfg); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}

	r := cfg.Repository
	testutil.AssertStringPtr(t, r.Description, false, "My project", "description")
	testutil.AssertStringPtr(t, r.Homepage, false, "https://example.com", "homepage")
	testutil.AssertStringPtr(t, r.MergeCommitTitle, false, "PR_TITLE", "merge_commit_title")
	testutil.AssertStringPtr(t, r.MergeCommitMessage, false, "PR_BODY", "merge_commit_message")
}
