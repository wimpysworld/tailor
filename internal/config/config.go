// Package config loads, validates, merges and writes project-local .tailor.yml files.
package config

import (
	"github.com/wimpysworld/tailor/internal/model"
	"github.com/wimpysworld/tailor/internal/swatch"
)

// Config holds the contents of .tailor.yml and tracks inferred repository metadata.
type Config struct {
	ImmutableReleases *model.ImmutableReleasesSettings `yaml:"immutable_releases,omitempty"`
	License           string                           `yaml:"license"`
	Languages         *LanguageSettings                `yaml:"languages,omitempty"`
	MCP               *MCPSettings                     `yaml:"mcp,omitempty"`
	Repository        *model.RepositorySettings        `yaml:"repository,omitempty"`
	Actions           *model.ActionsSettings           `yaml:"actions,omitempty"`
	CodeScanning      *model.CodeScanningSettings      `yaml:"code_scanning,omitempty"`
	CodeQuality       *model.CodeQualitySettings       `yaml:"code_quality,omitempty"`
	Ruleset           *model.RulesetSettings           `yaml:"ruleset,omitempty"`
	Pages             *model.PagesSettings             `yaml:"pages,omitempty"`
	Labels            []model.LabelEntry               `yaml:"labels,omitempty"`
	Variables         []model.VariableEntry            `yaml:"variables,omitempty"`
	Swatches          []SwatchEntry                    `yaml:"swatches"`
	// InferredHomepage records the automatic value so an edited homepage becomes explicit.
	InferredHomepage string `yaml:"-"`

	// Extra captures any YAML keys not mapped to fields above.
	// validate rejects these unrecognised top-level settings.
	Extra map[string]any `yaml:",inline"`
}

// HomepageDeclared reports whether the homepage is an explicit user value.
func (cfg *Config) HomepageDeclared() bool {
	return cfg.Repository != nil && cfg.Repository.Homepage != nil &&
		(cfg.InferredHomepage == "" || *cfg.Repository.Homepage != cfg.InferredHomepage)
}

// SwatchEntry describes a single swatch entry in the config file.
type SwatchEntry struct {
	Path       string                `yaml:"path"`
	Alteration swatch.AlterationMode `yaml:"alteration"`
}
