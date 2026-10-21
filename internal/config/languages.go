package config

import (
	"fmt"

	"github.com/wimpysworld/tailor/internal/swatch"
	"gopkg.in/yaml.v3"
)

// LanguageSettings controls the language-specific swatches. Nil fields leave the language undeclared.
type LanguageSettings struct {
	Go   *bool `yaml:"-"`
	Rust *bool `yaml:"rust,omitempty"`
	// GoOptions is non-nil for the mapping form, even when all options are absent.
	// Go holds the enabled value in both forms for compatibility with existing callers.
	GoOptions *GoOptions `yaml:"-"`
}

// GoOptions holds optional settings from a languages.go mapping.
// Nil fields preserve omission. Options have no effect when Go is disabled.
type GoOptions struct {
	CGO           *bool `yaml:"cgo,omitempty"`
	FFmpegStatigo *bool `yaml:"ffmpeg-statigo,omitempty"`
}

// UnmarshalYAML accepts the legacy Boolean and the explicit mapping form.
func (languages *LanguageSettings) UnmarshalYAML(node *yaml.Node) error {
	if node.Kind != yaml.MappingNode {
		return fmt.Errorf("languages must be a mapping")
	}
	var fields map[string]yaml.Node
	if err := node.Decode(&fields); err != nil {
		return err
	}
	extra := make(map[string]any)
	for i := 0; i < len(node.Content); i += 2 {
		key := node.Content[i]
		if key.Tag != "!!str" || (key.Value != "go" && key.Value != "rust") {
			extra[key.Value] = nil
		}
	}
	if err := rejectExtra("languages", extra, []string{"go", "rust"}); err != nil {
		return err
	}
	*languages = LanguageSettings{}
	if rustNode, ok := fields["rust"]; ok {
		if rustNode.Kind != yaml.ScalarNode || rustNode.Tag != "!!bool" {
			return fmt.Errorf("languages.rust must be a bool")
		}
		if err := rustNode.Decode(&languages.Rust); err != nil {
			return err
		}
	}
	goNode, ok := fields["go"]
	if !ok {
		return nil
	}
	if goNode.Kind == yaml.ScalarNode && goNode.Tag == "!!bool" {
		return goNode.Decode(&languages.Go)
	}
	if goNode.Kind != yaml.MappingNode {
		return fmt.Errorf("languages.go must be a bool or mapping")
	}
	var options map[string]yaml.Node
	if err := goNode.Decode(&options); err != nil {
		return err
	}
	extra = make(map[string]any)
	for i := 0; i < len(goNode.Content); i += 2 {
		keyNode, value := goNode.Content[i], goNode.Content[i+1]
		key := keyNode.Value
		if keyNode.Tag != "!!str" {
			extra[key] = nil
			continue
		}
		switch key {
		case "enabled", "cgo", "ffmpeg-statigo":
			if value.Kind != yaml.ScalarNode || value.Tag != "!!bool" {
				return fmt.Errorf("languages.go.%s must be a bool", key)
			}
		default:
			extra[key] = nil
		}
	}
	if err := rejectExtra("languages.go", extra, []string{"enabled", "cgo", "ffmpeg-statigo"}); err != nil {
		return err
	}
	if _, ok := options["enabled"]; !ok {
		return fmt.Errorf("languages.go.enabled is required")
	}
	var mapping struct {
		Enabled   *bool `yaml:"enabled"`
		GoOptions `yaml:",inline"`
	}
	if err := goNode.Decode(&mapping); err != nil {
		return err
	}
	if mapping.FFmpegStatigo != nil && *mapping.FFmpegStatigo {
		if !*mapping.Enabled {
			return fmt.Errorf("languages.go.ffmpeg-statigo requires enabled: true")
		}
		if mapping.CGO != nil && !*mapping.CGO {
			return fmt.Errorf("languages.go.ffmpeg-statigo requires cgo: true")
		}
	}
	languages.Go = mapping.Enabled
	languages.GoOptions = &mapping.GoOptions
	return nil
}

// MarshalYAML preserves Boolean and mapping forms, including omitted options.
func (languages LanguageSettings) MarshalYAML() (any, error) {
	if languages.GoOptions != nil {
		if languages.Go == nil {
			return nil, fmt.Errorf("languages.go.enabled is required")
		}
		return struct {
			Go   any   `yaml:"go"`
			Rust *bool `yaml:"rust,omitempty"`
		}{Rust: languages.Rust, Go: struct {
			Enabled    bool `yaml:"enabled"`
			*GoOptions `yaml:",inline"`
		}{Enabled: *languages.Go, GoOptions: languages.GoOptions}}, nil
	}
	return struct {
		Go   *bool `yaml:"go,omitempty"`
		Rust *bool `yaml:"rust,omitempty"`
	}{Go: languages.Go, Rust: languages.Rust}, nil
}

func (cfg *Config) RustDeclared() bool {
	return cfg != nil && cfg.Languages != nil && cfg.Languages.Rust != nil
}

func (cfg *Config) RustEnabled() bool {
	return cfg.RustDeclared() && *cfg.Languages.Rust
}

// GoDeclared reports whether languages.go is explicitly set, including false.
func (cfg *Config) GoDeclared() bool {
	return cfg != nil && cfg.Languages != nil && cfg.Languages.Go != nil
}

// GoEnabled reports whether either form of languages.go explicitly enables Go.
func (cfg *Config) GoEnabled() bool {
	return cfg.GoDeclared() && *cfg.Languages.Go
}

// GoCGODeclared reports whether cgo is explicitly set, including inactive options.
func (cfg *Config) GoCGODeclared() bool {
	return cfg.GoDeclared() && cfg.Languages.GoOptions != nil && cfg.Languages.GoOptions.CGO != nil
}

// EffectiveGoCGO returns nil when Go is disabled or no CGO policy is declared.
// Enabled FFmpeg implies true. Otherwise an explicit cgo value wins.
// The returned pointer is a copy and does not change the authored configuration.
func (cfg *Config) EffectiveGoCGO() *bool {
	if !cfg.GoEnabled() {
		return nil
	}
	if cfg.GoCGODeclared() {
		return new(*cfg.Languages.GoOptions.CGO)
	}
	if cfg.FFmpegStatigoEnabled() {
		return new(true)
	}
	return nil
}

// GoCGOEnabled reports whether the effective CGO policy is explicitly true.
func (cfg *Config) GoCGOEnabled() bool {
	value := cfg.EffectiveGoCGO()
	return value != nil && *value
}

// FFmpegStatigoDeclared reports whether ffmpeg-statigo is explicitly set.
// Callers must preserve owned files for an absent option unless Go is explicitly disabled.
// When GoDeclared is true and GoEnabled is false, callers must remove owned component files.
func (cfg *Config) FFmpegStatigoDeclared() bool {
	return cfg.GoDeclared() && cfg.Languages.GoOptions != nil && cfg.Languages.GoOptions.FFmpegStatigo != nil
}

// FFmpegStatigoEnabled reports whether Go and ffmpeg-statigo are both enabled.
// Callers must remove owned component files for an explicit false option.
func (cfg *Config) FFmpegStatigoEnabled() bool {
	return cfg.GoEnabled() && cfg.FFmpegStatigoDeclared() && *cfg.Languages.GoOptions.FFmpegStatigo
}

// SwatchActive applies the language opt-in requirement, not Pages or wiki readiness checks.
func (cfg *Config) SwatchActive(path string) bool {
	switch path {
	case ".golangci.yml", ".goreleaser.yaml", ".github/workflows/build-go.yml", "Dockerfile":
		return cfg.GoEnabled()
	default:
		return true
	}
}

// ActiveDefaultSwatches returns registered swatches that pass the language opt-in requirement, in registry order.
func (cfg *Config) ActiveDefaultSwatches() []swatch.Swatch {
	var active []swatch.Swatch
	for _, s := range swatch.All() {
		if cfg.SwatchActive(s.Path) {
			active = append(active, s)
		}
	}
	return active
}

// validateLanguageNodes checks YAML types before decoding can coerce scalar values.
func validateLanguageNodes(document *yaml.Node) error {
	var sections map[string]yaml.Node
	if err := document.Decode(&sections); err != nil {
		return err
	}
	node, ok := sections["languages"]
	if !ok {
		return nil
	}
	var languages LanguageSettings
	return languages.UnmarshalYAML(&node)
}
