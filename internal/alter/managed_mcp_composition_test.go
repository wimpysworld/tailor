package alter

import (
	"bytes"
	"encoding/json"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/wimpysworld/tailor/internal/config"
	"github.com/wimpysworld/tailor/internal/swatch"
)

func TestManagedMCPProductionRegistryMatchesEmbeddedStarters(t *testing.T) {
	if err := validateManagedMCPRegistry(fixedManagedMCPRegistry()); err != nil {
		t.Fatal(err)
	}
	enabled := true
	cfg := &config.Config{MCP: &config.MCPSettings{Playwright: &enabled}}
	selections, err := selectManagedFiles(cfg)
	if err != nil {
		t.Fatal(err)
	}
	rendered, err := renderManagedFiles(cfg, selections)
	if err != nil {
		t.Fatal(err)
	}
	for _, destination := range managedMCPDestinations() {
		embedded, readErr := swatch.Content(destination)
		if readErr != nil {
			t.Fatal(readErr)
		}
		if !bytes.Equal(rendered[destination], embedded) {
			t.Errorf("rendered %q differs from its embedded starter\nrendered:\n%s\nembedded:\n%s", destination, rendered[destination], embedded)
		}
	}
}

func TestManagedMCPCompositionIsDeterministicAndMaterialisesEachClientOnce(t *testing.T) {
	cfg := &config.Config{}
	fileRegistry := managedMCPFileRegistry()
	registry := []managedMCPServerDefinition{
		syntheticManagedMCPServer("zulu", true, true),
		syntheticManagedMCPServer("alpha", true, true),
		syntheticManagedMCPServer("absent", false, false),
		syntheticManagedMCPServer("disabled", true, false),
	}

	selections, err := selectManagedFilesFromRegistries(cfg, fileRegistry, registry)
	if err != nil {
		t.Fatal(err)
	}
	if len(selections) != len(managedMCPDestinations()) {
		t.Fatalf("MCP selections = %d, want %d", len(selections), len(managedMCPDestinations()))
	}
	seen := make(map[string]int, len(selections))
	for _, selection := range selections {
		seen[selection.Entry.Path]++
	}
	for _, destination := range managedMCPDestinations() {
		if seen[destination] != 1 {
			t.Errorf("destination %q selected %d times, want once", destination, seen[destination])
		}
	}

	baseline, err := renderManagedFilesFromRegistries(cfg, selections, fileRegistry, registry)
	if err != nil {
		t.Fatal(err)
	}
	repeated, err := renderManagedFilesFromRegistries(cfg, selections, fileRegistry, registry)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(repeated, baseline) {
		t.Fatal("repeated MCP rendering changed output")
	}

	reversed := slices.Clone(registry)
	slices.Reverse(reversed)
	reversedSelections, err := selectManagedFilesFromRegistries(cfg, fileRegistry, reversed)
	if err != nil {
		t.Fatal(err)
	}
	reversedRendered, err := renderManagedFilesFromRegistries(cfg, reversedSelections, fileRegistry, reversed)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(reversedRendered, baseline) {
		t.Fatal("MCP rendering depends on registry order")
	}

	for _, destination := range managedMCPDestinations() {
		content := baseline[destination]
		alpha := bytes.Index(content, []byte("alpha"))
		zulu := bytes.Index(content, []byte("zulu"))
		if alpha < 0 || zulu < 0 || alpha >= zulu {
			t.Errorf("%q does not use lexical server order:\n%s", destination, content)
		}
		if bytes.Contains(content, []byte(`"absent": {`)) || bytes.Contains(content, []byte(`"disabled": {`)) || bytes.Contains(content, []byte("mcp_servers.absent")) || bytes.Contains(content, []byte("mcp_servers.disabled")) {
			t.Errorf("%q contains a false or absent server:\n%s", destination, content)
		}
	}
	for _, destination := range []string{".mcp.json", "opencode.json", ".pi/mcp.json"} {
		var parsed map[string]any
		if err := json.Unmarshal(baseline[destination], &parsed); err != nil {
			t.Errorf("parse rendered %q: %v", destination, err)
		}
	}

	type codexServer struct {
		Command string   `json:"command"`
		Args    []string `json:"args"`
		Enabled bool     `json:"enabled"`
	}
	var codex struct {
		MCPServers map[string]codexServer `json:"mcp_servers"`
	}
	decodeManagedTOML(t, ".codex/config.toml", baseline[".codex/config.toml"], &codex)
	wantCodex := map[string]codexServer{
		"alpha": {Command: "alpha-mcp", Args: []string{"--alpha"}, Enabled: true},
		"zulu":  {Command: "zulu-mcp", Args: []string{"--zulu"}, Enabled: true},
	}
	if !reflect.DeepEqual(codex.MCPServers, wantCodex) {
		t.Errorf("Codex server tables = %#v, want %#v", codex.MCPServers, wantCodex)
	}
}

func TestManagedMCPCompositionExcludesFalseAndAbsentDefinitions(t *testing.T) {
	cfg := &config.Config{}
	registry := []managedMCPServerDefinition{
		syntheticManagedMCPServer("absent", false, false),
		syntheticManagedMCPServer("disabled", true, false),
	}
	selections, err := selectManagedFilesFromRegistries(cfg, managedMCPFileRegistry(), registry)
	if err != nil {
		t.Fatal(err)
	}
	if len(selections) != 0 {
		t.Fatalf("MCP selections = %#v, want none", selections)
	}
}

func TestManagedMCPPackageSelectionRemainsIndependent(t *testing.T) {
	disabled := false
	cfg := &config.Config{MCP: &config.MCPSettings{Playwright: &disabled}}
	selections, err := selectManagedFilesFromRegistries(cfg, fixedManagedRegistry(), []managedMCPServerDefinition{
		syntheticManagedMCPServer("synthetic", true, true),
	})
	if err != nil {
		t.Fatal(err)
	}
	selected := make(map[string]bool, len(selections))
	for _, selection := range selections {
		selected[selection.Entry.Path] = selection.Enabled
	}
	if selected["nix/playwright.nix"] {
		t.Fatal("synthetic MCP server enabled the Playwright package fragment")
	}
	for _, destination := range managedMCPDestinations() {
		if !selected[destination] {
			t.Errorf("synthetic MCP server did not select %q", destination)
		}
	}
}

func TestValidateManagedMCPRegistryRejectsInvalidDefinitions(t *testing.T) {
	valid := syntheticManagedMCPServer("valid", true, true)
	withoutProvider := func(provider managedMCPProvider) []managedMCPProviderDefinition {
		return slices.DeleteFunc(slices.Clone(valid.Definitions), func(definition managedMCPProviderDefinition) bool {
			return definition.Provider == provider
		})
	}
	trueValue := true
	tests := []struct {
		name     string
		registry []managedMCPServerDefinition
		want     string
	}{
		{name: "duplicate name", registry: []managedMCPServerDefinition{valid, valid}, want: "duplicate server name"},
		{name: "missing state", registry: []managedMCPServerDefinition{{Name: "server", Definitions: valid.Definitions}}, want: "requires declaration state"},
		{name: "empty definitions", registry: []managedMCPServerDefinition{{Name: "server", State: valid.State}}, want: "requires a provider definition"},
		{name: "missing Claude", registry: []managedMCPServerDefinition{{Name: "server", State: valid.State, Definitions: withoutProvider(managedMCPProviderClaude)}}, want: "requires a Claude provider definition"},
		{name: "missing Codex", registry: []managedMCPServerDefinition{{Name: "server", State: valid.State, Definitions: withoutProvider(managedMCPProviderCodex)}}, want: "requires a Codex provider definition"},
		{name: "missing OpenCode", registry: []managedMCPServerDefinition{{Name: "server", State: valid.State, Definitions: withoutProvider(managedMCPProviderOpenCode)}}, want: "requires a OpenCode provider definition"},
		{name: "missing Pi", registry: []managedMCPServerDefinition{{Name: "server", State: valid.State, Definitions: withoutProvider(managedMCPProviderPi)}}, want: "requires a Pi provider definition"},
		{name: "duplicate provider", registry: []managedMCPServerDefinition{{Name: "server", State: valid.State, Definitions: []managedMCPProviderDefinition{valid.Definitions[0], valid.Definitions[0]}}}, want: "duplicate \"claude\" provider"},
		{name: "unsupported provider", registry: []managedMCPServerDefinition{{Name: "server", State: valid.State, Definitions: []managedMCPProviderDefinition{{Provider: "unknown", Command: "server"}}}}, want: "unsupported provider"},
		{name: "unsupported fields", registry: []managedMCPServerDefinition{{Name: "server", State: valid.State, Definitions: []managedMCPProviderDefinition{{Provider: managedMCPProviderClaude, Type: "stdio", Command: "server", Enabled: &trueValue}}}}, want: "unsupported Claude provider fields"},
		{name: "missing launcher", registry: []managedMCPServerDefinition{{Name: "server", State: valid.State, Definitions: []managedMCPProviderDefinition{{Provider: managedMCPProviderClaude, Type: "stdio"}}}}, want: "requires a command"},
		{name: "unresolved token", registry: []managedMCPServerDefinition{{Name: "server", State: valid.State, Definitions: []managedMCPProviderDefinition{{Provider: managedMCPProviderClaude, Type: "stdio", Command: "[[TAILOR_COMMAND]]"}}}}, want: "unresolved Tailor token"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateManagedMCPRegistry(tt.registry)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("validateManagedMCPRegistry() error = %v, want substring %q", err, tt.want)
			}
		})
	}

	if err := validateManagedMCPRegistry(fixedManagedMCPRegistry()); err != nil {
		t.Fatalf("literal HTTPS_PROXY default syntax was rejected: %v", err)
	}
}

func TestValidateManagedMCPRegistryRejectsUnsafeStrings(t *testing.T) {
	unsafeValues := []struct {
		name  string
		value string
	}{
		{name: "bell", value: "\a"},
		{name: "vertical tab", value: "\v"},
		{name: "delete", value: "\x7f"},
		{name: "invalid UTF-8", value: string([]byte{0xff})},
	}
	fields := []struct {
		name   string
		mutate func(*managedMCPServerDefinition, string)
	}{
		{name: "server", mutate: func(server *managedMCPServerDefinition, value string) { server.Name = "server" + value }},
		{name: "type", mutate: func(server *managedMCPServerDefinition, value string) { server.Definitions[0].Type = "stdio" + value }},
		{name: "command", mutate: func(server *managedMCPServerDefinition, value string) {
			server.Definitions[0].Command = "server" + value
		}},
		{name: "lifecycle", mutate: func(server *managedMCPServerDefinition, value string) {
			server.Definitions[3].Lifecycle = "lazy" + value
		}},
		{name: "argument", mutate: func(server *managedMCPServerDefinition, value string) {
			server.Definitions[0].Args = []string{"--value" + value}
		}},
	}

	for _, unsafe := range unsafeValues {
		for _, field := range fields {
			t.Run(unsafe.name+"/"+field.name, func(t *testing.T) {
				server := syntheticManagedMCPServer("server", true, true)
				field.mutate(&server, unsafe.value)
				if err := validateManagedMCPRegistry([]managedMCPServerDefinition{server}); err == nil {
					t.Fatal("validateManagedMCPRegistry() accepted unsafe string")
				}
			})
		}
	}
}

func TestManagedMCPRenderingPreservesPrintableStrings(t *testing.T) {
	const (
		name      = `serveur "café" \\ local`
		command   = `mcp "café" \\ launcher`
		argument  = `--label="naïve"\\value`
		lifecycle = `paresseux "été" \\ mode`
	)
	server := syntheticManagedMCPServer(name, true, true)
	for index := range server.Definitions {
		server.Definitions[index].Command = command
		server.Definitions[index].Args = []string{argument}
	}
	server.Definitions[3].Lifecycle = lifecycle
	registry := []managedMCPServerDefinition{server}
	if err := validateManagedMCPRegistry(registry); err != nil {
		t.Fatal(err)
	}

	for _, destination := range managedMCPDestinations() {
		content, err := renderManagedMCPDestination(&config.Config{}, destination, registry)
		if err != nil {
			t.Fatal(err)
		}
		if destination == ".codex/config.toml" {
			var parsed struct {
				MCPServers map[string]struct {
					Command string   `json:"command"`
					Args    []string `json:"args"`
					Enabled bool     `json:"enabled"`
				} `json:"mcp_servers"`
			}
			decodeManagedTOML(t, destination, content, &parsed)
			got, exists := parsed.MCPServers[name]
			if !exists || got.Command != command || !reflect.DeepEqual(got.Args, []string{argument}) || !got.Enabled {
				t.Errorf("parsed Codex server = %#v, exists %t", got, exists)
			}
			continue
		}
		var parsed map[string]any
		if err := json.Unmarshal(content, &parsed); err != nil {
			t.Errorf("parse rendered %q: %v", destination, err)
			continue
		}
		containerName := "mcpServers"
		if destination == "opencode.json" {
			containerName = "mcp"
		}
		container, ok := parsed[containerName].(map[string]any)
		if !ok {
			t.Errorf("rendered %q lacks %q object", destination, containerName)
			continue
		}
		entry, ok := container[name].(map[string]any)
		if !ok {
			t.Errorf("rendered %q lacks server %q", destination, name)
			continue
		}
		if destination == "opencode.json" {
			got, ok := entry["command"].([]any)
			if !ok || !reflect.DeepEqual(got, []any{command, argument}) {
				t.Errorf("parsed OpenCode command = %#v", entry["command"])
			}
			continue
		}
		if entry["command"] != command {
			t.Errorf("parsed %q command = %#v, want %q", destination, entry["command"], command)
		}
		if !reflect.DeepEqual(entry["args"], []any{argument}) {
			t.Errorf("parsed %q arguments = %#v, want %q", destination, entry["args"], argument)
		}
		if destination == ".pi/mcp.json" && entry["lifecycle"] != lifecycle {
			t.Errorf("parsed Pi lifecycle = %#v, want %q", entry["lifecycle"], lifecycle)
		}
	}
}

func managedMCPDestinations() []string {
	var destinations []string
	for _, entry := range fixedManagedRegistry() {
		if entry.Policy == managedPolicySharedStarter {
			destinations = append(destinations, entry.Path)
		}
	}
	return destinations
}

func managedMCPFileRegistry() []managedRegistryEntry {
	entries := make([]managedRegistryEntry, 0, len(managedMCPDestinations()))
	for _, destination := range managedMCPDestinations() {
		entries = append(entries, managedRegistryEntry{Path: destination, Policy: managedPolicySharedStarter})
	}
	return entries
}

func syntheticManagedMCPServer(name string, declared, enabled bool) managedMCPServerDefinition {
	enabledField := true
	disabledField := false
	return managedMCPServerDefinition{
		Name: name,
		State: func(*config.Config) (bool, bool) {
			return declared, enabled
		},
		Definitions: []managedMCPProviderDefinition{
			{Provider: managedMCPProviderClaude, Type: "stdio", Command: name + "-mcp", Args: []string{"--" + name}},
			{Provider: managedMCPProviderCodex, Command: name + "-mcp", Args: []string{"--" + name}, Enabled: &enabledField},
			{Provider: managedMCPProviderOpenCode, Type: "local", Command: name + "-mcp", Args: []string{"--" + name}, Enabled: &enabledField},
			{Provider: managedMCPProviderPi, Command: name + "-mcp", Args: []string{"--" + name}, Disabled: &disabledField, Lifecycle: "lazy", DirectTools: &disabledField},
		},
	}
}
