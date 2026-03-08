package config

import (
	"reflect"

	"github.com/wimpysworld/tailor/internal/swatch"
)

// repoSettingsSkipFields lists RepositorySettings field names excluded from
// default merging. Description and Homepage are project-specific (nil'd by
// DefaultConfig). Topics are project-specific per spec.
var repoSettingsSkipFields = map[string]bool{
	"Description": true,
	"Homepage":    true,
	"Topics":      true,
}

// MergeDefaultRepoSettings fills nil pointer fields in cfg.Repository from the
// embedded default configuration. It skips Description, Homepage, and Topics.
// If cfg.Repository is nil, it allocates a new RepositorySettings. Returns true
// when at least one field was added.
func MergeDefaultRepoSettings(cfg *Config) bool {
	defaults, err := DefaultConfig("_")
	if err != nil || defaults.Repository == nil {
		return false
	}

	if cfg.Repository == nil {
		cfg.Repository = &RepositorySettings{}
	}

	dv := reflect.ValueOf(defaults.Repository).Elem()
	cv := reflect.ValueOf(cfg.Repository).Elem()
	dt := dv.Type()

	changed := false

	for i := range dt.NumField() {
		field := dt.Field(i)

		if repoSettingsSkipFields[field.Name] {
			continue
		}

		// Only process pointer fields; skip the Extra inline map.
		if field.Tag.Get("yaml") == "" || field.Tag.Get("yaml") == ",inline" {
			continue
		}

		dfv := dv.Field(i)
		if dfv.Kind() != reflect.Ptr || dfv.IsNil() {
			continue
		}

		cfv := cv.Field(i)
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

// MergeDefaultLabels populates cfg.Labels from the embedded default
// configuration when the slice is empty. Both present-but-empty (labels: [])
// and absent (no labels key) result in len==0 after YAML unmarshalling, so
// both cases receive the default labels. If cfg.Labels already contains
// entries, the function leaves them unchanged and returns false.
func MergeDefaultLabels(cfg *Config) bool {
	if len(cfg.Labels) > 0 {
		return false
	}

	defaults, err := DefaultConfig("_")
	if err != nil || len(defaults.Labels) == 0 {
		return false
	}

	cfg.Labels = make([]LabelEntry, len(defaults.Labels))
	copy(cfg.Labels, defaults.Labels)

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
