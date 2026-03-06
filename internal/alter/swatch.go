package alter

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/wimpysworld/tailor/internal/config"
	"github.com/wimpysworld/tailor/internal/swatch"
)

// SwatchCategory classifies the outcome of processing a single swatch entry.
type SwatchCategory string

const (
	WouldCopy       SwatchCategory = "would copy"
	WouldOverwrite  SwatchCategory = "would overwrite"
	WouldRemove     SwatchCategory = "would remove"
	Removed         SwatchCategory = "removed"
	NoChange        SwatchCategory = "no change"
	SkippedFirstFit SwatchCategory = "skipped (first-fit, exists)"
	SkippedNever    SwatchCategory = "skip (never)"
)

// SwatchResult records the destination path and categorised outcome for one
// swatch entry. Annotation carries optional context such as the trigger
// condition name, appended to the category label in formatted output.
type SwatchResult struct {
	Destination string
	Category    SwatchCategory
	Annotation  string
}

// configSource is the source path of the config.yml swatch entry.
const configSource = config.ConfigSwatchSource

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

		if !isInsideDir(dir, dest) {
			return nil, fmt.Errorf("swatch %q: destination %q escapes project root", entry.Source, entry.Destination)
		}

		result, err := processSwatch(cfg, entry, content, dest, mode)
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
func processSwatch(cfg *config.Config, entry config.SwatchEntry, content []byte, dest string, mode ApplyMode) (SwatchResult, error) {
	// Never mode skips unconditionally, regardless of apply mode or file existence.
	if entry.Alteration == swatch.Never {
		return SwatchResult{Destination: entry.Destination, Category: SkippedNever}, nil
	}

	exists := fileExists(dest)

	if mode == Recut {
		return processRecut(entry, content, dest, exists, mode)
	}

	switch entry.Alteration {
	case swatch.FirstFit:
		return processFirstFit(entry, content, dest, exists, mode)
	case swatch.Always:
		return processAlways(entry, content, dest, exists, mode)
	case swatch.Triggered:
		return processTriggered(cfg, entry, content, dest, exists, mode)
	default:
		return SwatchResult{}, fmt.Errorf("unknown alteration mode %q for swatch %q", entry.Alteration, entry.Source)
	}
}

func processFirstFit(entry config.SwatchEntry, content []byte, dest string, exists bool, mode ApplyMode) (SwatchResult, error) {
	if exists {
		return SwatchResult{Destination: entry.Destination, Category: SkippedFirstFit}, nil
	}
	if mode.ShouldWrite() {
		if err := writeFile(dest, content); err != nil {
			return SwatchResult{}, err
		}
	}
	return SwatchResult{Destination: entry.Destination, Category: WouldCopy}, nil
}

func processAlways(entry config.SwatchEntry, content []byte, dest string, exists bool, mode ApplyMode) (SwatchResult, error) {
	if !exists {
		if mode.ShouldWrite() {
			if err := writeFile(dest, content); err != nil {
				return SwatchResult{}, err
			}
		}
		return SwatchResult{Destination: entry.Destination, Category: WouldCopy}, nil
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

func processTriggered(cfg *config.Config, entry config.SwatchEntry, content []byte, dest string, exists bool, mode ApplyMode) (SwatchResult, error) {
	annotation := triggerAnnotation(entry.Source)

	if swatch.EvaluateTrigger(entry.Source, cfg.Repository) {
		result, err := processAlways(entry, content, dest, exists, mode)
		if err != nil {
			return result, err
		}
		result.Annotation = annotation
		return result, nil
	}

	if exists {
		if mode.ShouldWrite() {
			if err := os.Remove(dest); err != nil {
				return SwatchResult{}, fmt.Errorf("removing file %q: %w", dest, err)
			}
			return SwatchResult{Destination: entry.Destination, Category: Removed, Annotation: annotation}, nil
		}
		return SwatchResult{Destination: entry.Destination, Category: WouldRemove, Annotation: annotation}, nil
	}

	return SwatchResult{Destination: entry.Destination, Category: SkippedNever, Annotation: annotation}, nil
}

// triggerAnnotation returns the formatted trigger context string for a source
// path, e.g. "triggered: allow_auto_merge". Returns empty if no trigger exists.
func triggerAnnotation(source string) string {
	tc, ok := swatch.LookupTrigger(source)
	if !ok {
		return ""
	}
	return "triggered: " + tc.ConfigField
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

// isInsideDir reports whether path is inside dir after cleaning. Prevents path
// traversal via ".." components in swatch destinations.
func isInsideDir(dir, path string) bool {
	absDir := filepath.Clean(dir) + string(filepath.Separator)
	absPath := filepath.Clean(path)
	return strings.HasPrefix(absPath, absDir)
}
