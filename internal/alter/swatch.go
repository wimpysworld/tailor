package alter

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"

	"github.com/wimpysworld/tailor/internal/config"
	"github.com/wimpysworld/tailor/internal/swatch"
)

// SwatchCategory classifies the outcome of processing a single swatch entry.
type SwatchCategory string

const (
	WouldCopy      SwatchCategory = "would copy"
	WouldOverwrite SwatchCategory = "would overwrite"
	NoChange       SwatchCategory = "no change"
	Skipped        SwatchCategory = "skipped (first-fit, exists)"
)

// SwatchResult records the destination path and categorised outcome for one
// swatch entry.
type SwatchResult struct {
	Destination string
	Category    SwatchCategory
}

// configDestination is exempt from recut overwrite.
const configDestination = ".tailor/config.yml"

// ProcessSwatches evaluates each swatch entry in cfg and returns results.
// When mode is Apply or Recut, it writes files to disk.
func ProcessSwatches(cfg *config.Config, dir string, mode ApplyMode, tokens *TokenContext) ([]SwatchResult, error) {
	results := make([]SwatchResult, 0, len(cfg.Swatches))

	for _, entry := range cfg.Swatches {
		content, err := swatch.Content(entry.Source)
		if err != nil {
			return nil, fmt.Errorf("reading swatch %q: %w", entry.Source, err)
		}

		content = tokens.Substitute(content, entry.Source)
		dest := filepath.Join(dir, entry.Destination)

		result, err := processSwatch(entry, content, dest, mode, tokens)
		if err != nil {
			return nil, err
		}
		results = append(results, result)
	}

	return results, nil
}

// processSwatch determines the category for a single swatch and writes
// the file when the mode permits. Token substitution occurs upstream in
// ProcessSwatches before this function is called. The tokens parameter is
// passed through only for processAlways, which uses it to skip the hash
// comparison when a source has active substitutions.
func processSwatch(entry config.SwatchEntry, content []byte, dest string, mode ApplyMode, tokens *TokenContext) (SwatchResult, error) {
	exists := fileExists(dest)

	// Recut exemption: .tailor/config.yml behaves as first-fit.
	// Pass DryRun to suppress writes; config.yml is never overwritten.
	if mode == Recut && entry.Destination == configDestination {
		return processFirstFit(entry, content, dest, exists, DryRun)
	}

	if mode == Recut {
		return processRecut(entry, content, dest, exists, mode)
	}

	switch entry.Alteration {
	case swatch.FirstFit:
		return processFirstFit(entry, content, dest, exists, mode)
	case swatch.Always:
		return processAlways(entry, content, dest, exists, mode, tokens)
	default:
		return SwatchResult{}, fmt.Errorf("unknown alteration mode %q for swatch %q", entry.Alteration, entry.Source)
	}
}

func processFirstFit(entry config.SwatchEntry, content []byte, dest string, exists bool, mode ApplyMode) (SwatchResult, error) {
	if exists {
		return SwatchResult{Destination: entry.Destination, Category: Skipped}, nil
	}
	if mode.ShouldWrite() {
		if err := writeFile(dest, content); err != nil {
			return SwatchResult{}, err
		}
	}
	return SwatchResult{Destination: entry.Destination, Category: WouldCopy}, nil
}

func processAlways(entry config.SwatchEntry, content []byte, dest string, exists bool, mode ApplyMode, tokens *TokenContext) (SwatchResult, error) {
	if !exists {
		if mode.ShouldWrite() {
			if err := writeFile(dest, content); err != nil {
				return SwatchResult{}, err
			}
		}
		return SwatchResult{Destination: entry.Destination, Category: WouldCopy}, nil
	}

	// Substituted sources always overwrite; MD5 comparison is skipped.
	if tokens.HasSubstitution(entry.Source) {
		if mode.ShouldWrite() {
			if err := writeFile(dest, content); err != nil {
				return SwatchResult{}, err
			}
		}
		return SwatchResult{Destination: entry.Destination, Category: WouldOverwrite}, nil
	}

	onDisk, err := contentHashFile(dest)
	if err != nil {
		return SwatchResult{}, fmt.Errorf("hashing on-disk file %q: %w", dest, err)
	}

	if contentHash(content) == onDisk {
		return SwatchResult{Destination: entry.Destination, Category: NoChange}, nil
	}

	if mode.ShouldWrite() {
		if err := writeFile(dest, content); err != nil {
			return SwatchResult{}, err
		}
	}
	return SwatchResult{Destination: entry.Destination, Category: WouldOverwrite}, nil
}

func processRecut(entry config.SwatchEntry, content []byte, dest string, exists bool, mode ApplyMode) (SwatchResult, error) {
	category := WouldOverwrite
	if !exists {
		category = WouldCopy
	}
	if mode.ShouldWrite() {
		if err := writeFile(dest, content); err != nil {
			return SwatchResult{}, err
		}
	}
	return SwatchResult{Destination: entry.Destination, Category: category}, nil
}

// writeFile creates parent directories and writes data to path.
func writeFile(path string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("creating directories for %q: %w", path, err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return fmt.Errorf("writing file %q: %w", path, err)
	}
	return nil
}

// fileExists reports whether a file exists at path.
func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// contentHash returns the hex-encoded SHA-256 digest of data.
func contentHash(data []byte) string {
	h := sha256.Sum256(data)
	return hex.EncodeToString(h[:])
}

// contentHashFile returns the hex-encoded SHA-256 digest of the file at path.
func contentHashFile(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	return contentHash(data), nil
}
