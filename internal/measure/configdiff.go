package measure

import (
	"cmp"
	"fmt"
	"slices"

	"github.com/wimpysworld/tailor/internal/config"
	"github.com/wimpysworld/tailor/internal/swatch"
)

// DiffCategory classifies a config-diff result.
type DiffCategory string

const (
	NotConfigured DiffCategory = "not-configured"
	ConfigOnly    DiffCategory = "config-only"
	ModeDiffers   DiffCategory = "mode-differs"
)

// Label returns the category string with a trailing colon, suitable for formatted output.
func (c DiffCategory) Label() string { return string(c) + ":" }

// DiffResult describes a single config-diff finding.
type DiffResult struct {
	Path     string
	Category DiffCategory
	Detail   string
}

// CheckConfigDiff compares the loaded config's swatch list against the
// default swatch set. Returns results grouped by category in the order:
// not-configured, config-only, mode-differs. Within each category, entries
// are sorted lexicographically by path.
func CheckConfigDiff(cfg *config.Config, defaults []swatch.Swatch) []DiffResult {
	// Build lookup maps by path.
	configByPath := make(map[string]config.SwatchEntry, len(cfg.Swatches))
	for _, s := range cfg.Swatches {
		configByPath[s.Path] = s
	}

	defaultByPath := make(map[string]swatch.Swatch, len(defaults))
	for _, s := range defaults {
		defaultByPath[s.Path] = s
	}

	var notConfigured, configOnly, modeDiffers []DiffResult

	// Paths in default set but not in config.
	for _, s := range defaults {
		if _, found := configByPath[s.Path]; !found {
			notConfigured = append(notConfigured, DiffResult{
				Path:     s.Path,
				Category: NotConfigured,
			})
		}
	}

	// Paths in config but not in default set, or in both but with differing
	// alteration mode.
	for _, s := range cfg.Swatches {
		def, found := defaultByPath[s.Path]
		if !found {
			configOnly = append(configOnly, DiffResult{
				Path:     s.Path,
				Category: ConfigOnly,
			})
		} else if s.Alteration != def.DefaultAlteration {
			modeDiffers = append(modeDiffers, DiffResult{
				Path:     s.Path,
				Category: ModeDiffers,
				Detail:   fmt.Sprintf("(config: %s, default: %s)", s.Alteration, def.DefaultAlteration),
			})
		}
	}

	sortByPath := func(a, b DiffResult) int {
		return cmp.Compare(a.Path, b.Path)
	}
	slices.SortFunc(notConfigured, sortByPath)
	slices.SortFunc(configOnly, sortByPath)
	slices.SortFunc(modeDiffers, sortByPath)

	return slices.Concat(notConfigured, configOnly, modeDiffers)
}
