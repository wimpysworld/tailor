package config

import (
	"reflect"

	"github.com/wimpysworld/tailor/internal/swatch"
)

// repoSettingsSkipFields lists RepositorySettings field names excluded from
// default merging. Description and Homepage are project-specific (nil'd by
// DefaultConfig). Topics are project-specific per spec.
var repoSettingsSkipFields = map[string]struct{}{
	"Description": {},
	"Homepage":    {},
	"Topics":      {},
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

		if _, skip := repoSettingsSkipFields[field.Name]; skip {
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

// ConfigSwatchSource is the source path of the config.yml swatch entry,
// which is excluded from merge because it describes the config file itself.
const ConfigSwatchSource = ".tailor/config.yml"

// MergeDefaultSwatches appends missing default swatch entries to cfg.Swatches.
// It skips the config.yml entry itself. Existing entries are matched by Source
// path, so a remapped destination or altered mode does not cause duplication.
// It returns the slice of newly added entries.
func MergeDefaultSwatches(cfg *Config) []SwatchEntry {
	present := make(map[string]struct{}, len(cfg.Swatches))
	for _, e := range cfg.Swatches {
		present[e.Source] = struct{}{}
	}

	var added []SwatchEntry
	for _, s := range swatch.All() {
		if s.Source == ConfigSwatchSource {
			continue
		}
		if _, ok := present[s.Source]; ok {
			continue
		}
		entry := SwatchEntry{
			Source:      s.Source,
			Destination: s.Destination,
			Alteration:  s.DefaultAlteration,
		}
		cfg.Swatches = append(cfg.Swatches, entry)
		added = append(added, entry)
	}
	return added
}
