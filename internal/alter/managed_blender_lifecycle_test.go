package alter

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/wimpysworld/tailor/internal/config"
	"github.com/wimpysworld/tailor/internal/swatch"
)

func TestManagedBlenderPreservesFlakeAndReportsManualOutput(t *testing.T) {
	for _, mode := range []ApplyMode{DryRun, Apply, Recut} {
		for _, state := range []string{"true", "false", "absent"} {
			t.Run(managedModeName(mode)+"/"+state, func(t *testing.T) {
				root := t.TempDir()
				content := []byte("# project output\r\n{ packages.custom = 42; }\n")
				writeManagedTestFile(t, root, "flake.nix", content)
				cfg := &config.Config{License: "none", Swatches: []config.SwatchEntry{{Path: "flake.nix", Alteration: swatch.Always}}}
				if state != "absent" {
					cfg.MCP = &config.MCPSettings{Blender: new(state == "true")}
				}
				report, err := Execute(cfg, root, mode, nil, io.Discard, Options{})
				if err != nil {
					t.Fatal(err)
				}
				assertManagedBytes(t, filepath.Join(root, "flake.nix"), content)
				guidance := report.Plain
				if strings.Contains(guidance, "blender-agent-bridge-extension") != (state == "true") {
					t.Fatalf("extension guidance disagrees with declaration: %s", guidance)
				}
				if state == "true" {
					for _, want := range []string{
						"per-system packages", "blender-agent-bridge-extension = (import ./nix/lib/blender.nix { inherit pkgs; }).extension;",
						"pkgs.lib.optionalAttrs (builtins.pathExists ./nix/lib/blender.nix)", "same overridden `pkgs`",
						"nix build .#blender-agent-bridge-extension", "Install the ZIP separately after approval",
					} {
						if !strings.Contains(guidance, want) {
							t.Errorf("guidance lacks %q: %s", want, guidance)
						}
					}
				}
			})
		}
	}
}

func TestManagedBlenderSupportRejectsUnsafePathsBeforeWrites(t *testing.T) {
	for _, state := range []bool{true, false} {
		for _, path := range []string{"nix/lib/blender.nix", "nix/patches/blender-agent-bridge-local-glb.patch"} {
			for _, conflict := range []string{"symlink parent", "unowned file"} {
				t.Run(path+"/"+conflict+"/"+map[bool]string{true: "enabled", false: "disabled"}[state], func(t *testing.T) {
					root, outside := t.TempDir(), t.TempDir()
					writeManagedTestFile(t, outside, filepath.Base(path), []byte("external content\n"))
					if conflict == "symlink parent" {
						if err := os.MkdirAll(filepath.Join(root, "nix"), 0o755); err != nil {
							t.Fatal(err)
						}
						managedSymlinkOrSkip(t, outside, filepath.Join(root, filepath.Dir(path)))
					} else {
						writeManagedTestFile(t, root, path, []byte("project content\n"))
					}
					before, externalBefore := snapshotManagedTree(t, root), snapshotManagedTree(t, outside)
					cfg := &config.Config{MCP: &config.MCPSettings{Blender: new(state)}}
					execution, err := prepareManagedExecution(cfg, root, func(s []managedSelection) (managedRenderedFiles, error) {
						return renderManagedFiles(cfg, s)
					})
					if execution != nil || err == nil {
						t.Fatalf("unsafe destination accepted: execution=%v error=%v", execution, err)
					}
					if conflict == "symlink parent" {
						if !strings.Contains(err.Error(), "is a symlink") {
							t.Fatalf("wrong parent error: %v", err)
						}
					} else {
						var ownershipErr *managedOwnershipError
						if !errors.As(err, &ownershipErr) || ownershipErr.Path != path {
							t.Fatalf("wrong ownership error: %v", err)
						}
					}
					if !reflect.DeepEqual(before, snapshotManagedTree(t, root)) || !reflect.DeepEqual(externalBefore, snapshotManagedTree(t, outside)) {
						t.Fatal("failed preflight changed project or external files")
					}
				})
			}
		}
	}
}

func TestManagedBlenderSupportLifecycle(t *testing.T) {
	paths := []string{"nix/blender.nix", "nix/lib/blender.nix", "nix/patches/blender-agent-bridge-local-glb.patch"}
	for _, state := range []string{"absent", "false", "true", "create"} {
		t.Run(state, func(t *testing.T) {
			root := t.TempDir()
			if state != "create" {
				for _, path := range paths {
					writeManagedTestFile(t, root, path, managedContent(path, "old"))
				}
			}
			writeManagedTestFile(t, root, "flake.nix", []byte("project root\n"))
			cfg := &config.Config{}
			if state != "absent" {
				cfg.MCP = &config.MCPSettings{Blender: new(state == "true" || state == "create")}
			}
			before := snapshotManagedTree(t, root)
			execution, err := prepareManagedExecution(cfg, root, func(s []managedSelection) (managedRenderedFiles, error) {
				return renderManagedFiles(cfg, s)
			})
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(before, snapshotManagedTree(t, root)) {
				t.Fatal("preview changed files")
			}
			if _, err := applyManagedFiles(root, execution.plan); err != nil {
				t.Fatal(err)
			}
			for _, path := range paths {
				content, err := os.ReadFile(filepath.Join(root, path))
				if state == "false" {
					if !errors.Is(err, os.ErrNotExist) {
						t.Fatalf("%s retained: %v", path, err)
					}
					continue
				}
				if err != nil {
					t.Fatal(err)
				}
				if state == "absent" && string(content) != string(managedContent(path, "old")) {
					t.Fatalf("absent declaration changed %s", path)
				}
				if (state == "true" || state == "create") && string(content) != string(renderSelectedManagedFiles(t, cfg)[path]) {
					t.Fatalf("enabled declaration did not update %s", path)
				}
			}
			assertManagedFileContent(t, filepath.Join(root, "flake.nix"), "project root\n")
			for _, loader := range []string{"nix/loader.nix", "nix/hooks.nix"} {
				content := renderSelectedManagedFiles(t, cfg)[loader]
				if strings.Contains(string(content), "./lib/") || strings.Contains(string(content), "./patches/") {
					t.Fatalf("%s imports support files", loader)
				}
			}
		})
	}
}
