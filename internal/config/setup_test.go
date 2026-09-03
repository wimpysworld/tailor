package config

import (
	"strings"
	"testing"

	"github.com/wimpysworld/tailor/internal/model"
	"github.com/wimpysworld/tailor/internal/testutil"
)

func TestValidateSecretScanning(t *testing.T) {
	tests := []struct {
		name           string
		scanning       *string
		pushProtection *string
		nonProvider    *string
		wantErr        string
	}{
		{name: "absent"},
		{name: "enabled", scanning: new("enabled"), pushProtection: new("enabled"), nonProvider: new("enabled")},
		{name: "disabled", scanning: new("disabled"), pushProtection: new("disabled"), nonProvider: new("disabled")},
		{name: "invalid scanning", scanning: new("on"), wantErr: `invalid secret_scanning "on"; must be "enabled" or "disabled"`},
		{name: "invalid push protection", pushProtection: new("true"), wantErr: `invalid secret_scanning_push_protection "true"; must be "enabled" or "disabled"`},
		{name: "invalid non-provider patterns", nonProvider: new("yes"), wantErr: `invalid secret_scanning_non_provider_patterns "yes"; must be "enabled" or "disabled"`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &Config{Repository: &model.RepositorySettings{
				SecretScanning:                    tt.scanning,
				SecretScanningPushProtection:      tt.pushProtection,
				SecretScanningNonProviderPatterns: tt.nonProvider,
			}}
			err := ValidateSecretScanning(cfg)
			assertErrorContains(t, err, tt.wantErr)
		})
	}
}

func TestValidateCodeScanning(t *testing.T) {
	tests := []struct {
		name    string
		section *model.CodeScanningSettings
		wantErr string
	}{
		{name: "absent"},
		{name: "empty", section: &model.CodeScanningSettings{}},
		{name: "valid", section: &model.CodeScanningSettings{
			State: new("configured"), QuerySuite: new("extended"), ThreatModel: new("remote_and_local"),
			Languages: &[]string{"go", "actions"},
		}},
		{name: "empty languages", section: &model.CodeScanningSettings{Languages: &[]string{}}},
		{name: "invalid state", section: &model.CodeScanningSettings{State: new("enabled")}, wantErr: `invalid code_scanning.state "enabled"; must be "configured" or "not-configured"`},
		{name: "invalid query suite", section: &model.CodeScanningSettings{QuerySuite: new("security")}, wantErr: `invalid code_scanning.query_suite "security"; must be "default" or "extended"`},
		{name: "invalid threat model", section: &model.CodeScanningSettings{ThreatModel: new("local")}, wantErr: `invalid code_scanning.threat_model "local"; must be "remote" or "remote_and_local"`},
		{name: "unknown language", section: &model.CodeScanningSettings{Languages: &[]string{"go", "rust"}}, wantErr: `code_scanning.languages[1]: unrecognised language "rust"; valid languages: actions, c-cpp, csharp, go, java-kotlin, javascript-typescript, python, ruby, swift`},
		{name: "duplicate language", section: &model.CodeScanningSettings{Languages: &[]string{"go", "go"}}, wantErr: `code_scanning.languages[1]: duplicate language "go"`},
		{name: "unknown key", section: &model.CodeScanningSettings{Extra: map[string]any{"runner_type": "standard"}}, wantErr: `unrecognised code_scanning setting "runner_type" in config; valid settings: languages, query_suite, state, threat_model`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateCodeScanning(&Config{CodeScanning: tt.section})
			assertErrorContains(t, err, tt.wantErr)
		})
	}
}

func TestValidateCodeQuality(t *testing.T) {
	tests := []struct {
		name    string
		section *model.CodeQualitySettings
		wantErr string
	}{
		{name: "absent"},
		{name: "valid", section: &model.CodeQualitySettings{State: new("not-configured"), Languages: &[]string{"python", "go"}}},
		{name: "invalid state", section: &model.CodeQualitySettings{State: new("on")}, wantErr: `invalid code_quality.state "on"; must be "configured" or "not-configured"`},
		{name: "unknown language", section: &model.CodeQualitySettings{Languages: &[]string{"swift"}}, wantErr: `code_quality.languages[0]: unrecognised language "swift"; valid languages: csharp, go, java-kotlin, javascript-typescript, python, ruby`},
		{name: "duplicate language", section: &model.CodeQualitySettings{Languages: &[]string{"go", "python", "go"}}, wantErr: `code_quality.languages[2]: duplicate language "go"`},
		{name: "unknown key", section: &model.CodeQualitySettings{Extra: map[string]any{"ai_findings_option": "enabled"}}, wantErr: `unrecognised code_quality setting "ai_findings_option" in config; valid settings: languages, state`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateCodeQuality(&Config{CodeQuality: tt.section})
			assertErrorContains(t, err, tt.wantErr)
		})
	}
}

func assertErrorContains(t *testing.T, err error, want string) {
	t.Helper()
	if want == "" {
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		return
	}
	if err == nil || !strings.Contains(err.Error(), want) {
		t.Fatalf("error = %v, want %q", err, want)
	}
}

func TestLoadRejectsInvalidSetupSections(t *testing.T) {
	tests := []struct {
		name string
		yaml string
		want string
	}{
		{name: "secret scanning", yaml: "license: none\nrepository:\n  secret_scanning: yes\nswatches: []\n", want: "invalid secret_scanning"},
		{name: "code scanning", yaml: "license: none\ncode_scanning:\n  state: on\nswatches: []\n", want: "invalid code_scanning.state"},
		{name: "code quality", yaml: "license: none\ncode_quality:\n  languages: [go, go]\nswatches: []\n", want: "duplicate language"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			testutil.WriteConfig(t, dir, tt.yaml)
			_, err := Load(dir)
			assertErrorContains(t, err, tt.want)
		})
	}
}

func TestNormaliseSecretScanningPrerequisites(t *testing.T) {
	const (
		pushWarning        = "warning: set secret_scanning to enabled because secret_scanning_push_protection requires secret scanning"
		nonProviderWarning = "warning: set secret_scanning to enabled because secret_scanning_non_provider_patterns requires secret scanning"
	)
	tests := []struct {
		name           string
		scanning       *string
		pushProtection *string
		nonProvider    *string
		wantWarnings   []string
		wantScanning   *string
	}{
		{name: "nil scanning", pushProtection: new("enabled"), wantWarnings: []string{pushWarning}, wantScanning: new("enabled")},
		{name: "disabled scanning", scanning: new("disabled"), pushProtection: new("enabled"), wantWarnings: []string{pushWarning}, wantScanning: new("enabled")},
		{name: "enabled scanning", scanning: new("enabled"), pushProtection: new("enabled"), wantScanning: new("enabled")},
		{name: "disabled push protection", scanning: new("disabled"), pushProtection: new("disabled"), wantScanning: new("disabled")},
		{name: "nil push protection", scanning: new("disabled"), wantScanning: new("disabled")},
		{name: "non-provider patterns nil scanning", nonProvider: new("enabled"), wantWarnings: []string{nonProviderWarning}, wantScanning: new("enabled")},
		{name: "non-provider patterns disabled scanning", scanning: new("disabled"), nonProvider: new("enabled"), wantWarnings: []string{nonProviderWarning}, wantScanning: new("enabled")},
		{name: "non-provider patterns enabled scanning", scanning: new("enabled"), nonProvider: new("enabled"), wantScanning: new("enabled")},
		{name: "non-provider patterns disabled", scanning: new("disabled"), nonProvider: new("disabled"), wantScanning: new("disabled")},
		{name: "both features", scanning: new("disabled"), pushProtection: new("enabled"), nonProvider: new("enabled"), wantWarnings: []string{pushWarning, nonProviderWarning}, wantScanning: new("enabled")},
		{name: "nil repository"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &Config{}
			if tt.scanning != nil || tt.pushProtection != nil || tt.nonProvider != nil {
				cfg.Repository = &model.RepositorySettings{
					SecretScanning:                    tt.scanning,
					SecretScanningPushProtection:      tt.pushProtection,
					SecretScanningNonProviderPatterns: tt.nonProvider,
				}
			}
			got := NormaliseSecretScanningPrerequisites(cfg)
			if strings.Join(got, "\n") != strings.Join(tt.wantWarnings, "\n") {
				t.Fatalf("NormaliseSecretScanningPrerequisites() = %q, want %q", got, tt.wantWarnings)
			}
			if cfg.Repository == nil {
				return
			}
			testutil.AssertPtrEqual(t, cfg.Repository.SecretScanning, tt.wantScanning, "secret_scanning")
			if got := NormaliseSecretScanningPrerequisites(cfg); len(got) != 0 {
				t.Fatalf("second NormaliseSecretScanningPrerequisites() call changed the config: %q", got)
			}
		})
	}
}

func TestDefaultConfigSetupSections(t *testing.T) {
	got, err := DefaultConfig("none")
	if err != nil {
		t.Fatalf("DefaultConfig() error: %v", err)
	}
	testutil.AssertPtr(t, got.Repository.SecretScanning, false, "enabled", "secret_scanning")
	testutil.AssertPtr(t, got.Repository.SecretScanningPushProtection, false, "enabled", "secret_scanning_push_protection")
	testutil.AssertPtr(t, got.Repository.SecretScanningNonProviderPatterns, false, "enabled", "secret_scanning_non_provider_patterns")
	if got.CodeScanning == nil || got.CodeQuality == nil {
		t.Fatal("default code_scanning or code_quality section is nil")
	}
	testutil.AssertPtr(t, got.CodeScanning.State, false, "configured", "code_scanning.state")
	testutil.AssertPtr(t, got.CodeScanning.QuerySuite, false, "default", "code_scanning.query_suite")
	testutil.AssertPtr(t, got.CodeScanning.ThreatModel, false, "remote", "code_scanning.threat_model")
	if got.CodeScanning.Languages == nil || len(*got.CodeScanning.Languages) != 0 {
		t.Errorf("code_scanning.languages = %v, want empty list", got.CodeScanning.Languages)
	}
	testutil.AssertPtr(t, got.CodeQuality.State, false, "not-configured", "code_quality.state")
	if got.CodeQuality.Languages == nil || len(*got.CodeQuality.Languages) != 0 {
		t.Errorf("code_quality.languages = %v, want empty list", got.CodeQuality.Languages)
	}
}

func TestMergeDefaultsAddsSetupSections(t *testing.T) {
	cfg := &Config{License: "none"}
	changed, err := MergeDefaults(cfg)
	if err != nil {
		t.Fatalf("MergeDefaults() error: %v", err)
	}
	if !changed {
		t.Fatal("expected changed=true for an empty config")
	}
	testutil.AssertPtr(t, cfg.Repository.SecretScanning, false, "enabled", "secret_scanning")
	testutil.AssertPtr(t, cfg.Repository.SecretScanningPushProtection, false, "enabled", "secret_scanning_push_protection")
	testutil.AssertPtr(t, cfg.Repository.SecretScanningNonProviderPatterns, false, "enabled", "secret_scanning_non_provider_patterns")
	if cfg.CodeScanning == nil || cfg.CodeQuality == nil {
		t.Fatal("merged config is missing a setup section")
	}
	testutil.AssertPtr(t, cfg.CodeScanning.State, false, "configured", "code_scanning.state")
	testutil.AssertPtr(t, cfg.CodeScanning.QuerySuite, false, "default", "code_scanning.query_suite")
	testutil.AssertPtr(t, cfg.CodeScanning.ThreatModel, false, "remote", "code_scanning.threat_model")
	if cfg.CodeScanning.Languages == nil || len(*cfg.CodeScanning.Languages) != 0 {
		t.Errorf("code_scanning.languages = %v, want empty list", cfg.CodeScanning.Languages)
	}
	testutil.AssertPtr(t, cfg.CodeQuality.State, false, "not-configured", "code_quality.state")
	if cfg.CodeQuality.Languages == nil || len(*cfg.CodeQuality.Languages) != 0 {
		t.Errorf("code_quality.languages = %v, want empty list", cfg.CodeQuality.Languages)
	}
}

func TestMergeDefaultsKeepsExistingSetupEntries(t *testing.T) {
	cfg := &Config{
		License:      "none",
		CodeScanning: &model.CodeScanningSettings{State: new("not-configured"), Languages: &[]string{"go"}},
		CodeQuality:  &model.CodeQualitySettings{State: new("configured")},
	}
	if _, err := MergeDefaults(cfg); err != nil {
		t.Fatalf("MergeDefaults() error: %v", err)
	}
	testutil.AssertPtr(t, cfg.CodeScanning.State, false, "not-configured", "code_scanning.state")
	testutil.AssertPtr(t, cfg.CodeScanning.QuerySuite, false, "default", "code_scanning.query_suite")
	if cfg.CodeScanning.Languages == nil || len(*cfg.CodeScanning.Languages) != 1 {
		t.Errorf("code_scanning.languages = %v, want [go]", cfg.CodeScanning.Languages)
	}
	testutil.AssertPtr(t, cfg.CodeQuality.State, false, "configured", "code_quality.state")
	if cfg.CodeQuality.Languages == nil || len(*cfg.CodeQuality.Languages) != 0 {
		t.Errorf("code_quality.languages = %v, want empty list", cfg.CodeQuality.Languages)
	}

	changed, err := MergeDefaults(cfg)
	if err != nil {
		t.Fatalf("second MergeDefaults() error: %v", err)
	}
	if changed {
		t.Fatal("second MergeDefaults() changed a complete config")
	}
}

func TestWriteSetupSections(t *testing.T) {
	tests := []struct {
		name         string
		cfg          *Config
		want         []string
		wantMissing  []string
		wantSections bool
	}{
		{
			name: "non-empty languages keep block style",
			cfg: &Config{
				License:      "none",
				CodeScanning: &model.CodeScanningSettings{State: new("configured"), Languages: &[]string{"go", "actions"}},
				CodeQuality:  &model.CodeQualitySettings{State: new("configured"), Languages: &[]string{"python"}},
			},
			want: []string{
				"\ncode_scanning:\n  state: configured\n" +
					"  # An empty list means GitHub detects the languages. Valid values:\n" +
					"  # actions, c-cpp, csharp, go, java-kotlin, javascript-typescript, python, ruby, swift\n" +
					"  languages:\n    - go\n    - actions\n",
				"\ncode_quality:\n  state: configured\n" +
					"  # An empty list means GitHub detects the languages. Valid values:\n" +
					"  # csharp, go, java-kotlin, javascript-typescript, python, ruby\n" +
					"  languages:\n    - python\n",
			},
		},
		{
			name: "nil languages omit the comment",
			cfg: &Config{
				License:      "none",
				CodeScanning: &model.CodeScanningSettings{State: new("configured")},
				CodeQuality:  &model.CodeQualitySettings{State: new("not-configured")},
			},
			want:        []string{"\ncode_scanning:\n  state: configured\n\ncode_quality:\n  state: not-configured\n\nswatches:\n"},
			wantMissing: []string{"languages", "# An empty list"},
		},
		{
			name:        "absent sections",
			cfg:         &Config{License: "none"},
			wantMissing: []string{"code_scanning:", "code_quality:"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			content := writeConfig(t, tt.cfg, "2026-03-02", "Refitted")
			for _, want := range tt.want {
				if !strings.Contains(content, want) {
					t.Errorf("config missing %q:\n%s", want, content)
				}
			}
			for _, missing := range tt.wantMissing {
				if strings.Contains(content, missing) {
					t.Errorf("config contains %q:\n%s", missing, content)
				}
			}
		})
	}
}

func TestWriteSetupSectionsRoundTrip(t *testing.T) {
	dir := t.TempDir()
	cfg := &Config{
		License:      "none",
		CodeScanning: &model.CodeScanningSettings{State: new("configured"), QuerySuite: new("extended"), Languages: &[]string{"go"}},
		CodeQuality:  &model.CodeQualitySettings{State: new("configured"), Languages: &[]string{}},
	}
	if err := Write(dir, cfg, "2026-03-02", "Refitted"); err != nil {
		t.Fatalf("Write() error: %v", err)
	}
	loaded, err := Load(dir)
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}
	testutil.AssertPtr(t, loaded.CodeScanning.QuerySuite, false, "extended", "code_scanning.query_suite")
	if loaded.CodeScanning.Languages == nil || len(*loaded.CodeScanning.Languages) != 1 || (*loaded.CodeScanning.Languages)[0] != "go" {
		t.Errorf("code_scanning.languages = %v, want [go]", loaded.CodeScanning.Languages)
	}
	if loaded.CodeQuality.Languages == nil || len(*loaded.CodeQuality.Languages) != 0 {
		t.Errorf("code_quality.languages = %v, want empty list", loaded.CodeQuality.Languages)
	}
}

func TestMergeSetupWritesEmptyLanguages(t *testing.T) {
	cfg := &Config{}
	MergeCodeScanningSetup(cfg, &model.CodeScanningSettings{
		State: new("configured"), QuerySuite: new("extended"), ThreatModel: new("remote_and_local"),
		Languages: &[]string{"go", "actions"},
	})
	MergeCodeQualitySetup(cfg, &model.CodeQualitySettings{State: new("configured"), Languages: &[]string{"go"}})

	testutil.AssertPtr(t, cfg.CodeScanning.State, false, "configured", "code_scanning.state")
	testutil.AssertPtr(t, cfg.CodeScanning.QuerySuite, false, "extended", "code_scanning.query_suite")
	testutil.AssertPtr(t, cfg.CodeScanning.ThreatModel, false, "remote_and_local", "code_scanning.threat_model")
	if cfg.CodeScanning.Languages == nil || len(*cfg.CodeScanning.Languages) != 0 {
		t.Errorf("code_scanning.languages = %v, want empty list", cfg.CodeScanning.Languages)
	}
	testutil.AssertPtr(t, cfg.CodeQuality.State, false, "configured", "code_quality.state")
	if cfg.CodeQuality.Languages == nil || len(*cfg.CodeQuality.Languages) != 0 {
		t.Errorf("code_quality.languages = %v, want empty list", cfg.CodeQuality.Languages)
	}
}
