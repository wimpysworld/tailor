package alter

import (
	"crypto/sha256"
	"errors"
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
	// WouldUpdateConfig marks a config rewrite, not replacement with the embedded swatch.
	WouldUpdateConfig SwatchCategory = "would update"
	// WouldCopy marks creation of an absent destination.
	WouldCopy SwatchCategory = "would copy"
	// WouldOverwrite marks replacement of existing file content.
	WouldOverwrite SwatchCategory = "would overwrite"
	// WouldRemove marks removal of a managed or retired workflow.
	WouldRemove SwatchCategory = "would remove"
	// NoChange marks content that already matches.
	NoChange SwatchCategory = "no change"
	// Skipped marks a file that Tailor preserves under its alteration rules.
	Skipped SwatchCategory = "skipped"
	// ManagedConflict marks a managed destination that needs manual resolution.
	ManagedConflict SwatchCategory = "conflict"
)

// SwatchReason explains why a swatch was skipped.
type SwatchReason string

const (
	// SkipFirstFitExists preserves an existing first-fit destination.
	SkipFirstFitExists SwatchReason = "first-fit, exists"
	// SkipModeNever preserves a destination excluded from alterations.
	SkipModeNever SwatchReason = "mode never"
	// SkipManagedRootExists preserves a root that needs manual loader adoption.
	SkipManagedRootExists SwatchReason = "existing root preserved"
	// SkipManagedSharedExists preserves shared settings that Tailor does not own.
	SkipManagedSharedExists SwatchReason = "existing shared settings preserved"
	// ManagedNotOwned identifies an unmarked managed destination.
	ManagedNotOwned SwatchReason = "not owned by Tailor"
)

// SwatchResult records the path and categorised outcome for one swatch entry.
type SwatchResult struct {
	Path     string
	Category SwatchCategory
	Reason   SwatchReason
}

const configPath = config.ConfigSwatchPath

// ProcessSwatches previews or writes active swatches after token substitution.
// Config, Pages and wiki files use separate processors.
func ProcessSwatches(cfg *config.Config, dir string, mode ApplyMode, tokens *TokenContext) ([]SwatchResult, error) {
	return processSwatches(cfg, dir, mode, tokens, nil)
}

func processSwatches(cfg *config.Config, dir string, mode ApplyMode, tokens *TokenContext, excluded map[string]struct{}) ([]SwatchResult, error) {
	if tokens == nil {
		tokens = &TokenContext{}
	}
	contents := tokens.rendered
	if contents == nil {
		var err error
		contents, err = prepareGoSwatches(cfg, dir, mode, tokens.DefaultBranch)
		if err != nil {
			return nil, err
		}
	}
	results := make([]SwatchResult, 0, len(cfg.Swatches))
	root, err := os.OpenRoot(dir)
	if err != nil {
		return nil, fmt.Errorf("opening project root %q: %w", dir, err)
	}
	defer root.Close()

	for _, entry := range cfg.Swatches {
		if !cfg.SwatchActive(entry.Path) {
			continue
		}
		if _, skip := excluded[entry.Path]; skip {
			continue
		}
		if entry.Path == configPath || entry.Path == swatch.PagesDestination || swatch.IsWiki(entry.Path) || swatch.IsPagesStarter(entry.Path) {
			continue
		}

		content, rendered := contents[entry.Path]
		if !rendered {
			content, err = swatch.Content(entry.Path)
			if err != nil {
				return nil, fmt.Errorf("reading swatch %q: %w", entry.Path, err)
			}
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

// processSwatch previews or writes a swatch whose content the caller has already rendered.
func processSwatch(root *os.Root, entry config.SwatchEntry, content []byte, mode ApplyMode) (SwatchResult, error) {
	// Never mode skips unconditionally, regardless of apply mode or file existence.
	if entry.Alteration == swatch.Never {
		return SwatchResult{Path: entry.Path, Category: Skipped, Reason: SkipModeNever}, nil
	}

	if err := checkParents(root, entry.Path, "swatch parent"); err != nil {
		return SwatchResult{}, err
	}

	exists, err := prepareSwatchDestination(root, entry.Path, mode.ShouldWrite())
	if err != nil {
		return SwatchResult{}, fmt.Errorf("checking swatch %q: %w", entry.Path, err)
	}

	if mode == Recut {
		category := WouldOverwrite
		if !exists {
			category = WouldCopy
		}
		return writeSwatch(root, entry, content, category, true)
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

	return writeSwatch(root, entry, content, WouldCopy, mode.ShouldWrite())
}

// writeSwatch writes content to the entry path when write is true and
// returns the result with the given category.
func writeSwatch(root *os.Root, entry config.SwatchEntry, content []byte, category SwatchCategory, write bool) (SwatchResult, error) {
	if write {
		if err := writeFile(root, entry.Path, content); err != nil {
			return SwatchResult{}, err
		}
	}
	return SwatchResult{Path: entry.Path, Category: category}, nil
}

// checkParents walks each parent directory of the root-relative path and
// rejects a symlink or a non-directory. A missing parent is not an error.
// subject prefixes each error message, for example "swatch parent".
func checkParents(root *os.Root, path, subject string) error {
	parent := filepath.Dir(path)
	if parent == "." {
		return nil
	}

	current := ""
	for component := range strings.SplitSeq(parent, string(filepath.Separator)) {
		current = filepath.Join(current, component)
		info, err := root.Lstat(current)
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				return nil
			}
			return fmt.Errorf("checking %s %q: %w", subject, current, err)
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("%s %q is a symlink", subject, current)
		}
		if !info.IsDir() {
			return fmt.Errorf("%s %q is not a directory", subject, current)
		}
	}
	return nil
}

// prepareSwatchDestination counts only regular files as existing destinations.
// A write removes a destination symlink without following it.
func prepareSwatchDestination(root *os.Root, path string, shouldWrite bool) (bool, error) {
	info, err := root.Lstat(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return false, nil
		}
		return false, err
	}
	if info.IsDir() {
		return false, fmt.Errorf("swatch destination %q is a directory", path)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		if shouldWrite {
			if err := root.Remove(path); err != nil {
				return false, fmt.Errorf("removing destination symlink: %w", err)
			}
		}
		return false, nil
	}
	if !info.Mode().IsRegular() {
		return false, fmt.Errorf("swatch destination %q is not a regular file", path)
	}
	return true, nil
}

func processAlways(root *os.Root, entry config.SwatchEntry, content []byte, mode ApplyMode) (SwatchResult, error) {
	onDisk, err := contentHashFile(root, entry.Path)
	if err != nil {
		return SwatchResult{}, fmt.Errorf("hashing on-disk file %q: %w", entry.Path, err)
	}

	if sha256.Sum256(content) == onDisk {
		return SwatchResult{Path: entry.Path, Category: NoChange}, nil
	}

	return writeSwatch(root, entry, content, WouldOverwrite, mode.ShouldWrite())
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
