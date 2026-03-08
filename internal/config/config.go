package config

import (
	"github.com/wimpysworld/tailor/internal/model"
	"github.com/wimpysworld/tailor/internal/swatch"
)

// Config represents the contents of .tailor.yml.
type Config struct {
	License    string                    `yaml:"license"`
	Repository *model.RepositorySettings `yaml:"repository,omitempty"`
	Labels     []model.LabelEntry        `yaml:"labels,omitempty"`
	Swatches   []SwatchEntry             `yaml:"swatches"`
}

// SwatchEntry describes a single swatch entry in the config file.
type SwatchEntry struct {
	Path       string                `yaml:"path"`
	Alteration swatch.AlterationMode `yaml:"alteration"`
}
