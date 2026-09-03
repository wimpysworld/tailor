package alter

import (
	"fmt"
	"io"
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

// Run executes the alter command. It validates the config, verifies the
// token against the API before any local file change, applies repository
// settings, fetches the licence, and processes swatches.
// When client is nil, a default GitHub REST client is created.
func Run(cfg *config.Config, dir string, mode ApplyMode, client *api.RESTClient, stdout, stderr io.Writer) error {
	if stdout == nil {
		stdout = io.Discard
	}
	if stderr == nil {
		stderr = io.Discard
	}

	configChanged := config.RemoveRetiredWorkflowEntries(cfg)
	securityNormalised := config.NormaliseSecurityPrerequisites(cfg)
	if securityNormalised {
		fmt.Fprintln(stderr, "warning: set vulnerability_alerts_enabled to true because automated_security_fixes_enabled requires vulnerability alerts")
	}
	secretScanningNormalised := config.NormaliseSecretScanningPrerequisites(cfg)
	if secretScanningNormalised {
		fmt.Fprintln(stderr, "warning: set secret_scanning to enabled because secret_scanning_push_protection requires secret scanning")
	}
	configChanged = configChanged || securityNormalised || secretScanningNormalised
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

	repo, hasRepo, err := gh.RepoContextAt(dir)
	if err != nil {
		return err
	}

	if client == nil {
		client, err = gh.NewRESTClient(gh.ResolveHost(repo.Host))
		if err != nil {
			return fmt.Errorf("creating GitHub API client: %w", err)
		}
	}

	// Verify the token against the API before any local file change. The
	// same request resolves {{GITHUB_USERNAME}} for token substitution.
	username, err := gh.FetchUsername(client)
	if err != nil {
		return fmt.Errorf("verifying GitHub authentication: %w", err)
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

	tokens := TokenContext{
		GitHubUsername: username,
		Owner:          repo.Owner,
		Name:           repo.Name,
	}
	target := RepoTarget{Client: client, Owner: repo.Owner, Name: repo.Name, HasRepo: hasRepo, Stderr: stderr}

	repoResults, err := processRepoStages(cfg, mode, target)
	if err != nil {
		return err
	}

	labelResults, err := ProcessLabels(cfg, mode, target)
	if err != nil {
		return err
	}

	licenceResult, err := ProcessLicence(cfg, dir, mode, client, stderr)
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
		fmt.Fprint(stdout, output)
	}

	return nil
}

// processRepoStages runs the repository API stages in order: repository
// settings, Actions policy, code scanning, then Code Quality.
func processRepoStages(cfg *config.Config, mode ApplyMode, target RepoTarget) ([]RepoSettingResult, error) {
	var results []RepoSettingResult
	for _, stage := range []func(*config.Config, ApplyMode, RepoTarget) ([]RepoSettingResult, error){
		ProcessRepoSettings,
		ProcessActions,
		ProcessCodeScanning,
		ProcessCodeQuality,
	} {
		stageResults, err := stage(cfg, mode, target)
		if err != nil {
			return nil, err
		}
		results = append(results, stageResults...)
	}
	return results, nil
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
	if err := config.ValidateActions(cfg); err != nil {
		return err
	}
	if err := config.ValidateCodeScanning(cfg); err != nil {
		return err
	}
	return config.ValidateCodeQuality(cfg)
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
