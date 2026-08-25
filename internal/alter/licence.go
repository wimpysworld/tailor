package alter

import (
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/cli/go-gh/v2/pkg/api"
	"github.com/wimpysworld/tailor/internal/config"
	"github.com/wimpysworld/tailor/internal/gh"
)

const licenceDestination = "LICENSE"

// ProcessLicence evaluates and optionally writes the LICENSE file.
// Returns a SwatchResult (reusing the same type for consistent formatting)
// and an error.
func ProcessLicence(cfg *config.Config, dir string, mode ApplyMode, client *api.RESTClient, stderr io.Writer) (*SwatchResult, error) {
	if stderr == nil {
		stderr = io.Discard
	}

	root, err := os.OpenRoot(dir)
	if err != nil {
		return nil, fmt.Errorf("opening project root %q: %w", dir, err)
	}
	defer root.Close()

	info, err := root.Lstat(licenceDestination)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, fmt.Errorf("checking licence %q: %w", licenceDestination, err)
	}
	present := err == nil

	if cfg.License == "" || cfg.License == "none" {
		if !present {
			fmt.Fprintln(stderr, "No licence file found and no licence configured. Add 'license: BlueOak-1.0.0' (or another identifier) to '.tailor.yml' and run 'tailor alter'.")
		}
		return nil, nil
	}

	// Mirror the swatch destination policy: only a regular file counts as
	// an existing licence, a destination symlink is removed without being
	// followed before a write, and anything else is an error.
	exists := false
	if present {
		switch {
		case info.IsDir():
			return nil, fmt.Errorf("licence destination %q is a directory", licenceDestination)
		case info.Mode()&os.ModeSymlink != 0:
			if mode.ShouldWrite() {
				if err := root.Remove(licenceDestination); err != nil {
					return nil, fmt.Errorf("removing destination symlink: %w", err)
				}
			}
		case !info.Mode().IsRegular():
			return nil, fmt.Errorf("licence destination %q is not a regular file", licenceDestination)
		default:
			exists = true
		}
	}

	// Licence is exempt from recut: never overwrite an existing LICENSE.
	if exists {
		return &SwatchResult{Path: licenceDestination, Category: Skipped, Reason: SkipFirstFitExists}, nil
	}

	// Fetch only when writing, so dry-run can report without calling the
	// licence body endpoint.
	if mode.ShouldWrite() {
		body, err := gh.FetchLicence(client, cfg.License)
		if err != nil {
			return nil, err
		}
		if err := writeFile(root, licenceDestination, []byte(body)); err != nil {
			return nil, err
		}
	}

	return &SwatchResult{Path: licenceDestination, Category: WouldCopy}, nil
}
