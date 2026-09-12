package config

import (
	"fmt"
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"
)

const (
	maxVariables         = 500
	maxVariableValueSize = 48 * 1024
)

var variableNameRegexp = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

// validateVariableNodes rejects non-string values before YAML decoding can coerce them.
func validateVariableNodes(document *yaml.Node) error {
	var sections map[string]yaml.Node
	if err := document.Decode(&sections); err != nil {
		return err
	}
	node, ok := sections["variables"]
	if !ok {
		return nil
	}
	if node.Kind != yaml.SequenceNode {
		return fmt.Errorf("variables must be a sequence")
	}
	for i, entry := range node.Content {
		if entry.Kind != yaml.MappingNode {
			return fmt.Errorf("variable[%d]: entry must be a mapping", i)
		}
		seen := make(map[string]bool, 2)
		for j := 0; j < len(entry.Content); j += 2 {
			key, value := entry.Content[j], entry.Content[j+1]
			if key.Kind != yaml.ScalarNode || key.Tag != "!!str" || (key.Value != "name" && key.Value != "value") {
				return fmt.Errorf("variable[%d]: unrecognised setting %q, valid settings: name, value", i, key.Value)
			}
			if seen[key.Value] {
				return fmt.Errorf("variable[%d]: duplicate setting %q", i, key.Value)
			}
			seen[key.Value] = true
			if value.Kind != yaml.ScalarNode || value.Tag != "!!str" {
				return fmt.Errorf("variable[%d]: %s must be a string", i, key.Value)
			}
		}
		for _, key := range []string{"name", "value"} {
			if !seen[key] {
				return fmt.Errorf("variable[%d]: %s is required", i, key)
			}
		}
	}
	return nil
}

// ValidateVariables checks variable names, value presence, and limits.
func ValidateVariables(cfg *Config) error {
	if len(cfg.Variables) > maxVariables {
		return fmt.Errorf("variables: %d entries exceed the maximum of %d", len(cfg.Variables), maxVariables)
	}
	seen := make(map[string]bool, len(cfg.Variables))
	for i, entry := range cfg.Variables {
		if !variableNameRegexp.MatchString(entry.Name) {
			return fmt.Errorf("variable[%d]: name %q must match [A-Za-z_][A-Za-z0-9_]*", i, entry.Name)
		}
		key := strings.ToUpper(entry.Name)
		if strings.HasPrefix(key, "GITHUB_") {
			return fmt.Errorf("variable[%d]: name %q must not start with GITHUB_ (case-insensitive)", i, entry.Name)
		}
		if seen[key] {
			return fmt.Errorf("variable[%d]: duplicate variable name %q (case-insensitive)", i, entry.Name)
		}
		seen[key] = true
		if entry.Value == nil {
			return fmt.Errorf("variable[%d]: value is required", i)
		}
		if len(*entry.Value) > maxVariableValueSize {
			return fmt.Errorf("variable[%d]: value exceeds the maximum of %d bytes", i, maxVariableValueSize)
		}
	}
	return nil
}
