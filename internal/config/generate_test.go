package config

import (
	"reflect"
	"slices"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"

	"github.com/wimpysworld/tailor/internal/model"
	"github.com/wimpysworld/tailor/internal/swatch"
	"github.com/wimpysworld/tailor/internal/testutil"
)

func TestDefaultConfigMatchesEmbedded(t *testing.T) {
	// Parse the embedded config directly for comparison.
	data, err := swatch.Content(".tailor.yml")
	if err != nil {
		t.Fatalf("reading embedded config: %v", err)
	}
	var want Config
	if err := yaml.Unmarshal(data, &want); err != nil {
		t.Fatalf("unmarshalling embedded config: %v", err)
	}
	want.Swatches = slices.DeleteFunc(want.Swatches, func(entry SwatchEntry) bool {
		return !want.SwatchActive(entry.Path)
	})

	got, err := DefaultConfig("BlueOak-1.0.0")
	if err != nil {
		t.Fatalf("DefaultConfig() error: %v", err)
	}

	// DefaultConfig uses the requested licence value, not the embedded one.
	if got.License != "BlueOak-1.0.0" {
		t.Errorf("License = %q, want %q", got.License, "BlueOak-1.0.0")
	}

	if got.Languages == nil {
		t.Fatal("Languages is nil, want explicit defaults")
	}
	testutil.AssertPtrEqual(t, got.Languages.Go, new(false), "languages.go")
	if got.MCP == nil {
		t.Fatal("MCP is nil, want explicit defaults")
	}
	testutil.AssertPtrEqual(t, got.MCP.Playwright, new(false), "mcp.playwright")
	if got.Pages == nil {
		t.Fatal("Pages is nil, want explicit defaults")
	}
	testutil.AssertPtrEqual(t, got.Pages.Enabled, new(false), "pages.enabled")

	// Managed repository settings retain the embedded defaults, apart from project metadata.
	if got.Repository == nil {
		t.Fatal("Repository is nil, want non-nil")
	}
	testutil.AssertPtrEqual(t, got.Repository.HasWiki, new(false), "has_wiki")
	testutil.AssertPtrEqual(t, got.Repository.HasDiscussions, new(false), "has_discussions")
	testutil.AssertPtrEqual(t, got.Repository.HasProjects, new(false), "has_projects")
	testutil.AssertPtrEqual(t, got.Repository.HasIssues, new(true), "has_issues")
	testutil.AssertPtrEqual(t, got.Repository.AllowMergeCommit, new(false), "allow_merge_commit")
	testutil.AssertPtrEqual(t, got.Repository.AllowSquashMerge, new(true), "allow_squash_merge")
	testutil.AssertPtrEqual(t, got.Repository.AllowRebaseMerge, new(true), "allow_rebase_merge")
	testutil.AssertPtrEqual(t, got.Repository.SquashMergeCommitTitle, new("PR_TITLE"), "squash_merge_commit_title")
	testutil.AssertPtrEqual(t, got.Repository.SquashMergeCommitMessage, new("PR_BODY"), "squash_merge_commit_message")
	testutil.AssertPtrEqual(t, got.Repository.DeleteBranchOnMerge, new(true), "delete_branch_on_merge")
	testutil.AssertPtrEqual(t, got.Repository.AllowUpdateBranch, new(true), "allow_update_branch")
	testutil.AssertPtrEqual(t, got.Repository.AllowAutoMerge, new(true), "allow_auto_merge")
	testutil.AssertPtrEqual(t, got.Repository.WebCommitSignoffRequired, new(false), "web_commit_signoff_required")
	testutil.AssertPtrEqual(t, got.Repository.PrivateVulnerabilityReportEnabled, new(true), "private_vulnerability_reporting_enabled")
	testutil.AssertPtrEqual(t, got.Repository.VulnerabilityAlertsEnabled, new(true), "vulnerability_alerts_enabled")
	testutil.AssertPtrEqual(t, got.Repository.AutomatedSecurityFixesEnabled, new(true), "automated_security_fixes_enabled")
	testutil.AssertPtrEqual(t, got.Repository.DefaultWorkflowPermissions, new("read"), "default_workflow_permissions")
	testutil.AssertPtrEqual(t, got.Repository.CanApprovePullRequestReviews, new(false), "can_approve_pull_request_reviews")
	if got.Actions == nil {
		t.Fatal("Actions is nil, want default policy")
	}
	testutil.AssertPtrEqual(t, got.Actions.Enabled, new(true), "actions.enabled")
	testutil.AssertPtrEqual(t, got.Actions.AllowedActions, new("all"), "actions.allowed_actions")
	testutil.AssertPtrEqual(t, got.Actions.SHAPinningRequired, new(false), "actions.sha_pinning_required")
	if got.Actions.GitHubOwnedAllowed != nil || got.Actions.VerifiedAllowed != nil || got.Actions.PatternsAllowed != nil {
		t.Fatal("default all policy contains selected-only fields")
	}

	// Labels retain the embedded defaults.
	if len(got.Labels) != 12 {
		t.Fatalf("Labels count = %d, want 12", len(got.Labels))
	}
	wantLabels := []model.LabelEntry{
		{Name: "bug", Color: "d20f39", Description: "Something isn't working"},
		{Name: "documentation", Color: "04a5e5", Description: "Documentation improvement"},
		{Name: "duplicate", Color: "8839ef", Description: "Already exists"},
		{Name: "enhancement", Color: "1e66f5", Description: "New feature request"},
		{Name: "good first issue", Color: "40a02b", Description: "Good for newcomers"},
		{Name: "help wanted", Color: "179299", Description: "Extra attention needed"},
		{Name: "invalid", Color: "e64553", Description: "Not valid or relevant"},
		{Name: "question", Color: "7287fd", Description: "Needs more information"},
		{Name: "wontfix", Color: "dc8a78", Description: "Will not be worked on"},
		{Name: "dependencies", Color: "fe640b", Description: "Dependency update"},
		{Name: "github_actions", Color: "ea76cb", Description: "GitHub Actions update"},
		{Name: "hacktoberfest-accepted", Color: "df8e1d", Description: "Hacktoberfest contribution"},
	}
	for i, wl := range wantLabels {
		gl := got.Labels[i]
		if gl.Name != wl.Name || gl.Color != wl.Color || gl.Description != wl.Description {
			t.Errorf("label[%d] = {%q, %q, %q}, want {%q, %q, %q}",
				i, gl.Name, gl.Color, gl.Description, wl.Name, wl.Color, wl.Description)
		}
	}

	// Project metadata is cleared, while undeclared merge settings stay nil.
	if got.Repository.Description != nil {
		t.Errorf("Description = %q, want nil", *got.Repository.Description)
	}
	if got.Repository.Homepage != nil {
		t.Errorf("Homepage = %q, want nil", *got.Repository.Homepage)
	}
	if got.Repository.MergeCommitTitle != nil {
		t.Errorf("MergeCommitTitle = %q, want nil", *got.Repository.MergeCommitTitle)
	}
	if got.Repository.MergeCommitMessage != nil {
		t.Errorf("MergeCommitMessage = %q, want nil", *got.Repository.MergeCommitMessage)
	}

	// Swatch count and ordering must match exactly.
	if len(got.Swatches) != len(want.Swatches) {
		t.Fatalf("Swatches count = %d, want %d", len(got.Swatches), len(want.Swatches))
	}
	for i, g := range got.Swatches {
		w := want.Swatches[i]
		if g.Path != w.Path || g.Alteration != w.Alteration {
			t.Errorf("swatch[%d] = {%q, %q}, want {%q, %q}",
				i, g.Path, g.Alteration, w.Path, w.Alteration)
		}
	}
}

func TestDefaultConfigHasNoVariables(t *testing.T) {
	cfg, err := DefaultConfig("none")
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.Variables) != 0 {
		t.Fatalf("default variables = %#v, want none", cfg.Variables)
	}
}

func TestDefaultConfigSwatchCount(t *testing.T) {
	cfg, err := DefaultConfig("BlueOak-1.0.0")
	if err != nil {
		t.Fatalf("DefaultConfig() error: %v", err)
	}
	if len(cfg.Swatches) != 25 {
		t.Errorf("Swatches count = %d, want 25", len(cfg.Swatches))
	}
}

func TestDefaultConfigSwatchOrder(t *testing.T) {
	cfg, err := DefaultConfig("BlueOak-1.0.0")
	if err != nil {
		t.Fatalf("DefaultConfig() error: %v", err)
	}

	first := cfg.Swatches[0]
	if first.Path != ".github/dependabot.yml" {
		t.Errorf("first swatch Path = %q, want %q", first.Path, ".github/dependabot.yml")
	}
	if first.Alteration != swatch.FirstFit {
		t.Errorf("first swatch Alteration = %q, want %q", first.Alteration, swatch.FirstFit)
	}

	second := cfg.Swatches[1]
	if second.Path != ".github/FUNDING.yml" {
		t.Errorf("second swatch Path = %q, want %q", second.Path, ".github/FUNDING.yml")
	}
	if second.Alteration != swatch.FirstFit {
		t.Errorf("second swatch Alteration = %q, want %q", second.Alteration, swatch.FirstFit)
	}

	last := cfg.Swatches[len(cfg.Swatches)-1]
	if last.Path != ".tailor.yml" {
		t.Errorf("last swatch Path = %q, want %q", last.Path, ".tailor.yml")
	}
	if last.Alteration != swatch.Always {
		t.Errorf("last swatch Alteration = %q, want %q", last.Alteration, swatch.Always)
	}
}

func TestMergeRepoMetadata(t *testing.T) {
	tests := []struct {
		name        string
		description *string
		homepage    *string
		override    *string
		wantDesc    *string
	}{
		{name: "live metadata", description: new("live desc"), homepage: new("https://example.com"), wantDesc: new("live desc")},
		{name: "empty metadata", description: new(""), homepage: new(""), wantDesc: new("")},
		{name: "absent metadata", wantDesc: nil},
		{name: "description override", description: new("live desc"), homepage: new("https://example.com"), override: new("flag desc"), wantDesc: new("flag desc")},
		{name: "empty description override", description: new("live desc"), override: new(""), wantDesc: new("")},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg, err := DefaultConfig("MIT")
			if err != nil {
				t.Fatal(err)
			}
			want, err := DefaultConfig("MIT")
			if err != nil {
				t.Fatal(err)
			}
			live := &model.RepositorySettings{
				Description: tt.description, Homepage: tt.homepage,
				HasWiki: new(true), HasIssues: new(false), Topics: &[]string{"live"},
				MergeCommitTitle: new("MERGE_MESSAGE"),
			}
			MergeRepoMetadata(cfg, live, tt.override)
			testutil.AssertPtrEqual(t, cfg.Repository.Description, tt.wantDesc, "description")
			testutil.AssertPtrEqual(t, cfg.Repository.Homepage, tt.homepage, "homepage")
			testutil.AssertPtrEqual(t, live.Description, tt.description, "live description")
			want.Repository.Description = tt.wantDesc
			want.Repository.Homepage = tt.homepage
			if !reflect.DeepEqual(cfg.Repository, want.Repository) {
				t.Errorf("repository settings changed beyond metadata: got %+v, want %+v", cfg.Repository, want.Repository)
			}
			if tt.homepage != nil && *tt.homepage != "" && cfg.HomepageDeclared() {
				t.Error("imported homepage lost its inferred provenance")
			}
		})
	}
}

func TestMergeRepoMetadataPreservesDeclaredHomepage(t *testing.T) {
	for _, homepage := range []string{"", "https://declared.example.com"} {
		t.Run(homepage, func(t *testing.T) {
			cfg := &Config{Repository: &model.RepositorySettings{Homepage: new(homepage)}}
			MergeRepoMetadata(cfg, &model.RepositorySettings{Homepage: new("https://live.example.com")}, nil)
			testutil.AssertPtrEqual(t, cfg.Repository.Homepage, new(homepage), "homepage")
			if !cfg.HomepageDeclared() {
				t.Fatal("explicit homepage lost its declaration")
			}
		})
	}
}

func TestApplyRepoDefaults(t *testing.T) {
	description := "existing description"
	homepage := "https://example.com"
	repoURL := "https://github.com/octocat/widgets"
	tests := []struct {
		name            string
		repo            *model.RepositorySettings
		url             string
		wantDescription string
		wantHomepage    *string
	}{
		{name: "fills empty fields", repo: &model.RepositorySettings{}, url: repoURL, wantDescription: "widgets", wantHomepage: &repoURL},
		{name: "preserves set fields", repo: &model.RepositorySettings{Description: &description, Homepage: &homepage}, url: repoURL, wantDescription: description, wantHomepage: &homepage},
		{name: "empty url omits homepage", repo: &model.RepositorySettings{}, url: "", wantDescription: "widgets", wantHomepage: nil},
		{name: "nil repository", repo: nil, url: repoURL, wantDescription: "widgets", wantHomepage: &repoURL},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &Config{Repository: tt.repo}
			ApplyRepoDefaults(cfg, "widgets", tt.url)

			if cfg.Repository.Description == nil || *cfg.Repository.Description != tt.wantDescription {
				t.Errorf("Description = %v, want %q", cfg.Repository.Description, tt.wantDescription)
			}
			switch {
			case tt.wantHomepage == nil:
				if cfg.Repository.Homepage != nil {
					t.Errorf("Homepage = %q, want nil", *cfg.Repository.Homepage)
				}
			case cfg.Repository.Homepage == nil || *cfg.Repository.Homepage != *tt.wantHomepage:
				t.Errorf("Homepage = %v, want %q", cfg.Repository.Homepage, *tt.wantHomepage)
			}
		})
	}
}

func TestDefaultConfigLicenseValues(t *testing.T) {
	tests := []struct {
		name    string
		license string
	}{
		{name: "BlueOak-1.0.0", license: "BlueOak-1.0.0"},
		{name: "Apache-2.0", license: "Apache-2.0"},
		{name: "none", license: "none"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg, err := DefaultConfig(tt.license)
			if err != nil {
				t.Fatalf("DefaultConfig(%q) error: %v", tt.license, err)
			}
			if cfg.License != tt.license {
				t.Errorf("License = %q, want %q", cfg.License, tt.license)
			}
		})
	}
}

func TestDefaultConfigEmptyLicenseError(t *testing.T) {
	_, err := DefaultConfig("")
	if err == nil {
		t.Fatal("DefaultConfig(\"\") expected error, got nil")
	}
	if !strings.Contains(err.Error(), "license must not be empty") {
		t.Errorf("error = %q, want it to mention license must not be empty", err.Error())
	}
}
