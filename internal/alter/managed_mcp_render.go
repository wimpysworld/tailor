package alter

import (
	"bytes"
	"fmt"
	"slices"
	"strconv"
	"strings"

	"github.com/wimpysworld/tailor/internal/config"
)

type managedMCPNamedDefinition struct {
	Name       string
	Definition managedMCPProviderDefinition
}

func renderManagedMCPDestination(cfg *config.Config, destination string, registry []managedMCPServerDefinition) ([]byte, error) {
	if cfg == nil {
		return nil, fmt.Errorf("managed MCP rendering requires a config")
	}
	if err := validateManagedMCPRegistry(registry); err != nil {
		return nil, fmt.Errorf("validating managed MCP registry: %w", err)
	}
	provider, supported := managedMCPProviderForDestination(destination)
	if !supported {
		return nil, fmt.Errorf("unsupported managed MCP destination %q", destination)
	}
	definitions := enabledManagedMCPDefinitions(cfg, provider, registry)
	if len(definitions) == 0 {
		return nil, fmt.Errorf("managed MCP destination %q has no enabled server definitions", destination)
	}

	var content []byte
	switch provider {
	case managedMCPProviderClaude:
		content = renderManagedMCPClaude(definitions)
	case managedMCPProviderCodex:
		content = renderManagedMCPCodex(definitions)
	case managedMCPProviderOpenCode:
		content = renderManagedMCPOpenCode(definitions)
	case managedMCPProviderPi:
		content = renderManagedMCPPi(definitions)
	}
	if bytes.Contains(content, []byte("[[TAILOR_")) {
		return nil, fmt.Errorf("managed MCP destination %q contains an unresolved Tailor token", destination)
	}
	return content, nil
}

func enabledManagedMCPDefinitions(cfg *config.Config, provider managedMCPProvider, registry []managedMCPServerDefinition) []managedMCPNamedDefinition {
	definitions := make([]managedMCPNamedDefinition, 0, len(registry))
	for _, server := range registry {
		declared, enabled := server.State(cfg)
		if !declared || !enabled {
			continue
		}
		definition, exists := managedMCPDefinitionForProvider(server, provider)
		if exists {
			definitions = append(definitions, managedMCPNamedDefinition{Name: server.Name, Definition: definition})
		}
	}
	slices.SortFunc(definitions, func(left, right managedMCPNamedDefinition) int {
		return strings.Compare(left.Name, right.Name)
	})
	return definitions
}

func renderManagedMCPClaude(definitions []managedMCPNamedDefinition) []byte {
	var output strings.Builder
	output.WriteString("{\n  \"mcpServers\": {")
	for index, named := range definitions {
		definition := named.Definition
		if index > 0 {
			output.WriteByte(',')
		}
		output.WriteString("\n    ")
		output.WriteString(managedMCPJSONString(named.Name))
		output.WriteString(": {\n      \"type\": ")
		output.WriteString(managedMCPJSONString(definition.Type))
		output.WriteString(",\n      \"command\": ")
		output.WriteString(managedMCPJSONString(definition.Command))
		output.WriteString(",\n      \"args\": ")
		output.WriteString(managedMCPJSONArray(definition.Args, ""))
		output.WriteString("\n    }")
	}
	output.WriteString("\n  }\n}\n")
	return []byte(output.String())
}

func renderManagedMCPCodex(definitions []managedMCPNamedDefinition) []byte {
	var output strings.Builder
	for index, named := range definitions {
		if index > 0 {
			output.WriteByte('\n')
		}
		definition := named.Definition
		output.WriteString("[mcp_servers.")
		output.WriteString(managedMCPTOMLKey(named.Name))
		output.WriteString("]\ncommand = ")
		output.WriteString(strconv.Quote(definition.Command))
		output.WriteString("\nargs = [")
		for argumentIndex, argument := range definition.Args {
			if argumentIndex > 0 {
				output.WriteString(", ")
			}
			output.WriteString(strconv.Quote(argument))
		}
		output.WriteString("]\nenabled = true\n")
	}
	return []byte(output.String())
}

func renderManagedMCPOpenCode(definitions []managedMCPNamedDefinition) []byte {
	var output strings.Builder
	output.WriteString("{\n  \"$schema\": \"https://opencode.ai/config.json\",\n  \"mcp\": {")
	for index, named := range definitions {
		definition := named.Definition
		command := make([]string, 0, len(definition.Args)+1)
		command = append(command, definition.Command)
		command = append(command, definition.Args...)
		if index > 0 {
			output.WriteByte(',')
		}
		output.WriteString("\n    ")
		output.WriteString(managedMCPJSONString(named.Name))
		output.WriteString(": {\n      \"type\": ")
		output.WriteString(managedMCPJSONString(definition.Type))
		output.WriteString(",\n      \"command\": ")
		output.WriteString(managedMCPJSONArray(command, ""))
		output.WriteString(",\n      \"enabled\": ")
		output.WriteString(strconv.FormatBool(*definition.Enabled))
		output.WriteString("\n    }")
	}
	output.WriteString("\n  }\n}\n")
	return []byte(output.String())
}

func renderManagedMCPPi(definitions []managedMCPNamedDefinition) []byte {
	var output strings.Builder
	output.WriteString("{\n  \"mcpServers\": {")
	for index, named := range definitions {
		definition := named.Definition
		if index > 0 {
			output.WriteByte(',')
		}
		output.WriteString("\n    ")
		output.WriteString(managedMCPJSONString(named.Name))
		output.WriteString(": {\n      \"command\": ")
		output.WriteString(managedMCPJSONString(definition.Command))
		output.WriteString(",\n      \"args\": ")
		output.WriteString(managedMCPJSONArray(definition.Args, "        "))
		output.WriteString(",\n      \"disabled\": ")
		output.WriteString(strconv.FormatBool(*definition.Disabled))
		output.WriteString(",\n      \"lifecycle\": ")
		output.WriteString(managedMCPJSONString(definition.Lifecycle))
		output.WriteString(",\n      \"directTools\": ")
		output.WriteString(strconv.FormatBool(*definition.DirectTools))
		output.WriteString("\n    }")
	}
	output.WriteString("\n  }\n}\n")
	return []byte(output.String())
}

func managedMCPJSONString(value string) string {
	return strconv.Quote(value)
}

func managedMCPJSONArray(values []string, indent string) string {
	if indent == "" {
		quoted := make([]string, len(values))
		for index, value := range values {
			quoted[index] = managedMCPJSONString(value)
		}
		return "[" + strings.Join(quoted, ", ") + "]"
	}
	quoted := make([]string, len(values))
	for index, value := range values {
		quoted[index] = indent + managedMCPJSONString(value)
	}
	return "[\n" + strings.Join(quoted, ",\n") + "\n      ]"
}

func managedMCPTOMLKey(value string) string {
	for _, character := range value {
		if (character < 'a' || character > 'z') && (character < 'A' || character > 'Z') && (character < '0' || character > '9') && character != '_' && character != '-' {
			return strconv.Quote(value)
		}
	}
	return value
}
