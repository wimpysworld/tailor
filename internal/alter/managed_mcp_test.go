package alter

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"testing"

	"github.com/wimpysworld/tailor/internal/config"
)

type managedMCPLaunch struct {
	Command string
	Args    []string
}

func TestManagedMCPProviderSchemas(t *testing.T) {
	launches := loadManagedMCPLaunches(t)
	wantDirect := managedMCPLaunch{Command: "playwright-mcp", Args: []string{"--headless", "--isolated"}}
	for _, provider := range []string{"claude", "codex", "opencode"} {
		if got := launches[provider]; !reflect.DeepEqual(got, wantDirect) {
			t.Errorf("%s launch = %#v, want %#v", provider, got, wantDirect)
		}
	}

	pi := launches["pi"]
	if pi.Command != "sh" || len(pi.Args) != 2 || pi.Args[0] != "-c" || pi.Args[1] == "" {
		t.Fatalf("Pi launch = %#v, want sh with one non-empty shell command", pi)
	}
	for _, forbidden := range []string{"--port", "npx", "npm", "--cdp-endpoint", "--no-sandbox", "--ignore-https-errors"} {
		if strings.Contains(pi.Args[1], forbidden) {
			t.Errorf("Pi shell command contains forbidden option %q", forbidden)
		}
	}
}

func TestManagedMCPLaunchArguments(t *testing.T) {
	launches := loadManagedMCPLaunches(t)
	bin := filepath.Join(t.TempDir(), "bin")
	stub := filepath.Join(bin, "playwright-mcp")
	writeManagedExecutable(t, stub, "#!/bin/sh\n: > \"$TAILOR_MCP_ARGS\"\nfor argument do\n  printf '%s\\0' \"$argument\" >> \"$TAILOR_MCP_ARGS\"\ndone\n")
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))

	proxy := "https://proxy.example/path with space/'quoted'?marker=sensitive&x=$HOME;echo leaked"
	tests := []struct {
		name       string
		launch     managedMCPLaunch
		proxy      *string
		wantArgs   []string
		secretText string
	}{
		{name: "claude direct", launch: launches["claude"], wantArgs: []string{"--headless", "--isolated"}},
		{name: "codex direct", launch: launches["codex"], wantArgs: []string{"--headless", "--isolated"}},
		{name: "opencode direct", launch: launches["opencode"], wantArgs: []string{"--headless", "--isolated"}},
		{name: "pi proxy populated", launch: launches["pi"], proxy: &proxy, wantArgs: []string{"--headless", "--isolated", "--proxy-server", proxy}, secretText: "sensitive"},
		{name: "pi proxy empty", launch: launches["pi"], proxy: new(string), wantArgs: []string{"--headless", "--isolated"}},
		{name: "pi proxy unset", launch: launches["pi"], wantArgs: []string{"--headless", "--isolated"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			argsFile := filepath.Join(t.TempDir(), "args")
			command := exec.CommandContext(t.Context(), tt.launch.Command, tt.launch.Args...) // #nosec G204 -- Embedded launch settings execute only the isolated stub.
			command.Env = append(managedEnvironmentWithout("HTTPS_PROXY", "TAILOR_MCP_ARGS"), "TAILOR_MCP_ARGS="+argsFile)
			if tt.proxy != nil {
				command.Env = append(command.Env, "HTTPS_PROXY="+*tt.proxy)
			}
			output, err := command.CombinedOutput()
			if err != nil {
				t.Fatalf("launch stub: %v\n%s", err, output)
			}
			if len(output) != 0 {
				t.Errorf("launch output = %q, want no output", output)
			}
			if tt.secretText != "" && bytes.Contains(output, []byte(tt.secretText)) {
				t.Error("launch output exposed the proxy credential fixture")
			}
			gotArgs := readManagedNULArguments(t, argsFile)
			if !reflect.DeepEqual(gotArgs, tt.wantArgs) {
				t.Errorf("stub arguments = %#v, want %#v", gotArgs, tt.wantArgs)
			}
		})
	}
}

func loadManagedMCPLaunches(t *testing.T) map[string]managedMCPLaunch {
	t.Helper()

	enabled := true
	rendered := renderSelectedManagedFiles(t, &config.Config{MCP: &config.MCPSettings{Playwright: &enabled}})

	var claude struct {
		MCPServers struct {
			Playwright struct {
				Type    string   `json:"type"`
				Command string   `json:"command"`
				Args    []string `json:"args"`
			} `json:"playwright"`
		} `json:"mcpServers"`
	}
	decodeManagedJSON(t, ".mcp.json", rendered[".mcp.json"], &claude)
	if claude.MCPServers.Playwright.Type != "stdio" {
		t.Errorf("Claude transport = %q, want stdio", claude.MCPServers.Playwright.Type)
	}

	var codex struct {
		MCPServers struct {
			Playwright struct {
				Command string   `json:"command"`
				Args    []string `json:"args"`
				Enabled bool     `json:"enabled"`
			} `json:"playwright"`
		} `json:"mcp_servers"`
	}
	decodeManagedTOML(t, ".codex/config.toml", rendered[".codex/config.toml"], &codex)
	if !codex.MCPServers.Playwright.Enabled {
		t.Error("Codex Playwright server is disabled")
	}

	var opencode struct {
		Schema string `json:"$schema"`
		MCP    struct {
			Playwright struct {
				Type    string   `json:"type"`
				Command []string `json:"command"`
				Enabled bool     `json:"enabled"`
			} `json:"playwright"`
		} `json:"mcp"`
	}
	decodeManagedJSON(t, "opencode.json", rendered["opencode.json"], &opencode)
	if opencode.Schema != "https://opencode.ai/config.json" || opencode.MCP.Playwright.Type != "local" || !opencode.MCP.Playwright.Enabled {
		t.Errorf("OpenCode settings = schema %q, type %q, enabled %t", opencode.Schema, opencode.MCP.Playwright.Type, opencode.MCP.Playwright.Enabled)
	}
	if len(opencode.MCP.Playwright.Command) == 0 {
		t.Fatal("OpenCode command is empty")
	}

	var pi struct {
		MCPServers struct {
			Playwright struct {
				Command     string   `json:"command"`
				Args        []string `json:"args"`
				Disabled    *bool    `json:"disabled"`
				Lifecycle   string   `json:"lifecycle"`
				DirectTools *bool    `json:"directTools"`
			} `json:"playwright"`
		} `json:"mcpServers"`
	}
	decodeManagedJSON(t, ".pi/mcp.json", rendered[".pi/mcp.json"], &pi)
	piServer := pi.MCPServers.Playwright
	if piServer.Disabled == nil || *piServer.Disabled || piServer.Lifecycle != "lazy" || piServer.DirectTools == nil || *piServer.DirectTools {
		t.Errorf("Pi settings = disabled %v, lifecycle %q, directTools %v", piServer.Disabled, piServer.Lifecycle, piServer.DirectTools)
	}

	return map[string]managedMCPLaunch{
		"claude":   {Command: claude.MCPServers.Playwright.Command, Args: claude.MCPServers.Playwright.Args},
		"codex":    {Command: codex.MCPServers.Playwright.Command, Args: codex.MCPServers.Playwright.Args},
		"opencode": {Command: opencode.MCP.Playwright.Command[0], Args: opencode.MCP.Playwright.Command[1:]},
		"pi":       {Command: pi.MCPServers.Playwright.Command, Args: pi.MCPServers.Playwright.Args},
	}
}

func decodeManagedJSON(t *testing.T, name string, content []byte, target any) {
	t.Helper()
	if len(content) == 0 {
		t.Fatalf("rendered %s content is empty", name)
	}
	decoder := json.NewDecoder(bytes.NewReader(content))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		t.Fatalf("decode %s: %v", name, err)
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		t.Fatalf("decode trailing %s content: %v", name, err)
	}
}

func decodeManagedTOML(t *testing.T, name string, content []byte, target any) {
	t.Helper()
	if len(content) == 0 {
		t.Fatalf("rendered %s content is empty", name)
	}
	nix := requireManagedExecutable(t, "nix")
	configPath := filepath.Join(t.TempDir(), "config.toml")
	if err := os.WriteFile(configPath, content, 0o600); err != nil {
		t.Fatal(err)
	}
	expression := "builtins.fromTOML (builtins.readFile " + strconv.Quote(configPath) + ")"
	command := exec.CommandContext(t.Context(), nix, "eval", "--impure", "--json", "--expr", expression) // #nosec G204 -- The executable and fixture are controlled by the test.
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("parse %s: %v\n%s", name, err, output)
	}
	decoder := json.NewDecoder(bytes.NewReader(output))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		t.Fatalf("decode parsed %s: %v", name, err)
	}
}

func readManagedNULArguments(t *testing.T, name string) []string {
	t.Helper()
	content, err := os.ReadFile(name)
	if err != nil {
		t.Fatal(err)
	}
	parts := bytes.Split(content, []byte{0})
	if len(parts) > 0 && len(parts[len(parts)-1]) == 0 {
		parts = parts[:len(parts)-1]
	}
	arguments := make([]string, len(parts))
	for i, part := range parts {
		arguments[i] = string(part)
	}
	return arguments
}
