package config

import (
	"github.com/wimpysworld/tailor/internal/model"
	"github.com/wimpysworld/tailor/internal/swatch"
)

// Config represents the contents of .tailor.yml.
type Config struct {
	ImmutableReleases *model.ImmutableReleasesSettings `yaml:"immutable_releases,omitempty"`
	License           string                           `yaml:"license"`
	Repository        *model.RepositorySettings        `yaml:"repository,omitempty"`
	Actions           *model.ActionsSettings           `yaml:"actions,omitempty"`
	CodeScanning      *model.CodeScanningSettings      `yaml:"code_scanning,omitempty"`
	CodeQuality       *model.CodeQualitySettings       `yaml:"code_quality,omitempty"`
	Ruleset           *model.RulesetSettings           `yaml:"ruleset,omitempty"`
	Labels            []model.LabelEntry               `yaml:"labels,omitempty"`
	Variables         []model.VariableEntry            `yaml:"variables,omitempty"`
	Swatches          []SwatchEntry                    `yaml:"swatches"`

	// Extra captures any YAML keys not mapped to fields above.
	// validate rejects these unrecognised top-level settings.
	Extra map[string]any `yaml:",inline"`
}

// SwatchEntry describes a single swatch entry in the config file.
type SwatchEntry struct {
	Path       string                `yaml:"path"`
	Alteration swatch.AlterationMode `yaml:"alteration"`
}
