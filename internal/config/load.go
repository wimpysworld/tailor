package config

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"

	"gopkg.in/yaml.v3"

	"github.com/wimpysworld/tailor/internal/swatch"
)

const (
	configPath    = ".tailor.yml"
	maxConfigSize = 1 << 20
)

// Exists reports whether a .tailor.yml path is present in dir.
func Exists(dir string) (bool, error) {
	root, err := os.OpenRoot(dir)
	if err != nil {
		return false, fmt.Errorf("checking config: opening project root %q: %w", dir, err)
	}
	defer root.Close()

	_, err = root.Lstat(configPath)
	switch {
	case err == nil:
		return true, nil
	case errors.Is(err, fs.ErrNotExist):
		return false, nil
	default:
		return false, fmt.Errorf("checking config: %w", err)
	}
}

// Load reads and parses .tailor.yml from dir, returning the validated Config
// or an error.
func Load(dir string) (*Config, error) {
	root, err := os.OpenRoot(dir)
	if err != nil {
		return nil, fmt.Errorf("reading config: opening project root %q: %w", dir, err)
	}
	defer root.Close()

	info, err := root.Lstat(configPath)
	if err != nil {
		return nil, fmt.Errorf("reading config: %w", err)
	}
	if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("reading config: %s is not a regular file", configPath)
	}

	file, err := root.Open(configPath)
	if err != nil {
		return nil, fmt.Errorf("reading config: %w", err)
	}
	defer file.Close()

	// Re-check the open handle: the path can change to a non-regular
	// file between Lstat and Open.
	info, err = file.Stat()
	if err != nil {
		return nil, fmt.Errorf("reading config metadata: %w", err)
	}
	if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("reading config: %s is not a regular file", configPath)
	}
	if info.Size() > maxConfigSize {
		return nil, fmt.Errorf("reading config: %s exceeds maximum size of 1 MiB", configPath)
	}

	data, err := io.ReadAll(io.LimitReader(file, maxConfigSize+1))
	if err != nil {
		return nil, fmt.Errorf("reading config: %w", err)
	}
	if len(data) > maxConfigSize {
		return nil, fmt.Errorf("reading config: %s exceeds maximum size of 1 MiB", configPath)
	}

	return parseAndValidate(data, "config")
}

// parseAndValidate unmarshals YAML data into a Config and validates it.
// The context string is used in error messages to identify the source.
func parseAndValidate(data []byte, context string) (*Config, error) {
	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parsing %s: %w", context, err)
	}

	if err := validate(&cfg); err != nil {
		return nil, err
	}

	return &cfg, nil
}

// validate checks the parsed config for structural correctness.
func validate(cfg *Config) error {
	if err := validateTopLevelSettings(cfg); err != nil {
		return err
	}
	if err := validateSwatches(cfg, true); err != nil {
		return err
	}
	if err := ValidateRepoSettings(cfg); err != nil {
		return err
	}
	if err := ValidateWorkflowPermissions(cfg); err != nil {
		return err
	}
	if err := ValidateRepoStringSettings(cfg); err != nil {
		return err
	}
	if err := ValidateSecretScanning(cfg); err != nil {
		return err
	}
	if err := ValidateActions(cfg); err != nil {
		return err
	}
	if err := ValidateCodeScanning(cfg); err != nil {
		return err
	}
	if err := ValidateCodeQuality(cfg); err != nil {
		return err
	}
	if err := ValidateRuleset(cfg); err != nil {
		return err
	}
	if err := ValidateTopics(cfg); err != nil {
		return err
	}
	if err := ValidateLabels(cfg); err != nil {
		return err
	}
	return nil
}

// ValidateSwatches checks active swatch entries without legacy allowances.
func ValidateSwatches(cfg *Config) error {
	return validateSwatches(cfg, false)
}

func validateSwatches(cfg *Config, allowLegacyRetired bool) error {
	for i, s := range cfg.Swatches {
		if s.Path == "" {
			return fmt.Errorf("swatch[%d]: path must not be empty", i)
		}
		switch s.Alteration {
		case swatch.Always, swatch.FirstFit, swatch.Never:
			// valid
		default:
			if allowLegacyRetired && isLegacyRetiredEntry(s) {
				continue
			}
			return fmt.Errorf("swatch[%d]: alteration must be %q, %q, or %q, got %q",
				i, swatch.Always, swatch.FirstFit, swatch.Never, s.Alteration)
		}
	}
	return nil
}
