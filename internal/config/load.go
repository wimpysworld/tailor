package config

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"

	"github.com/wimpysworld/tailor/internal/fsutil"
	"github.com/wimpysworld/tailor/internal/swatch"
)

const configPath = ".tailor/config.yml"

// Exists reports whether .tailor/config.yml is present in dir.
func Exists(dir string) bool {
	return fsutil.FileExists(filepath.Join(dir, configPath))
}

// Load reads and parses .tailor/config.yml from dir, returning
// the validated Config or an error.
func Load(dir string) (*Config, error) {
	path := filepath.Join(dir, configPath)
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading config: %w", err)
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parsing config: %w", err)
	}

	if err := validate(&cfg); err != nil {
		return nil, err
	}

	return &cfg, nil
}

// validate checks the parsed config for structural correctness.
func validate(cfg *Config) error {
	for i, s := range cfg.Swatches {
		if s.Source == "" {
			return fmt.Errorf("swatch[%d]: source must not be empty", i)
		}
		if s.Destination == "" {
			return fmt.Errorf("swatch[%d]: destination must not be empty", i)
		}
		if s.Alteration != swatch.Always && s.Alteration != swatch.FirstFit {
			return fmt.Errorf("swatch[%d]: alteration must be %q or %q, got %q", i, swatch.Always, swatch.FirstFit, s.Alteration)
		}
	}
	return nil
}
