package alter

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/wimpysworld/tailor/internal/config"
	"github.com/wimpysworld/tailor/internal/swatch"
)

func localFixture(t *testing.T) (string, *config.Config, []string) {
	t.Helper()
	dir := t.TempDir()
	cfg := &config.Config{
		Languages: &config.LanguageSettings{Go: new(true)},
		Swatches:  []config.SwatchEntry{{Path: dependabotPath, Alteration: swatch.FirstFit}},
	}
	paths := []string{dependabotPath, "nix/hooks/go.nix", "just/tailor.just"}
	for _, name := range paths {
		writeManagedTestFile(t, dir, name, []byte(managedMarker(name)+"\nold\n"))
	}
	writeManagedTestFile(t, dir, ".tailor.yml", []byte("must stay unchanged\n"))
	writeManagedTestFile(t, dir, "unrelated", []byte("keep\n"))
	return dir, cfg, paths
}

func TestApplyLocalMatchesNormalRenderers(t *testing.T) {
	dir, cfg, paths := localFixture(t)
	before := snapshotManagedTree(t, dir)
	want := renderSelectedManagedFiles(t, cfg)
	plan, err := prepareDependabotExecution(cfg, dir)
	if err != nil {
		t.Fatal(err)
	}
	want[dependabotPath] = plan.Output
	updated, err := ApplyLocal(cfg, dir, paths)
	if err != nil || !reflect.DeepEqual(updated, paths) {
		t.Fatalf("apply: %v %v", updated, err)
	}
	for _, name := range paths {
		got, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil || !bytes.Equal(got, want[name]) {
			t.Fatalf("output %s differs from normal renderer: %v", name, err)
		}
	}
	after := snapshotManagedTree(t, dir)
	for _, name := range paths {
		delete(before, name)
		delete(after, name)
	}
	if !reflect.DeepEqual(before, after) {
		t.Fatal("changed unrelated files")
	}
	updated, err = ApplyLocal(cfg, dir, paths)
	if err != nil || len(updated) != 0 {
		t.Fatalf("second apply: %v %v", updated, err)
	}
}

func TestApplyLocalRejectsBeforeAnyWrite(t *testing.T) {
	for _, name := range []string{"empty", "unknown", "traversal", "duplicate", "missing", "unowned", "symlink", "parent-symlink", "directory", "never", "absent", "disabled-go", "alternate", "invalid-mode"} {
		t.Run(name, func(t *testing.T) {
			dir, cfg, paths := localFixture(t)
			destination := filepath.Join(dir, "just/tailor.just")
			switch name {
			case "empty":
				paths = nil
			case "unknown":
				paths = append(paths, "SECURITY.md")
			case "traversal":
				paths = append(paths, "../just/tailor.just")
			case "duplicate":
				paths = append(paths, paths[0])
			case "missing", "symlink", "directory":
				if err := os.Remove(destination); err != nil {
					t.Fatal(err)
				}
				if name == "symlink" {
					managedSymlinkOrSkip(t, filepath.Join(dir, "unrelated"), destination)
				}
				if name == "directory" {
					if err := os.Mkdir(destination, 0o755); err != nil {
						t.Fatal(err)
					}
				}
			case "parent-symlink":
				if err := os.Rename(filepath.Join(dir, "just"), filepath.Join(dir, "original")); err != nil {
					t.Fatal(err)
				}
				managedSymlinkOrSkip(t, filepath.Join(dir, "original"), filepath.Join(dir, "just"))
			case "unowned":
				writeManagedTestFile(t, dir, "just/tailor.just", []byte("custom\n"))
			case "never":
				cfg.Swatches[0].Alteration = swatch.Never
			case "absent":
				cfg.Swatches = nil
			case "disabled-go":
				cfg.Languages.Go = new(false)
			case "alternate":
				writeManagedTestFile(t, dir, dependabotAlternatePath, []byte("custom\n"))
			case "invalid-mode":
				cfg.Swatches[0].Alteration = "invalid"
			}
			before := snapshotManagedTree(t, dir)
			if _, err := ApplyLocal(cfg, dir, paths); err == nil {
				t.Fatal("accepted invalid local request")
			}
			if !reflect.DeepEqual(before, snapshotManagedTree(t, dir)) {
				t.Fatal("failed preflight changed files")
			}
		})
	}
}

func TestLocalManagedInspectionErrors(t *testing.T) {
	for _, name := range []string{"just/tailor.just", "nix/hooks/go.nix"} {
		for _, stage := range []string{"preflight", "recheck"} {
			t.Run(name+"/"+stage, func(t *testing.T) {
				dir, cfg, _ := localFixture(t)
				root, err := os.OpenRoot(dir)
				if err != nil {
					t.Fatal(err)
				}
				defer root.Close()
				file, err := prepareLocalFile(root, cfg, dir, name)
				if err != nil {
					t.Fatal(err)
				}
				defer file.parent.Close()
				writeManagedTestFile(t, dir, name, bytes.Repeat([]byte("x"), dependabotMaxBytes+1))
				if stage == "preflight" {
					_, err = ApplyLocal(cfg, dir, []string{name})
				} else {
					err = file.recheck(root)
				}
				if err == nil || !strings.Contains(err.Error(), name) || !strings.Contains(err.Error(), "exceeds 1048576 bytes") || strings.Contains(strings.ToLower(err.Error()), "dependabot") {
					t.Fatalf("unexpected managed inspection error: %v", err)
				}
				var cause *dependabotConflictError
				if !errors.As(err, &cause) {
					t.Fatalf("inspection error lost its cause: %v", err)
				}
			})
		}
	}
}

func TestLocalFileRejectsChangesAfterPreflight(t *testing.T) {
	for _, change := range []string{"content", "symlink", "parent"} {
		t.Run(change, func(t *testing.T) {
			dir, cfg, _ := localFixture(t)
			root, err := os.OpenRoot(dir)
			if err != nil {
				t.Fatal(err)
			}
			defer root.Close()
			file, err := prepareLocalFile(root, cfg, dir, "just/tailor.just")
			if err != nil {
				t.Fatal(err)
			}
			defer file.parent.Close()
			switch change {
			case "content":
				writeManagedTestFile(t, dir, file.name, []byte("changed\n"))
			case "symlink":
				if err := os.Remove(filepath.Join(dir, file.name)); err != nil {
					t.Fatal(err)
				}
				managedSymlinkOrSkip(t, filepath.Join(dir, "unrelated"), filepath.Join(dir, file.name))
			case "parent":
				if err := os.Rename(filepath.Join(dir, "just"), filepath.Join(dir, "old")); err != nil {
					t.Fatal(err)
				}
				writeManagedTestFile(t, dir, file.name, file.before.Content)
			}
			if err := file.recheck(root); err == nil {
				t.Fatal("accepted changed destination")
			}
		})
	}
}
