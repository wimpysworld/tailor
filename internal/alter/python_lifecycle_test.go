package alter

import (
	"bytes"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/wimpysworld/tailor/internal/config"
)

func TestPythonNixInterpreter(t *testing.T) {
	nix, err := exec.LookPath("nix")
	if err != nil {
		t.Skip("nix required")
	}
	root := t.TempDir()
	cfg := &config.Config{Languages: &config.LanguageSettings{Python: new(true)}}
	files := renderSelectedManagedFiles(t, cfg)
	writeManagedRenderFixture(t, root, "nix/python.nix", files["nix/python.nix"])
	expression := `let pkgs = rec {
  python3 = { identity = "selected"; withPackages = f:
    assert f { packaging = "packaging"; toPythonModule = x: x; } == [ "packaging" "audit" ];
    "/managed-python"; };
  pip-audit.override = args: assert args.python3.identity == "selected"; "audit";
  writeShellScriptBin = name: script:
    assert name == "tailor-python";
    assert builtins.match ".*exec /managed-python/bin/python3.*" script != null;
    name;
  uv = "uv"; ruff = "ruff"; basedpyright = "basedpyright";
}; in import ` + strconv.Quote(filepath.Join(root, "nix/python.nix")) + ` { inherit pkgs; }`
	output, err := exec.CommandContext(t.Context(), nix, "eval", "--offline", "--impure", "--json", "--expr", expression).CombinedOutput()
	if err != nil {
		t.Fatalf("Python package eval: %v %s", err, output)
	}
	if !strings.Contains(string(output), `"tailor-python"`) {
		t.Fatal(string(output))
	}
}

func TestPythonLifecycleAndOrder(t *testing.T) {
	for _, state := range []*bool{nil, new(false), new(true)} {
		cfg := &config.Config{Languages: &config.LanguageSettings{Python: state}}
		selections, err := selectManagedFiles(cfg)
		if err != nil {
			t.Fatal(err)
		}
		count := 0
		for _, selection := range selections {
			if selection.Entry.Capability == managedCapabilityPython {
				count++
				if state == nil || selection.Enabled != *state {
					t.Fatal("incorrect Python lifecycle")
				}
			}
		}
		if state == nil && count != 0 || state != nil && count != 2 {
			t.Fatalf("Python fragments: %d", count)
		}
		rendered := renderSelectedManagedFiles(t, cfg)
		if bytes.Contains(rendered["just/tailor.just"], []byte("uv lock --check")) != (state != nil && *state) {
			t.Fatal("incorrect Python activation")
		}
	}
	cfg := &config.Config{Languages: &config.LanguageSettings{Go: new(true), Python: new(true), Rust: new(true)}}
	body := string(renderSelectedManagedFiles(t, cfg)["just/tailor.just"])
	_, body, found := strings.Cut(body, "lint $mode")
	if !found {
		t.Fatal("missing lint recipe")
	}
	goIndex, pythonIndex, rustIndex := strings.Index(body, "go mod tidy -diff"), strings.Index(body, "ruff format --check ."), strings.Index(body, "cargo fmt --all -- --check")
	if goIndex < 0 || goIndex >= pythonIndex || pythonIndex >= rustIndex {
		t.Fatal("incorrect Go/Python/Rust order")
	}
	if strings.Count(body, "    check_workflows\n") != 1 {
		t.Fatal("workflow lint does not run once")
	}
	if strings.Index(body, "project_script=") < rustIndex {
		t.Fatal("project hook precedes managed checks")
	}
}
