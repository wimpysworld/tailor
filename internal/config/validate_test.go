package config

import (
	"strings"
	"testing"

	"gopkg.in/yaml.v3"

	"github.com/wimpysworld/tailor/internal/ptr"
	"github.com/wimpysworld/tailor/internal/swatch"
)

func TestValidateSourcesAcceptsValidConfig(t *testing.T) {
	cfg := &Config{
		Swatches: []SwatchEntry{
			{Source: ".gitignore", Destination: ".gitignore", Alteration: swatch.FirstFit},
			{Source: "justfile", Destination: "justfile", Alteration: swatch.FirstFit},
		},
	}
	if err := ValidateSources(cfg); err != nil {
		t.Fatalf("ValidateSources() returned unexpected error: %v", err)
	}
}

func TestValidateSourcesRejectsUnknownSource(t *testing.T) {
	cfg := &Config{
		Swatches: []SwatchEntry{
			{Source: "nonexistent.txt", Destination: "nonexistent.txt", Alteration: swatch.Always},
		},
	}
	err := ValidateSources(cfg)
	if err == nil {
		t.Fatal("ValidateSources() expected error for unknown source, got nil")
	}
	if !strings.Contains(err.Error(), `unrecognised swatch source "nonexistent.txt"`) {
		t.Errorf("error = %q, want it to contain unrecognised source message", err)
	}
	if !strings.Contains(err.Error(), "valid sources:") {
		t.Errorf("error = %q, want it to list valid sources", err)
	}
}

func TestValidateSourcesAcceptsEmptySwatches(t *testing.T) {
	cfg := &Config{}
	if err := ValidateSources(cfg); err != nil {
		t.Fatalf("ValidateSources() on empty swatches: %v", err)
	}
}

func TestValidateDuplicateDestinationsAcceptsUnique(t *testing.T) {
	cfg := &Config{
		Swatches: []SwatchEntry{
			{Source: ".gitignore", Destination: ".gitignore", Alteration: swatch.FirstFit},
			{Source: "justfile", Destination: "justfile", Alteration: swatch.FirstFit},
		},
	}
	if err := ValidateDuplicateDestinations(cfg); err != nil {
		t.Fatalf("ValidateDuplicateDestinations() returned unexpected error: %v", err)
	}
}

func TestValidateDuplicateDestinationsRejectsDuplicate(t *testing.T) {
	cfg := &Config{
		Swatches: []SwatchEntry{
			{Source: ".gitignore", Destination: "shared.txt", Alteration: swatch.FirstFit},
			{Source: "justfile", Destination: "shared.txt", Alteration: swatch.FirstFit},
		},
	}
	err := ValidateDuplicateDestinations(cfg)
	if err == nil {
		t.Fatal("ValidateDuplicateDestinations() expected error for duplicate destination, got nil")
	}
	if !strings.Contains(err.Error(), `duplicate destination "shared.txt"`) {
		t.Errorf("error = %q, want it to contain duplicate destination message", err)
	}
	if !strings.Contains(err.Error(), `".gitignore"`) || !strings.Contains(err.Error(), `"justfile"`) {
		t.Errorf("error = %q, want it to identify both conflicting sources", err)
	}
}

func TestValidateRepoSettingsAcceptsValidConfig(t *testing.T) {
	cfg := &Config{
		Repository: &RepositorySettings{
			HasWiki:   ptr.Bool(false),
			HasIssues: ptr.Bool(true),
			Homepage:  ptr.String("https://example.com"),
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

func TestValidateAllPassesSpecYAML(t *testing.T) {
	// The specYAML from config_test.go is a valid config. Verify all three
	// validators accept it.
	var cfg Config
	if err := yaml.Unmarshal([]byte(specYAML), &cfg); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}
	if err := ValidateSources(&cfg); err != nil {
		t.Errorf("ValidateSources: %v", err)
	}
	if err := ValidateDuplicateDestinations(&cfg); err != nil {
		t.Errorf("ValidateDuplicateDestinations: %v", err)
	}
	if err := ValidateRepoSettings(&cfg); err != nil {
		t.Errorf("ValidateRepoSettings: %v", err)
	}
}
