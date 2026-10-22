package alter

import (
	"bytes"
	"encoding/json"
	"reflect"
	"testing"

	"github.com/wimpysworld/tailor/internal/config"
)

func TestManagedBlenderCompositions(t *testing.T) {
	states := []struct {
		name  string
		value *bool
	}{
		{name: "nil"},
		{name: "false", value: new(false)},
		{name: "true", value: new(true)},
	}
	for _, blender := range states {
		for _, playwright := range states {
			t.Run("blender="+blender.name+"/playwright="+playwright.name, func(t *testing.T) {
				cfg := &config.Config{MCP: &config.MCPSettings{Blender: blender.value, Playwright: playwright.value}}
				rendered := renderSelectedManagedFiles(t, cfg)
				for _, destination := range managedMCPDestinations() {
					content, exists := rendered[destination]
					if exists != (cfg.BlenderEnabled() || cfg.PlaywrightEnabled()) {
						t.Fatalf("unexpected selection %s", destination)
					}
					if !exists {
						continue
					}
					var parsed map[string]any
					if destination == ".codex/config.toml" {
						decodeManagedTOML(t, destination, content, &parsed)
					} else if err := json.Unmarshal(content, &parsed); err != nil {
						t.Fatal(err)
					}
					container := "mcpServers"
					if destination == ".codex/config.toml" {
						container = "mcp_servers"
					}
					if destination == "opencode.json" {
						container = "mcp"
					}
					servers := parsed[container].(map[string]any)
					_, hasBlender := servers["blender-agent-bridge"]
					_, hasPlaywright := servers["playwright"]
					if hasBlender != cfg.BlenderEnabled() || hasPlaywright != cfg.PlaywrightEnabled() {
						t.Fatalf("incorrect servers: %v", servers)
					}
					if !hasBlender {
						continue
					}
					entry := servers["blender-agent-bridge"].(map[string]any)
					args := []any{"--bridge-url", "http://127.0.0.1:8765"}
					if destination == "opencode.json" {
						if !reflect.DeepEqual(entry["command"], append([]any{"blender-bridge"}, args...)) || entry["type"] != "local" {
							t.Fatalf("wrong command: %v", entry)
						}
					} else if entry["command"] != "blender-bridge" || !reflect.DeepEqual(entry["args"], args) {
						t.Fatalf("wrong launcher: %v", entry)
					}
					if destination == ".pi/mcp.json" && (entry["type"] != "stdio" || entry["enabled"] != true || entry["disabled"] != nil || entry["directTools"] != nil || entry["lifecycle"] != nil) {
						t.Fatalf("wrong native Pi fields: %v", entry)
					}
					if hasPlaywright && bytes.Index(content, []byte("blender-agent-bridge")) > bytes.Index(content, []byte("playwright")) {
						t.Fatal("server order is not lexical")
					}
				}
			})
		}
	}
}

func TestManagedPiRejectsMixedNativeAndLegacyFields(t *testing.T) {
	definition, ok := managedMCPDefinitionForProvider(fixedManagedMCPRegistry()[0], managedMCPProviderPi)
	if !ok || !validManagedMCPPiDefinition(definition) {
		t.Fatal("native definition is invalid")
	}
	definition.Disabled = new(false)
	if validManagedMCPPiDefinition(definition) {
		t.Fatal("mixed native and legacy fields accepted")
	}
}
