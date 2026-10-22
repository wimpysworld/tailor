package alter

import (
	"encoding/json"
	"os/exec"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/wimpysworld/tailor/internal/config"
	"github.com/wimpysworld/tailor/internal/swatch"
)

func TestManagedBlenderGeneratedFlakeExtensionOutput(t *testing.T) {
	nix := requireManagedExecutable(t, "nix")
	for _, state := range []string{"enabled", "helper missing", "undeclared"} {
		t.Run(state, func(t *testing.T) {
			root := t.TempDir()
			cfg := &config.Config{Swatches: []config.SwatchEntry{{Path: "flake.nix", Alteration: swatch.FirstFit}}}
			if state != "undeclared" {
				cfg.MCP = &config.MCPSettings{Blender: new(true)}
			}
			files := renderSelectedManagedFiles(t, cfg)
			writeManagedRenderFixture(t, root, "flake.nix", files["flake.nix"])
			if state == "enabled" {
				// The helper fails if the root uses another package set.
				writeManagedRenderFixture(t, root, "nix/lib/blender.nix", []byte(`{ pkgs }: assert pkgs.marker == "caller override"; { extension = "paired-extension"; }`))
			}
			if state == "undeclared" && files["nix/lib/blender.nix"] != nil {
				t.Fatal("undeclared Blender generated its helper")
			}
			writeManagedRenderFixture(t, root, "pkgs/default.nix", []byte(`{ system }: {
  marker = "caller override";
  lib.optionalAttrs = condition: attrs: if condition then attrs else {};
}`))
			expression := `let
  flake = import ` + strconv.Quote(filepath.Join(root, "flake.nix")) + `;
  outputs = flake.outputs {
    nixpkgs = {
      outPath = ` + strconv.Quote(filepath.Join(root, "pkgs")) + `;
      lib.genAttrs = names: f: builtins.listToAttrs (map (name: { inherit name; value = f name; }) names);
    };
    nix-packages = {};
  };
in outputs.packages`
			command := exec.CommandContext(t.Context(), nix, "eval", "--offline", "--impure", "--json", "--expr", expression) // #nosec G204 -- The executable and isolated fixture are controlled by the test.
			output, err := command.CombinedOutput()
			if err != nil {
				t.Fatalf("evaluate generated flake packages: %v\n%s", err, output)
			}
			var packages map[string]map[string]string
			if err := json.Unmarshal(output, &packages); err != nil {
				t.Fatal(err)
			}
			for _, system := range []string{"x86_64-linux", "aarch64-linux", "aarch64-darwin"} {
				attributes, ok := packages[system]
				if !ok {
					t.Fatalf("missing package system %s: %s", system, output)
				}
				extension, present := attributes["blender-agent-bridge-extension"]
				if present != (state == "enabled") || present && extension != "paired-extension" {
					t.Fatalf("incorrect conditional extension output: %s", output)
				}
			}
		})
	}
}

func TestManagedBlenderNixUsesOnlySuppliedPackages(t *testing.T) {
	nix := requireManagedExecutable(t, "nix")
	root := t.TempDir()
	rendered := renderSelectedManagedFiles(t, &config.Config{MCP: &config.MCPSettings{Blender: new(true)}})
	for _, path := range []string{"nix/loader.nix", "nix/blender.nix", "nix/lib/blender.nix", "nix/patches/blender-agent-bridge-local-glb.patch"} {
		writeManagedRenderFixture(t, root, path, rendered[path])
	}
	expression := `let pkgs = {
  python3Packages = {
    buildPythonApplication = attrs: attrs;
    setuptools = "supplied-setuptools";
    wheel = "supplied-wheel";
  };
  fetchFromGitHub = attrs: attrs;
  applyPatches = attrs: attrs.src // { patches = attrs.patches; };
}; in import ` + strconv.Quote(filepath.Join(root, "nix/loader.nix")) + ` { inherit pkgs; }`
	command := exec.CommandContext(t.Context(), nix, "eval", "--offline", "--impure", "--json", "--expr", expression) // #nosec G204 -- The executable and isolated fixture are controlled by the test.
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("evaluate Blender fragment: %v\n%s", err, output)
	}
	var packages []struct {
		Pname       string   `json:"pname"`
		Version     string   `json:"version"`
		BuildSystem []string `json:"build-system"`
		Imports     []string `json:"pythonImportsCheck"`
		Source      struct {
			Owner   string   `json:"owner"`
			Repo    string   `json:"repo"`
			Rev     string   `json:"rev"`
			Hash    string   `json:"hash"`
			Patches []string `json:"patches"`
		} `json:"src"`
	}
	if err := json.Unmarshal(output, &packages); err != nil {
		t.Fatal(err)
	}
	if len(packages) != 1 {
		t.Fatalf("package list = %s", output)
	}
	bridge := packages[0]
	if bridge.Pname != "blender-agent-bridge" || bridge.Version != "0.5.6" || bridge.Source.Owner != "CallMeJones" || bridge.Source.Repo != "blender-agent-bridge" || bridge.Source.Rev != "8ef8fc8e0c8220dc812427f9ab89a4634b36cd78" || bridge.Source.Hash != "sha256-5/33dOD0qv5QnPgtsOXgXzw0MrDO2i0iyaSwLzZQ1oc=" {
		t.Fatalf("incorrect source pin: %s", output)
	}
	if len(bridge.Source.Patches) != 1 || filepath.Base(bridge.Source.Patches[0]) != "blender-agent-bridge-local-glb.patch" {
		t.Fatalf("missing shared patch: %s", output)
	}
	if len(bridge.BuildSystem) != 2 || bridge.BuildSystem[0] != "supplied-setuptools" || bridge.BuildSystem[1] != "supplied-wheel" || len(bridge.Imports) != 1 || bridge.Imports[0] != "claude_blender.mcp_runtime.server" {
		t.Fatalf("incorrect Python build contract: %s", output)
	}
}
