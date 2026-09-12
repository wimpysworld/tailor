package config

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/wimpysworld/tailor/internal/model"
	"github.com/wimpysworld/tailor/internal/testutil"
)

var approvedDefaultActionPatterns = []string{
	"freerangebytes/setup-actionlint@*",
	"golang/govulncheck-action@*",
	"golangci/golangci-lint-action@*",
	"nick-fields/retry@*",
	"robherley/go-test-action@*",
	"softprops/action-gh-release@*",
}

func TestActionsParsingAndWriting(t *testing.T) {
	dir := t.TempDir()
	input := `license: none
actions:
  enabled: true
  allowed_actions: selected
  sha_pinning_required: true
  github_owned_allowed: true
  verified_allowed: false
  patterns_allowed:
    - acme/*
    - actions/checkout@v4
swatches: []
`
	testutil.WriteConfig(t, dir, input)
	cfg, err := Load(dir)
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}
	if cfg.Actions == nil || cfg.Actions.AllowedActions == nil || *cfg.Actions.AllowedActions != "selected" {
		t.Fatalf("Actions = %+v, want selected policy", cfg.Actions)
	}
	if err := Write(dir, cfg, "2026-08-24", "Refitted"); err != nil {
		t.Fatalf("Write() error: %v", err)
	}
	written, err := os.ReadFile(filepath.Join(dir, ConfigSwatchPath))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"actions:\n", "  enabled: true\n", "  allowed_actions: selected\n", "  patterns_allowed:\n", "    - \"acme/*\"\n"} {
		if !strings.Contains(string(written), want) {
			t.Errorf("written config does not contain %q:\n%s", want, written)
		}
	}
}

func TestActionsOmittedWhenAbsent(t *testing.T) {
	cfg := &Config{License: "none", Swatches: []SwatchEntry{}}
	written := writeConfig(t, cfg, "2026-08-24", "Refitted")
	if strings.Contains(written, "actions:") {
		t.Fatalf("config contains actions section:\n%s", written)
	}
}

func TestValidateActions(t *testing.T) {
	tests := []struct {
		name    string
		actions *model.ActionsSettings
		wantErr string
	}{
		{name: "absent"},
		{name: "all", actions: &model.ActionsSettings{AllowedActions: new("all")}},
		{name: "local only", actions: &model.ActionsSettings{AllowedActions: new("local_only")}},
		{name: "selected", actions: &model.ActionsSettings{AllowedActions: new("selected"), PatternsAllowed: &[]string{"acme/*"}}},
		{name: "invalid enum", actions: &model.ActionsSettings{AllowedActions: new("private")}, wantErr: "invalid allowed_actions"},
		{name: "selected field without enum", actions: &model.ActionsSettings{VerifiedAllowed: new(true)}, wantErr: "require allowed_actions"},
		{name: "selected field with all", actions: &model.ActionsSettings{AllowedActions: new("all"), GitHubOwnedAllowed: new(true)}, wantErr: "require allowed_actions"},
		{name: "unknown", actions: &model.ActionsSettings{Extra: map[string]any{"unknown": true}}, wantErr: "unrecognised actions setting"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assertErrorContains(t, ValidateActions(&Config{Actions: tt.actions}), tt.wantErr)
		})
	}
}

func TestValidateActionsPatternsControlCharacters(t *testing.T) {
	tests := []struct {
		name    string
		pattern string
		wantErr bool
	}{
		{name: "ANSI CSI", pattern: "\x1b[31mocto/*", wantErr: true},
		{name: "OSC 8 hyperlink", pattern: "\x1b]8;;https://evil.example\x07octo/*", wantErr: true},
		{name: "OSC 52 clipboard write", pattern: "\x1b]52;c;Zm9v\x07octo/*", wantErr: true},
		{name: "carriage return", pattern: "safe\rspoofed/*", wantErr: true},
		{name: "C1 CSI", pattern: "\u009b31mocto/*", wantErr: true},
		{name: "benign owner wildcard", pattern: "octocat/*"},
		{name: "benign pinned action", pattern: "actions/checkout@v4"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &Config{Actions: &model.ActionsSettings{
				AllowedActions:  new("selected"),
				PatternsAllowed: &[]string{tt.pattern},
			}}
			err := ValidateActions(cfg)
			if !tt.wantErr {
				if err != nil {
					t.Fatalf("ValidateActions() error: %v", err)
				}
				return
			}
			if err == nil {
				t.Fatal("ValidateActions() returned nil, want error")
			}
			if !strings.Contains(err.Error(), "patterns_allowed") || !strings.Contains(err.Error(), "control characters") {
				t.Errorf("error = %q, want mention of patterns_allowed control characters", err)
			}
		})
	}
}

func TestValidateCompleteActions(t *testing.T) {
	completePatterns := []string{}
	tests := []struct {
		name    string
		actions *model.ActionsSettings
		wantErr bool
	}{
		{name: "absent"},
		{name: "all", actions: &model.ActionsSettings{AllowedActions: new("all")}},
		{name: "local only", actions: &model.ActionsSettings{AllowedActions: new("local_only")}},
		{
			name: "complete selected",
			actions: &model.ActionsSettings{
				AllowedActions:     new("selected"),
				GitHubOwnedAllowed: new(true),
				VerifiedAllowed:    new(true),
				PatternsAllowed:    &completePatterns,
			},
		},
		{name: "missing all selected fields", actions: &model.ActionsSettings{AllowedActions: new("selected")}, wantErr: true},
		{
			name: "missing github owned",
			actions: &model.ActionsSettings{
				AllowedActions:  new("selected"),
				VerifiedAllowed: new(true),
				PatternsAllowed: &completePatterns,
			},
			wantErr: true,
		},
		{
			name: "missing verified",
			actions: &model.ActionsSettings{
				AllowedActions:     new("selected"),
				GitHubOwnedAllowed: new(true),
				PatternsAllowed:    &completePatterns,
			},
			wantErr: true,
		},
		{
			name: "missing patterns",
			actions: &model.ActionsSettings{
				AllowedActions:     new("selected"),
				GitHubOwnedAllowed: new(true),
				VerifiedAllowed:    new(true),
			},
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateCompleteActions(&Config{Actions: tt.actions})
			if tt.wantErr && (err == nil || !strings.Contains(err.Error(), "requires github_owned_allowed, verified_allowed, and patterns_allowed")) {
				t.Fatalf("ValidateCompleteActions() error = %v", err)
			}
			if !tt.wantErr && err != nil {
				t.Fatalf("ValidateCompleteActions() error = %v", err)
			}
		})
	}
}

func TestMergeActionsDefaults(t *testing.T) {
	defaults := defaultConfig(t)

	t.Run("absent section gets complete defaults", func(t *testing.T) {
		cfg := &Config{}
		if !mergeActionsFrom(cfg, defaults) {
			t.Fatal("mergeActionsFrom() changed = false, want true")
		}
		if cfg.Actions == nil {
			t.Fatal("Actions is nil after merge")
		}
		if err := ValidateActions(cfg); err != nil {
			t.Fatalf("ValidateActions() error: %v", err)
		}
		if cfg.Actions.Enabled == nil || !*cfg.Actions.Enabled {
			t.Errorf("enabled = %v, want true", cfg.Actions.Enabled)
		}
		if cfg.Actions.AllowedActions == nil || *cfg.Actions.AllowedActions != "all" {
			t.Errorf("allowed_actions = %v, want all", cfg.Actions.AllowedActions)
		}
		if cfg.Actions.SHAPinningRequired == nil || *cfg.Actions.SHAPinningRequired {
			t.Errorf("sha_pinning_required = %v, want false", cfg.Actions.SHAPinningRequired)
		}
		if cfg.Actions.GitHubOwnedAllowed != nil || cfg.Actions.VerifiedAllowed != nil || cfg.Actions.PatternsAllowed != nil {
			t.Fatal("default all policy contains selected-only fields")
		}
	})

	t.Run("partial selected gets default patterns", func(t *testing.T) {
		cfg := &Config{Actions: &model.ActionsSettings{
			AllowedActions:     new("selected"),
			GitHubOwnedAllowed: new(false),
		}}
		if !mergeActionsFrom(cfg, defaults) {
			t.Fatal("mergeActionsFrom() changed = false, want true")
		}
		if cfg.Actions.PatternsAllowed == nil || !slices.Equal(*cfg.Actions.PatternsAllowed, approvedDefaultActionPatterns) {
			t.Errorf("patterns_allowed = %v, want %v", cfg.Actions.PatternsAllowed, approvedDefaultActionPatterns)
		}
	})

	t.Run("partial selected preserves explicit values", func(t *testing.T) {
		emptyPatterns := []string{}
		cfg := &Config{Actions: &model.ActionsSettings{
			Enabled:            new(false),
			AllowedActions:     new("selected"),
			SHAPinningRequired: new(false),
			GitHubOwnedAllowed: new(false),
			PatternsAllowed:    &emptyPatterns,
		}}
		patternsBefore := cfg.Actions.PatternsAllowed
		if !mergeActionsFrom(cfg, defaults) {
			t.Fatal("mergeActionsFrom() changed = false, want true")
		}
		if *cfg.Actions.Enabled {
			t.Error("explicit enabled: false was replaced")
		}
		if *cfg.Actions.GitHubOwnedAllowed {
			t.Error("explicit github_owned_allowed: false was replaced")
		}
		if cfg.Actions.PatternsAllowed != patternsBefore || len(*cfg.Actions.PatternsAllowed) != 0 {
			t.Error("explicit patterns_allowed: [] was replaced")
		}
		if cfg.Actions.SHAPinningRequired == nil || *cfg.Actions.SHAPinningRequired {
			t.Errorf("sha_pinning_required = %v, want explicit false", cfg.Actions.SHAPinningRequired)
		}
		if cfg.Actions.VerifiedAllowed == nil || !*cfg.Actions.VerifiedAllowed {
			t.Errorf("verified_allowed = %v, want default true", cfg.Actions.VerifiedAllowed)
		}
		if err := ValidateActions(cfg); err != nil {
			t.Fatalf("ValidateActions() error: %v", err)
		}
	})

	t.Run("partial selected preserves explicit custom patterns", func(t *testing.T) {
		patterns := []string{"acme/private-action@v1"}
		cfg := &Config{Actions: &model.ActionsSettings{
			AllowedActions:  new("selected"),
			PatternsAllowed: &patterns,
		}}
		patternsBefore := cfg.Actions.PatternsAllowed
		if !mergeActionsFrom(cfg, defaults) {
			t.Fatal("mergeActionsFrom() changed = false, want true")
		}
		if cfg.Actions.PatternsAllowed != patternsBefore || !slices.Equal(*cfg.Actions.PatternsAllowed, patterns) {
			t.Error("explicit custom patterns list was replaced")
		}
	})

	for _, policy := range []string{"all", "local_only"} {
		t.Run(policy+" skips selected fields", func(t *testing.T) {
			cfg := &Config{Actions: &model.ActionsSettings{
				Enabled:        new(false),
				AllowedActions: new(policy),
			}}
			if !mergeActionsFrom(cfg, defaults) {
				t.Fatal("mergeActionsFrom() changed = false, want true")
			}
			if *cfg.Actions.Enabled {
				t.Error("explicit enabled: false was replaced")
			}
			if cfg.Actions.SHAPinningRequired == nil || *cfg.Actions.SHAPinningRequired {
				t.Errorf("sha_pinning_required = %v, want default false", cfg.Actions.SHAPinningRequired)
			}
			if cfg.Actions.GitHubOwnedAllowed != nil || cfg.Actions.VerifiedAllowed != nil || cfg.Actions.PatternsAllowed != nil {
				t.Fatalf("selected fields were added for %s: %+v", policy, cfg.Actions)
			}
			if err := ValidateActions(cfg); err != nil {
				t.Fatalf("ValidateActions() error: %v", err)
			}
		})
	}

	t.Run("complete selected policy is unchanged", func(t *testing.T) {
		emptyPatterns := []string{}
		cfg := &Config{Actions: &model.ActionsSettings{
			Enabled:                   new(false),
			AllowedActions:            new("selected"),
			SHAPinningRequired:        new(true),
			GitHubOwnedAllowed:        new(false),
			VerifiedAllowed:           new(false),
			PatternsAllowed:           &emptyPatterns,
			ForkPRContributorApproval: &model.ForkPRContributorApprovalSettings{ApprovalPolicy: new("first_time_contributors")},
		}}
		patternsBefore := cfg.Actions.PatternsAllowed
		if mergeActionsFrom(cfg, defaults) {
			t.Fatal("mergeActionsFrom() changed = true, want false")
		}
		if cfg.Actions.SHAPinningRequired == nil || !*cfg.Actions.SHAPinningRequired {
			t.Error("explicit sha_pinning_required: true was replaced")
		}
		if cfg.Actions.PatternsAllowed != patternsBefore {
			t.Error("explicit empty patterns list was replaced")
		}
	})
}

func TestForkPRContributorApprovalParsingAndWriting(t *testing.T) {
	type testCase struct {
		name       string
		input      string
		wantPolicy *string
		wantObject bool
		wantErr    string
	}
	tests := []testCase{
		{name: "absent actions", input: ""},
		{name: "omitted", input: "actions: {}\n"},
		{name: "null object", input: "actions:\n  fork_pr_contributor_approval: null\n"},
		{name: "empty object", input: "actions:\n  fork_pr_contributor_approval: {}\n", wantObject: true},
		{name: "null policy", input: "actions:\n  fork_pr_contributor_approval:\n    approval_policy: null\n", wantObject: true},
		{name: "empty policy", input: "actions:\n  fork_pr_contributor_approval:\n    approval_policy: \"\"\n", wantErr: "invalid actions.fork_pr_contributor_approval.approval_policy"},
		{name: "unknown policy", input: "actions:\n  fork_pr_contributor_approval:\n    approval_policy: never\n", wantErr: "invalid actions.fork_pr_contributor_approval.approval_policy"},
		{name: "unknown key", input: "actions:\n  fork_pr_contributor_approval:\n    enabled: true\n", wantErr: "unrecognised actions.fork_pr_contributor_approval setting"},
		{name: "string object", input: "actions:\n  fork_pr_contributor_approval: invalid\n", wantErr: "cannot unmarshal"},
		{name: "list object", input: "actions:\n  fork_pr_contributor_approval: []\n", wantErr: "cannot unmarshal"},
		{name: "map policy", input: "actions:\n  fork_pr_contributor_approval:\n    approval_policy: {}\n", wantErr: "cannot unmarshal"},
		{name: "list policy", input: "actions:\n  fork_pr_contributor_approval:\n    approval_policy: []\n", wantErr: "cannot unmarshal"},
		{name: "boolean policy", input: "actions:\n  fork_pr_contributor_approval:\n    approval_policy: true\n", wantErr: "invalid actions.fork_pr_contributor_approval.approval_policy"},
		{name: "number policy", input: "actions:\n  fork_pr_contributor_approval:\n    approval_policy: 42\n", wantErr: "invalid actions.fork_pr_contributor_approval.approval_policy"},
	}
	for _, policy := range []string{"first_time_contributors_new_to_github", "first_time_contributors", "all_external_contributors"} {
		tests = append(tests, testCase{name: policy, input: "actions:\n  fork_pr_contributor_approval:\n    approval_policy: " + policy + "\n", wantPolicy: new(policy), wantObject: true})
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg, err := parseAndValidate([]byte("license: none\n"+tt.input+"swatches: []\n"), "test")
			assertErrorContains(t, err, tt.wantErr)
			if tt.wantErr != "" {
				return
			}
			var approval *model.ForkPRContributorApprovalSettings
			if cfg.Actions != nil {
				approval = cfg.Actions.ForkPRContributorApproval
			}
			if (approval != nil) != tt.wantObject {
				t.Fatalf("approval = %+v, want object %t", approval, tt.wantObject)
			}
			if tt.wantPolicy != nil && (approval.ApprovalPolicy == nil || *approval.ApprovalPolicy != *tt.wantPolicy) {
				t.Fatalf("approval = %+v, want %s", approval, *tt.wantPolicy)
			}
			if tt.wantPolicy == nil && approval != nil && approval.ApprovalPolicy != nil {
				t.Fatal("parsing added a policy")
			}
			written := writeConfig(t, cfg, "2026-09-07", "Fitted")
			reloaded, err := parseAndValidate([]byte(written), "round trip")
			if err != nil {
				t.Fatal(err)
			}
			if tt.wantPolicy != nil {
				if reloaded.Actions == nil || reloaded.Actions.ForkPRContributorApproval == nil || reloaded.Actions.ForkPRContributorApproval.ApprovalPolicy == nil || *reloaded.Actions.ForkPRContributorApproval.ApprovalPolicy != *tt.wantPolicy {
					t.Fatalf("round trip lost %s", *tt.wantPolicy)
				}
			} else if strings.Contains(written, "fork_pr_contributor_approval") {
				t.Fatal("writing added an unmanaged approval policy")
			}
		})
	}
}

func TestForkPRContributorApprovalDefaults(t *testing.T) {
	defaults := defaultConfig(t)
	if defaults.Actions == nil || defaults.Actions.ForkPRContributorApproval == nil || defaults.Actions.ForkPRContributorApproval.ApprovalPolicy == nil || *defaults.Actions.ForkPRContributorApproval.ApprovalPolicy != "first_time_contributors" {
		t.Fatal("default approval policy is not first_time_contributors")
	}
	for _, corePolicy := range []string{"", "all", "local_only", "selected"} {
		for _, approvalPolicy := range append([]string{"omitted", "empty"}, model.ForkPRContributorApprovalPolicies...) {
			t.Run(corePolicy+"/"+approvalPolicy, func(t *testing.T) {
				cfg := &Config{}
				if corePolicy != "" {
					cfg.Actions = &model.ActionsSettings{AllowedActions: new(corePolicy)}
				}
				var explicit *string
				if approvalPolicy != "omitted" {
					if cfg.Actions == nil {
						cfg.Actions = &model.ActionsSettings{}
					}
					cfg.Actions.ForkPRContributorApproval = &model.ForkPRContributorApprovalSettings{}
					if approvalPolicy != "empty" {
						explicit = new(approvalPolicy)
						cfg.Actions.ForkPRContributorApproval.ApprovalPolicy = explicit
					}
				}
				changed, err := MergeDefaults(cfg)
				if err != nil || !changed {
					t.Fatalf("MergeDefaults() = %t, %v", changed, err)
				}
				approval := cfg.Actions.ForkPRContributorApproval
				want := "first_time_contributors"
				if explicit != nil {
					want = *explicit
					if approval.ApprovalPolicy != explicit {
						t.Fatal("merge replaced an explicit policy pointer")
					}
				}
				if approval == nil || approval.ApprovalPolicy == nil || *approval.ApprovalPolicy != want {
					t.Fatalf("approval = %+v, want %s", approval, want)
				}
				if corePolicy != "" && *cfg.Actions.AllowedActions != corePolicy {
					t.Fatal("merge replaced core policy")
				}
				if corePolicy == "all" || corePolicy == "local_only" {
					if cfg.Actions.GitHubOwnedAllowed != nil || cfg.Actions.VerifiedAllowed != nil || cfg.Actions.PatternsAllowed != nil {
						t.Fatal("merge added selected fields")
					}
				}
				if err := ValidateActions(cfg); err != nil {
					t.Fatal(err)
				}
				if changed, err := MergeDefaults(cfg); err != nil || changed {
					t.Fatalf("second MergeDefaults() = %t, %v", changed, err)
				}
				other := &Config{}
				if _, err := MergeDefaults(other); err != nil {
					t.Fatal(err)
				}
				*approval.ApprovalPolicy = "all_external_contributors"
				if *other.Actions.ForkPRContributorApproval.ApprovalPolicy != "first_time_contributors" || *defaults.Actions.ForkPRContributorApproval.ApprovalPolicy != "first_time_contributors" {
					t.Fatal("merged approval policies share pointers")
				}
			})
		}
	}
}

func TestWriteActionsEmptyPatterns(t *testing.T) {
	cfg := &Config{License: "none", Actions: &model.ActionsSettings{
		AllowedActions:  new("selected"),
		PatternsAllowed: &[]string{},
	}}
	written := writeConfig(t, cfg, "2026-08-24", "Refitted")
	if !strings.Contains(written, "  patterns_allowed: []\n") {
		t.Fatalf("config does not contain explicit empty patterns list:\n%s", written)
	}
}

func TestActionsRetentionParsingAndWriting(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		wantDays *int
		wantErr  string
	}{
		{name: "omitted", input: "{}"},
		{name: "missing days", input: "\n  artifact_and_log_retention: {}"},
		{name: "null days", input: "\n  artifact_and_log_retention:\n    days: null"},
		{name: "minimum", input: "\n  artifact_and_log_retention:\n    days: 1", wantDays: new(1)},
		{name: "maximum", input: "\n  artifact_and_log_retention:\n    days: 90", wantDays: new(90)},
		{name: "zero", input: "\n  artifact_and_log_retention:\n    days: 0", wantErr: "must be between 1 and 90"},
		{name: "negative", input: "\n  artifact_and_log_retention:\n    days: -1", wantErr: "must be between 1 and 90"},
		{name: "above maximum", input: "\n  artifact_and_log_retention:\n    days: 91", wantErr: "must be between 1 and 90"},
		{name: "unknown nested key", input: "\n  artifact_and_log_retention:\n    day: 30", wantErr: "unrecognised actions.artifact_and_log_retention setting"},
		{name: "live cap is not configurable", input: "\n  artifact_and_log_retention:\n    days: 30\n    maximum_allowed_days: 90", wantErr: "unrecognised actions.artifact_and_log_retention setting"},
		{name: "non integer", input: "\n  artifact_and_log_retention:\n    days: invalid", wantErr: "cannot unmarshal"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			testutil.WriteConfig(t, dir, "license: none\nactions: "+tt.input+"\nswatches: []\n")
			cfg, err := Load(dir)
			assertErrorContains(t, err, tt.wantErr)
			if tt.wantErr != "" {
				return
			}
			if err := ValidateCompleteActions(cfg); err != nil {
				t.Fatalf("retention-only config requires selected fields: %v", err)
			}
			if tt.wantDays != nil {
				if cfg.Actions.ArtifactAndLogRetention == nil || cfg.Actions.ArtifactAndLogRetention.Days == nil || *cfg.Actions.ArtifactAndLogRetention.Days != *tt.wantDays {
					t.Fatalf("retention = %+v, want days %d", cfg.Actions.ArtifactAndLogRetention, *tt.wantDays)
				}
			} else if cfg.Actions.ArtifactAndLogRetention != nil && cfg.Actions.ArtifactAndLogRetention.Days != nil {
				t.Fatal("missing days became managed")
			}
			if err := Write(dir, cfg, "2026-09-07", "Refitted"); err != nil {
				t.Fatal(err)
			}
			written, err := os.ReadFile(filepath.Join(dir, ConfigSwatchPath))
			if err != nil {
				t.Fatal(err)
			}
			if got := strings.Contains(string(written), "  artifact_and_log_retention:\n    days: "); got != (tt.wantDays != nil) {
				t.Fatalf("written retention presence = %t, want %t", got, tt.wantDays != nil)
			}
			reloaded, err := Load(dir)
			if err != nil {
				t.Fatal(err)
			}
			if tt.wantDays != nil && (reloaded.Actions == nil || reloaded.Actions.ArtifactAndLogRetention == nil || reloaded.Actions.ArtifactAndLogRetention.Days == nil || *reloaded.Actions.ArtifactAndLogRetention.Days != *tt.wantDays) {
				t.Fatalf("written retention did not preserve %d days", *tt.wantDays)
			}
		})
	}
}

func TestActionsRetentionDefaults(t *testing.T) {
	defaults := defaultConfig(t)
	if defaults.Actions == nil || defaults.Actions.ArtifactAndLogRetention != nil {
		t.Fatal("default Actions config must omit retention")
	}
	if strings.Contains(writeConfig(t, defaults, "2026-09-07", "Fitted"), "artifact_and_log_retention") {
		t.Fatal("written defaults contain retention")
	}
	for _, days := range []*int{nil, new(30)} {
		name := "omitted"
		cfg := &Config{}
		if days != nil {
			name = "explicit"
			cfg.Actions = &model.ActionsSettings{ArtifactAndLogRetention: &model.ArtifactAndLogRetentionSettings{Days: days}}
		}
		t.Run(name, func(t *testing.T) {
			for range 2 {
				if _, err := MergeDefaults(cfg); err != nil {
					t.Fatal(err)
				}
				if days == nil {
					if cfg.Actions.ArtifactAndLogRetention != nil {
						t.Fatal("default merging added retention")
					}
				} else if cfg.Actions.ArtifactAndLogRetention == nil || cfg.Actions.ArtifactAndLogRetention.Days != days || *days != 30 {
					t.Fatal("default merging changed explicit days")
				}
			}
		})
	}
}
