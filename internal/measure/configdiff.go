package measure

import (
	"fmt"
	"sort"

	"github.com/wimpysworld/tailor/internal/config"
	"github.com/wimpysworld/tailor/internal/swatch"
)

// diffResults implements sort.Interface, ordering by Destination.
type diffResults []DiffResult

func (d diffResults) Len() int           { return len(d) }
func (d diffResults) Less(i, j int) bool { return d[i].Destination < d[j].Destination }
func (d diffResults) Swap(i, j int)      { d[i], d[j] = d[j], d[i] }

// DiffCategory classifies a config-diff result.
type DiffCategory string

const (
	NotConfigured DiffCategory = "not-configured"
	ConfigOnly    DiffCategory = "config-only"
	ModeDiffers   DiffCategory = "mode-differs"
)

// DiffResult describes a single config-diff finding.
type DiffResult struct {
	Destination string
	Category    DiffCategory
	Annotation  string
}

// CheckConfigDiff compares the loaded config's swatch list against the
// default swatch set. Returns results grouped by category in the order:
// not-configured, config-only, mode-differs. Within each category, entries
// are sorted lexicographically by destination.
func CheckConfigDiff(cfg *config.Config, defaults []swatch.Swatch) []DiffResult {
	// Build lookup maps by destination.
	configByDest := make(map[string]config.SwatchEntry, len(cfg.Swatches))
	for _, s := range cfg.Swatches {
		configByDest[s.Destination] = s
	}

	defaultByDest := make(map[string]swatch.Swatch, len(defaults))
	for _, s := range defaults {
		defaultByDest[s.Destination] = s
	}

	var notConfigured, configOnly, modeDiffers []DiffResult

	// Destinations in default set but not in config.
	for _, s := range defaults {
		if _, found := configByDest[s.Destination]; !found {
			notConfigured = append(notConfigured, DiffResult{
				Destination: s.Destination,
				Category:    NotConfigured,
			})
		}
	}

	// Destinations in config but not in default set, or in both but with
	// differing alteration mode.
	for _, s := range cfg.Swatches {
		def, found := defaultByDest[s.Destination]
		if !found {
			configOnly = append(configOnly, DiffResult{
				Destination: s.Destination,
				Category:    ConfigOnly,
			})
		} else if s.Alteration != def.DefaultAlteration {
			modeDiffers = append(modeDiffers, DiffResult{
				Destination: s.Destination,
				Category:    ModeDiffers,
				Annotation:  fmt.Sprintf("(config: %s, default: %s)", s.Alteration, def.DefaultAlteration),
			})
		}
	}

	sort.Sort(diffResults(notConfigured))
	sort.Sort(diffResults(configOnly))
	sort.Sort(diffResults(modeDiffers))

	var results []DiffResult
	results = append(results, notConfigured...)
	results = append(results, configOnly...)
	results = append(results, modeDiffers...)
	return results
}
