package alter

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/wimpysworld/tailor/internal/config"
)

type managedTreeEntry struct {
	Mode    os.FileMode
	Content string
	Target  string
}

func TestPreflightManagedFilesSnapshotsEveryDestinationTypeReadOnly(t *testing.T) {
	const destination = "nested/managed"
	desired := managedContent(destination, "current")
	tests := []struct {
		name      string
		setup     func(*testing.T, string)
		want      managedOperation
		wantError string
	}{
		{
			name: "missing",
			want: managedOperationWrite,
		},
		{
			name: "regular",
			setup: func(t *testing.T, dir string) {
				writeManagedTestFile(t, dir, destination, desired)
			},
			want: managedOperationNoop,
		},
		{
			name: "final symlink",
			setup: func(t *testing.T, dir string) {
				if err := os.MkdirAll(filepath.Join(dir, "nested"), 0o755); err != nil {
					t.Fatal(err)
				}
				managedSymlinkOrSkip(t, filepath.Join(t.TempDir(), "outside"), filepath.Join(dir, destination))
			},
			want: managedOperationWrite,
		},
		{
			name: "directory",
			setup: func(t *testing.T, dir string) {
				if err := os.MkdirAll(filepath.Join(dir, destination), 0o755); err != nil {
					t.Fatal(err)
				}
			},
			wantError: "is a directory",
		},
		{
			name: "special file",
			setup: func(t *testing.T, dir string) {
				if err := os.MkdirAll(filepath.Join(dir, "nested"), 0o755); err != nil {
					t.Fatal(err)
				}
				managedFIFOOrSkip(t, filepath.Join(dir, destination))
			},
			wantError: "is not a regular file or symlink",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			if tt.setup != nil {
				tt.setup(t, dir)
			}
			before := snapshotManagedTree(t, dir)
			selection := managedSelection{
				Entry:   managedRegistryEntry{Path: destination, Policy: managedPolicyLoader},
				Enabled: true,
			}

			plan, err := preflightManagedFiles(
				dir,
				[]managedSelection{selection},
				managedRenderedFiles{destination: desired},
			)
			if tt.wantError != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantError) {
					t.Fatalf("preflightManagedFiles() error = %v, want substring %q", err, tt.wantError)
				}
			} else {
				if err != nil {
					t.Fatal(err)
				}
				if got := plan.Files[0].Operation; got != tt.want {
					t.Fatalf("operation = %d, want %d", got, tt.want)
				}
			}
			if after := snapshotManagedTree(t, dir); !reflect.DeepEqual(after, before) {
				t.Fatalf("preflight changed project tree: before=%v after=%v", before, after)
			}
		})
	}
}

func TestPreflightManagedFilesPreservesProtectedDestinations(t *testing.T) {
	tests := []struct {
		name  string
		setup func(*testing.T, string)
	}{
		{
			name: "regular file",
			setup: func(t *testing.T, dir string) {
				writeManagedTestFile(t, dir, "protected", []byte("custom content"))
			},
		},
		{
			name: "dangling final symlink",
			setup: func(t *testing.T, dir string) {
				managedSymlinkOrSkip(t, filepath.Join(t.TempDir(), "missing"), filepath.Join(dir, "protected"))
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			tt.setup(t, dir)
			before := snapshotManagedTree(t, dir)
			selection := managedSelection{
				Entry:   managedRegistryEntry{Path: "protected", Policy: managedPolicyRoot},
				Enabled: true,
			}

			plan, err := preflightManagedFiles(
				dir,
				[]managedSelection{selection},
				managedRenderedFiles{"protected": []byte("generated")},
			)
			if err != nil {
				t.Fatal(err)
			}
			if got := plan.Files[0].Operation; got != managedOperationPreserve {
				t.Fatalf("operation = %d, want preserve", got)
			}
			if after := snapshotManagedTree(t, dir); !reflect.DeepEqual(after, before) {
				t.Fatalf("preflight changed protected destination: before=%v after=%v", before, after)
			}
		})
	}
}

func TestPreflightManagedFilesIgnoresAbsentCapabilities(t *testing.T) {
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, ".mcp.json"), 0o755); err != nil {
		t.Fatal(err)
	}
	writeManagedTestFile(t, dir, "nix/playwright.nix", []byte("unmarked"))
	before := snapshotManagedTree(t, dir)

	selections, err := selectManagedFiles(&config.Config{})
	if err != nil {
		t.Fatal(err)
	}
	rendered := make(managedRenderedFiles, len(selections))
	for _, selection := range selections {
		rendered[selection.Entry.Path] = managedContent(selection.Entry.Path, "generated")
	}

	plan, err := preflightManagedFiles(dir, selections, rendered)
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Files) != 3 {
		t.Fatalf("planned files = %d, want 3 always-reconciled files", len(plan.Files))
	}
	if after := snapshotManagedTree(t, dir); !reflect.DeepEqual(after, before) {
		t.Fatalf("preflight changed unselected destinations: before=%v after=%v", before, after)
	}
}

func TestPreflightManagedFilesRejectsUnsafeParentReadOnly(t *testing.T) {
	dir := t.TempDir()
	outside := t.TempDir()
	writeManagedTestFile(t, outside, "config.toml", []byte("outside"))
	managedSymlinkOrSkip(t, outside, filepath.Join(dir, ".codex"))
	before := snapshotManagedTree(t, dir)
	selection := managedSelection{
		Entry: managedRegistryEntry{
			Path:       ".codex/config.toml",
			Policy:     managedPolicySharedStarter,
			Capability: managedCapabilityPlaywright,
		},
		Enabled: true,
	}

	_, err := preflightManagedFiles(
		dir,
		[]managedSelection{selection},
		managedRenderedFiles{selection.Entry.Path: []byte("generated")},
	)
	if err == nil || !strings.Contains(err.Error(), `managed destination parent ".codex" is a symlink`) {
		t.Fatalf("preflightManagedFiles() error = %v, want unsafe parent error", err)
	}
	if after := snapshotManagedTree(t, dir); !reflect.DeepEqual(after, before) {
		t.Fatalf("unsafe parent preflight changed project tree: before=%v after=%v", before, after)
	}
	content, readErr := os.ReadFile(filepath.Join(outside, "config.toml"))
	if readErr != nil || string(content) != "outside" {
		t.Fatalf("outside file changed: content=%q error=%v", content, readErr)
	}
}

func TestPreflightManagedFilesRejectsLateConflictBeforeWrites(t *testing.T) {
	dir := t.TempDir()
	writeManagedTestFile(t, dir, "nix/loader.nix", []byte("user content"))
	before := snapshotManagedTree(t, dir)
	selections := []managedSelection{
		{Entry: managedRegistryEntry{Path: "just/loader.just", Policy: managedPolicyLoader}, Enabled: true},
		{Entry: managedRegistryEntry{Path: "nix/loader.nix", Policy: managedPolicyLoader}, Enabled: true},
	}
	rendered := managedRenderedFiles{
		"just/loader.just": managedContent("just/loader.just", "generated"),
		"nix/loader.nix":   managedContent("nix/loader.nix", "generated"),
	}

	plan, err := preflightManagedFiles(dir, selections, rendered)
	var ownershipErr *managedOwnershipError
	if !errors.As(err, &ownershipErr) || ownershipErr.Path != "nix/loader.nix" {
		t.Fatalf("preflightManagedFiles() error = %v, want late ownership conflict", err)
	}
	if len(plan.Files) != 0 {
		t.Fatalf("failed preflight returned plan files: %v", plan.Files)
	}
	if after := snapshotManagedTree(t, dir); !reflect.DeepEqual(after, before) {
		t.Fatalf("late conflict changed project tree: before=%v after=%v", before, after)
	}
}

func TestManagedLintOwnershipConflictPreventsAllWrites(t *testing.T) {
	dir := t.TempDir()
	writeManagedTestFile(t, dir, "just/tailor.just", []byte("user lint recipe\n"))
	before := snapshotManagedTree(t, dir)
	cfg := managedLifecycleConfig("true")

	execution, err := prepareManagedExecution(cfg, dir, func(selections []managedSelection) (managedRenderedFiles, error) {
		return renderManagedFiles(cfg, selections)
	})
	var ownershipErr *managedOwnershipError
	if execution != nil || !errors.As(err, &ownershipErr) || ownershipErr.Path != "just/tailor.just" {
		t.Fatalf("prepareManagedExecution() execution=%v error=%v, want lint ownership conflict", execution, err)
	}
	if after := snapshotManagedTree(t, dir); !reflect.DeepEqual(after, before) {
		t.Fatalf("ownership preflight changed files: before=%v after=%v", before, after)
	}
}

func writeManagedTestFile(t *testing.T, dir, relative string, content []byte) {
	t.Helper()
	name := filepath.Join(dir, relative)
	if err := os.MkdirAll(filepath.Dir(name), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(name, content, 0o644); err != nil {
		t.Fatal(err)
	}
}

func managedSymlinkOrSkip(t *testing.T, target, name string) {
	t.Helper()
	if err := os.Symlink(target, name); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
}

func managedFIFOOrSkip(t *testing.T, name string) {
	t.Helper()
	if err := exec.CommandContext(t.Context(), "mkfifo", name).Run(); err != nil {
		t.Skipf("mkfifo unavailable: %v", err)
	}
}

func snapshotManagedTree(t *testing.T, root string) map[string]managedTreeEntry {
	t.Helper()
	rootFS, err := os.OpenRoot(root)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := rootFS.Close(); err != nil {
			t.Errorf("close snapshot root: %v", err)
		}
	}()

	snapshot := make(map[string]managedTreeEntry)
	err = filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		relative, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		if relative == "." {
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		state := managedTreeEntry{Mode: info.Mode()}
		switch {
		case info.Mode()&os.ModeSymlink != 0:
			state.Target, err = os.Readlink(path)
		case info.Mode().IsRegular():
			var content []byte
			content, err = rootFS.ReadFile(relative)
			state.Content = string(content)
		}
		if err != nil {
			return err
		}
		snapshot[filepath.ToSlash(relative)] = state
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return snapshot
}
