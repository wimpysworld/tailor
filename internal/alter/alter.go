package alter

import (
	"fmt"

	"github.com/cli/go-gh/v2/pkg/api"
	"github.com/wimpysworld/tailor/internal/config"
	"github.com/wimpysworld/tailor/internal/gh"
)

// ApplyMode controls whether changes are written to disk.
type ApplyMode int

const (
	DryRun ApplyMode = iota // preview only
	Apply                   // write if file is absent or alteration permits
	Recut                   // overwrite unconditionally
)

// ShouldWrite reports whether the mode permits writing to disk.
func (m ApplyMode) ShouldWrite() bool { return m == Apply || m == Recut }

// Run executes the alter command. It validates the config, applies
// repository settings, fetches the licence, and processes swatches.
// When client is nil, a default GitHub REST client is created.
func Run(cfg *config.Config, dir string, mode ApplyMode, client *api.RESTClient) error {
	if err := config.ValidateSources(cfg); err != nil {
		return err
	}
	if err := config.ValidateDuplicateDestinations(cfg); err != nil {
		return err
	}
	if err := config.ValidateRepoSettings(cfg); err != nil {
		return err
	}

	if client == nil {
		var err error
		client, err = api.DefaultRESTClient()
		if err != nil {
			return fmt.Errorf("creating GitHub API client: %w", err)
		}
	}

	username, err := gh.FetchUsername(client)
	if err != nil {
		return fmt.Errorf("fetching GitHub username: %w", err)
	}

	owner, name, hasRepo := gh.RepoContext()
	tokens := TokenContext{
		GitHubUsername: username,
		Owner:          owner,
		Name:           name,
		Repository:     cfg.Repository,
	}

	// Repository settings processing.
	repoResults, err := ProcessRepoSettings(cfg, mode, client, owner, name, hasRepo)
	if err != nil {
		return err
	}

	// Licence processing.
	licenceResult, err := ProcessLicence(cfg, dir, mode, client)
	if err != nil {
		return err
	}

	// Swatch processing.
	swatchResults, err := ProcessSwatches(cfg, dir, mode, &tokens)
	if err != nil {
		return err
	}

	// Merge licence result into swatch results for unified output.
	if licenceResult != nil {
		swatchResults = append([]SwatchResult{*licenceResult}, swatchResults...)
	}

	output := FormatOutput(repoResults, swatchResults)
	if output != "" {
		fmt.Print(output)
	}

	return nil
}
