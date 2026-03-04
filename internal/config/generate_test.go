package config

import (
	"io/fs"
	"testing"

	"gopkg.in/yaml.v3"

	"github.com/wimpysworld/tailor"
	"github.com/wimpysworld/tailor/internal/ptr"
	"github.com/wimpysworld/tailor/internal/swatch"
	"github.com/wimpysworld/tailor/internal/testutil"
)

func TestDefaultConfigMatchesEmbedded(t *testing.T) {
	// Parse the embedded config directly for comparison.
	data, err := fs.ReadFile(tailor.SwatchFS, embeddedConfigPath)
	if err != nil {
		t.Fatalf("reading embedded config: %v", err)
	}
	var want Config
	if err := yaml.Unmarshal(data, &want); err != nil {
		t.Fatalf("unmarshalling embedded config: %v", err)
	}

	got, err := DefaultConfig("MIT")
	if err != nil {
		t.Fatalf("DefaultConfig() error: %v", err)
	}

	// License should be the value we passed, not the embedded one.
	if got.License != "MIT" {
		t.Errorf("License = %q, want %q", got.License, "MIT")
	}

	// Repository settings should match the embedded config exactly.
	if got.Repository == nil {
		t.Fatal("Repository is nil, want non-nil")
	}
	testutil.AssertBoolPtr(t, got.Repository.HasWiki, false, false, "has_wiki")
	testutil.AssertBoolPtr(t, got.Repository.HasDiscussions, false, false, "has_discussions")
	testutil.AssertBoolPtr(t, got.Repository.HasProjects, false, false, "has_projects")
	testutil.AssertBoolPtr(t, got.Repository.HasIssues, false, true, "has_issues")
	testutil.AssertBoolPtr(t, got.Repository.AllowMergeCommit, false, false, "allow_merge_commit")
	testutil.AssertBoolPtr(t, got.Repository.AllowSquashMerge, false, true, "allow_squash_merge")
	testutil.AssertBoolPtr(t, got.Repository.AllowRebaseMerge, false, true, "allow_rebase_merge")
	testutil.AssertStringPtr(t, got.Repository.SquashMergeCommitTitle, false, "PR_TITLE", "squash_merge_commit_title")
	testutil.AssertStringPtr(t, got.Repository.SquashMergeCommitMessage, false, "PR_BODY", "squash_merge_commit_message")
	testutil.AssertBoolPtr(t, got.Repository.DeleteBranchOnMerge, false, true, "delete_branch_on_merge")
	testutil.AssertBoolPtr(t, got.Repository.AllowUpdateBranch, false, true, "allow_update_branch")
	testutil.AssertBoolPtr(t, got.Repository.AllowAutoMerge, false, true, "allow_auto_merge")
	testutil.AssertBoolPtr(t, got.Repository.WebCommitSignoffRequired, false, false, "web_commit_signoff_required")
	testutil.AssertBoolPtr(t, got.Repository.PrivateVulnerabilityReportEnabled, false, true, "private_vulnerability_reporting_enabled")

	// Fields absent from the embedded config should remain nil.
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
		if g.Source != w.Source || g.Destination != w.Destination || g.Alteration != w.Alteration {
			t.Errorf("swatch[%d] = {%q, %q, %q}, want {%q, %q, %q}",
				i, g.Source, g.Destination, g.Alteration, w.Source, w.Destination, w.Alteration)
		}
	}
}

func TestDefaultConfigSwatchCount(t *testing.T) {
	cfg, err := DefaultConfig("MIT")
	if err != nil {
		t.Fatalf("DefaultConfig() error: %v", err)
	}
	if len(cfg.Swatches) != 16 {
		t.Errorf("Swatches count = %d, want 16", len(cfg.Swatches))
	}
}

func TestDefaultConfigSwatchOrder(t *testing.T) {
	cfg, err := DefaultConfig("MIT")
	if err != nil {
		t.Fatalf("DefaultConfig() error: %v", err)
	}

	first := cfg.Swatches[0]
	if first.Source != ".github/workflows/tailor.yml" {
		t.Errorf("first swatch Source = %q, want %q", first.Source, ".github/workflows/tailor.yml")
	}
	if first.Alteration != swatch.Always {
		t.Errorf("first swatch Alteration = %q, want %q", first.Alteration, swatch.Always)
	}

	last := cfg.Swatches[len(cfg.Swatches)-1]
	if last.Source != ".tailor/config.yml" {
		t.Errorf("last swatch Source = %q, want %q", last.Source, ".tailor/config.yml")
	}
	if last.Alteration != swatch.FirstFit {
		t.Errorf("last swatch Alteration = %q, want %q", last.Alteration, swatch.FirstFit)
	}
}

func TestMergeRepoSettings(t *testing.T) {
	tests := []struct {
		name        string
		live        *RepositorySettings
		description string
		wantDesc    *string // nil means expect nil
		wantHome    *string
	}{
		{
			name: "live settings override defaults entirely",
			live: &RepositorySettings{
				Description: ptr.String("live desc"),
				Homepage:    ptr.String("https://live.example.com"),
				HasWiki:     ptr.Bool(true),
				HasIssues:   ptr.Bool(false),
			},
			description: "",
			wantDesc:    ptr.String("live desc"),
			wantHome:    ptr.String("https://live.example.com"),
		},
		{
			name: "description flag overrides live description",
			live: &RepositorySettings{
				Description: ptr.String("live desc"),
				Homepage:    ptr.String("https://live.example.com"),
			},
			description: "flag desc",
			wantDesc:    ptr.String("flag desc"),
			wantHome:    ptr.String("https://live.example.com"),
		},
		{
			name: "empty description from live produces nil",
			live: &RepositorySettings{
				Description: ptr.String(""),
				Homepage:    ptr.String("https://live.example.com"),
			},
			description: "",
			wantDesc:    nil,
			wantHome:    ptr.String("https://live.example.com"),
		},
		{
			name: "empty homepage from live produces nil",
			live: &RepositorySettings{
				Description: ptr.String("live desc"),
				Homepage:    ptr.String(""),
			},
			description: "",
			wantDesc:    ptr.String("live desc"),
			wantHome:    nil,
		},
		{
			name: "non-empty description flag with empty live description sets flag value",
			live: &RepositorySettings{
				Description: ptr.String(""),
				Homepage:    ptr.String("https://live.example.com"),
			},
			description: "flag desc",
			wantDesc:    ptr.String("flag desc"),
			wantHome:    ptr.String("https://live.example.com"),
		},
		{
			name: "empty description flag with non-empty live description preserves live value",
			live: &RepositorySettings{
				Description: ptr.String("live desc"),
				Homepage:    ptr.String("https://live.example.com"),
			},
			description: "",
			wantDesc:    ptr.String("live desc"),
			wantHome:    ptr.String("https://live.example.com"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &Config{
				License: "MIT",
				Repository: &RepositorySettings{
					HasWiki:   ptr.Bool(false),
					HasIssues: ptr.Bool(true),
				},
			}

			MergeRepoSettings(cfg, tt.live, tt.description)

			// Repository must point to the live object.
			if cfg.Repository != tt.live {
				t.Fatal("Repository was not replaced with live settings")
			}

			// Check Description.
			if tt.wantDesc == nil {
				if cfg.Repository.Description != nil {
					t.Errorf("Description = %q, want nil", *cfg.Repository.Description)
				}
			} else {
				testutil.AssertStringPtr(t, cfg.Repository.Description, false, *tt.wantDesc, "description")
			}

			// Check Homepage.
			if tt.wantHome == nil {
				if cfg.Repository.Homepage != nil {
					t.Errorf("Homepage = %q, want nil", *cfg.Repository.Homepage)
				}
			} else {
				testutil.AssertStringPtr(t, cfg.Repository.Homepage, false, *tt.wantHome, "homepage")
			}
		})
	}
}

func TestMergeRepoSettingsPreservesMergeCommitFields(t *testing.T) {
	mergeTitle := "PR_TITLE"
	mergeMessage := "PR_BODY"
	live := &RepositorySettings{
		Description:        ptr.String("desc"),
		AllowMergeCommit:   ptr.Bool(false),
		MergeCommitTitle:   &mergeTitle,
		MergeCommitMessage: &mergeMessage,
	}

	cfg := &Config{License: "MIT"}
	MergeRepoSettings(cfg, live, "")

	testutil.AssertStringPtr(t, cfg.Repository.MergeCommitTitle, false, "PR_TITLE", "merge_commit_title")
	testutil.AssertStringPtr(t, cfg.Repository.MergeCommitMessage, false, "PR_BODY", "merge_commit_message")
}

func TestDefaultConfigLicenseValues(t *testing.T) {
	tests := []struct {
		name    string
		license string
	}{
		{name: "MIT", license: "MIT"},
		{name: "Apache-2.0", license: "Apache-2.0"},
		{name: "none", license: "none"},
		{name: "empty", license: ""},
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
