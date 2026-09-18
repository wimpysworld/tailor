package alter

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestApplyManagedFilesUsesLifecycleOrder(t *testing.T) {
	dir := t.TempDir()
	writeManagedTestFile(t, dir, "just/go.just", managedContent("just/go.just", "old"))

	plan := managedPlan{Files: []managedPlanFile{
		managedTestPlanFile("just/go.just", managedPolicyFragment, managedCapabilityGo, false, managedOperationRemove, nil),
		managedTestPlanFile("nix/go.nix", managedPolicyFragment, managedCapabilityGo, true, managedOperationWrite, managedContent("nix/go.nix", "new")),
		managedTestPlanFile("just/tailor.just", managedPolicyCore, managedCapabilityNone, true, managedOperationWrite, managedContent("just/tailor.just", "new")),
		managedTestPlanFile("justfile", managedPolicyRoot, managedCapabilityNone, true, managedOperationWrite, []byte("root\n")),
		managedTestPlanFile("just/loader.just", managedPolicyLoader, managedCapabilityNone, true, managedOperationWrite, managedContent("just/loader.just", "new")),
	}}

	var operations []string
	hooks := managedApplyHooks{
		beforeTempCreate: func(destination string) error {
			operations = append(operations, "write "+destination)
			return nil
		},
		beforeRemove: func(destination string) error {
			operations = append(operations, "remove "+destination)
			return nil
		},
	}
	results, err := applyManagedFilesWithHooks(dir, plan, hooks)
	if err != nil {
		t.Fatal(err)
	}

	wantOperations := []string{
		"write just/loader.just",
		"write justfile",
		"write just/tailor.just",
		"write nix/go.nix",
		"remove just/go.just",
	}
	if !reflect.DeepEqual(operations, wantOperations) {
		t.Fatalf("operation order = %v, want %v", operations, wantOperations)
	}
	if got := managedResultPaths(results); !reflect.DeepEqual(got, []string{
		"just/loader.just", "justfile", "just/tailor.just", "nix/go.nix", "just/go.just",
	}) {
		t.Fatalf("result paths = %v", got)
	}
}

func TestApplyManagedFilesPreservesRegularFileBeforeRename(t *testing.T) {
	injected := errors.New("injected failure")
	tests := []struct {
		name string
		set  func(*managedApplyHooks)
	}{
		{name: "temporary create", set: func(h *managedApplyHooks) { h.beforeTempCreate = func(string) error { return injected } }},
		{name: "temporary write", set: func(h *managedApplyHooks) { h.beforeTempWrite = func(string) error { return injected } }},
		{name: "temporary chmod", set: func(h *managedApplyHooks) { h.beforeTempChmod = func(string) error { return injected } }},
		{name: "temporary sync", set: func(h *managedApplyHooks) { h.beforeTempSync = func(string) error { return injected } }},
		{name: "temporary close", set: func(h *managedApplyHooks) { h.beforeTempClose = func(string) error { return injected } }},
		{name: "rename", set: func(h *managedApplyHooks) { h.beforeRename = func(string) error { return injected } }},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			const destination = "just/loader.just"
			oldContent := managedContent(destination, "old")
			writeManagedTestFile(t, dir, destination, oldContent)
			if err := os.Chmod(filepath.Join(dir, destination), 0o600); err != nil {
				t.Fatal(err)
			}
			plan := managedSingleWritePlan(destination, managedContent(destination, "new"))
			var hooks managedApplyHooks
			tt.set(&hooks)

			results, err := applyManagedFilesWithHooks(dir, plan, hooks)
			if !errors.Is(err, injected) {
				t.Fatalf("applyManagedFilesWithHooks() error = %v, want injected failure", err)
			}
			if len(results) != 0 {
				t.Fatalf("results = %v, want none", results)
			}
			content, readErr := os.ReadFile(filepath.Join(dir, destination))
			if readErr != nil || !reflect.DeepEqual(content, oldContent) {
				t.Fatalf("destination content = %q, error = %v", content, readErr)
			}
			info, statErr := os.Stat(filepath.Join(dir, destination))
			if statErr != nil {
				t.Fatalf("stat destination: %v", statErr)
			}
			if info.Mode().Perm() != 0o600 {
				t.Fatalf("destination mode = %v, want 0600", info.Mode().Perm())
			}
			matches, globErr := filepath.Glob(filepath.Join(dir, "just", "loader.just.tmp-*"))
			if globErr != nil || len(matches) != 0 {
				t.Fatalf("temporary files = %v, error = %v", matches, globErr)
			}
		})
	}
}

func TestApplyManagedFilesPreservesRegularPermissions(t *testing.T) {
	dir := t.TempDir()
	const destination = "just/loader.just"
	writeManagedTestFile(t, dir, destination, managedContent(destination, "old"))
	if err := os.Chmod(filepath.Join(dir, destination), 0o640); err != nil {
		t.Fatal(err)
	}

	results, err := applyManagedFiles(dir, managedSingleWritePlan(destination, managedContent(destination, "new")))
	if err != nil {
		t.Fatal(err)
	}
	if got := managedResultPaths(results); !reflect.DeepEqual(got, []string{destination}) {
		t.Fatalf("result paths = %v", got)
	}
	info, err := os.Stat(filepath.Join(dir, destination))
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o640 {
		t.Fatalf("destination mode = %v, want 0640", info.Mode().Perm())
	}
}

func TestApplyManagedFilesReplacesFinalSymlinkOnlyAtRename(t *testing.T) {
	dir := t.TempDir()
	outside := filepath.Join(t.TempDir(), "outside")
	if err := os.WriteFile(outside, []byte("outside"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(dir, "just"), 0o755); err != nil {
		t.Fatal(err)
	}
	const destination = "just/loader.just"
	link := filepath.Join(dir, destination)
	managedSymlinkOrSkip(t, outside, link)
	plan := managedSingleWritePlan(destination, managedContent(destination, "new"))
	injected := errors.New("stop before rename")

	results, err := applyManagedFilesWithHooks(dir, plan, managedApplyHooks{
		beforeRename: func(string) error {
			info, statErr := os.Lstat(link)
			if statErr != nil {
				t.Fatalf("lstat destination before rename: %v", statErr)
			}
			if info.Mode()&os.ModeSymlink == 0 {
				t.Fatalf("destination mode before rename = %v, want symlink", info.Mode())
			}
			return injected
		},
	})
	if !errors.Is(err, injected) || len(results) != 0 {
		t.Fatalf("first apply results = %v, error = %v", results, err)
	}
	info, statErr := os.Lstat(link)
	if statErr != nil {
		t.Fatalf("lstat destination after failed rename: %v", statErr)
	}
	if info.Mode()&os.ModeSymlink == 0 {
		t.Fatalf("destination mode after failed rename = %v, want symlink", info.Mode())
	}

	results, err = applyManagedFiles(dir, plan)
	if err != nil {
		t.Fatal(err)
	}
	if got := managedResultPaths(results); !reflect.DeepEqual(got, []string{destination}) {
		t.Fatalf("retry result paths = %v", got)
	}
	info, statErr = os.Lstat(link)
	if statErr != nil {
		t.Fatalf("lstat destination after retry: %v", statErr)
	}
	if !info.Mode().IsRegular() {
		t.Fatalf("destination mode after retry = %v, want regular", info.Mode())
	}
	if content, readErr := os.ReadFile(outside); readErr != nil || string(content) != "outside" {
		t.Fatalf("symlink target changed: content=%q error=%v", content, readErr)
	}
}

func TestApplyManagedFilesDirectorySyncFailureReturnsChangeAndRetryConverges(t *testing.T) {
	dir := t.TempDir()
	first := managedTestPlanFile("a", managedPolicyLoader, managedCapabilityNone, true, managedOperationWrite, managedContent("a", "new"))
	second := managedTestPlanFile("b", managedPolicyLoader, managedCapabilityNone, true, managedOperationWrite, managedContent("b", "new"))
	plan := managedPlan{Files: []managedPlanFile{second, first}}
	injected := errors.New("directory sync failed")

	results, err := applyManagedFilesWithHooks(dir, plan, managedApplyHooks{
		beforeDirectorySync: func(destination string) error {
			if destination == "a" {
				return injected
			}
			return nil
		},
	})
	if !errors.Is(err, injected) {
		t.Fatalf("apply error = %v, want directory sync failure", err)
	}
	if got := managedResultPaths(results); !reflect.DeepEqual(got, []string{"a"}) {
		t.Fatalf("partial result paths = %v", got)
	}
	if _, statErr := os.Stat(filepath.Join(dir, "a")); statErr != nil {
		t.Fatalf("first write was not retained: %v", statErr)
	}
	if _, statErr := os.Stat(filepath.Join(dir, "b")); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("second write ran after failure: %v", statErr)
	}

	results, err = applyManagedFiles(dir, plan)
	if err != nil {
		t.Fatal(err)
	}
	if got := managedResultPaths(results); !reflect.DeepEqual(got, []string{"b"}) {
		t.Fatalf("retry result paths = %v, want only b", got)
	}
}

func TestApplyManagedFilesRemovalFailuresAreTruthful(t *testing.T) {
	const destination = "nix/go.nix"
	selection := managedSelection{
		Entry: managedRegistryEntry{Path: destination, Policy: managedPolicyFragment, Capability: managedCapabilityGo},
	}
	plan := managedPlan{Files: []managedPlanFile{{Selection: selection, Operation: managedOperationRemove}}}

	t.Run("remove failure preserves final symlink", func(t *testing.T) {
		dir := t.TempDir()
		outside := filepath.Join(t.TempDir(), "outside")
		if err := os.WriteFile(outside, []byte("outside"), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := os.MkdirAll(filepath.Join(dir, "nix"), 0o755); err != nil {
			t.Fatal(err)
		}
		link := filepath.Join(dir, destination)
		managedSymlinkOrSkip(t, outside, link)
		injected := errors.New("remove failed")

		results, err := applyManagedFilesWithHooks(dir, plan, managedApplyHooks{
			beforeRemove: func(string) error { return injected },
		})
		if !errors.Is(err, injected) || len(results) != 0 {
			t.Fatalf("apply results = %v, error = %v", results, err)
		}
		info, statErr := os.Lstat(link)
		if statErr != nil {
			t.Fatalf("lstat destination after remove failure: %v", statErr)
		}
		if info.Mode()&os.ModeSymlink == 0 {
			t.Fatalf("destination mode after remove failure = %v, want symlink", info.Mode())
		}

		results, err = applyManagedFiles(dir, plan)
		if err != nil {
			t.Fatal(err)
		}
		if got := managedResultPaths(results); !reflect.DeepEqual(got, []string{destination}) {
			t.Fatalf("retry result paths = %v", got)
		}
		if content, readErr := os.ReadFile(outside); readErr != nil || string(content) != "outside" {
			t.Fatalf("removed symlink target changed: content=%q error=%v", content, readErr)
		}
	})

	t.Run("directory sync failure records removal", func(t *testing.T) {
		dir := t.TempDir()
		writeManagedTestFile(t, dir, destination, managedContent(destination, "old"))
		injected := errors.New("directory sync failed")

		results, err := applyManagedFilesWithHooks(dir, plan, managedApplyHooks{
			beforeDirectorySync: func(string) error { return injected },
		})
		if !errors.Is(err, injected) {
			t.Fatalf("apply error = %v, want directory sync failure", err)
		}
		if got := managedResultPaths(results); !reflect.DeepEqual(got, []string{destination}) {
			t.Fatalf("partial result paths = %v", got)
		}
		if _, statErr := os.Lstat(filepath.Join(dir, destination)); !errors.Is(statErr, os.ErrNotExist) {
			t.Fatalf("removed destination still exists: %v", statErr)
		}

		results, err = applyManagedFiles(dir, plan)
		if err != nil || len(results) != 0 {
			t.Fatalf("retry results = %v, error = %v", results, err)
		}
	})
}

func TestApplyManagedFilesRechecksDestinations(t *testing.T) {
	t.Run("new unowned destination conflicts", func(t *testing.T) {
		dir := t.TempDir()
		const destination = "just/loader.just"
		selection := managedSelection{Entry: managedRegistryEntry{Path: destination, Policy: managedPolicyLoader}, Enabled: true}
		plan, err := preflightManagedFiles(dir, []managedSelection{selection}, managedRenderedFiles{
			destination: managedContent(destination, "new"),
		})
		if err != nil {
			t.Fatal(err)
		}
		writeManagedTestFile(t, dir, destination, []byte("user content"))

		results, err := applyManagedFiles(dir, plan)
		var ownershipErr *managedOwnershipError
		if !errors.As(err, &ownershipErr) || ownershipErr.Path != destination {
			t.Fatalf("apply error = %v, want ownership conflict", err)
		}
		if len(results) != 0 {
			t.Fatalf("results = %v, want none", results)
		}
		content, readErr := os.ReadFile(filepath.Join(dir, destination))
		if readErr != nil || string(content) != "user content" {
			t.Fatalf("unowned destination changed: content=%q error=%v", content, readErr)
		}
	})

	t.Run("new protected destination is preserved", func(t *testing.T) {
		dir := t.TempDir()
		const destination = "justfile"
		selection := managedSelection{Entry: managedRegistryEntry{Path: destination, Policy: managedPolicyRoot}, Enabled: true}
		plan, err := preflightManagedFiles(dir, []managedSelection{selection}, managedRenderedFiles{
			destination: []byte("generated\n"),
		})
		if err != nil {
			t.Fatal(err)
		}
		writeManagedTestFile(t, dir, destination, []byte("user content"))

		results, err := applyManagedFiles(dir, plan)
		if err != nil || len(results) != 0 {
			t.Fatalf("apply results = %v, error = %v", results, err)
		}
		content, readErr := os.ReadFile(filepath.Join(dir, destination))
		if readErr != nil || string(content) != "user content" {
			t.Fatalf("protected destination changed: content=%q error=%v", content, readErr)
		}
	})

	t.Run("new linked parent is rejected", func(t *testing.T) {
		dir := t.TempDir()
		outside := t.TempDir()
		const destination = "nested/loader"
		selection := managedSelection{Entry: managedRegistryEntry{Path: destination, Policy: managedPolicyLoader}, Enabled: true}
		plan, err := preflightManagedFiles(dir, []managedSelection{selection}, managedRenderedFiles{
			destination: managedContent(destination, "new"),
		})
		if err != nil {
			t.Fatal(err)
		}
		managedSymlinkOrSkip(t, outside, filepath.Join(dir, "nested"))

		results, err := applyManagedFiles(dir, plan)
		if err == nil || !strings.Contains(err.Error(), `managed destination parent "nested" is a symlink`) {
			t.Fatalf("apply error = %v, want linked parent error", err)
		}
		if len(results) != 0 {
			t.Fatalf("results = %v, want none", results)
		}
		entries, readErr := os.ReadDir(outside)
		if readErr != nil || len(entries) != 0 {
			t.Fatalf("outside directory changed: entries=%v error=%v", entries, readErr)
		}
	})
}

func managedSingleWritePlan(destination string, content []byte) managedPlan {
	return managedPlan{Files: []managedPlanFile{
		managedTestPlanFile(destination, managedPolicyLoader, managedCapabilityNone, true, managedOperationWrite, content),
	}}
}

func managedTestPlanFile(destination string, policy managedPolicy, capability managedCapability, enabled bool, operation managedOperation, content []byte) managedPlanFile {
	return managedPlanFile{
		Selection: managedSelection{
			Entry:   managedRegistryEntry{Path: destination, Policy: policy, Capability: capability},
			Enabled: enabled,
		},
		Operation: operation,
		Content:   content,
	}
}

func managedResultPaths(results []managedApplyResult) []string {
	paths := make([]string, 0, len(results))
	for _, result := range results {
		paths = append(paths, result.Path)
	}
	return paths
}
