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

	wikiDeclared := cfg.Repository != nil && cfg.Repository.HasWiki != nil
	configChanged, err := prepareAlterConfig(cfg, mode, stderr)
	if err != nil {
		return err
	}
	prepared, err := preparePagesSource(cfg, dir, mode)
	if err != nil {
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
	target := RepoTarget{Client: client, Owner: repo.Owner, Name: repo.Name, HasRepo: hasRepo, Stderr: stderr}
	pages, err := preflightPages(cfg, dir, mode, target, prepared)
	if err != nil {
		return err
	}
	wiki, err := preflightWiki(cfg, dir, mode, target, wikiDeclared)
	if err != nil {
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

	tokens := TokenContext{
		GitHubUsername: username,
		Owner:          repo.Owner,
		Name:           repo.Name,
	}

	repoResults, err := processRepoStages(cfg, mode, target)
	if err != nil {
		return err
	}

	labelResults, err := ProcessLabels(cfg, mode, target)
	if err != nil {
		return err
	}

	variableResults, err := ProcessVariables(cfg, mode, target)
	if err != nil {
		fmt.Fprint(stdout, FormatOutput(repoResults, labelResults, variableResults, retiredResults, mode))
		return err
	}
	pagesResults, pagesWorkflow, err := processPages(cfg, dir, mode, target, pages)
	repoResults = append(repoResults, pagesResults...)
	if pagesWorkflow != nil {
		retiredResults = append(retiredResults, *pagesWorkflow)
	}
	if err != nil {
		fmt.Fprint(stdout, FormatOutput(repoResults, labelResults, variableResults, retiredResults, mode))
		return err
	}

	wikiResults, wikiSwatches, err := processWiki(cfg, dir, mode, wiki)
	repoResults = append(repoResults, wikiResults...)
	retiredResults = append(retiredResults, wikiSwatches...)
	if err != nil {
		fmt.Fprint(stdout, FormatOutput(repoResults, labelResults, variableResults, retiredResults, mode))
		return err
	}

	licenceResult, err := ProcessLicence(cfg, dir, mode, client, stderr)
	if err != nil {
		fmt.Fprint(stdout, FormatOutput(repoResults, labelResults, variableResults, retiredResults, mode))
		return err
	}

	swatchResults, err := ProcessSwatches(cfg, dir, mode, &tokens)
	if err != nil {
		fmt.Fprint(stdout, FormatOutput(repoResults, labelResults, variableResults, retiredResults, mode))
		return err
	}
	if pages != nil && len(pages.skipped) == 0 {
		files, err := processPagesFiles(cfg, dir, mode, prepared)
		swatchResults = append(swatchResults, files...)
		if err != nil {
			fmt.Fprint(stdout, FormatOutput(repoResults, labelResults, variableResults, append(swatchResults, retiredResults...), mode))
			return err
		}
	}

	// Merge licence result into swatch results for unified output.
	if licenceResult != nil {
		swatchResults = append([]SwatchResult{*licenceResult}, swatchResults...)
	}
	if configChanged {
		swatchResults = append(swatchResults, SwatchResult{Path: configPath, Category: WouldUpdateConfig})
	}
	swatchResults = append(swatchResults, retiredResults...)

	fmt.Fprint(stdout, FormatOutput(repoResults, labelResults, variableResults, swatchResults, mode))

	return nil
}

func prepareAlterConfig(cfg *config.Config, mode ApplyMode, stderr io.Writer) (bool, error) {
	changed := config.RemoveRetiredWorkflowEntries(cfg)
	securityNormalised := config.NormaliseSecurityPrerequisites(cfg)
	if securityNormalised {
		fmt.Fprintln(stderr, "warning: set vulnerability_alerts_enabled to true because automated_security_fixes_enabled requires vulnerability alerts")
	}
	secretScanningWarnings := config.NormaliseSecretScanningPrerequisites(cfg)
	for _, warning := range secretScanningWarnings {
		fmt.Fprintln(stderr, warning)
	}
	changed = changed || securityNormalised || len(secretScanningWarnings) > 0
	if err := validateConfig(cfg); err != nil {
		return false, err
	}
	if shouldMerge(cfg, mode) {
		defaultsChanged, err := config.MergeDefaults(cfg)
		if err != nil {
			return false, err
		}
		changed = changed || defaultsChanged
		if err := validateConfig(cfg); err != nil {
			return false, err
		}
	}
	if err := config.ValidateCompleteActions(cfg); err != nil {
		return false, err
	}
	if err := config.ValidateCompleteRuleset(cfg); err != nil {
		return false, err
	}
	for _, warning := range config.RulesetMergeMethodWarnings(cfg) {
		fmt.Fprintln(stderr, warning)
	}
	return changed, nil
}

// processRepoStages runs the repository API stages in order: repository
// settings, immutable releases, Actions policy, code scanning, Code Quality, then the ruleset.
func processRepoStages(cfg *config.Config, mode ApplyMode, target RepoTarget) ([]RepoSettingResult, error) {
	var results []RepoSettingResult
	for _, stage := range []func(*config.Config, ApplyMode, RepoTarget) ([]RepoSettingResult, error){
		ProcessRepoSettings,
		ProcessImmutableReleases,
		ProcessActions,
		ProcessCodeScanning,
		ProcessCodeQuality,
		ProcessRuleset,
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
	if err := config.ValidatePages(cfg); err != nil {
		return err
	}
	if err := config.ValidateVariables(cfg); err != nil {
		return err
	}
	if err := config.ValidateImmutableReleases(cfg); err != nil {
		return err
	}
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
	if err := config.ValidateCodeQuality(cfg); err != nil {
		return err
	}
	return config.ValidateRuleset(cfg)
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
