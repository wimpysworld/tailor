package alter

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/cli/go-gh/v2/pkg/api"
	"github.com/wimpysworld/tailor/internal/config"
	"github.com/wimpysworld/tailor/internal/gh"
)

const licenceDestination = "LICENSE"

// ProcessLicence evaluates and optionally writes the LICENSE file.
// Returns a SwatchResult (reusing the same type for consistent formatting)
// and an error.
func ProcessLicence(cfg *config.Config, dir string, mode ApplyMode, client *api.RESTClient) (*SwatchResult, error) {
	dest := filepath.Join(dir, licenceDestination)
	exists := fileExists(dest)

	if cfg.License == "" || cfg.License == "none" {
		if !exists {
			fmt.Fprintln(os.Stderr, "No licence file found and no licence configured. Add 'license: MIT' (or another identifier) to '.tailor/config.yml' and run 'tailor alter'.")
		}
		return nil, nil
	}

	// Licence is exempt from recut: never overwrite an existing LICENSE.
	if exists {
		return &SwatchResult{Destination: licenceDestination, Category: SkippedFirstFit}, nil
	}

	// LICENSE absent: fetch and (conditionally) write.
	if mode.ShouldWrite() {
		body, err := gh.FetchLicence(client, cfg.License)
		if err != nil {
			return nil, err
		}
		if err := writeFile(dest, []byte(body)); err != nil {
			return nil, err
		}
	}

	return &SwatchResult{Destination: licenceDestination, Category: WouldCopy}, nil
}
