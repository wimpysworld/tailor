package alter

import (
	"fmt"
	"time"

	"github.com/cli/go-gh/v2/pkg/api"
	"github.com/wimpysworld/tailor/internal/config"
	"github.com/wimpysworld/tailor/internal/gh"
	"github.com/wimpysworld/tailor/internal/swatch"
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
	if err := validateConfig(cfg); err != nil {
		return err
	}
	if err := config.ValidateRepoSettings(cfg); err != nil {
		return err
	}

	// Merge missing default swatch entries into the config when the
	// config.yml swatch is set to always, or when it is first-fit and
	// the caller requested a recut.
	if shouldMerge(cfg, mode) {
		added := config.MergeDefaultSwatches(cfg)
		if len(added) > 0 && mode.ShouldWrite() {
			todayDate := time.Now().Format("2006-01-02")
			if err := config.Write(dir, cfg, todayDate, "Refitted"); err != nil {
				return fmt.Errorf("writing refitted config: %w", err)
			}
		}
		// Re-validate after merge as a safety check.
		if err := validateConfig(cfg); err != nil {
			return err
		}
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

	// Labels processing.
	labelResults, err := ProcessLabels(cfg, mode, client, owner, name, hasRepo)
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

	output := FormatOutput(repoResults, labelResults, swatchResults)
	if output != "" {
		fmt.Print(output)
	}

	return nil
}

// validateConfig runs source and duplicate-destination validation in sequence.
func validateConfig(cfg *config.Config) error {
	if err := config.ValidateSources(cfg); err != nil {
		return err
	}
	return config.ValidateDuplicateDestinations(cfg)
}

// shouldMerge reports whether the config merge step should run. It looks up
// the config.yml swatch entry and returns true when the alteration mode is
// always, or when it is first-fit and the caller requested a recut.
func shouldMerge(cfg *config.Config, mode ApplyMode) bool {
	for _, e := range cfg.Swatches {
		if e.Source == configSource {
			if e.Alteration == swatch.Always {
				return true
			}
			if e.Alteration == swatch.FirstFit && mode == Recut {
				return true
			}
			return false
		}
	}
	return false
}
