package alter

import (
	"crypto/sha256"
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
	WouldUpdateConfig SwatchCategory = "would update"
	WouldCopy         SwatchCategory = "would copy"
	WouldOverwrite    SwatchCategory = "would overwrite"
	WouldRemove       SwatchCategory = "would remove"
	NoChange          SwatchCategory = "no change"
	Skipped           SwatchCategory = "skipped"
)

// SwatchReason explains why a swatch was skipped.
type SwatchReason string

const (
	SkipFirstFitExists SwatchReason = "first-fit, exists"
	SkipModeNever      SwatchReason = "mode never"
)

// SwatchResult records the path and categorised outcome for one swatch entry.
type SwatchResult struct {
	Path     string
	Category SwatchCategory
	Reason   SwatchReason
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

		result, err := processSwatch(root, entry, content, mode)
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
func processSwatch(root *os.Root, entry config.SwatchEntry, content []byte, mode ApplyMode) (SwatchResult, error) {
	// Never mode skips unconditionally, regardless of apply mode or file existence.
	if entry.Alteration == swatch.Never {
		return SwatchResult{Path: entry.Path, Category: Skipped, Reason: SkipModeNever}, nil
	}

	exists, err := rootFileExists(root, entry.Path)
	if err != nil {
		return SwatchResult{}, fmt.Errorf("checking swatch %q: %w", entry.Path, err)
	}

	if mode == Recut {
		return processRecut(root, entry, content, exists)
	}

	switch entry.Alteration {
	case swatch.FirstFit:
		if exists {
			return SwatchResult{Path: entry.Path, Category: Skipped, Reason: SkipFirstFitExists}, nil
		}
	case swatch.Always:
		if exists {
			return processAlways(root, entry, content, mode)
		}
	default:
		return SwatchResult{}, fmt.Errorf("unknown alteration mode %q for swatch %q", entry.Alteration, entry.Path)
	}

	if mode.ShouldWrite() {
		if err := writeFile(root, entry.Path, content); err != nil {
			return SwatchResult{}, err
		}
	}
	return SwatchResult{Path: entry.Path, Category: WouldCopy}, nil
}

func processAlways(root *os.Root, entry config.SwatchEntry, content []byte, mode ApplyMode) (SwatchResult, error) {
	onDisk, err := contentHashFile(root, entry.Path)
	if err != nil {
		return SwatchResult{}, fmt.Errorf("hashing on-disk file %q: %w", entry.Path, err)
	}

	if sha256.Sum256(content) == onDisk {
		return SwatchResult{Path: entry.Path, Category: NoChange}, nil
	}

	if mode.ShouldWrite() {
		if err := writeFile(root, entry.Path, content); err != nil {
			return SwatchResult{}, err
		}
	}
	return SwatchResult{Path: entry.Path, Category: WouldOverwrite}, nil
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

// contentHashFile returns the SHA-256 digest of the root-relative file at path.
func contentHashFile(root *os.Root, path string) ([sha256.Size]byte, error) {
	data, err := root.ReadFile(path)
	if err != nil {
		return [sha256.Size]byte{}, err
	}
	return sha256.Sum256(data), nil
}
