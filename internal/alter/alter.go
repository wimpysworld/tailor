package alter

import (
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/cli/go-gh/v2/pkg/api"
	"github.com/wimpysworld/tailor/internal/config"
	"github.com/wimpysworld/tailor/internal/gh"
	"github.com/wimpysworld/tailor/internal/output"
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

func stageLabel(mode ApplyMode, dryRun, mutation string) string {
	if mode.ShouldWrite() {
		return mutation
	}
	return dryRun
}

// Options configures typed execution events.
type Options struct{ Observer func(output.StageEvent) }

func (o Options) stage(id, label, phase string) {
	if o.Observer != nil {
		o.Observer(output.StageEvent{ID: id, Label: label, Phase: phase})
	}
}

func (o Options) stageError(err error) {
	if o.Observer != nil {
		o.Observer(output.StageEvent{ID: "execution", Label: "Tailor failed", Phase: "error", Err: err})
	}
}

// Execute retains typed results, including partial results when a later stage fails.
// The sequence is deliberately linear because operation order is part of the safety contract.
//
//nolint:gocyclo
func Execute(cfg *config.Config, dir string, mode ApplyMode, client *api.RESTClient, stderr io.Writer, options Options) (Report, error) {
	if stderr == nil {
		stderr = io.Discard
	}
	command := "alter"
	if mode == DryRun {
		command = "baste"
	}
	var repoResults []RepoSettingResult
	var labelResults []LabelResult
	var variableResults []VariableResult
	var retiredResults []SwatchResult
	var swatchResults []SwatchResult
	context := ""
	wikiGuidance := ""
	partial := func(err error) (Report, error) {
		allSwatches := append(append([]SwatchResult{}, swatchResults...), retiredResults...)
		report := buildReport(command, context, repoResults, labelResults, variableResults, allSwatches, mode)
		appendGuidance(&report, wikiGuidance)
		options.stageError(err)
		return report, err
	}
	options.stage("config", stageLabel(mode, "Checking configuration", "Preparing configuration"), "start")
	wikiDeclared := cfg.Repository != nil && cfg.Repository.HasWiki != nil
	configChanged, err := prepareAlterConfig(cfg, mode, stderr)
	if err != nil {
		return partial(err)
	}
	prepared, err := preparePagesSource(cfg, dir, mode)
	if err != nil {
		return partial(err)
	}
	repo, hasRepo, err := gh.RepoContextAt(dir)
	if err != nil {
		return partial(err)
	}
	if repo.Owner != "" && repo.Name != "" {
		context = repo.Owner + "/" + repo.Name
	}
	options.stage("config", stageLabel(mode, "Configuration checked", "Configuration prepared"), "complete")
	options.stage("auth", "Verifying GitHub authentication", "start")
	if client == nil {
		client, err = gh.NewRESTClient(gh.ResolveHost(repo.Host))
		if err != nil {
			return partial(fmt.Errorf("creating GitHub API client: %w", err))
		}
	}
	username, err := gh.FetchUsername(client)
	if err != nil {
		return partial(fmt.Errorf("verifying GitHub authentication: %w", err))
	}
	options.stage("auth", "GitHub authentication verified", "complete")
	target := RepoTarget{Client: client, Host: repo.Host, Owner: repo.Owner, Name: repo.Name, HasRepo: hasRepo, Stderr: stderr}
	options.stage("pages-preflight", stageLabel(mode, "Checking Pages readiness", "Updating Pages readiness"), "start")
	pages, err := preflightPages(cfg, dir, mode, target, prepared)
	if err != nil {
		return partial(err)
	}
	options.stage("pages-preflight", stageLabel(mode, "Pages readiness checked", "Pages readiness updated"), "complete")
	options.stage("wiki-preflight", stageLabel(mode, "Checking wiki readiness", "Updating wiki readiness"), "start")
	if err := preflightWikiWrites(cfg, dir, wikiDeclared, &TokenContext{GitHubUsername: username, Owner: repo.Owner, Name: repo.Name}); err != nil {
		return partial(err)
	}
	wiki, err := preflightWiki(cfg, dir, mode, target, wikiDeclared)
	if err != nil {
		return partial(err)
	}
	options.stage("wiki-preflight", stageLabel(mode, "Wiki readiness checked", "Wiki readiness updated"), "complete")
	if wiki != nil {
		wikiGuidance = wiki.nextSteps
	}
	target.wikiEnabled = wiki.didEnable()
	if configChanged && mode.ShouldWrite() {
		options.stage("config-write", "Writing configuration", "start")
		todayDate := time.Now().Format("2006-01-02")
		if err := config.Write(dir, cfg, todayDate, "Refitted"); err != nil {
			return partial(fmt.Errorf("writing refitted config: %w", err))
		}
		swatchResults = append(swatchResults, SwatchResult{Path: configPath, Category: WouldUpdateConfig})
		options.stage("config-write", "Configuration written", "complete")
	}
	options.stage("retired-workflows", stageLabel(mode, "Checking retired workflows", "Updating retired workflows"), "start")
	retiredResults, err = ProcessRetiredWorkflows(dir, mode)
	if err != nil {
		return partial(err)
	}
	options.stage("retired-workflows", stageLabel(mode, "Retired workflows checked", "Retired workflows updated"), "complete")
	tokens := TokenContext{GitHubUsername: username, Owner: repo.Owner, Name: repo.Name}
	options.stage("repository", stageLabel(mode, "Reading GitHub settings", "Applying GitHub settings"), "start")
	repoResults, err = processRepoStages(cfg, mode, target)
	if err != nil {
		return partial(err)
	}
	options.stage("repository", stageLabel(mode, "GitHub settings read", "GitHub settings applied"), "complete")
	options.stage("labels", stageLabel(mode, "Reading labels", "Applying labels"), "start")
	labelResults, err = ProcessLabels(cfg, mode, target)
	if err != nil {
		return partial(err)
	}
	options.stage("labels", stageLabel(mode, "Labels read", "Labels applied"), "complete")
	options.stage("variables", stageLabel(mode, "Reading variables", "Applying variables"), "start")
	variableResults, err = ProcessVariables(cfg, mode, target)
	if err != nil {
		return partial(err)
	}
	options.stage("variables", stageLabel(mode, "Variables read", "Variables applied"), "complete")
	options.stage("pages", stageLabel(mode, "Planning Pages changes", "Applying Pages changes"), "start")
	pagesResults, pagesWorkflow, err := processPages(cfg, dir, mode, target, pages)
	repoResults = append(repoResults, pagesResults...)
	if pagesWorkflow != nil {
		retiredResults = append(retiredResults, *pagesWorkflow)
	}
	if err != nil {
		return partial(err)
	}
	options.stage("pages", stageLabel(mode, "Pages changes planned", "Pages changes applied"), "complete")
	options.stage("wiki", stageLabel(mode, "Planning wiki changes", "Applying wiki changes"), "start")
	wikiResults, wikiSwatches, err := processWiki(cfg, dir, mode, wiki)
	repoResults = append(repoResults, wikiResults...)
	retiredResults = append(retiredResults, wikiSwatches...)
	if err != nil {
		return partial(err)
	}
	options.stage("wiki", stageLabel(mode, "Wiki changes planned", "Wiki changes applied"), "complete")
	options.stage("licence", stageLabel(mode, "Planning licence", "Writing licence"), "start")
	licenceResult, err := ProcessLicence(cfg, dir, mode, client, stderr)
	if err != nil {
		return partial(err)
	}
	if licenceResult != nil {
		swatchResults = append([]SwatchResult{*licenceResult}, swatchResults...)
	}
	options.stage("licence", stageLabel(mode, "Licence planned", "Licence written"), "complete")
	options.stage("swatches", stageLabel(mode, "Planning swatches", "Writing swatches"), "start")
	processedSwatches, err := ProcessSwatches(cfg, dir, mode, &tokens)
	swatchResults = append(swatchResults, processedSwatches...)
	if err != nil {
		return partial(err)
	}
	options.stage("swatches", stageLabel(mode, "Swatches planned", "Swatches written"), "complete")
	if pages != nil && len(pages.skipped) == 0 {
		options.stage("pages-files", stageLabel(mode, "Planning Pages files", "Writing Pages files"), "start")
		files, filesErr := processPagesFiles(cfg, dir, mode, prepared)
		swatchResults = append(swatchResults, files...)
		if filesErr != nil {
			return partial(filesErr)
		}
		options.stage("pages-files", stageLabel(mode, "Pages files planned", "Pages files written"), "complete")
	}
	if configChanged && !mode.ShouldWrite() {
		swatchResults = append(swatchResults, SwatchResult{Path: configPath, Category: WouldUpdateConfig})
	}
	swatchResults = append(swatchResults, retiredResults...)
	report := buildReport(command, context, repoResults, labelResults, variableResults, swatchResults, mode)
	if wiki != nil {
		appendGuidance(&report, wiki.nextSteps)
	}
	options.stage("complete", "Complete", "complete")
	return report, nil
}

// Run preserves the exact legacy output contract.
func Run(cfg *config.Config, dir string, mode ApplyMode, client *api.RESTClient, stdout, stderr io.Writer) error {
	if stdout == nil {
		stdout = io.Discard
	}
	report, err := Execute(cfg, dir, mode, client, stderr, Options{})
	fmt.Fprint(stdout, report.Plain)
	return err
}

func preflightWikiWrites(cfg *config.Config, dir string, declared bool, tokens *TokenContext) error {
	if !declared || !*cfg.Repository.HasWiki {
		return nil
	}
	if _, err := ProcessRetiredWorkflows(dir, DryRun); err != nil {
		return err
	}
	_, err := ProcessSwatches(cfg, dir, DryRun, tokens)
	return err
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
		results = append(results, stageResults...)
		if err != nil {
			return results, err
		}
	}
	return results, nil
}

func appendGuidance(report *Report, guidance string) {
	if guidance == "" {
		return
	}
	report.Plain += guidance
	for i, line := range strings.Split(strings.TrimSuffix(guidance, "\n"), "\n") {
		report.Document.Guidance = append(report.Document.Guidance, output.Guidance{Order: i + 1, Text: line})
	}
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
