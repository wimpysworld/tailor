package alter

import (
	"errors"
	"net"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/wimpysworld/tailor/internal/testutil"
)

func TestManagedSymlinkLifecyclePreservesOutsideTargets(t *testing.T) {
	t.Run("enabled fragment is replaced while protected starter remains", func(t *testing.T) {
		dir := t.TempDir()
		outside := t.TempDir()
		fragmentTarget := filepath.Join(outside, "fragment")
		starterTarget := filepath.Join(outside, "starter")
		testutil.WriteFile(t, outside, "fragment", "outside fragment\n")
		testutil.WriteFile(t, outside, "starter", "outside starter\n")
		writeManagedTestFile(t, dir, "unrelated.txt", []byte("keep\n"))
		if err := os.MkdirAll(filepath.Join(dir, "nix"), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.MkdirAll(filepath.Join(dir, ".codex"), 0o755); err != nil {
			t.Fatal(err)
		}
		managedSymlinkOrSkip(t, fragmentTarget, filepath.Join(dir, "nix/playwright.nix"))
		managedSymlinkOrSkip(t, starterTarget, filepath.Join(dir, ".codex/config.toml"))

		execution, err := prepareManagedExecution(managedLifecycleConfig("true"), dir, managedTestRenderer)
		if err != nil {
			t.Fatal(err)
		}
		results, err := applyManagedFiles(dir, execution.plan)
		if err != nil {
			t.Fatal(err)
		}
		if !containsManagedApplyPath(results, "nix/playwright.nix") {
			t.Fatalf("apply results omit replaced fragment: %v", results)
		}
		fragmentInfo, err := os.Lstat(filepath.Join(dir, "nix/playwright.nix"))
		if err != nil || !fragmentInfo.Mode().IsRegular() {
			t.Fatalf("enabled fragment mode = %v, error = %v", fragmentInfo, err)
		}
		starterInfo, err := os.Lstat(filepath.Join(dir, ".codex/config.toml"))
		if err != nil || starterInfo.Mode()&os.ModeSymlink == 0 {
			t.Fatalf("protected starter mode = %v, error = %v", starterInfo, err)
		}
		assertManagedFileContent(t, fragmentTarget, "outside fragment\n")
		assertManagedFileContent(t, starterTarget, "outside starter\n")
		assertManagedFileContent(t, filepath.Join(dir, "unrelated.txt"), "keep\n")
	})

	t.Run("disabled fragment removes only the final link", func(t *testing.T) {
		dir := t.TempDir()
		outside := t.TempDir()
		target := filepath.Join(outside, "fragment")
		testutil.WriteFile(t, outside, "fragment", "outside fragment\n")
		if err := os.MkdirAll(filepath.Join(dir, "nix"), 0o755); err != nil {
			t.Fatal(err)
		}
		managedSymlinkOrSkip(t, target, filepath.Join(dir, "nix/playwright.nix"))

		execution, err := prepareManagedExecution(managedLifecycleConfig("false"), dir, managedTestRenderer)
		if err != nil {
			t.Fatal(err)
		}
		results, err := applyManagedFiles(dir, execution.plan)
		if err != nil {
			t.Fatal(err)
		}
		if !containsManagedApplyPath(results, "nix/playwright.nix") {
			t.Fatalf("apply results omit removed fragment: %v", results)
		}
		if _, err := os.Lstat(filepath.Join(dir, "nix/playwright.nix")); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("disabled fragment link still exists: %v", err)
		}
		assertManagedFileContent(t, target, "outside fragment\n")
	})
}

func TestManagedPreflightRejectsActiveUnsafeTypesWithoutWrites(t *testing.T) {
	tests := []struct {
		name      string
		wantError string
		setup     func(*testing.T, string)
	}{
		{
			name:      "linked parent",
			wantError: `managed destination parent ".codex" is a symlink`,
			setup: func(t *testing.T, dir string) {
				outside := t.TempDir()
				testutil.WriteFile(t, outside, "config.toml", "outside\n")
				managedSymlinkOrSkip(t, outside, filepath.Join(dir, ".codex"))
				t.Cleanup(func() { assertManagedFileContent(t, filepath.Join(outside, "config.toml"), "outside\n") })
			},
		},
		{
			name:      "directory",
			wantError: `managed destination "just/tailor.just" is a directory`,
			setup: func(t *testing.T, dir string) {
				if err := os.MkdirAll(filepath.Join(dir, "just/tailor.just"), 0o755); err != nil {
					t.Fatal(err)
				}
			},
		},
		{
			name:      "special file",
			wantError: `managed destination "nix/pages.nix" is not a regular file or symlink`,
			setup: func(t *testing.T, dir string) {
				if err := os.MkdirAll(filepath.Join(dir, "nix"), 0o755); err != nil {
					t.Fatal(err)
				}
				listener, err := (&net.ListenConfig{}).Listen(t.Context(), "unix", filepath.Join(dir, "nix/pages.nix"))
				if err != nil {
					t.Skipf("Unix sockets unavailable: %v", err)
				}
				t.Cleanup(func() { _ = listener.Close() })
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			tt.setup(t, dir)
			before := snapshotManagedTree(t, dir)

			execution, err := prepareManagedExecution(managedLifecycleConfig("true"), dir, managedTestRenderer)
			if execution != nil || err == nil || !strings.Contains(err.Error(), tt.wantError) {
				t.Fatalf("prepareManagedExecution() execution=%v error=%v, want %q", execution, err, tt.wantError)
			}
			if after := snapshotManagedTree(t, dir); !reflect.DeepEqual(after, before) {
				t.Fatalf("failed preflight changed files: before=%v after=%v", before, after)
			}
		})
	}
}

func TestManagedLateConflictPreventsEveryWrite(t *testing.T) {
	dir := t.TempDir()
	writeManagedTestFile(t, dir, "unrelated.txt", []byte("keep\n"))
	writeManagedTestFile(t, dir, "nix/playwright.nix", []byte("user content\n"))
	before := snapshotManagedTree(t, dir)

	execution, err := prepareManagedExecution(managedLifecycleConfig("true"), dir, managedTestRenderer)
	var ownershipErr *managedOwnershipError
	if execution != nil || !errors.As(err, &ownershipErr) || ownershipErr.Path != "nix/playwright.nix" {
		t.Fatalf("prepareManagedExecution() execution=%v error=%v, want late ownership conflict", execution, err)
	}
	if after := snapshotManagedTree(t, dir); !reflect.DeepEqual(after, before) {
		t.Fatalf("late conflict changed files: before=%v after=%v", before, after)
	}
	for _, destination := range []string{"just/loader.just", "nix/loader.nix", "just/tailor.just"} {
		if _, statErr := os.Lstat(filepath.Join(dir, destination)); !errors.Is(statErr, os.ErrNotExist) {
			t.Errorf("early destination %q was written: %v", destination, statErr)
		}
	}
}

func containsManagedApplyPath(results []managedApplyResult, path string) bool {
	for _, result := range results {
		if result.Path == path {
			return true
		}
	}
	return false
}

func assertManagedFileContent(t *testing.T, name, want string) {
	t.Helper()
	content, err := os.ReadFile(name)
	if err != nil || string(content) != want {
		t.Fatalf("file %q content=%q error=%v, want %q", name, content, err, want)
	}
}
