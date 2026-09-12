package config

import (
	"fmt"
	"slices"

	"github.com/wimpysworld/tailor/internal/model"
	"github.com/wimpysworld/tailor/internal/swatch"
)

// DefaultConfig validates the embedded configuration and applies the requested licence.
// It clears project metadata and omits language swatches that are not enabled.
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

	// The description and homepage must not come from the embedded defaults.
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

// ApplyRepoDefaults fills an absent description from name and an absent homepage from url.
// Empty fallback values leave the corresponding fields omitted.
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
