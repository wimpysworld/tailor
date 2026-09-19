package alter

import (
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/wimpysworld/tailor/internal/config"
	"github.com/wimpysworld/tailor/internal/swatch"
)

func TestManagedMCPAcceptanceLifecycle(t *testing.T) {
	dir := t.TempDir()
	trueConfig := managedMCPAcceptanceConfig("true")
	before := snapshotManagedTree(t, dir)

	if _, err := Execute(trueConfig, dir, DryRun, managedProductionClient(t), io.Discard, Options{}); err != nil {
		t.Fatal(err)
	}
	if after := snapshotManagedTree(t, dir); !reflect.DeepEqual(after, before) {
		t.Fatalf("preview changed files: before=%v after=%v", before, after)
	}

	if _, err := Execute(trueConfig, dir, Apply, managedProductionClient(t), io.Discard, Options{}); err != nil {
		t.Fatal(err)
	}
	rendered := renderSelectedManagedFiles(t, trueConfig)
	for _, destination := range managedPlaywrightPaths {
		assertManagedBytes(t, filepath.Join(dir, filepath.FromSlash(destination)), rendered[destination])
	}
	for _, destination := range []string{"just/pages.just", "nix/pages.nix"} {
		assertManagedMissing(t, filepath.Join(dir, filepath.FromSlash(destination)))
	}
	applied := snapshotManagedTree(t, dir)

	for _, mode := range []ApplyMode{Apply, Recut} {
		if _, err := Execute(trueConfig, dir, mode, managedProductionClient(t), io.Discard, Options{}); err != nil {
			t.Fatal(err)
		}
		if after := snapshotManagedTree(t, dir); !reflect.DeepEqual(after, applied) {
			t.Fatalf("repeated %s changed converged files: before=%v after=%v", managedModeName(mode), applied, after)
		}
	}

	if _, err := Execute(managedMCPAcceptanceConfig("absent"), dir, Apply, managedProductionClient(t), io.Discard, Options{}); err != nil {
		t.Fatal(err)
	}
	retained := snapshotManagedTree(t, dir)
	for _, destination := range managedPlaywrightPaths {
		if !reflect.DeepEqual(retained[destination], applied[destination]) {
			t.Errorf("true-to-absent changed %q", destination)
		}
	}

	report, err := Execute(managedMCPAcceptanceConfig("false"), dir, Apply, managedProductionClient(t), io.Discard, Options{})
	if err != nil {
		t.Fatal(err)
	}
	assertManagedMissing(t, filepath.Join(dir, "nix/playwright.nix"))
	for _, destination := range managedPlaywrightPaths[1:] {
		after := snapshotManagedTree(t, dir)
		if !reflect.DeepEqual(after[destination], retained[destination]) {
			t.Errorf("false changed protected destination %q", destination)
		}
	}
	if !strings.Contains(report.Plain, "missing `playwright-mcp` executable") {
		t.Fatalf("false report lacks retained-settings warning: %s", report.Plain)
	}
}

func TestManagedMCPAcceptancePreservesExistingClientDestinations(t *testing.T) {
	states := []string{"true", "false", "absent"}
	modes := []ApplyMode{DryRun, Apply, Recut}
	for _, state := range states {
		for _, mode := range modes {
			t.Run(state+"/"+managedModeName(mode), func(t *testing.T) {
				dir := t.TempDir()
				writeManagedTestFile(t, dir, ".mcp.json", []byte("custom malformed Claude settings\n"))
				writeManagedTestFile(t, dir, ".codex/config.toml", []byte("[mcp_servers.playwright]\nenabled = false\ncustom = true\n"))

				outside := filepath.Join(t.TempDir(), "opencode.json")
				if err := os.WriteFile(outside, []byte("custom malformed OpenCode settings\n"), 0o600); err != nil {
					t.Fatal(err)
				}
				managedSymlinkOrSkip(t, outside, filepath.Join(dir, "opencode.json"))
				if err := os.MkdirAll(filepath.Join(dir, ".pi"), 0o755); err != nil {
					t.Fatal(err)
				}
				managedSymlinkOrSkip(t, filepath.Join(t.TempDir(), "missing-pi.json"), filepath.Join(dir, ".pi/mcp.json"))

				seed := managedContent("nix/playwright.nix", "seed")
				writeManagedTestFile(t, dir, "nix/playwright.nix", seed)
				before := snapshotManagedTree(t, dir)
				cfg := managedMCPAcceptanceConfig(state)
				report, err := Execute(cfg, dir, mode, managedProductionClient(t), io.Discard, Options{})
				if err != nil {
					t.Fatal(err)
				}
				after := snapshotManagedTree(t, dir)

				for _, destination := range managedPlaywrightPaths[1:] {
					if !reflect.DeepEqual(after[destination], before[destination]) {
						t.Errorf("%s changed protected destination %q", state, destination)
					}
				}
				assertManagedBytes(t, outside, []byte("custom malformed OpenCode settings\n"))

				if mode == DryRun {
					if !reflect.DeepEqual(after, before) {
						t.Fatalf("%s preview changed files: before=%v after=%v", state, before, after)
					}
					return
				}
				switch state {
				case "true":
					assertManagedBytes(t, filepath.Join(dir, "nix/playwright.nix"), renderSelectedManagedFiles(t, cfg)["nix/playwright.nix"])
					for _, destination := range managedPlaywrightPaths[1:] {
						if !containsGuidancePath(report.Document.Guidance, destination) {
							t.Errorf("report lacks preservation guidance for %q", destination)
						}
					}
				case "false":
					assertManagedMissing(t, filepath.Join(dir, "nix/playwright.nix"))
					if !strings.Contains(report.Plain, "missing `playwright-mcp` executable") {
						t.Errorf("false report lacks retained-settings warning: %s", report.Plain)
					}
				case "absent":
					assertManagedBytes(t, filepath.Join(dir, "nix/playwright.nix"), seed)
				}
			})
		}
	}
}

func managedMCPAcceptanceConfig(state string) *config.Config {
	cfg := &config.Config{
		License: "none",
		Swatches: []config.SwatchEntry{
			{Path: "justfile", Alteration: swatch.Always},
			{Path: "flake.nix", Alteration: swatch.Always},
		},
	}
	if state != "absent" {
		enabled := state == "true"
		cfg.MCP = &config.MCPSettings{Playwright: &enabled}
	}
	return cfg
}
