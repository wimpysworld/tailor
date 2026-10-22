package alter

import (
	"encoding/json"
	"os/exec"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/wimpysworld/tailor/internal/config"
)

func TestManagedBlenderNixUsesOnlySuppliedPackages(t *testing.T) {
	nix := requireManagedExecutable(t, "nix")
	root := t.TempDir()
	rendered := renderSelectedManagedFiles(t, &config.Config{MCP: &config.MCPSettings{Blender: new(true)}})
	for _, path := range []string{"nix/loader.nix", "nix/blender.nix"} {
		writeManagedRenderFixture(t, root, path, rendered[path])
	}
	expression := `let pkgs = {
  python3Packages = {
    buildPythonApplication = attrs: attrs;
    setuptools = "supplied-setuptools";
    wheel = "supplied-wheel";
  };
  fetchFromGitHub = attrs: attrs;
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
			Owner string `json:"owner"`
			Repo  string `json:"repo"`
			Rev   string `json:"rev"`
			Hash  string `json:"hash"`
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
	if len(bridge.BuildSystem) != 2 || bridge.BuildSystem[0] != "supplied-setuptools" || bridge.BuildSystem[1] != "supplied-wheel" || len(bridge.Imports) != 1 || bridge.Imports[0] != "claude_blender.mcp_runtime.server" {
		t.Fatalf("incorrect Python build contract: %s", output)
	}
}
