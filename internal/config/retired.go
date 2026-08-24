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
	originalLength := len(cfg.Swatches)
	cfg.Swatches = slices.DeleteFunc(cfg.Swatches, func(entry SwatchEntry) bool {
		return IsRetiredWorkflowPath(entry.Path)
	})
	return len(cfg.Swatches) != originalLength
}

func isLegacyRetiredEntry(entry SwatchEntry) bool {
	return IsRetiredWorkflowPath(entry.Path) && entry.Alteration == legacyTriggered
}
