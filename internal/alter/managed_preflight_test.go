package alter

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

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

func TestSnapshotManagedDestinationDoesNotReadProtectedRegularFiles(t *testing.T) {
	for _, tt := range []struct {
		name   string
		policy managedPolicy
	}{
		{name: "root", policy: managedPolicyRoot},
		{name: "shared starter", policy: managedPolicySharedStarter},
	} {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			protected := filepath.Join(dir, "protected")
			writeManagedTestFile(t, dir, "protected", []byte("content that must not enter the snapshot"))
			oldAccess := time.Unix(946684800, 0)
			accessBefore, hasAccessTime := time.Time{}, false
			if err := os.Chtimes(protected, oldAccess, time.Now()); err == nil {
				accessBefore, hasAccessTime = managedFileAccessTime(t, protected)
			}
			root, err := os.OpenRoot(dir)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() {
				if err := root.Close(); err != nil {
					t.Errorf("close protected snapshot root: %v", err)
				}
			})

			snapshot, err := snapshotManagedDestination(root, managedSelection{
				Entry:   managedRegistryEntry{Path: "protected", Policy: tt.policy},
				Enabled: true,
			})
			if err != nil {
				t.Fatal(err)
			}
			if snapshot.Kind != managedDestinationRegular || snapshot.Content != nil {
				t.Fatalf("protected snapshot = %#v, want regular metadata without file content", snapshot)
			}
			if hasAccessTime {
				if accessAfter, available := managedFileAccessTime(t, protected); available && !accessAfter.Equal(accessBefore) {
					t.Fatalf("protected snapshot accessed file content: access time changed from %s to %s", accessBefore, accessAfter)
				}
			}
		})
	}
}

func managedFileAccessTime(t *testing.T, name string) (time.Time, bool) {
	t.Helper()
	info, err := os.Stat(name)
	if err != nil {
		t.Fatal(err)
	}
	value := reflect.ValueOf(info.Sys())
	if !value.IsValid() {
		return time.Time{}, false
	}
	if value.Kind() == reflect.Pointer {
		if value.IsNil() {
			return time.Time{}, false
		}
		value = value.Elem()
	}
	if value.Kind() != reflect.Struct {
		return time.Time{}, false
	}
	for _, fieldName := range []string{"Atim", "Atimespec"} {
		field := value.FieldByName(fieldName)
		if !field.IsValid() {
			continue
		}
		seconds := field.FieldByName("Sec")
		nanoseconds := field.FieldByName("Nsec")
		if seconds.IsValid() && nanoseconds.IsValid() {
			return time.Unix(seconds.Int(), nanoseconds.Int()), true
		}
	}
	return time.Time{}, false
}

func TestPreflightManagedFilesDoesNotInspectClientsWhenPlaywrightIsFalseOrAbsent(t *testing.T) {
	falseValue := false
	for _, tt := range []struct {
		name          string
		cfg           *config.Config
		wantPlanFiles int
	}{
		{name: "false", cfg: &config.Config{MCP: &config.MCPSettings{Playwright: &falseValue}}, wantPlanFiles: 6},
		{name: "blender false", cfg: &config.Config{MCP: &config.MCPSettings{Blender: &falseValue}}, wantPlanFiles: 8},
		{name: "both false", cfg: &config.Config{MCP: &config.MCPSettings{Blender: &falseValue, Playwright: &falseValue}}, wantPlanFiles: 9},
		{name: "absent", cfg: &config.Config{}, wantPlanFiles: 5},
	} {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			for _, destination := range managedMCPDestinations() {
				if err := os.MkdirAll(filepath.Join(dir, destination), 0o755); err != nil {
					t.Fatal(err)
				}
			}
			writeManagedTestFile(t, dir, "nix/playwright.nix", managedContent("nix/playwright.nix", "owned"))
			before := snapshotManagedTree(t, dir)

			selections, err := selectManagedFiles(tt.cfg)
			if err != nil {
				t.Fatal(err)
			}
			rendered, err := renderManagedFiles(tt.cfg, selections)
			if err != nil {
				t.Fatal(err)
			}
			plan, err := preflightManagedFiles(dir, selections, rendered)
			if err != nil {
				t.Fatal(err)
			}
			if len(plan.Files) != tt.wantPlanFiles {
				t.Fatalf("planned files = %d, want %d", len(plan.Files), tt.wantPlanFiles)
			}
			for _, file := range plan.Files {
				if _, client := managedMCPProviderForDestination(file.Selection.Entry.Path); client {
					t.Errorf("plan inspected unselected client destination %q", file.Selection.Entry.Path)
				}
			}
			if after := snapshotManagedTree(t, dir); !reflect.DeepEqual(after, before) {
				t.Fatalf("preflight changed unselected destinations: before=%v after=%v", before, after)
			}
		})
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
			Capability: managedCapabilityNone,
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

func TestPreflightManagedFilesRejectsRetainedLegacyGoBuildBody(t *testing.T) {
	dir := t.TempDir()
	legacy := managedContent("just/go.just", "\n# Build Go packages\nbuild:\n    @go build ./...")
	writeManagedTestFile(t, dir, "just/go.just", legacy)
	before := snapshotManagedTree(t, dir)
	cfg := &config.Config{}
	selections, err := selectManagedFiles(cfg)
	if err != nil {
		t.Fatal(err)
	}
	rendered, err := renderManagedFiles(cfg, selections)
	if err != nil {
		t.Fatal(err)
	}
	_, err = preflightManagedFiles(dir, selections, rendered)
	if err == nil || !strings.Contains(err.Error(), "set languages.go to true to migrate it") {
		t.Fatalf("preflightManagedFiles() error = %v, want migration guidance", err)
	}
	if after := snapshotManagedTree(t, dir); !reflect.DeepEqual(after, before) {
		t.Fatalf("migration preflight changed files: before=%v after=%v", before, after)
	}
}

func TestPreflightManagedFilesRetainedUnmarkedGoRecipes(t *testing.T) {
	for _, test := range []struct {
		name     string
		body     string
		conflict bool
	}{
		{name: "build", body: "build:\n    @true\n", conflict: true},
		{name: "test", body: "test:\n    @true\n", conflict: true},
		{name: "lint arguments", body: "lint mode='check':\n    @true\n", conflict: true},
		{name: "quiet recipe", body: "@build:\n    true\n", conflict: true},
		{name: "private attribute", body: "[private]\nbuild:\n    @true\n", conflict: true},
		{name: "unique", body: "custom:\n    @true\n"},
		{name: "private name", body: "_build:\n    @true\n"},
		{name: "legacy helper", body: "build-go:\n    @true\n"},
		{name: "comment", body: "# build:\ncustom:\n    @true\n"},
		{name: "body text", body: "custom:\n    echo build:\n"},
		{name: "variable", body: "build := 'custom'\n"},
	} {
		t.Run(test.name, func(t *testing.T) {
			dir := t.TempDir()
			writeManagedTestFile(t, dir, "just/go.just", []byte(test.body))
			before := snapshotManagedTree(t, dir)
			cfg := &config.Config{}
			selections, err := selectManagedFiles(cfg)
			if err != nil {
				t.Fatal(err)
			}
			rendered, err := renderManagedFiles(cfg, selections)
			if err != nil {
				t.Fatal(err)
			}
			_, err = preflightManagedFiles(dir, selections, rendered)
			if test.conflict {
				if err == nil || !strings.Contains(err.Error(), "remove or rename") || strings.Contains(err.Error(), "set languages.go") {
					t.Fatalf("expected manual collision guidance, got %v", err)
				}
			} else if err != nil {
				t.Fatal(err)
			}
			if after := snapshotManagedTree(t, dir); !reflect.DeepEqual(after, before) {
				t.Fatalf("preflight changed files: before=%v after=%v", before, after)
			}
			writeManagedTestFile(t, dir, "justfile", []byte("import 'just/loader.just'\n"))
			for path, content := range rendered {
				writeManagedTestFile(t, dir, path, content)
			}
			// #nosec G204 -- The executable is resolved from the fixed name "just" and only parses the temporary fixture.
			command := exec.CommandContext(t.Context(), managedLintExecutable(t), "--summary")
			command.Dir = dir
			output, parseErr := command.CombinedOutput()
			if (parseErr != nil) != test.conflict {
				t.Fatalf("Just parser disagrees with preflight: %v\n%s", parseErr, output)
			}
		})
	}
}

func TestPreflightManagedFilesRejectsMissingEnabledBuildBody(t *testing.T) {
	selection := managedSelection{
		Entry: managedRegistryEntry{
			Path:       "just/go.just",
			Policy:     managedPolicyFragment,
			Capability: managedCapabilityGo,
			BuildBody:  "build-go",
		},
		Enabled: true,
	}
	_, err := preflightManagedFiles(t.TempDir(), []managedSelection{selection}, managedRenderedFiles{
		"just/go.just": managedContent("just/go.just", "lint-go:\n    @true"),
	})
	if err == nil || !strings.Contains(err.Error(), `enabled managed build contribution "build-go" is missing`) {
		t.Fatalf("preflightManagedFiles() error = %v, want missing recipe conflict", err)
	}
}

func writeManagedTestFile(t *testing.T, dir, relative string, content []byte) {
	t.Helper()
	name := filepath.Join(dir, relative)
	if err := os.MkdirAll(filepath.Dir(name), 0o755); err != nil {
		t.Fatal(err)
	}
	// #nosec G703 -- Test paths are controlled and stay in the temporary fixture.
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
