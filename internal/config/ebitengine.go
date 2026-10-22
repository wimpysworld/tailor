package config

import (
	"fmt"

	"gopkg.in/yaml.v3"
)

// EbitengineOptions preserves Boolean and mapping declarations.
type EbitengineOptions struct {
	Enabled bool                      `yaml:"enabled"`
	Android *EbitengineAndroidOptions `yaml:"android,omitempty"`
	Mapping bool                      `yaml:"-"`
}

type EbitengineAndroidOptions struct {
	Enabled bool `yaml:"enabled"`
}

func decodeEbitengine(node *yaml.Node, name string, component bool) (bool, error) {
	if component && node.Kind == yaml.ScalarNode && node.Tag == "!!bool" {
		return false, nil
	}
	if node.Kind != yaml.MappingNode {
		return false, fmt.Errorf("%s must be a mapping%s", name, map[bool]string{true: " or bool"}[component])
	}
	var fields map[string]yaml.Node
	if err := node.Decode(&fields); err != nil {
		return false, err
	}
	if _, ok := fields["enabled"]; !ok {
		return false, fmt.Errorf("%s.enabled is required", name)
	}
	for i := 0; i < len(node.Content); i += 2 {
		key, value := node.Content[i], node.Content[i+1]
		if key.Tag != "!!str" {
			return false, fmt.Errorf("%s keys must be strings", name)
		}
		if component && key.Value == "android" {
			var android EbitengineAndroidOptions
			if err := android.UnmarshalYAML(value); err != nil {
				return false, err
			}
			continue
		}
		if key.Value != "enabled" {
			return false, fmt.Errorf("unknown %s setting %q", name, key.Value)
		}
		if value.Kind != yaml.ScalarNode || value.Tag != "!!bool" {
			return false, fmt.Errorf("%s.%s must be bool", name, key.Value)
		}
	}
	return true, nil
}

func (e *EbitengineOptions) UnmarshalYAML(node *yaml.Node) error {
	mapping, err := decodeEbitengine(node, "languages.go.ebitengine", true)
	if err != nil {
		return err
	}
	*e = EbitengineOptions{Mapping: mapping}
	if !mapping {
		return node.Decode(&e.Enabled)
	}
	type plain EbitengineOptions
	return node.Decode((*plain)(e))
}

func (a *EbitengineAndroidOptions) UnmarshalYAML(node *yaml.Node) error {
	if _, err := decodeEbitengine(node, "languages.go.ebitengine.android", false); err != nil {
		return err
	}
	type plain EbitengineAndroidOptions
	return node.Decode((*plain)(a))
}

func (e EbitengineOptions) MarshalYAML() (any, error) {
	if !e.Mapping && e.Android == nil {
		return e.Enabled, nil
	}
	type plain EbitengineOptions
	return plain(e), nil
}

func (cfg *Config) Ebitengine() *EbitengineOptions {
	if !cfg.GoDeclared() || cfg.Languages.GoOptions == nil {
		return nil
	}
	return cfg.Languages.GoOptions.Ebitengine
}
func (cfg *Config) EbitengineDeclared() bool { return cfg.Ebitengine() != nil }
func (cfg *Config) EbitengineEnabled() bool {
	return cfg.GoEnabled() && cfg.EbitengineDeclared() && cfg.Ebitengine().Enabled
}

func (cfg *Config) EbitengineAndroidEnabled() bool {
	return cfg.EbitengineEnabled() && cfg.Ebitengine().Android != nil && cfg.Ebitengine().Android.Enabled
}

func ValidateEbitengine(cfg *Config) error {
	e := cfg.Ebitengine()
	if e == nil {
		return nil
	}
	if e.Enabled && !cfg.GoEnabled() {
		return fmt.Errorf("languages.go.ebitengine requires enabled Go")
	}
	if !e.Enabled && e.Android != nil && e.Android.Enabled {
		return fmt.Errorf("ebitengine.android requires enabled Ebitengine")
	}
	return nil
}
