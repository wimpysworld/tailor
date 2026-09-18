package alter

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/wimpysworld/tailor/internal/swatch"
)

func TestManagedPlaywrightNixFixture(t *testing.T) {
	nix := requireManagedExecutable(t, "nix")
	root := managedPlaywrightNixFixture(t)
	flakePath := "path:" + filepath.ToSlash(root)

	t.Run("evaluate declared systems", func(t *testing.T) {
		check := exec.CommandContext(t.Context(), nix, "flake", "check", "--all-systems", "--no-build", "--no-write-lock-file", flakePath) // #nosec G204 -- The executable and isolated fixture are controlled by the test.
		if output, err := check.CombinedOutput(); err != nil {
			t.Fatalf("nix flake check: %v\n%s", err, output)
		}
	})

	t.Run("build native package", func(t *testing.T) {
		hostExpression := `let
  fixture = builtins.getFlake "` + flakePath + `";
  system = builtins.currentSystem;
in
{
  inherit system;
  declared = builtins.hasAttr system fixture.devShells;
}`
		hostCommand := exec.CommandContext(t.Context(), nix, "eval", "--impure", "--json", "--expr", hostExpression) // #nosec G204 -- The executable, expression, and isolated fixture are controlled by the test.
		hostOutput, err := hostCommand.CombinedOutput()
		if err != nil {
			t.Fatalf("nix eval host system: %v\n%s", err, hostOutput)
		}
		var host struct {
			System   string `json:"system"`
			Declared bool   `json:"declared"`
		}
		if err := json.Unmarshal(hostOutput, &host); err != nil {
			t.Fatalf("decode Nix host system %q: %v", hostOutput, err)
		}
		if !host.Declared {
			t.Skipf("native Playwright Nix build is not declared for %s", host.System)
		}

		buildContext, cancel := context.WithTimeout(t.Context(), 4*time.Minute)
		defer cancel()
		expression := `let
  fixture = builtins.getFlake "` + flakePath + `";
  pkgs = import fixture.inputs.nixpkgs { system = builtins.currentSystem; };
in
builtins.head (import ` + strconv.Quote(filepath.Join(root, "nix", "playwright.nix")) + ` { inherit pkgs; })`
		build := exec.CommandContext(buildContext, nix, "build", "--impure", "--no-link", "--no-write-lock-file", "--print-out-paths", "--expr", expression) // #nosec G204 -- The executable, expression, and isolated fixture are controlled by the test.
		var stdout, stderr bytes.Buffer
		build.Stdout = &stdout
		build.Stderr = &stderr
		if err := build.Run(); err != nil {
			t.Fatalf("nix build: %v\n%s", err, stderr.Bytes())
		}
		packagePath := strings.TrimSpace(stdout.String())
		if packagePath == "" || strings.Contains(packagePath, "\n") {
			t.Fatalf("nix build returned an invalid package path %q", packagePath)
		}

		wrapper, err := os.ReadFile(filepath.Join(packagePath, "bin", "playwright-mcp"))
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Contains(wrapper, []byte("PLAYWRIGHT_BROWSERS_PATH")) || !bytes.Contains(wrapper, []byte("playwright-browsers")) {
			t.Fatalf("playwright-mcp wrapper lacks its browser path:\n%s", wrapper)
		}
		assertManagedPlaywrightClosure(t, nix, packagePath)
	})
}

func managedPlaywrightNixFixture(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	for _, name := range []string{"flake.nix", "nix/loader.nix", "nix/playwright.nix"} {
		content, err := swatch.Content(name)
		if err != nil {
			t.Fatal(err)
		}
		if name == "nix/loader.nix" {
			content, err = renderManagedLoader(name, content, []managedRegistryEntry{
				{Path: "nix/loader.nix", Policy: managedPolicyLoader},
				{Path: "nix/playwright.nix", Policy: managedPolicyFragment, Capability: managedCapabilityPlaywright},
			})
			if err != nil {
				t.Fatal(err)
			}
		}
		writeManagedRenderFixture(t, root, name, content)
	}

	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("locating repository flake.lock")
	}
	lock, err := os.ReadFile(filepath.Join(filepath.Dir(filename), "..", "..", "flake.lock"))
	if err != nil {
		t.Fatal(err)
	}
	writeManagedRenderFixture(t, root, "flake.lock", lock)
	return root
}

func assertManagedPlaywrightClosure(t *testing.T, nix, packagePath string) {
	t.Helper()
	query := exec.CommandContext(t.Context(), nix, "path-info", "--recursive", packagePath) // #nosec G204 -- The executable and built package path are controlled by the test.
	output, err := query.CombinedOutput()
	if err != nil {
		t.Fatalf("nix path-info: %v\n%s", err, output)
	}
	closure := strings.ToLower(string(output))
	for _, unwanted := range []string{"playwright-firefox", "playwright-webkit", "chromium-headless-shell"} {
		if strings.Contains(closure, unwanted) {
			t.Errorf("runtime closure contains %s:\n%s", unwanted, output)
		}
	}
	if !strings.Contains(closure, "playwright-chromium") {
		t.Fatalf("runtime closure lacks playwright-chromium:\n%s", output)
	}
}
