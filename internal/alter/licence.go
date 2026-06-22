package alter

import (
	"fmt"
	"os"

	"github.com/cli/go-gh/v2/pkg/api"
	"github.com/wimpysworld/tailor/internal/config"
	"github.com/wimpysworld/tailor/internal/gh"
)

const licenceDestination = "LICENSE"

// ProcessLicence evaluates and optionally writes the LICENSE file.
// Returns a SwatchResult (reusing the same type for consistent formatting)
// and an error.
func ProcessLicence(cfg *config.Config, dir string, mode ApplyMode, client *api.RESTClient) (*SwatchResult, error) {
	root, err := os.OpenRoot(dir)
	if err != nil {
		return nil, fmt.Errorf("opening project root %q: %w", dir, err)
	}
	defer root.Close()

	exists, err := rootFileExists(root, licenceDestination)
	if err != nil {
		return nil, fmt.Errorf("checking licence %q: %w", licenceDestination, err)
	}

	if cfg.License == "" || cfg.License == "none" {
		if !exists {
			fmt.Fprintln(os.Stderr, "No licence file found and no licence configured. Add 'license: BlueOak-1.0.0' (or another identifier) to '.tailor.yml' and run 'tailor alter'.")
		}
		return nil, nil
	}

	// Licence is exempt from recut: never overwrite an existing LICENSE.
	if exists {
		return &SwatchResult{Path: licenceDestination, Category: SkippedFirstFit}, nil
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
