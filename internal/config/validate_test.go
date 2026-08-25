package config

import (
	"fmt"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"

	"github.com/wimpysworld/tailor/internal/model"
	"github.com/wimpysworld/tailor/internal/swatch"
)

func TestValidatePathsAcceptsValidConfig(t *testing.T) {
	cfg := &Config{
		Swatches: []SwatchEntry{
			{Path: ".gitignore", Alteration: swatch.FirstFit},
			{Path: "justfile", Alteration: swatch.FirstFit},
		},
	}
	if err := ValidatePaths(cfg); err != nil {
		t.Fatalf("ValidatePaths() returned unexpected error: %v", err)
	}
}

func TestValidatePathsRejectsUnknownPath(t *testing.T) {
	cfg := &Config{
		Swatches: []SwatchEntry{
			{Path: "nonexistent.txt", Alteration: swatch.Always},
		},
	}
	err := ValidatePaths(cfg)
	if err == nil {
		t.Fatal("ValidatePaths() expected error for unknown path, got nil")
	}
	if !strings.Contains(err.Error(), `unrecognised swatch path "nonexistent.txt"`) {
		t.Errorf("error = %q, want it to contain unrecognised path message", err)
	}
	if !strings.Contains(err.Error(), "valid paths:") {
		t.Errorf("error = %q, want it to list valid paths", err)
	}
}

func TestValidatePathsRejectsRetiredAutomergeWorkflow(t *testing.T) {
	const path = ".github/workflows/tailor-automerge.yml"
	cfg := &Config{
		Swatches: []SwatchEntry{
			{Path: path, Alteration: swatch.Always},
		},
	}

	err := ValidatePaths(cfg)
	if err == nil {
		t.Fatal("ValidatePaths() expected error for retired automerge workflow, got nil")
	}
	want := fmt.Sprintf("unrecognised swatch path %q in config; valid paths: %s", path, strings.Join(swatch.Paths(), ", "))
	if err.Error() != want {
		t.Errorf("error = %q, want %q", err, want)
	}
}

func TestValidatePathsAcceptsEmptySwatches(t *testing.T) {
	cfg := &Config{}
	if err := ValidatePaths(cfg); err != nil {
		t.Fatalf("ValidatePaths() on empty swatches: %v", err)
	}
}

func TestValidateDuplicatePathsAcceptsUnique(t *testing.T) {
	cfg := &Config{
		Swatches: []SwatchEntry{
			{Path: ".gitignore", Alteration: swatch.FirstFit},
			{Path: "justfile", Alteration: swatch.FirstFit},
		},
	}
	if err := ValidateDuplicatePaths(cfg); err != nil {
		t.Fatalf("ValidateDuplicatePaths() returned unexpected error: %v", err)
	}
}

func TestValidateDuplicatePathsRejectsDuplicate(t *testing.T) {
	cfg := &Config{
		Swatches: []SwatchEntry{
			{Path: ".gitignore", Alteration: swatch.FirstFit},
			{Path: ".gitignore", Alteration: swatch.Always},
		},
	}
	err := ValidateDuplicatePaths(cfg)
	if err == nil {
		t.Fatal("ValidateDuplicatePaths() expected error for duplicate path, got nil")
	}
	if !strings.Contains(err.Error(), `duplicate swatch path ".gitignore"`) {
		t.Errorf("error = %q, want it to contain duplicate path message", err)
	}
}

func TestValidateRepoSettingsAcceptsValidConfig(t *testing.T) {
	cfg := &Config{
		Repository: &model.RepositorySettings{
			HasWiki:   new(false),
			HasIssues: new(true),
			Homepage:  new("https://example.com"),
		},
	}
	if err := ValidateRepoSettings(cfg); err != nil {
		t.Fatalf("ValidateRepoSettings() returned unexpected error: %v", err)
	}
}

func TestValidateRepoSettingsAcceptsNilRepository(t *testing.T) {
	cfg := &Config{}
	if err := ValidateRepoSettings(cfg); err != nil {
		t.Fatalf("ValidateRepoSettings() on nil repository: %v", err)
	}
}

func TestValidateRepoSettingsAcceptsNormalisableSecuritySettings(t *testing.T) {
	cfg := &Config{Repository: &model.RepositorySettings{
		VulnerabilityAlertsEnabled:    new(false),
		AutomatedSecurityFixesEnabled: new(true),
	}}
	if err := ValidateRepoSettings(cfg); err != nil {
		t.Fatalf("ValidateRepoSettings() returned unexpected error: %v", err)
	}
}

func TestValidateRepoSettingsRejectsUnknownSetting(t *testing.T) {
	// Unmarshal YAML with an unknown key to populate the Extra map.
	input := `repository:
  has_wiki: false
  bogus_setting: true
swatches: []
`
	var cfg Config
	if err := yaml.Unmarshal([]byte(input), &cfg); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}
	err := ValidateRepoSettings(&cfg)
	if err == nil {
		t.Fatal("ValidateRepoSettings() expected error for unknown setting, got nil")
	}
	if !strings.Contains(err.Error(), `unrecognised repository setting "bogus_setting"`) {
		t.Errorf("error = %q, want it to identify bogus_setting", err)
	}
	if !strings.Contains(err.Error(), "valid settings:") {
		t.Errorf("error = %q, want it to list valid settings", err)
	}
}

func TestRepoSettingNamesContainsExpectedFields(t *testing.T) {
	names := repoSettingNames()
	expected := []string{
		"allow_auto_merge",
		"allow_merge_commit",
		"allow_rebase_merge",
		"allow_squash_merge",
		"allow_update_branch",
		"automated_security_fixes_enabled",
		"can_approve_pull_request_reviews",
		"default_workflow_permissions",
		"delete_branch_on_merge",
		"description",
		"has_discussions",
		"has_issues",
		"has_projects",
		"has_wiki",
		"homepage",
		"merge_commit_message",
		"merge_commit_title",
		"private_vulnerability_reporting_enabled",
		"squash_merge_commit_message",
		"squash_merge_commit_title",
		"topics",
		"vulnerability_alerts_enabled",
		"web_commit_signoff_required",
	}
	if len(names) != len(expected) {
		t.Fatalf("repoSettingNames() returned %d names, want %d", len(names), len(expected))
	}
	for i, name := range names {
		if name != expected[i] {
			t.Errorf("repoSettingNames()[%d] = %q, want %q", i, name, expected[i])
		}
	}
}

func TestValidateWorkflowPermissionsAcceptsRead(t *testing.T) {
	cfg := &Config{Repository: &model.RepositorySettings{DefaultWorkflowPermissions: new("read")}}
	if err := ValidateWorkflowPermissions(cfg); err != nil {
		t.Fatalf("ValidateWorkflowPermissions(read): %v", err)
	}
}

func TestValidateWorkflowPermissionsAcceptsWrite(t *testing.T) {
	cfg := &Config{Repository: &model.RepositorySettings{DefaultWorkflowPermissions: new("write")}}
	if err := ValidateWorkflowPermissions(cfg); err != nil {
		t.Fatalf("ValidateWorkflowPermissions(write): %v", err)
	}
}

func TestValidateWorkflowPermissionsAcceptsNil(t *testing.T) {
	cfg := &Config{Repository: &model.RepositorySettings{}}
	if err := ValidateWorkflowPermissions(cfg); err != nil {
		t.Fatalf("ValidateWorkflowPermissions(nil): %v", err)
	}
}

func TestValidateWorkflowPermissionsAcceptsNilRepository(t *testing.T) {
	cfg := &Config{}
	if err := ValidateWorkflowPermissions(cfg); err != nil {
		t.Fatalf("ValidateWorkflowPermissions(nil repo): %v", err)
	}
}

func TestValidateWorkflowPermissionsRejectsInvalid(t *testing.T) {
	cfg := &Config{Repository: &model.RepositorySettings{DefaultWorkflowPermissions: new("admin")}}
	err := ValidateWorkflowPermissions(cfg)
	if err == nil {
		t.Fatal("ValidateWorkflowPermissions(admin) expected error, got nil")
	}
	if !strings.Contains(err.Error(), `"admin"`) {
		t.Errorf("error = %q, want it to mention the invalid value", err)
	}
}

func TestValidateTopicsAcceptsValid(t *testing.T) {
	topics := []string{"go", "cli-tool", "3d-printing"}
	cfg := &Config{Repository: &model.RepositorySettings{Topics: &topics}}
	if err := ValidateTopics(cfg); err != nil {
		t.Fatalf("ValidateTopics(valid): %v", err)
	}
}

func TestValidateTopicsAcceptsNil(t *testing.T) {
	cfg := &Config{Repository: &model.RepositorySettings{}}
	if err := ValidateTopics(cfg); err != nil {
		t.Fatalf("ValidateTopics(nil): %v", err)
	}
}

func TestValidateTopicsAcceptsEmpty(t *testing.T) {
	topics := []string{}
	cfg := &Config{Repository: &model.RepositorySettings{Topics: &topics}}
	if err := ValidateTopics(cfg); err != nil {
		t.Fatalf("ValidateTopics(empty): %v", err)
	}
}

func TestValidateTopicsRejectsUppercase(t *testing.T) {
	topics := []string{"Go"}
	cfg := &Config{Repository: &model.RepositorySettings{Topics: &topics}}
	err := ValidateTopics(cfg)
	if err == nil {
		t.Fatal("ValidateTopics(uppercase) expected error, got nil")
	}
	if !strings.Contains(err.Error(), `"Go"`) {
		t.Errorf("error = %q, want it to mention the invalid topic", err)
	}
}

func TestValidateTopicsRejectsStartingWithHyphen(t *testing.T) {
	topics := []string{"-invalid"}
	cfg := &Config{Repository: &model.RepositorySettings{Topics: &topics}}
	err := ValidateTopics(cfg)
	if err == nil {
		t.Fatal("ValidateTopics(hyphen start) expected error, got nil")
	}
}

func TestValidateTopicsRejectsDuplicate(t *testing.T) {
	topics := []string{"go", "cli", "go"}
	cfg := &Config{Repository: &model.RepositorySettings{Topics: &topics}}
	err := ValidateTopics(cfg)
	if err == nil {
		t.Fatal("ValidateTopics(duplicate) expected error, got nil")
	}
	if !strings.Contains(err.Error(), `duplicate topic "go"`) {
		t.Errorf("error = %q, want it to mention the duplicate topic", err)
	}
}

func TestValidateTopicsRejectsTooLong(t *testing.T) {
	topics := []string{strings.Repeat("a", 51)}
	cfg := &Config{Repository: &model.RepositorySettings{Topics: &topics}}
	err := ValidateTopics(cfg)
	if err == nil {
		t.Fatal("ValidateTopics(too long) expected error, got nil")
	}
	if !strings.Contains(err.Error(), "exceeds 50 characters") {
		t.Errorf("error = %q, want it to mention length", err)
	}
}

func TestValidateTopicsRejectsSpecialChars(t *testing.T) {
	topics := []string{"hello_world"}
	cfg := &Config{Repository: &model.RepositorySettings{Topics: &topics}}
	err := ValidateTopics(cfg)
	if err == nil {
		t.Fatal("ValidateTopics(underscore) expected error, got nil")
	}
}

func TestValidateLabelsAcceptsValid(t *testing.T) {
	cfg := &Config{
		Labels: []model.LabelEntry{
			{Name: "bug", Color: "d73a4a", Description: "Something is not working"},
			{Name: "enhancement", Color: "a2eeef", Description: "New feature or request"},
		},
	}
	if err := ValidateLabels(cfg); err != nil {
		t.Fatalf("ValidateLabels(valid): %v", err)
	}
}

func TestValidateLabelsAcceptsNil(t *testing.T) {
	cfg := &Config{}
	if err := ValidateLabels(cfg); err != nil {
		t.Fatalf("ValidateLabels(nil): %v", err)
	}
}

func TestValidateLabelsAcceptsEmpty(t *testing.T) {
	cfg := &Config{Labels: []model.LabelEntry{}}
	if err := ValidateLabels(cfg); err != nil {
		t.Fatalf("ValidateLabels(empty): %v", err)
	}
}

func TestValidateLabelsCollectionLimit(t *testing.T) {
	makeLabels := func(count int) []model.LabelEntry {
		labels := make([]model.LabelEntry, 0, count)
		for i := range count {
			labels = append(labels, model.LabelEntry{
				Name:        fmt.Sprintf("label-%d", i),
				Color:       "d73a4a",
				Description: "desc",
			})
		}
		return labels
	}

	tests := []struct {
		name    string
		count   int
		wantErr string
	}{
		{name: "at limit", count: 1000, wantErr: ""},
		{name: "over limit", count: 1001, wantErr: "1001 entries exceed the maximum of 1000"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &Config{Labels: makeLabels(tt.count)}
			err := ValidateLabels(cfg)
			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("ValidateLabels(%d labels): %v", tt.count, err)
				}
				return
			}
			if err == nil {
				t.Fatalf("ValidateLabels(%d labels) expected error, got nil", tt.count)
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("error = %q, want %q", err, tt.wantErr)
			}
		})
	}
}

func TestValidateLabelsRejectsEmptyName(t *testing.T) {
	cfg := &Config{
		Labels: []model.LabelEntry{
			{Name: "", Color: "d73a4a", Description: "desc"},
		},
	}
	err := ValidateLabels(cfg)
	if err == nil {
		t.Fatal("ValidateLabels(empty name) expected error, got nil")
	}
	if !strings.Contains(err.Error(), "name must not be empty") {
		t.Errorf("error = %q, want name must not be empty", err)
	}
}

func TestValidateLabelsRejectsLongName(t *testing.T) {
	cfg := &Config{
		Labels: []model.LabelEntry{
			{Name: strings.Repeat("a", 51), Color: "d73a4a", Description: "desc"},
		},
	}
	err := ValidateLabels(cfg)
	if err == nil {
		t.Fatal("ValidateLabels(long name) expected error, got nil")
	}
	if !strings.Contains(err.Error(), "exceeds 50 characters") {
		t.Errorf("error = %q, want exceeds 50 characters", err)
	}
}

func TestValidateLabelsAcceptsMaxName(t *testing.T) {
	cfg := &Config{
		Labels: []model.LabelEntry{
			{Name: strings.Repeat("a", 50), Color: "d73a4a", Description: "desc"},
		},
	}
	if err := ValidateLabels(cfg); err != nil {
		t.Fatalf("ValidateLabels(50-char name): %v", err)
	}
}

func TestValidateLabelsRejectsEmptyColor(t *testing.T) {
	cfg := &Config{
		Labels: []model.LabelEntry{
			{Name: "bug", Color: "", Description: "desc"},
		},
	}
	err := ValidateLabels(cfg)
	if err == nil {
		t.Fatal("ValidateLabels(empty color) expected error, got nil")
	}
	if !strings.Contains(err.Error(), "color must not be empty") {
		t.Errorf("error = %q, want color must not be empty", err)
	}
}

func TestValidateLabelsRejectsHashPrefix(t *testing.T) {
	cfg := &Config{
		Labels: []model.LabelEntry{
			{Name: "bug", Color: "#d73a4a", Description: "desc"},
		},
	}
	err := ValidateLabels(cfg)
	if err == nil {
		t.Fatal("ValidateLabels(# prefix) expected error, got nil")
	}
	if !strings.Contains(err.Error(), "not a valid 6-character hex") {
		t.Errorf("error = %q, want hex validation error", err)
	}
}

func TestValidateLabelsRejectsShortColor(t *testing.T) {
	cfg := &Config{
		Labels: []model.LabelEntry{
			{Name: "bug", Color: "d73", Description: "desc"},
		},
	}
	err := ValidateLabels(cfg)
	if err == nil {
		t.Fatal("ValidateLabels(short color) expected error, got nil")
	}
	if !strings.Contains(err.Error(), "not a valid 6-character hex") {
		t.Errorf("error = %q, want hex validation error", err)
	}
}

func TestValidateLabelsRejectsInvalidHex(t *testing.T) {
	cfg := &Config{
		Labels: []model.LabelEntry{
			{Name: "bug", Color: "zzzzzz", Description: "desc"},
		},
	}
	err := ValidateLabels(cfg)
	if err == nil {
		t.Fatal("ValidateLabels(invalid hex) expected error, got nil")
	}
	if !strings.Contains(err.Error(), "not a valid 6-character hex") {
		t.Errorf("error = %q, want hex validation error", err)
	}
}

func TestValidateLabelsAcceptsUppercaseHex(t *testing.T) {
	cfg := &Config{
		Labels: []model.LabelEntry{
			{Name: "bug", Color: "D73A4A", Description: "desc"},
		},
	}
	if err := ValidateLabels(cfg); err != nil {
		t.Fatalf("ValidateLabels(uppercase hex): %v", err)
	}
}

func TestValidateLabelsRejectsEmptyDescription(t *testing.T) {
	cfg := &Config{
		Labels: []model.LabelEntry{
			{Name: "bug", Color: "d73a4a", Description: ""},
		},
	}
	err := ValidateLabels(cfg)
	if err == nil {
		t.Fatal("ValidateLabels(empty description) expected error, got nil")
	}
	if !strings.Contains(err.Error(), "description must not be empty") {
		t.Errorf("error = %q, want description must not be empty", err)
	}
}

func TestValidateLabelsRejectsLongDescription(t *testing.T) {
	cfg := &Config{
		Labels: []model.LabelEntry{
			{Name: "bug", Color: "d73a4a", Description: strings.Repeat("a", 101)},
		},
	}
	err := ValidateLabels(cfg)
	if err == nil {
		t.Fatal("ValidateLabels(long description) expected error, got nil")
	}
	if !strings.Contains(err.Error(), "description exceeds 100 characters") {
		t.Errorf("error = %q, want description exceeds 100 characters", err)
	}
}

func TestValidateLabelsAcceptsMaxDescription(t *testing.T) {
	cfg := &Config{
		Labels: []model.LabelEntry{
			{Name: "bug", Color: "d73a4a", Description: strings.Repeat("a", 100)},
		},
	}
	if err := ValidateLabels(cfg); err != nil {
		t.Fatalf("ValidateLabels(100-char description): %v", err)
	}
}

func TestValidateLabelsMultibyteLimits(t *testing.T) {
	tests := []struct {
		name    string
		label   model.LabelEntry
		wantErr string
	}{
		{
			name:  "multibyte name at limit",
			label: model.LabelEntry{Name: strings.Repeat("é", 50), Color: "d73a4a", Description: "desc"},
		},
		{
			name:    "multibyte name over limit",
			label:   model.LabelEntry{Name: strings.Repeat("é", 51), Color: "d73a4a", Description: "desc"},
			wantErr: "exceeds 50 characters",
		},
		{
			name:  "multibyte description at limit",
			label: model.LabelEntry{Name: "bug", Color: "d73a4a", Description: strings.Repeat("界", 100)},
		},
		{
			name:    "multibyte description over limit",
			label:   model.LabelEntry{Name: "bug", Color: "d73a4a", Description: strings.Repeat("界", 101)},
			wantErr: "description exceeds 100 characters",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &Config{Labels: []model.LabelEntry{tt.label}}
			err := ValidateLabels(cfg)
			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("ValidateLabels(%s): %v", tt.name, err)
				}
				return
			}
			if err == nil {
				t.Fatalf("ValidateLabels(%s) expected error, got nil", tt.name)
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("error = %q, want %q", err, tt.wantErr)
			}
		})
	}
}

func TestValidateLabelsRejectsDuplicateNames(t *testing.T) {
	cfg := &Config{
		Labels: []model.LabelEntry{
			{Name: "bug", Color: "d73a4a", Description: "first"},
			{Name: "bug", Color: "ff0000", Description: "second"},
		},
	}
	err := ValidateLabels(cfg)
	if err == nil {
		t.Fatal("ValidateLabels(duplicate names) expected error, got nil")
	}
	if !strings.Contains(err.Error(), "duplicate label name") {
		t.Errorf("error = %q, want duplicate label name", err)
	}
}

func TestValidateLabelsRejectsDuplicateNamesCaseInsensitive(t *testing.T) {
	cfg := &Config{
		Labels: []model.LabelEntry{
			{Name: "Bug", Color: "d73a4a", Description: "first"},
			{Name: "bug", Color: "ff0000", Description: "second"},
		},
	}
	err := ValidateLabels(cfg)
	if err == nil {
		t.Fatal("ValidateLabels(case-insensitive duplicate) expected error, got nil")
	}
	if !strings.Contains(err.Error(), "duplicate label name") {
		t.Errorf("error = %q, want duplicate label name", err)
	}
}

func TestValidateAllPassesSpecYAML(t *testing.T) {
	var cfg Config
	if err := yaml.Unmarshal([]byte(specYAML), &cfg); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}
	if err := ValidatePaths(&cfg); err != nil {
		t.Errorf("ValidatePaths: %v", err)
	}
	if err := ValidateDuplicatePaths(&cfg); err != nil {
		t.Errorf("ValidateDuplicatePaths: %v", err)
	}
	if err := ValidateRepoSettings(&cfg); err != nil {
		t.Errorf("ValidateRepoSettings: %v", err)
	}
}

func TestValidateLabelsRejectsControlCharacters(t *testing.T) {
	tests := []struct {
		name  string
		label model.LabelEntry
		want  string
	}{
		{
			name:  "ANSI CSI in name",
			label: model.LabelEntry{Name: "\x1b[31mbug", Color: "d73a4a", Description: "Something is broken"},
			want:  "name",
		},
		{
			name:  "OSC 8 hyperlink in name",
			label: model.LabelEntry{Name: "\x1b]8;;https://evil.example\x07bug", Color: "d73a4a", Description: "Something is broken"},
			want:  "name",
		},
		{
			name:  "OSC 52 clipboard write in name",
			label: model.LabelEntry{Name: "\x1b]52;c;Zm9v\x07bug", Color: "d73a4a", Description: "Something is broken"},
			want:  "name",
		},
		{
			name:  "C1 CSI in name",
			label: model.LabelEntry{Name: "\u009b31mbug", Color: "d73a4a", Description: "Something is broken"},
			want:  "name",
		},
		{
			name:  "carriage return in description",
			label: model.LabelEntry{Name: "bug", Color: "d73a4a", Description: "safe\rspoofed"},
			want:  "description",
		},
		{
			name:  "line feed in description",
			label: model.LabelEntry{Name: "bug", Color: "d73a4a", Description: "safe\ninjected"},
			want:  "description",
		},
		{
			name:  "C1 control in description",
			label: model.LabelEntry{Name: "bug", Color: "d73a4a", Description: "safe\u009bspoofed"},
			want:  "description",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &Config{Labels: []model.LabelEntry{tt.label}}
			err := ValidateLabels(cfg)
			if err == nil {
				t.Fatal("ValidateLabels() returned nil, want error")
			}
			if !strings.Contains(err.Error(), tt.want) || !strings.Contains(err.Error(), "control characters") {
				t.Errorf("error = %q, want mention of %s control characters", err, tt.want)
			}
		})
	}
}

func TestValidateRepoStringSettingsRejectsControlCharacters(t *testing.T) {
	tests := []struct {
		name        string
		description string
	}{
		{"ANSI CSI", "\x1b[31mred\x1b[0m"},
		{"OSC 8 hyperlink", "\x1b]8;;https://evil.example\x07link"},
		{"OSC 52 clipboard write", "\x1b]52;c;Zm9v\x07"},
		{"carriage return", "safe\rspoofed"},
		{"line feed", "safe\ninjected"},
		{"C1 CSI", "safe\u009b31m"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &Config{Repository: &model.RepositorySettings{Description: &tt.description}}
			err := ValidateRepoStringSettings(cfg)
			if err == nil {
				t.Fatal("ValidateRepoStringSettings() returned nil, want error")
			}
			if !strings.Contains(err.Error(), "description") || !strings.Contains(err.Error(), "control characters") {
				t.Errorf("error = %q, want mention of description control characters", err)
			}
		})
	}
}

func TestValidateRepoStringSettingsAcceptsBenignValues(t *testing.T) {
	description := "A CLI for managing project templates"
	homepage := "https://example.com"
	cfg := &Config{Repository: &model.RepositorySettings{
		Description: &description,
		Homepage:    &homepage,
	}}
	if err := ValidateRepoStringSettings(cfg); err != nil {
		t.Fatalf("ValidateRepoStringSettings() returned unexpected error: %v", err)
	}
}

func TestValidateRepoStringSettingsAcceptsNilRepository(t *testing.T) {
	if err := ValidateRepoStringSettings(&Config{}); err != nil {
		t.Fatalf("ValidateRepoStringSettings() returned unexpected error: %v", err)
	}
}
