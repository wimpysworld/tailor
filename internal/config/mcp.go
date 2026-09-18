package config

import (
	"fmt"

	"gopkg.in/yaml.v3"
)

// MCPSettings controls Model Context Protocol integrations. Nil fields leave the integration undeclared.
type MCPSettings struct {
	Playwright *bool `yaml:"playwright,omitempty"`
}

// PlaywrightDeclared reports whether mcp.playwright is explicitly set, including false.
func (cfg *Config) PlaywrightDeclared() bool {
	return cfg != nil && cfg.MCP != nil && cfg.MCP.Playwright != nil
}

// PlaywrightEnabled reports whether mcp.playwright is explicitly true.
func (cfg *Config) PlaywrightEnabled() bool {
	return cfg.PlaywrightDeclared() && *cfg.MCP.Playwright
}

// validateMCPNodes checks YAML types before decoding can coerce scalar values.
func validateMCPNodes(document *yaml.Node) error {
	sections := make(map[string]yaml.Node)
	if err := document.Decode(&sections); err != nil {
		return err
	}
	node, ok := sections["mcp"]
	if !ok {
		return nil
	}
	if node.Kind != yaml.MappingNode {
		return fmt.Errorf("mcp must be a mapping")
	}
	extra := make(map[string]any)
	for i := 0; i < len(node.Content); i += 2 {
		key, value := node.Content[i], node.Content[i+1]
		if key.Tag != "!!str" || key.Value != "playwright" {
			extra[key.Value] = nil
			continue
		}
		if value.Kind != yaml.ScalarNode || value.Tag != "!!bool" {
			return fmt.Errorf("mcp.playwright must be a bool")
		}
	}
	return rejectExtra("mcp", extra, []string{"playwright"})
}
