package config

import (
	"fmt"
	"io/fs"

	"github.com/wimpysworld/tailor"
)

const embeddedConfigPath = "swatches/.tailor/config.yml"

// DefaultConfig returns the embedded default configuration with the given
// license. It parses swatches/.tailor/config.yml from the embedded filesystem,
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
	return cfg, nil
}

// MergeRepoSettings replaces cfg.Repository with the live settings retrieved
// from the GitHub API. The description flag, when non-empty, overrides
// whatever the live settings carried. Empty string pointer fields for
// Description and Homepage are normalised to nil so they are omitted from YAML.
func MergeRepoSettings(cfg *Config, live *RepositorySettings, description string) {
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
