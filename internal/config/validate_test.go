package config

import (
	"strings"
	"testing"

	"gopkg.in/yaml.v3"

	"github.com/wimpysworld/tailor/internal/model"
	"github.com/wimpysworld/tailor/internal/ptr"
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
			HasWiki:   ptr.Ptr(false),
			HasIssues: ptr.Ptr(true),
			Homepage:  ptr.Ptr("https://example.com"),
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
	cfg := &Config{Repository: &model.RepositorySettings{DefaultWorkflowPermissions: ptr.Ptr("read")}}
	if err := ValidateWorkflowPermissions(cfg); err != nil {
		t.Fatalf("ValidateWorkflowPermissions(read): %v", err)
	}
}

func TestValidateWorkflowPermissionsAcceptsWrite(t *testing.T) {
	cfg := &Config{Repository: &model.RepositorySettings{DefaultWorkflowPermissions: ptr.Ptr("write")}}
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
	cfg := &Config{Repository: &model.RepositorySettings{DefaultWorkflowPermissions: ptr.Ptr("admin")}}
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
