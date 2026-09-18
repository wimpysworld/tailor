package alter_test

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/wimpysworld/tailor/internal/alter"
)

func TestMCPDeclarationsAreInertAcrossAlterModes(t *testing.T) {
	declarations := []struct {
		name string
		yaml string
	}{
		{name: "absent"},
		{name: "false", yaml: "mcp:\n  playwright: false\n"},
		{name: "true", yaml: "mcp:\n  playwright: true\n"},
	}
	modes := []struct {
		name string
		mode alter.ApplyMode
	}{
		{name: "baste", mode: alter.DryRun},
		{name: "apply", mode: alter.Apply},
		{name: "recut", mode: alter.Recut},
	}
	providers := map[string]string{
		".mcp.json":             `{"mcpServers":{"existing":{"command":"keep-root"}}}`,
		".claude/settings.json": `{"mcpServers":{"existing":{"command":"keep-claude"}}}`,
		".codex/config.toml":    "[mcp_servers.existing]\ncommand = \"keep-codex\"\n",
		".cursor/mcp.json":      `{"mcpServers":{"existing":{"command":"keep-cursor"}}}`,
		".gemini/settings.json": `{"mcpServers":{"existing":{"command":"keep-gemini"}}}`,
		".pi/mcp.json":          `{"mcpServers":{"existing":{"command":"keep-pi"}}}`,
		"flake.nix":             "# keep flake root\n",
		"justfile":              "# keep just root\n",
		"opencode.json":         `{"mcp":{"existing":{"command":"keep-opencode"}}}`,
	}

	for _, mode := range modes {
		t.Run(mode.name, func(t *testing.T) {
			var baselineOutput string
			var baselineCalls []apiCall
			for i, declaration := range declarations {
				t.Run(declaration.name, func(t *testing.T) {
					configYAML := "license: none\n" + declaration.yaml + "swatches: []\n"
					tc := setupAlterTest(t, configYAML)
					for path, content := range providers {
						writeOnDisk(t, tc.Dir, path, []byte(content))
					}
					before := snapshotCapabilityFiles(t, tc.Dir)

					cfg := loadTestConfig(t, tc.Dir)
					output := captureAlterRun(t, cfg, tc.Dir, mode.mode, tc.Client)
					after := snapshotCapabilityFiles(t, tc.Dir)
					if !reflect.DeepEqual(after, before) {
						t.Fatalf("MCP declaration changed project files: before=%v after=%v", before, after)
					}
					for path, content := range providers {
						if got := after[path]; got != content {
							t.Errorf("provider file %s = %q, want %q", path, got, content)
						}
					}
					if strings.Contains(strings.ToLower(output), "mcp") || strings.Contains(strings.ToLower(output), "playwright") {
						t.Errorf("alter output contains inert MCP tooling: %s", output)
					}
					if calls := tc.MutatingCalls(); len(calls) != 0 {
						t.Fatalf("MCP declaration caused mutating API calls: %v", calls)
					}

					calls := tc.Calls()
					if i == 0 {
						baselineOutput = output
						baselineCalls = calls
						return
					}
					if output != baselineOutput {
						t.Errorf("%s output differs from absent MCP output\ngot:\n%s\nwant:\n%s", declaration.name, output, baselineOutput)
					}
					if !reflect.DeepEqual(calls, baselineCalls) {
						t.Errorf("%s API calls = %v, want %v", declaration.name, calls, baselineCalls)
					}
				})
			}
		})
	}
}

func snapshotCapabilityFiles(t *testing.T, root string) map[string]string {
	t.Helper()
	files := make(map[string]string)
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			return nil
		}
		relative, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		relative = filepath.ToSlash(relative)
		content, err := fs.ReadFile(os.DirFS(root), relative)
		if err != nil {
			return err
		}
		files[relative] = string(content)
		return nil
	})
	if err != nil {
		t.Fatal(fmt.Errorf("snapshot project files: %w", err))
	}
	return files
}
