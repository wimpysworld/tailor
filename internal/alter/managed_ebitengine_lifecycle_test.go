package alter

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/wimpysworld/tailor/internal/config"
	"github.com/wimpysworld/tailor/internal/swatch"
)

func TestEbitengineOwnedLifecycle(t *testing.T) {
	paths := []string{"nix/go-ebitengine.nix", "nix/lib/go-ebitengine-runtime.nix", "nix/hooks/go-ebitengine.nix", "nix/shells/go-ebitengine-android.nix"}
	for _, state := range []string{"absent", "false", "go false", "default", "all"} {
		t.Run(state, func(t *testing.T) {
			root := t.TempDir()
			for _, path := range paths {
				writeManagedTestFile(t, root, path, managedContent(path, "old"))
			}
			for _, path := range []string{"justfile", "flake.nix"} {
				writeManagedTestFile(t, root, path, []byte("project-owned root\n"))
			}
			cfg := ebitengineConfig()
			switch state {
			case "absent":
				cfg.Languages.GoOptions.Ebitengine = nil
			case "false":
				cfg.Languages.GoOptions.Ebitengine = &config.EbitengineOptions{Enabled: false}
			case "go false":
				cfg.Languages = &config.LanguageSettings{Go: new(false)}
			case "default":
				cfg.Languages.GoOptions.Ebitengine = &config.EbitengineOptions{Enabled: true}
			}
			cfg.Swatches = []config.SwatchEntry{{Path: "justfile", Alteration: swatch.Always}, {Path: "flake.nix", Alteration: swatch.Always}}
			before := snapshotManagedTree(t, root)
			execution, err := prepareManagedExecution(cfg, root, func(s []managedSelection) (managedRenderedFiles, error) { return renderManagedFiles(cfg, s) })
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
				removed := state == "false" || state == "go false" || state == "default" && strings.Contains(path, "/shells/")
				if removed {
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
			}
			for _, path := range []string{"justfile", "flake.nix"} {
				assertManagedFileContent(t, filepath.Join(root, path), "project-owned root\n")
			}
			after := snapshotManagedTree(t, root)
			execution, err = prepareManagedExecution(cfg, root, func(s []managedSelection) (managedRenderedFiles, error) { return renderManagedFiles(cfg, s) })
			if err != nil {
				t.Fatal(err)
			}
			if _, err := applyManagedFiles(root, execution.plan); err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(after, snapshotManagedTree(t, root)) {
				t.Fatal("second apply changed files")
			}
		})
	}
}

func TestEbitengineOwnershipAndParents(t *testing.T) {
	for _, path := range []string{"nix/go-ebitengine.nix", "nix/lib/go-ebitengine-runtime.nix", "nix/hooks/go-ebitengine.nix", "nix/shells/go-ebitengine-android.nix"} {
		for _, enabled := range []bool{false, true} {
			t.Run(path+"/"+map[bool]string{false: "disabled", true: "enabled"}[enabled], func(t *testing.T) {
				root := t.TempDir()
				writeManagedTestFile(t, root, path, []byte("user content\n"))
				cfg := ebitengineConfig()
				if !enabled {
					cfg.Languages.GoOptions.Ebitengine = &config.EbitengineOptions{Enabled: false}
				}
				before := snapshotManagedTree(t, root)
				_, err := prepareManagedExecution(cfg, root, func(s []managedSelection) (managedRenderedFiles, error) { return renderManagedFiles(cfg, s) })
				var conflict *managedOwnershipError
				if !errors.As(err, &conflict) || conflict.Path != path {
					t.Fatalf("expected ownership conflict: %v", err)
				}
				if !reflect.DeepEqual(before, snapshotManagedTree(t, root)) {
					t.Fatal("conflict changed files")
				}
			})
		}
	}
	for _, parent := range []string{"nix/lib", "nix/hooks", "nix/shells"} {
		t.Run(parent, func(t *testing.T) {
			root, outside := t.TempDir(), t.TempDir()
			if err := os.MkdirAll(filepath.Join(root, "nix"), 0o700); err != nil {
				t.Fatal(err)
			}
			managedSymlinkOrSkip(t, outside, filepath.Join(root, parent))
			before := snapshotManagedTree(t, root)
			_, err := prepareManagedExecution(ebitengineConfig(), root, func(s []managedSelection) (managedRenderedFiles, error) {
				return renderManagedFiles(ebitengineConfig(), s)
			})
			if err == nil || !strings.Contains(err.Error(), "symlink") {
				t.Fatalf("expected symlink rejection: %v", err)
			}
			if !reflect.DeepEqual(before, snapshotManagedTree(t, root)) || len(snapshotManagedTree(t, outside)) != 0 {
				t.Fatal("unsafe parent changed files")
			}
		})
	}
}
