package config

import (
	"fmt"

	"github.com/wimpysworld/tailor/internal/swatch"
	"gopkg.in/yaml.v3"
)

type LanguageSettings struct {
	Go *bool `yaml:"go,omitempty"`
}

func (cfg *Config) GoDeclared() bool {
	return cfg != nil && cfg.Languages != nil && cfg.Languages.Go != nil
}

func (cfg *Config) GoEnabled() bool {
	return cfg.GoDeclared() && *cfg.Languages.Go
}

func (cfg *Config) SwatchActive(path string) bool {
	switch path {
	case ".golangci.yml", ".goreleaser.yaml", ".github/workflows/build-go.yml", "Dockerfile":
		return cfg.GoEnabled()
	default:
		return true
	}
}

func (cfg *Config) ActiveDefaultSwatches() []swatch.Swatch {
	var active []swatch.Swatch
	for _, s := range swatch.All() {
		if cfg.SwatchActive(s.Path) {
			active = append(active, s)
		}
	}
	return active
}

func validateLanguageNodes(document *yaml.Node) error {
	var sections map[string]yaml.Node
	if err := document.Decode(&sections); err != nil {
		return err
	}
	node, ok := sections["languages"]
	if !ok {
		return nil
	}
	if node.Kind != yaml.MappingNode {
		return fmt.Errorf("languages must be a mapping")
	}
	extra := make(map[string]any)
	for i := 0; i < len(node.Content); i += 2 {
		key, value := node.Content[i], node.Content[i+1]
		if key.Tag != "!!str" || key.Value != "go" {
			extra[key.Value] = nil
			continue
		}
		if value.Kind != yaml.ScalarNode || value.Tag != "!!bool" {
			return fmt.Errorf("languages.go must be a bool")
		}
	}
	return rejectExtra("languages", extra, []string{"go"})
}
