package config

import (
	"fmt"
	"slices"

	"github.com/wimpysworld/tailor/internal/model"
	"github.com/wimpysworld/tailor/internal/swatch"
)

// DefaultConfig returns the embedded default configuration with the given
// license. It parses swatches/.tailor.yml from the embedded filesystem,
// validates its contents, and overrides the license field.
func DefaultConfig(license string) (*Config, error) {
	data, err := swatch.Content(".tailor.yml")
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
	// MergeRepoMetadata replaces them with live GitHub values when available.
	if cfg.Repository != nil {
		cfg.Repository.Description = nil
		cfg.Repository.Homepage = nil
	}
	cfg.InferredHomepage = ""
	cfg.Swatches = slices.DeleteFunc(cfg.Swatches, func(entry SwatchEntry) bool {
		return !cfg.SwatchActive(entry.Path)
	})

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
		cfg.InferredHomepage = url
	}
}

// MergeRepoMetadata imports project metadata without changing managed defaults.
// An explicit homepage declaration takes precedence over the imported value.
func MergeRepoMetadata(cfg *Config, live *model.RepositorySettings, description *string) {
	if cfg.Repository == nil {
		cfg.Repository = &model.RepositorySettings{}
	}
	declaredHomepage := cfg.HomepageDeclared()
	cfg.Repository.Description = live.Description
	if description != nil {
		cfg.Repository.Description = description
	}
	if !declaredHomepage {
		cfg.Repository.Homepage = live.Homepage
		cfg.InferredHomepage = ""
		if live.Homepage != nil {
			cfg.InferredHomepage = *live.Homepage
		}
	}
}
