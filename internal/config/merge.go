package config

import (
	"fmt"
	"reflect"
	"slices"

	"github.com/wimpysworld/tailor/internal/model"
	"github.com/wimpysworld/tailor/internal/swatch"
)

// repoSettingsSkipFields lists RepositorySettings YAML keys excluded from
// default merging. Description and Homepage are project-specific (nil'd by
// DefaultConfig). Topics are project-specific per spec.
var repoSettingsSkipFields = map[string]bool{
	"description": true,
	"homepage":    true,
	"topics":      true,
}

// MergeDefaults merges missing swatch entries, repository settings, Actions
// policy fields, and labels from the embedded defaults into cfg. It reports
// whether anything changed.
func MergeDefaults(cfg *Config) (bool, error) {
	swatchesChanged := len(MergeDefaultSwatches(cfg)) > 0

	defaults, err := DefaultConfig("_")
	if err != nil {
		return swatchesChanged, fmt.Errorf("loading default config: %w", err)
	}

	repoChanged := mergeRepoSettingsFrom(cfg, defaults)
	actionsChanged := mergeActionsFrom(cfg, defaults)
	labelsChanged := mergeLabelsFrom(cfg, defaults)
	return swatchesChanged || repoChanged || actionsChanged || labelsChanged, nil
}

// mergeRepoSettingsFrom fills nil pointer fields in cfg.Repository from the
// provided defaults.
func mergeRepoSettingsFrom(cfg *Config, defaults *Config) bool {
	if defaults.Repository == nil {
		return false
	}

	if cfg.Repository == nil {
		cfg.Repository = &model.RepositorySettings{}
	}

	cv := reflect.ValueOf(cfg.Repository).Elem()

	changed := false

	for _, field := range model.RepositorySettingFields(defaults.Repository) {
		if repoSettingsSkipFields[field.YAMLKey] {
			continue
		}
		if !field.Set {
			continue
		}

		dfv := field.Value
		cfv := cv.Field(field.Index)
		if !cfv.IsNil() {
			continue
		}

		// Allocate a new value and copy from the default.
		newVal := reflect.New(dfv.Elem().Type())
		newVal.Elem().Set(dfv.Elem())
		cfv.Set(newVal)
		changed = true
	}

	return changed
}

// mergeActionsFrom fills missing Actions policy fields from the defaults.
// Selected-action fields apply only when the effective policy is selected.
func mergeActionsFrom(cfg *Config, defaults *Config) bool {
	if defaults.Actions == nil {
		return false
	}
	if cfg.Actions == nil {
		cfg.Actions = &model.ActionsSettings{}
	}

	changed := false
	mergeBool := func(current **bool, fallback *bool) {
		if *current == nil && fallback != nil {
			value := *fallback
			*current = &value
			changed = true
		}
	}
	mergeString := func(current **string, fallback *string) {
		if *current == nil && fallback != nil {
			value := *fallback
			*current = &value
			changed = true
		}
	}

	mergeBool(&cfg.Actions.Enabled, defaults.Actions.Enabled)
	mergeString(&cfg.Actions.AllowedActions, defaults.Actions.AllowedActions)
	mergeBool(&cfg.Actions.SHAPinningRequired, defaults.Actions.SHAPinningRequired)

	if cfg.Actions.AllowedActions == nil || *cfg.Actions.AllowedActions != "selected" {
		return changed
	}

	mergeBool(&cfg.Actions.GitHubOwnedAllowed, defaults.Actions.GitHubOwnedAllowed)
	mergeBool(&cfg.Actions.VerifiedAllowed, defaults.Actions.VerifiedAllowed)
	if cfg.Actions.PatternsAllowed == nil && defaults.Actions.PatternsAllowed != nil {
		patterns := slices.Clone(*defaults.Actions.PatternsAllowed)
		cfg.Actions.PatternsAllowed = &patterns
		changed = true
	}

	return changed
}

// mergeLabelsFrom populates cfg.Labels from the provided defaults when the
// slice is empty.
func mergeLabelsFrom(cfg *Config, defaults *Config) bool {
	if len(cfg.Labels) > 0 {
		return false
	}

	if len(defaults.Labels) == 0 {
		return false
	}

	cfg.Labels = slices.Clone(defaults.Labels)

	return true
}

// ConfigSwatchPath is the path of the config swatch entry, which is excluded
// from merge because it describes the config file itself.
const ConfigSwatchPath = ".tailor.yml"

// MergeDefaultSwatches appends missing default swatch entries to cfg.Swatches.
// It skips the config swatch itself. Existing entries are matched by path, so
// an altered mode does not cause duplication. It returns the slice of newly
// added entries.
func MergeDefaultSwatches(cfg *Config) []SwatchEntry {
	present := make(map[string]bool, len(cfg.Swatches))
	for _, e := range cfg.Swatches {
		present[e.Path] = true
	}

	var added []SwatchEntry
	for _, s := range swatch.All() {
		if s.Path == ConfigSwatchPath {
			continue
		}
		if present[s.Path] {
			continue
		}
		entry := SwatchEntry{
			Path:       s.Path,
			Alteration: s.DefaultAlteration,
		}
		cfg.Swatches = append(cfg.Swatches, entry)
		added = append(added, entry)
	}
	return added
}
