package alter

import (
	"testing"

	"github.com/wimpysworld/tailor/internal/config"
)

// RenderManagedFilesForTest returns the selected renderer output for external tests.
func RenderManagedFilesForTest(t *testing.T, cfg *config.Config) map[string][]byte {
	t.Helper()
	return renderSelectedManagedFiles(t, cfg)
}

// ProcessOrdinarySwatchesForTest exposes ordinary swatch processing to external tests.
func ProcessOrdinarySwatchesForTest(cfg *config.Config, dir string, mode ApplyMode, tokens *TokenContext) ([]SwatchResult, error) {
	results, err := processSwatches(cfg, dir, mode, tokens, managedExcludedPaths())
	return results, err
}
