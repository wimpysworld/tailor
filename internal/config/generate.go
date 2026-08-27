package config

import (
	"fmt"
	"io/fs"

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
