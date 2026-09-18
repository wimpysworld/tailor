package alter

import "github.com/wimpysworld/tailor/internal/config"

// ProcessOrdinarySwatchesForTest exposes ordinary swatch processing to external tests.
func ProcessOrdinarySwatchesForTest(cfg *config.Config, dir string, mode ApplyMode, tokens *TokenContext) ([]SwatchResult, error) {
	results, err := processSwatches(cfg, dir, mode, tokens, managedExcludedPaths())
	return results, err
}
