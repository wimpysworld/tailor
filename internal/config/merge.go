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
	codeScanningChanged := mergeSettingsFrom(&cfg.CodeScanning, defaults.CodeScanning, model.CodeScanningSettingFields)
	codeQualityChanged := mergeSettingsFrom(&cfg.CodeQuality, defaults.CodeQuality, model.CodeQualitySettingFields)
	rulesetChanged := mergeRulesetFrom(cfg, defaults)
	labelsChanged := mergeLabelsFrom(cfg, defaults)
	return swatchesChanged || repoChanged || actionsChanged || codeScanningChanged || codeQualityChanged || rulesetChanged || labelsChanged, nil
}

// mergeRulesetFrom adds the complete default ruleset when cfg has none, and
// otherwise fills nil fields at every level without changing set values.
// Lists count as one field: a set list is kept whole. It reports whether it
// changed anything.
func mergeRulesetFrom(cfg *Config, defaults *Config) bool {
	if defaults.Ruleset == nil {
		return false
	}
	if cfg.Ruleset == nil {
		cfg.Ruleset = &model.RulesetSettings{}
	}
	return fillNilFields(reflect.ValueOf(cfg.Ruleset).Elem(), reflect.ValueOf(defaults.Ruleset).Elem(), false)
}

// fillNilFields walks the yaml-tagged pointer fields of two structs of the
// same type. A nil dst field takes a deep copy of the src field. When both
// point to structs it recurses. When overwrite is true a set src field
// replaces the dst field instead, which copies live values over defaults.
// It reports whether it changed dst.
func fillNilFields(dst, src reflect.Value, overwrite bool) bool {
	changed := false
	for i := range dst.NumField() {
		tag := dst.Type().Field(i).Tag.Get("yaml")
		if tag == "" || tag == ",inline" {
			continue
		}
		df, sf := dst.Field(i), src.Field(i)
		if sf.Kind() != reflect.Pointer || sf.IsNil() {
			continue
		}
		switch {
		case df.IsNil():
			df.Set(deepCopy(sf))
			changed = true
		case sf.Elem().Kind() == reflect.Struct:
			if fillNilFields(df.Elem(), sf.Elem(), overwrite) {
				changed = true
			}
		case overwrite:
			df.Set(deepCopy(sf))
			changed = true
		}
	}
	return changed
}

// deepCopy returns a copy of v that shares no pointers, slices, or maps
// with the original.
func deepCopy(v reflect.Value) reflect.Value {
	switch v.Kind() {
	case reflect.Pointer:
		if v.IsNil() {
			return v
		}
		copied := reflect.New(v.Type().Elem())
		copied.Elem().Set(deepCopy(v.Elem()))
		return copied
	case reflect.Slice:
		if v.IsNil() {
			return v
		}
		copied := reflect.MakeSlice(v.Type(), v.Len(), v.Len())
		for i := range v.Len() {
			copied.Index(i).Set(deepCopy(v.Index(i)))
		}
		return copied
	case reflect.Map:
		if v.IsNil() {
			return v
		}
		copied := reflect.MakeMapWithSize(v.Type(), v.Len())
		for _, key := range v.MapKeys() {
			copied.SetMapIndex(key, deepCopy(v.MapIndex(key)))
		}
		return copied
	case reflect.Struct:
		copied := reflect.New(v.Type()).Elem()
		for i := range v.NumField() {
			if copied.Field(i).CanSet() {
				copied.Field(i).Set(deepCopy(v.Field(i)))
			}
		}
		return copied
	default:
		return v
	}
}

// mergeSettingsFrom fills nil fields in the section from defaults, creating
// the section when it is absent. Set fields are never modified. It reports
// whether it changed anything.
func mergeSettingsFrom[T any](section **T, defaults *T, fields func(*T) []model.SettingField) bool {
	if defaults == nil {
		return false
	}
	if *section == nil {
		*section = new(T)
	}

	cv := reflect.ValueOf(*section).Elem()

	changed := false
	for _, field := range fields(defaults) {
		if mergeSettingField(cv.Field(field.Index), field) {
			changed = true
		}
	}
	return changed
}

// actionsSelectedOnlyFields lists ActionsSettings YAML keys merged only when
// the effective allowed_actions policy is "selected".
var actionsSelectedOnlyFields = map[string]bool{
	"github_owned_allowed": true,
	"verified_allowed":     true,
	"patterns_allowed":     true,
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
		if mergeSettingField(cv.Field(field.Index), field) {
			changed = true
		}
	}

	return changed
}

// mergeActionsFrom fills missing Actions policy fields from the defaults.
// Selected-action fields apply only when the effective policy is selected;
// allowed_actions precedes them in struct order, so the policy check sees
// the merged value.
func mergeActionsFrom(cfg *Config, defaults *Config) bool {
	if defaults.Actions == nil {
		return false
	}
	if cfg.Actions == nil {
		cfg.Actions = &model.ActionsSettings{}
	}

	cv := reflect.ValueOf(cfg.Actions).Elem()

	changed := false

	for _, field := range model.ActionsSettingFields(defaults.Actions) {
		if actionsSelectedOnlyFields[field.YAMLKey] {
			policy := cfg.Actions.AllowedActions
			if policy == nil || *policy != "selected" {
				continue
			}
		}
		if mergeSettingField(cv.Field(field.Index), field) {
			changed = true
		}
	}

	return changed
}

// mergeSettingField copies a set default field into cfv when cfv is nil,
// reporting whether it changed anything. Slice values are cloned so cfg does
// not share a backing array with the defaults.
func mergeSettingField(cfv reflect.Value, field model.SettingField) bool {
	if !field.Set || !cfv.IsNil() {
		return false
	}

	dfv := field.Value.Elem()
	newVal := reflect.New(dfv.Type())
	if dfv.Kind() == reflect.Slice {
		cloned := reflect.MakeSlice(dfv.Type(), dfv.Len(), dfv.Len())
		reflect.Copy(cloned, dfv)
		newVal.Elem().Set(cloned)
	} else {
		newVal.Elem().Set(dfv)
	}
	cfv.Set(newVal)
	return true
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
