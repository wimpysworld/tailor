package config

import (
	"slices"

	"github.com/wimpysworld/tailor/internal/swatch"
)

var retiredWorkflowPaths = []string{
	".github/workflows/tailor-automerge.yml",
	".github/workflows/tailor.yml",
}

const legacyTriggered swatch.AlterationMode = "triggered"

// RetiredWorkflowPaths returns the fixed retired workflow paths in lexical order.
func RetiredWorkflowPaths() []string {
	return slices.Clone(retiredWorkflowPaths)
}

// IsRetiredWorkflowPath reports whether path is a fixed retired workflow path.
func IsRetiredWorkflowPath(path string) bool {
	return slices.Contains(retiredWorkflowPaths, path)
}

// RemoveRetiredWorkflowEntries removes every retired workflow entry from cfg.
func RemoveRetiredWorkflowEntries(cfg *Config) bool {
	kept := cfg.Swatches[:0]
	removed := false
	for _, entry := range cfg.Swatches {
		if IsRetiredWorkflowPath(entry.Path) {
			removed = true
			continue
		}
		kept = append(kept, entry)
	}
	if removed {
		clear(cfg.Swatches[len(kept):])
		cfg.Swatches = kept
	}
	return removed
}

func isLegacyRetiredEntry(entry SwatchEntry) bool {
	return IsRetiredWorkflowPath(entry.Path) && entry.Alteration == legacyTriggered
}
