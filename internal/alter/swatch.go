package alter

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/wimpysworld/tailor/internal/config"
	"github.com/wimpysworld/tailor/internal/swatch"
)

// SwatchCategory classifies the outcome of processing a single swatch entry.
type SwatchCategory string

const (
	WouldCopy       SwatchCategory = "would copy"
	WouldOverwrite  SwatchCategory = "would overwrite"
	WouldDeploy     SwatchCategory = "would deploy"
	WouldRemove     SwatchCategory = "would remove"
	Removed         SwatchCategory = "removed"
	NoChange        SwatchCategory = "no change"
	SkippedFirstFit SwatchCategory = "skipped (first-fit, exists)"
	SkippedNever    SwatchCategory = "skip (never)"
)

// SwatchResult records the path and categorised outcome for one swatch entry.
// Annotation carries optional context such as the trigger condition name,
// appended to the category label in formatted output.
type SwatchResult struct {
	Path       string
	Category   SwatchCategory
	Annotation string
}

// configPath is the path of the config swatch entry.
const configPath = config.ConfigSwatchPath

// ProcessSwatches evaluates each swatch entry in cfg and returns results.
// When mode is Apply or Recut, it writes files to disk.
func ProcessSwatches(cfg *config.Config, dir string, mode ApplyMode, tokens *TokenContext) ([]SwatchResult, error) {
	results := make([]SwatchResult, 0, len(cfg.Swatches))
	root, err := os.OpenRoot(dir)
	if err != nil {
		return nil, fmt.Errorf("opening project root %q: %w", dir, err)
	}
	defer root.Close()

	for _, entry := range cfg.Swatches {
		if entry.Path == configPath {
			continue
		}

		content, err := swatch.Content(entry.Path)
		if err != nil {
			return nil, fmt.Errorf("reading swatch %q: %w", entry.Path, err)
		}

		content = tokens.Substitute(content, entry.Path)

		result, err := processSwatch(cfg, root, entry, content, mode)
		if err != nil {
			return nil, err
		}
		results = append(results, result)
	}

	return results, nil
}

// processSwatch determines the category for a single swatch and writes
// the file when the mode permits. Token substitution occurs upstream in
// ProcessSwatches before this function is called.
func processSwatch(cfg *config.Config, root *os.Root, entry config.SwatchEntry, content []byte, mode ApplyMode) (SwatchResult, error) {
	// Never mode skips unconditionally, regardless of apply mode or file existence.
	if entry.Alteration == swatch.Never {
		return SwatchResult{Path: entry.Path, Category: SkippedNever}, nil
	}

	exists, err := rootFileExists(root, entry.Path)
	if err != nil {
		return SwatchResult{}, fmt.Errorf("checking swatch %q: %w", entry.Path, err)
	}

	if mode == Recut {
		// Triggered swatches are never overwritten by --recut when the
		// trigger condition is false.
		if entry.Alteration == swatch.Triggered && !swatch.EvaluateTrigger(entry.Path, cfg.Repository) {
			return processTriggered(cfg, root, entry, content, exists, Apply)
		}
		result, err := processRecut(root, entry, content, exists)
		if err != nil {
			return result, err
		}
		// Triggered swatches use "would deploy" with annotation even
		// under --recut, per spec.
		if entry.Alteration == swatch.Triggered {
			if result.Category == WouldCopy || result.Category == WouldOverwrite {
				result.Category = WouldDeploy
			}
			result.Annotation = triggerAnnotation(entry.Path)
		}
		return result, nil
	}

	switch entry.Alteration {
	case swatch.FirstFit:
		return processFirstFit(root, entry, content, exists, mode)
	case swatch.Always:
		return processAlways(root, entry, content, exists, mode)
	case swatch.Triggered:
		return processTriggered(cfg, root, entry, content, exists, mode)
	default:
		return SwatchResult{}, fmt.Errorf("unknown alteration mode %q for swatch %q", entry.Alteration, entry.Path)
	}
}

func processFirstFit(root *os.Root, entry config.SwatchEntry, content []byte, exists bool, mode ApplyMode) (SwatchResult, error) {
	if exists {
		return SwatchResult{Path: entry.Path, Category: SkippedFirstFit}, nil
	}
	if mode.ShouldWrite() {
		if err := writeFile(root, entry.Path, content); err != nil {
			return SwatchResult{}, err
		}
	}
	return SwatchResult{Path: entry.Path, Category: WouldCopy}, nil
}

func processAlways(root *os.Root, entry config.SwatchEntry, content []byte, exists bool, mode ApplyMode) (SwatchResult, error) {
	if !exists {
		if mode.ShouldWrite() {
			if err := writeFile(root, entry.Path, content); err != nil {
				return SwatchResult{}, err
			}
		}
		return SwatchResult{Path: entry.Path, Category: WouldCopy}, nil
	}

	onDisk, err := contentHashFile(root, entry.Path)
	if err != nil {
		return SwatchResult{}, fmt.Errorf("hashing on-disk file %q: %w", entry.Path, err)
	}

	if contentHash(content) == onDisk {
		return SwatchResult{Path: entry.Path, Category: NoChange}, nil
	}

	if mode.ShouldWrite() {
		if err := writeFile(root, entry.Path, content); err != nil {
			return SwatchResult{}, err
		}
	}
	return SwatchResult{Path: entry.Path, Category: WouldOverwrite}, nil
}

func processTriggered(cfg *config.Config, root *os.Root, entry config.SwatchEntry, content []byte, exists bool, mode ApplyMode) (SwatchResult, error) {
	annotation := triggerAnnotation(entry.Path)

	if swatch.EvaluateTrigger(entry.Path, cfg.Repository) {
		result, err := processAlways(root, entry, content, exists, mode)
		if err != nil {
			return result, err
		}
		// Triggered swatches use "would deploy" instead of "would copy" or
		// "would overwrite" per spec.
		if result.Category == WouldCopy || result.Category == WouldOverwrite {
			result.Category = WouldDeploy
		}
		result.Annotation = annotation
		return result, nil
	}

	if exists {
		if mode.ShouldWrite() {
			if err := root.Remove(entry.Path); err != nil {
				return SwatchResult{}, fmt.Errorf("removing file %q: %w", entry.Path, err)
			}
			return SwatchResult{Path: entry.Path, Category: Removed, Annotation: annotation}, nil
		}
		return SwatchResult{Path: entry.Path, Category: WouldRemove, Annotation: annotation}, nil
	}

	return SwatchResult{Path: entry.Path, Category: SkippedNever, Annotation: annotation}, nil
}

// triggerAnnotation returns the formatted trigger context string for a swatch
// path, e.g. "triggered: allow_auto_merge". Returns empty if no trigger exists.
func triggerAnnotation(path string) string {
	tc, ok := swatch.LookupTrigger(path)
	if !ok {
		return ""
	}
	return "triggered: " + tc.ConfigField
}

func processRecut(root *os.Root, entry config.SwatchEntry, content []byte, exists bool) (SwatchResult, error) {
	category := WouldOverwrite
	if !exists {
		category = WouldCopy
	}
	if err := writeFile(root, entry.Path, content); err != nil {
		return SwatchResult{}, err
	}
	return SwatchResult{Path: entry.Path, Category: category}, nil
}

func rootFileExists(root *os.Root, path string) (bool, error) {
	if _, err := root.Stat(path); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

// writeFile creates parent directories and writes data to a root-relative path.
func writeFile(root *os.Root, path string, data []byte) error {
	if err := root.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("creating directories for %q: %w", path, err)
	}
	if err := root.WriteFile(path, data, 0o644); err != nil {
		return fmt.Errorf("writing file %q: %w", path, err)
	}
	return nil
}

// contentHash returns the hex-encoded SHA-256 digest of data.
func contentHash(data []byte) string {
	h := sha256.Sum256(data)
	return hex.EncodeToString(h[:])
}

// contentHashFile returns the hex-encoded SHA-256 digest of the root-relative file at path.
func contentHashFile(root *os.Root, path string) (string, error) {
	data, err := root.ReadFile(path)
	if err != nil {
		return "", err
	}
	return contentHash(data), nil
}
