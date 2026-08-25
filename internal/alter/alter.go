package alter

import (
	"fmt"
	"os"
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
	configChanged := config.RemoveRetiredWorkflowEntries(cfg)
	securityNormalised := config.NormaliseSecurityPrerequisites(cfg)
	if securityNormalised {
		fmt.Fprintln(os.Stderr, "warning: set vulnerability_alerts_enabled to true because automated_security_fixes_enabled requires vulnerability alerts")
	}
	configChanged = configChanged || securityNormalised
	if err := validateConfig(cfg); err != nil {
		return err
	}

	// Keep the local config aligned with built-in defaults only when the
	// config swatch mode allows tailor to rewrite it.
	if shouldMerge(cfg, mode) {
		defaultsChanged, err := config.MergeDefaults(cfg)
		if err != nil {
			return err
		}
		configChanged = configChanged || defaultsChanged
		// Re-validate after merge as a safety check.
		if err := validateConfig(cfg); err != nil {
			return err
		}
	}
	if err := config.ValidateCompleteActions(cfg); err != nil {
		return err
	}
	if configChanged && mode.ShouldWrite() {
		todayDate := time.Now().Format("2006-01-02")
		if err := config.Write(dir, cfg, todayDate, "Refitted"); err != nil {
			return fmt.Errorf("writing refitted config: %w", err)
		}
	}

	retiredResults, err := ProcessRetiredWorkflows(dir, mode)
	if err != nil {
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

	owner, name, hasRepo, err := gh.RepoContextAt(dir)
	if err != nil {
		return err
	}
	tokens := TokenContext{
		GitHubUsername: username,
		Owner:          owner,
		Name:           name,
	}
	target := RepoTarget{Client: client, Owner: owner, Name: name, HasRepo: hasRepo}

	repoResults, err := ProcessRepoSettings(cfg, mode, target)
	if err != nil {
		return err
	}
	actionsResults, err := ProcessActions(cfg, mode, target)
	if err != nil {
		return err
	}
	repoResults = append(repoResults, actionsResults...)

	labelResults, err := ProcessLabels(cfg, mode, target)
	if err != nil {
		return err
	}

	licenceResult, err := ProcessLicence(cfg, dir, mode, client)
	if err != nil {
		return err
	}

	swatchResults, err := ProcessSwatches(cfg, dir, mode, &tokens)
	if err != nil {
		return err
	}

	// Merge licence result into swatch results for unified output.
	if licenceResult != nil {
		swatchResults = append([]SwatchResult{*licenceResult}, swatchResults...)
	}
	if configChanged {
		swatchResults = append(swatchResults, SwatchResult{Path: configPath, Category: WouldUpdateConfig})
	}
	swatchResults = append(swatchResults, retiredResults...)

	output := FormatOutput(repoResults, labelResults, swatchResults, mode)
	if output != "" {
		fmt.Print(output)
	}

	return nil
}

// validateConfig runs the repeated config validation pass in sequence.
func validateConfig(cfg *config.Config) error {
	if err := config.ValidateSwatches(cfg); err != nil {
		return err
	}
	if err := config.ValidatePaths(cfg); err != nil {
		return err
	}
	if err := config.ValidateDuplicatePaths(cfg); err != nil {
		return err
	}
	if err := config.ValidateRepoSettings(cfg); err != nil {
		return err
	}
	return config.ValidateActions(cfg)
}

// shouldMerge reports whether the config merge step should run. It looks up
// the config swatch entry and returns true when the alteration mode is always,
// or when it is first-fit and the caller requested a recut.
func shouldMerge(cfg *config.Config, mode ApplyMode) bool {
	for _, e := range cfg.Swatches {
		if e.Path == configPath {
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
