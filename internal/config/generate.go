package config

import (
	"fmt"
	"io/fs"
	"reflect"
	"slices"

	"github.com/wimpysworld/tailor"
	"github.com/wimpysworld/tailor/internal/model"
)

const embeddedConfigPath = "swatches/.tailor.yml"

// DefaultConfig returns the embedded default configuration with the given
// license. It parses swatches/.tailor.yml from the embedded filesystem,
// validates its contents, and overrides the license field.
func DefaultConfig(license string) (*Config, error) {
	data, err := fs.ReadFile(tailor.SwatchFS, embeddedConfigPath)
	if err != nil {
		return nil, fmt.Errorf("reading embedded config: %w", err)
	}

	cfg, err := parseAndValidate(data, "embedded config")
	if err != nil {
		return nil, err
	}

	cfg.License = license
	if cfg.License == "" {
		return nil, fmt.Errorf("license must not be empty")
	}

	// Nil out the project-specific description and homepage defaults.
	// MergeRepoSettings replaces them with live GitHub values when available.
	if cfg.Repository != nil {
		cfg.Repository.Description = nil
		cfg.Repository.Homepage = nil
	}

	return cfg, nil
}

// ApplyRepoDefaults fills an absent description and homepage so the generated
// config always carries both keys. The description falls back to name and the
// homepage to url. An empty url leaves the homepage omitted, which covers a
// project without repository context.
func ApplyRepoDefaults(cfg *Config, name, url string) {
	if cfg.Repository == nil {
		cfg.Repository = &model.RepositorySettings{}
	}
	if cfg.Repository.Description == nil && name != "" {
		cfg.Repository.Description = &name
	}
	if cfg.Repository.Homepage == nil && url != "" {
		cfg.Repository.Homepage = &url
	}
}

// MergeCodeScanningSetup copies the live code scanning state, query suite,
// and threat model into cfg. Languages are always written as an empty list so
// GitHub detects them.
func MergeCodeScanningSetup(cfg *Config, live *model.CodeScanningSettings) {
	if cfg.CodeScanning == nil {
		cfg.CodeScanning = &model.CodeScanningSettings{}
	}
	if live.State != nil {
		cfg.CodeScanning.State = live.State
	}
	if live.QuerySuite != nil {
		cfg.CodeScanning.QuerySuite = live.QuerySuite
	}
	if live.ThreatModel != nil {
		cfg.CodeScanning.ThreatModel = live.ThreatModel
	}
	cfg.CodeScanning.Languages = &[]string{}
}

// MergeCodeQualitySetup copies the live Code Quality state into cfg.
// Languages are always written as an empty list so GitHub detects them.
func MergeCodeQualitySetup(cfg *Config, live *model.CodeQualitySettings) {
	if cfg.CodeQuality == nil {
		cfg.CodeQuality = &model.CodeQualitySettings{}
	}
	if live.State != nil {
		cfg.CodeQuality.State = live.State
	}
	cfg.CodeQuality.Languages = &[]string{}
}

// MergeRulesetSetup copies every set field of the live ruleset over cfg,
// creating the section when it is absent. Fields that live leaves nil keep
// their existing value, so the built-in parameters of a rule that GitHub
// does not carry stay in the config. An enforcement level that
// ValidateRuleset rejects, such as evaluate, is not copied, so the
// existing level stands. It reports whether it left the live enforcement
// out.
func MergeRulesetSetup(cfg *Config, live *model.RulesetSettings) bool {
	if cfg.Ruleset == nil {
		cfg.Ruleset = &model.RulesetSettings{}
	}
	source := *live
	skipped := source.Enforcement != nil && !slices.Contains(model.RulesetEnforcements, *source.Enforcement)
	if skipped {
		source.Enforcement = nil
	}
	fillNilFields(reflect.ValueOf(cfg.Ruleset).Elem(), reflect.ValueOf(&source).Elem(), true)
	return skipped
}

// MergeRepoSettings assigns live to cfg.Repository and mutates live in place.
// The description flag, when non-empty, overrides whatever the live settings
// carried. Empty string pointer fields for Description and Homepage are
// normalised to nil so they are omitted from YAML.
func MergeRepoSettings(cfg *Config, live *model.RepositorySettings, description string) {
	cfg.Repository = live

	if description != "" {
		cfg.Repository.Description = &description
	}

	if cfg.Repository.Description != nil && *cfg.Repository.Description == "" {
		cfg.Repository.Description = nil
	}
	if cfg.Repository.Homepage != nil && *cfg.Repository.Homepage == "" {
		cfg.Repository.Homepage = nil
	}
}
