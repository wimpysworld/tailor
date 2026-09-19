package alter

import (
	"fmt"
	"slices"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/wimpysworld/tailor/internal/config"
)

type managedMCPProvider string

const (
	managedMCPProviderClaude   managedMCPProvider = "claude"
	managedMCPProviderCodex    managedMCPProvider = "codex"
	managedMCPProviderOpenCode managedMCPProvider = "opencode"
	managedMCPProviderPi       managedMCPProvider = "pi"
)

type managedMCPProviderDefinition struct {
	Provider    managedMCPProvider
	Type        string
	Command     string
	Args        []string
	Enabled     *bool
	Disabled    *bool
	Lifecycle   string
	DirectTools *bool
}

type managedMCPServerDefinition struct {
	Name        string
	State       func(*config.Config) (declared, enabled bool)
	Definitions []managedMCPProviderDefinition
}

func fixedManagedMCPRegistry() []managedMCPServerDefinition {
	enabled := true
	disabled := false
	return []managedMCPServerDefinition{
		{
			Name: "playwright",
			State: func(cfg *config.Config) (bool, bool) {
				return cfg.PlaywrightDeclared(), cfg.PlaywrightEnabled()
			},
			Definitions: []managedMCPProviderDefinition{
				{Provider: managedMCPProviderClaude, Type: "stdio", Command: "playwright-mcp", Args: []string{"--headless", "--isolated"}},
				{Provider: managedMCPProviderCodex, Command: "playwright-mcp", Args: []string{"--headless", "--isolated"}, Enabled: &enabled},
				{Provider: managedMCPProviderOpenCode, Type: "local", Command: "playwright-mcp", Args: []string{"--headless", "--isolated"}, Enabled: &enabled},
				{
					Provider:    managedMCPProviderPi,
					Command:     "sh",
					Args:        []string{"-c", `if [ -n "${HTTPS_PROXY:-}" ]; then exec playwright-mcp --headless --isolated --proxy-server "$HTTPS_PROXY"; else exec playwright-mcp --headless --isolated; fi`},
					Disabled:    &disabled,
					Lifecycle:   "lazy",
					DirectTools: &disabled,
				},
			},
		},
	}
}

func validateManagedMCPRegistry(registry []managedMCPServerDefinition) error {
	seenNames := make(map[string]struct{}, len(registry))
	for _, server := range registry {
		if server.Name == "" || hasUnsafeManagedMCPString(server.Name) {
			return fmt.Errorf("managed MCP registry contains invalid server name %q", server.Name)
		}
		if hasUnresolvedManagedMCPToken(server.Name) {
			return fmt.Errorf("managed MCP server name %q contains an unresolved Tailor token", server.Name)
		}
		if _, exists := seenNames[server.Name]; exists {
			return fmt.Errorf("managed MCP registry contains duplicate server name %q", server.Name)
		}
		seenNames[server.Name] = struct{}{}
		if server.State == nil {
			return fmt.Errorf("managed MCP server %q requires declaration state", server.Name)
		}
		if len(server.Definitions) == 0 {
			return fmt.Errorf("managed MCP server %q requires a provider definition", server.Name)
		}

		seenProviders := make(map[managedMCPProvider]struct{}, len(server.Definitions))
		for _, definition := range server.Definitions {
			if _, exists := seenProviders[definition.Provider]; exists {
				return fmt.Errorf("managed MCP server %q contains duplicate %q provider definitions", server.Name, definition.Provider)
			}
			seenProviders[definition.Provider] = struct{}{}
			if err := validateManagedMCPProviderDefinition(server.Name, definition); err != nil {
				return err
			}
		}
		for _, provider := range []managedMCPProvider{
			managedMCPProviderClaude,
			managedMCPProviderCodex,
			managedMCPProviderOpenCode,
			managedMCPProviderPi,
		} {
			if _, exists := seenProviders[provider]; !exists {
				return fmt.Errorf("managed MCP server %q requires a %s provider definition", server.Name, managedMCPProviderLabel(provider))
			}
		}
	}
	return nil
}

func validateManagedMCPProviderDefinition(server string, definition managedMCPProviderDefinition) error {
	if err := validateManagedMCPLauncher(server, definition); err != nil {
		return err
	}

	var valid bool
	switch definition.Provider {
	case managedMCPProviderClaude:
		valid = validManagedMCPClaudeDefinition(definition)
	case managedMCPProviderCodex:
		valid = validManagedMCPCodexDefinition(definition)
	case managedMCPProviderOpenCode:
		valid = validManagedMCPOpenCodeDefinition(definition)
	case managedMCPProviderPi:
		valid = validManagedMCPPiDefinition(definition)
	default:
		return fmt.Errorf("managed MCP server %q has unsupported provider %q", server, definition.Provider)
	}
	if !valid {
		return fmt.Errorf("managed MCP server %q has unsupported %s provider fields", server, managedMCPProviderLabel(definition.Provider))
	}
	return nil
}

func validateManagedMCPLauncher(server string, definition managedMCPProviderDefinition) error {
	if definition.Command == "" {
		return fmt.Errorf("managed MCP server %q provider %q requires a command", server, definition.Provider)
	}
	values := []string{server, definition.Type, definition.Command, definition.Lifecycle}
	values = append(values, definition.Args...)
	if slices.ContainsFunc(values, hasUnresolvedManagedMCPToken) {
		return fmt.Errorf("managed MCP server %q provider %q contains an unresolved Tailor token", server, definition.Provider)
	}
	if slices.ContainsFunc(values, hasUnsafeManagedMCPString) {
		return fmt.Errorf("managed MCP server %q provider %q contains invalid launcher data", server, definition.Provider)
	}
	return nil
}

func hasUnsafeManagedMCPString(value string) bool {
	return !utf8.ValidString(value) || strings.IndexFunc(value, unicode.IsControl) >= 0
}

func validManagedMCPClaudeDefinition(definition managedMCPProviderDefinition) bool {
	return definition.Type == "stdio" && definition.Enabled == nil && definition.Disabled == nil && definition.Lifecycle == "" && definition.DirectTools == nil
}

func validManagedMCPCodexDefinition(definition managedMCPProviderDefinition) bool {
	return definition.Type == "" && definition.Enabled != nil && *definition.Enabled && definition.Disabled == nil && definition.Lifecycle == "" && definition.DirectTools == nil
}

func validManagedMCPOpenCodeDefinition(definition managedMCPProviderDefinition) bool {
	return definition.Type == "local" && definition.Enabled != nil && *definition.Enabled && definition.Disabled == nil && definition.Lifecycle == "" && definition.DirectTools == nil
}

func validManagedMCPPiDefinition(definition managedMCPProviderDefinition) bool {
	return definition.Type == "" && definition.Enabled == nil && definition.Disabled != nil && !*definition.Disabled && definition.Lifecycle != "" && definition.DirectTools != nil
}

func managedMCPProviderLabel(provider managedMCPProvider) string {
	switch provider {
	case managedMCPProviderClaude:
		return "Claude"
	case managedMCPProviderCodex:
		return "Codex"
	case managedMCPProviderOpenCode:
		return "OpenCode"
	case managedMCPProviderPi:
		return "Pi"
	default:
		return string(provider)
	}
}

func managedMCPProviderForDestination(destination string) (managedMCPProvider, bool) {
	switch destination {
	case ".mcp.json":
		return managedMCPProviderClaude, true
	case ".codex/config.toml":
		return managedMCPProviderCodex, true
	case "opencode.json":
		return managedMCPProviderOpenCode, true
	case ".pi/mcp.json":
		return managedMCPProviderPi, true
	default:
		return "", false
	}
}

func managedMCPDefinitionForProvider(server managedMCPServerDefinition, provider managedMCPProvider) (managedMCPProviderDefinition, bool) {
	for _, definition := range server.Definitions {
		if definition.Provider == provider {
			return definition, true
		}
	}
	return managedMCPProviderDefinition{}, false
}

func managedMCPDestinationEnabled(cfg *config.Config, destination string, registry []managedMCPServerDefinition) bool {
	provider, supported := managedMCPProviderForDestination(destination)
	if !supported {
		return false
	}
	for _, server := range registry {
		declared, enabled := server.State(cfg)
		if !declared || !enabled {
			continue
		}
		if _, exists := managedMCPDefinitionForProvider(server, provider); exists {
			return true
		}
	}
	return false
}

func hasUnresolvedManagedMCPToken(value string) bool {
	return strings.Contains(value, "[[TAILOR_")
}
