package config

import "github.com/wimpysworld/tailor/internal/swatch"

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
