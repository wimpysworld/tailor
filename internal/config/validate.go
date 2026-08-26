package config

import (
	"fmt"
	"maps"
	"reflect"
	"regexp"
	"slices"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/wimpysworld/tailor/internal/model"
	"github.com/wimpysworld/tailor/internal/swatch"
)

var (
	topicRegexp    = regexp.MustCompile(`^[a-z0-9][a-z0-9-]*$`)
	labelHexRegexp = regexp.MustCompile(`^[0-9a-fA-F]{6}$`)
)

// maxLabels caps the label list so a misconfigured file cannot drive an
// unbounded number of API mutations.
const maxLabels = 1000

func validateTopLevelSettings(cfg *Config) error {
	if len(cfg.Extra) == 0 {
		return nil
	}

	valid := []string{"actions", "labels", "license", "repository", "swatches"}
	return unrecognisedSettingError("top-level", cfg.Extra, valid)
}

// ValidatePaths checks that every swatch path in cfg matches a known embedded
// swatch. Returns an error listing the unrecognised path and all valid paths.
func ValidatePaths(cfg *Config) error {
	valid := swatch.Paths()
	for _, s := range cfg.Swatches {
		if !slices.Contains(valid, s.Path) {
			return fmt.Errorf("unrecognised swatch path %q in config; valid paths: %s",
				s.Path, strings.Join(valid, ", "))
		}
	}
	return nil
}

// ValidateDuplicatePaths checks that no two swatches share a path. Returns an
// error identifying the duplicate.
func ValidateDuplicatePaths(cfg *Config) error {
	seen := make(map[string]bool, len(cfg.Swatches))
	for _, s := range cfg.Swatches {
		if seen[s.Path] {
			return fmt.Errorf("duplicate swatch path %q in config", s.Path)
		}
		seen[s.Path] = true
	}
	return nil
}

// ValidateRepoSettings checks that every field name in cfg.Repository
// matches the supported settings list. Returns an error identifying the
// unrecognised field and listing all valid field names.
func ValidateRepoSettings(cfg *Config) error {
	if cfg.Repository == nil {
		return nil
	}

	if len(cfg.Repository.Extra) > 0 {
		return unrecognisedSettingError("repository", cfg.Repository.Extra, repoSettingNames())
	}
	return nil
}

// ValidateWorkflowPermissions checks that default_workflow_permissions, if set,
// is either "read" or "write".
func ValidateWorkflowPermissions(cfg *Config) error {
	if cfg.Repository == nil || cfg.Repository.DefaultWorkflowPermissions == nil {
		return nil
	}
	v := *cfg.Repository.DefaultWorkflowPermissions
	if v != "read" && v != "write" {
		return fmt.Errorf("invalid default_workflow_permissions %q; must be %q or %q", v, "read", "write")
	}
	return nil
}

// ValidateActions checks the Actions policy field names, enum values, and
// selected-action field combinations, and rejects patterns_allowed entries
// containing control characters.
func ValidateActions(cfg *Config) error {
	if cfg.Actions == nil {
		return nil
	}
	if len(cfg.Actions.Extra) > 0 {
		return unrecognisedSettingError("actions", cfg.Actions.Extra, settingNames(model.ActionsSettingFields(nil)))
	}

	a := cfg.Actions
	if a.AllowedActions != nil {
		v := *a.AllowedActions
		if v != "all" && v != "local_only" && v != "selected" {
			return fmt.Errorf("invalid allowed_actions %q; must be %q, %q, or %q", v, "all", "local_only", "selected")
		}
	}
	selectedFieldsSet := a.GitHubOwnedAllowed != nil || a.VerifiedAllowed != nil || a.PatternsAllowed != nil
	if selectedFieldsSet && (a.AllowedActions == nil || *a.AllowedActions != "selected") {
		return fmt.Errorf("github_owned_allowed, verified_allowed, and patterns_allowed require allowed_actions to be %q", "selected")
	}
	if a.PatternsAllowed != nil {
		for i, pattern := range *a.PatternsAllowed {
			if containsControl(pattern) {
				return fmt.Errorf("patterns_allowed[%d]: pattern %q contains control characters", i, pattern)
			}
		}
	}
	return nil
}

func unrecognisedSettingError(section string, extra map[string]interface{}, valid []string) error {
	keys := slices.Sorted(maps.Keys(extra))
	return fmt.Errorf("unrecognised %s setting %q in config; valid settings: %s",
		section, keys[0], strings.Join(valid, ", "))
}

// ValidateCompleteActions checks that a selected Actions policy includes the
// complete selected-actions endpoint payload after default merging.
func ValidateCompleteActions(cfg *Config) error {
	if cfg.Actions == nil || cfg.Actions.AllowedActions == nil || *cfg.Actions.AllowedActions != "selected" {
		return nil
	}

	a := cfg.Actions
	if a.GitHubOwnedAllowed == nil || a.VerifiedAllowed == nil || a.PatternsAllowed == nil {
		return fmt.Errorf("allowed_actions %q requires github_owned_allowed, verified_allowed, and patterns_allowed", "selected")
	}
	return nil
}

// ValidateTopics checks that every topic, if set, starts with a lowercase
// letter or number, contains only lowercase alphanumerics and hyphens, does
// not exceed 50 characters, and appears only once.
func ValidateTopics(cfg *Config) error {
	if cfg.Repository == nil || cfg.Repository.Topics == nil {
		return nil
	}
	seen := make(map[string]bool, len(*cfg.Repository.Topics))
	for _, topic := range *cfg.Repository.Topics {
		if len(topic) > 50 {
			return fmt.Errorf("topic %q exceeds 50 characters", topic)
		}
		if !topicRegexp.MatchString(topic) {
			return fmt.Errorf("topic %q is invalid; must start with a lowercase letter or number and contain only lowercase alphanumerics and hyphens", topic)
		}
		if seen[topic] {
			return fmt.Errorf("duplicate topic %q in config", topic)
		}
		seen[topic] = true
	}
	return nil
}

// containsControl reports whether s contains any control character, including
// C0 controls, DEL, and C1 controls.
func containsControl(s string) bool {
	return strings.ContainsFunc(s, unicode.IsControl)
}

// ValidateRepoStringSettings checks repository string enum values and rejects
// control characters, which could inject terminal control sequences into
// output. Topics carry their own stricter format validation in ValidateTopics.
func ValidateRepoStringSettings(cfg *Config) error {
	if cfg.Repository == nil {
		return nil
	}

	enumSettings := []struct {
		name    string
		value   *string
		allowed []string
	}{
		{"squash_merge_commit_title", cfg.Repository.SquashMergeCommitTitle, []string{"PR_TITLE", "COMMIT_OR_PR_TITLE"}},
		{"squash_merge_commit_message", cfg.Repository.SquashMergeCommitMessage, []string{"PR_BODY", "COMMIT_MESSAGES", "BLANK"}},
		{"merge_commit_title", cfg.Repository.MergeCommitTitle, []string{"PR_TITLE", "MERGE_MESSAGE"}},
		{"merge_commit_message", cfg.Repository.MergeCommitMessage, []string{"PR_TITLE", "PR_BODY", "BLANK"}},
	}
	for _, setting := range enumSettings {
		if setting.value == nil || slices.Contains(setting.allowed, *setting.value) {
			continue
		}
		quoted := make([]string, len(setting.allowed))
		for i, value := range setting.allowed {
			quoted[i] = fmt.Sprintf("%q", value)
		}
		separator := " or "
		if len(quoted) > 2 {
			separator = ", or "
		}
		allowed := strings.Join(quoted[:len(quoted)-1], ", ") + separator + quoted[len(quoted)-1]
		return fmt.Errorf("invalid %s %q; must be %s", setting.name, *setting.value, allowed)
	}

	for _, field := range model.RepositorySettingFields(cfg.Repository) {
		if !field.Set || field.Value.Elem().Kind() != reflect.String {
			continue
		}
		if containsControl(field.Value.Elem().String()) {
			return fmt.Errorf("repository setting %s contains control characters", field.YAMLKey)
		}
	}
	return nil
}

// ValidateLabels checks that every label entry has valid name, color, and
// description fields. Rejects control characters in names and descriptions
// and duplicate names (case-insensitive).
func ValidateLabels(cfg *Config) error {
	if cfg.Labels == nil {
		return nil
	}
	if len(cfg.Labels) > maxLabels {
		return fmt.Errorf("labels: %d entries exceed the maximum of %d", len(cfg.Labels), maxLabels)
	}
	seen := make(map[string]bool, len(cfg.Labels))
	for i, l := range cfg.Labels {
		if l.Name == "" {
			return fmt.Errorf("label[%d]: name must not be empty", i)
		}
		if utf8.RuneCountInString(l.Name) > 50 {
			return fmt.Errorf("label[%d]: name %q exceeds 50 characters", i, l.Name)
		}
		if containsControl(l.Name) {
			return fmt.Errorf("label[%d]: name %q contains control characters", i, l.Name)
		}
		if l.Color == "" {
			return fmt.Errorf("label[%d]: color must not be empty", i)
		}
		if !labelHexRegexp.MatchString(l.Color) {
			return fmt.Errorf("label[%d]: color %q is not a valid 6-character hex colour (no # prefix)", i, l.Color)
		}
		if l.Description == "" {
			return fmt.Errorf("label[%d]: description must not be empty", i)
		}
		if utf8.RuneCountInString(l.Description) > 100 {
			return fmt.Errorf("label[%d]: description exceeds 100 characters", i)
		}
		if containsControl(l.Description) {
			return fmt.Errorf("label[%d]: description contains control characters", i)
		}
		key := strings.ToLower(l.Name)
		if seen[key] {
			return fmt.Errorf("label[%d]: duplicate label name %q (case-insensitive)", i, l.Name)
		}
		seen[key] = true
	}
	return nil
}

// repoSettingNames returns the sorted list of recognised yaml tag names from
// RepositorySettings, excluding the inline Extra field.
func repoSettingNames() []string {
	return settingNames(model.RepositorySettingFields(nil))
}

// settingNames returns the sorted yaml tag names for fields.
func settingNames(fields []model.SettingField) []string {
	names := make([]string, 0, len(fields))
	for _, field := range fields {
		names = append(names, field.YAMLKey)
	}
	slices.Sort(names)
	return names
}
