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
		{name: "unknown", actions: &model.ActionsSettings{Extra: map[string]interface{}{"unknown": true}}, wantErr: "unrecognised actions setting"},
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
		if cfg.Actions.AllowedActions == nil || *cfg.Actions.AllowedActions != "selected" {
			t.Errorf("allowed_actions = %v, want selected", cfg.Actions.AllowedActions)
		}
		if cfg.Actions.SHAPinningRequired == nil || *cfg.Actions.SHAPinningRequired {
			t.Errorf("sha_pinning_required = %v, want false", cfg.Actions.SHAPinningRequired)
		}
		if cfg.Actions.GitHubOwnedAllowed == nil || !*cfg.Actions.GitHubOwnedAllowed {
			t.Errorf("github_owned_allowed = %v, want true", cfg.Actions.GitHubOwnedAllowed)
		}
		if cfg.Actions.VerifiedAllowed == nil || !*cfg.Actions.VerifiedAllowed {
			t.Errorf("verified_allowed = %v, want true", cfg.Actions.VerifiedAllowed)
		}
		if cfg.Actions.PatternsAllowed == nil || !slices.Equal(*cfg.Actions.PatternsAllowed, approvedDefaultActionPatterns) {
			t.Errorf("patterns_allowed = %v, want %v", cfg.Actions.PatternsAllowed, approvedDefaultActionPatterns)
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
			t.Errorf("sha_pinning_required = %v, want default false", cfg.Actions.SHAPinningRequired)
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
			Enabled:            new(false),
			AllowedActions:     new("selected"),
			SHAPinningRequired: new(true),
			GitHubOwnedAllowed: new(false),
			VerifiedAllowed:    new(false),
			PatternsAllowed:    &emptyPatterns,
		}}
		patternsBefore := cfg.Actions.PatternsAllowed
		if mergeActionsFrom(cfg, defaults) {
			t.Fatal("mergeActionsFrom() changed = true, want false")
		}
		if cfg.Actions.PatternsAllowed != patternsBefore {
			t.Error("explicit empty patterns list was replaced")
		}
	})
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
