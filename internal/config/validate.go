package config

import (
	"fmt"
	"reflect"
	"regexp"
	"sort"
	"strings"

	"github.com/wimpysworld/tailor/internal/swatch"
)

var (
	topicRegexp    = regexp.MustCompile(`^[a-z0-9][a-z0-9-]*$`)
	labelHexRegexp = regexp.MustCompile(`^[0-9a-fA-F]{6}$`)
)

// ValidateSources checks that every swatch source in cfg matches a known
// embedded swatch. Returns an error listing the unrecognised source and all
// valid source names.
func ValidateSources(cfg *Config) error {
	valid := swatch.SourceNames()
	known := make(map[string]bool, len(valid))
	for _, name := range valid {
		known[name] = true
	}
	for _, s := range cfg.Swatches {
		if !known[s.Source] {
			return fmt.Errorf("unrecognised swatch source %q in config; valid sources: %s",
				s.Source, strings.Join(valid, ", "))
		}
	}
	return nil
}

// ValidateDuplicateDestinations checks that no two swatches share a
// destination. Returns an error identifying the conflicting entries.
func ValidateDuplicateDestinations(cfg *Config) error {
	seen := make(map[string]string, len(cfg.Swatches))
	for _, s := range cfg.Swatches {
		if prev, ok := seen[s.Destination]; ok {
			return fmt.Errorf("duplicate destination %q in config: sources %q and %q both target the same file",
				s.Destination, prev, s.Source)
		}
		seen[s.Destination] = s.Source
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
		valid := repoSettingNames()
		for key := range cfg.Repository.Extra {
			return fmt.Errorf("unrecognised repository setting %q in config; valid settings: %s",
				key, strings.Join(valid, ", "))
		}
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

// ValidateTopics checks that every topic, if set, starts with a lowercase
// letter or number, contains only lowercase alphanumerics and hyphens, and
// does not exceed 50 characters.
func ValidateTopics(cfg *Config) error {
	if cfg.Repository == nil || cfg.Repository.Topics == nil {
		return nil
	}
	for _, topic := range *cfg.Repository.Topics {
		if len(topic) > 50 {
			return fmt.Errorf("topic %q exceeds 50 characters", topic)
		}
		if !topicRegexp.MatchString(topic) {
			return fmt.Errorf("topic %q is invalid; must start with a lowercase letter or number and contain only lowercase alphanumerics and hyphens", topic)
		}
	}
	return nil
}

// ValidateLabels checks that every label entry has valid name, color, and
// description fields. Rejects duplicate names (case-insensitive).
func ValidateLabels(cfg *Config) error {
	if cfg.Labels == nil {
		return nil
	}
	seen := make(map[string]bool, len(cfg.Labels))
	for i, l := range cfg.Labels {
		if l.Name == "" {
			return fmt.Errorf("label[%d]: name must not be empty", i)
		}
		if len(l.Name) > 50 {
			return fmt.Errorf("label[%d]: name %q exceeds 50 characters", i, l.Name)
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
		if len(l.Description) > 100 {
			return fmt.Errorf("label[%d]: description exceeds 100 characters", i)
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
	t := reflect.TypeOf(RepositorySettings{})
	var names []string
	for i := range t.NumField() {
		tag := t.Field(i).Tag.Get("yaml")
		if tag == "" || tag == ",inline" {
			continue
		}
		name, _, _ := strings.Cut(tag, ",")
		if name != "" {
			names = append(names, name)
		}
	}
	sort.Strings(names)
	return names
}
