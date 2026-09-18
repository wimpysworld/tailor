package alter

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/wimpysworld/tailor/internal/output"
)

func TestManagedWriteFailuresKeepTruthfulPartialResultsAndRetry(t *testing.T) {
	for _, failureStage := range []string{"create", "write", "chmod", "sync", "close", "rename"} {
		t.Run(failureStage, func(t *testing.T) {
			dir := t.TempDir()
			old := managedContent("nix/loader.nix", "old")
			writeManagedTestFile(t, dir, "nix/loader.nix", old)
			plan := managedFailureWritePlan()
			planned, err := managedPlannedResults(dir, plan)
			if err != nil {
				t.Fatal(err)
			}
			injected := errors.New("injected " + failureStage + " failure")
			var trace []string

			results, err := applyManagedFilesWithHooks(dir, plan, tracingManagedHooks(failureStage, injected, &trace))
			if !errors.Is(err, injected) {
				t.Fatalf("apply error = %v, want injected failure", err)
			}
			if got := managedResultPaths(results); !reflect.DeepEqual(got, []string{"just/loader.just"}) {
				t.Fatalf("partial result paths = %v", got)
			}
			if want := managedFailureTrace(failureStage); !reflect.DeepEqual(trace, want) {
				t.Fatalf("operation trace = %v, want %v", trace, want)
			}
			assertManagedAppliedReport(t, planned, results, []string{"just/loader.just"})
			assertManagedBytes(t, filepath.Join(dir, "nix/loader.nix"), old)
			assertManagedMissing(t, filepath.Join(dir, "just/tailor.just"))
			assertNoManagedTemporaryFiles(t, dir)

			retry, err := applyManagedFiles(dir, plan)
			if err != nil {
				t.Fatal(err)
			}
			if got := managedResultPaths(retry); !reflect.DeepEqual(got, []string{"nix/loader.nix", "just/tailor.just"}) {
				t.Fatalf("retry result paths = %v", got)
			}
			assertManagedBytes(t, filepath.Join(dir, "nix/loader.nix"), managedContent("nix/loader.nix", "new"))
			assertManagedBytes(t, filepath.Join(dir, "just/tailor.just"), managedContent("just/tailor.just", "new"))
		})
	}
}

func TestManagedDirectorySyncFailureRecordsBothCompletedWrites(t *testing.T) {
	dir := t.TempDir()
	writeManagedTestFile(t, dir, "nix/loader.nix", managedContent("nix/loader.nix", "old"))
	plan := managedFailureWritePlan()
	planned, err := managedPlannedResults(dir, plan)
	if err != nil {
		t.Fatal(err)
	}
	injected := errors.New("injected directory sync failure")
	var trace []string

	results, err := applyManagedFilesWithHooks(dir, plan, tracingManagedHooks("directory-sync", injected, &trace))
	if !errors.Is(err, injected) {
		t.Fatalf("apply error = %v, want injected failure", err)
	}
	if got := managedResultPaths(results); !reflect.DeepEqual(got, []string{"just/loader.just", "nix/loader.nix"}) {
		t.Fatalf("partial result paths = %v", got)
	}
	if want := managedFailureTrace("directory-sync"); !reflect.DeepEqual(trace, want) {
		t.Fatalf("operation trace = %v, want %v", trace, want)
	}
	assertManagedAppliedReport(t, planned, results, []string{"just/loader.just", "nix/loader.nix"})
	assertManagedBytes(t, filepath.Join(dir, "nix/loader.nix"), managedContent("nix/loader.nix", "new"))
	assertManagedMissing(t, filepath.Join(dir, "just/tailor.just"))
	assertNoManagedTemporaryFiles(t, dir)

	retry, err := applyManagedFiles(dir, plan)
	if err != nil {
		t.Fatal(err)
	}
	if got := managedResultPaths(retry); !reflect.DeepEqual(got, []string{"just/tailor.just"}) {
		t.Fatalf("retry result paths = %v", got)
	}
}

func TestManagedRemovalFailureStopsLaterRemovalAndRetryConverges(t *testing.T) {
	dir := t.TempDir()
	for _, destination := range []string{"nix/go.nix", "nix/pages.nix"} {
		writeManagedTestFile(t, dir, destination, managedContent(destination, "old"))
	}
	plan := managedPlan{Files: []managedPlanFile{
		managedTestPlanFile("just/loader.just", managedPolicyLoader, managedCapabilityNone, true, managedOperationWrite, managedContent("just/loader.just", "new")),
		managedTestPlanFile("nix/go.nix", managedPolicyFragment, managedCapabilityGo, false, managedOperationRemove, nil),
		managedTestPlanFile("nix/pages.nix", managedPolicyFragment, managedCapabilityPages, false, managedOperationRemove, nil),
	}}
	planned, err := managedPlannedResults(dir, plan)
	if err != nil {
		t.Fatal(err)
	}
	injected := errors.New("injected removal failure")
	var trace []string
	hooks := managedApplyHooks{
		beforeTempCreate: func(destination string) error {
			trace = append(trace, "write "+destination)
			return nil
		},
		beforeRemove: func(destination string) error {
			trace = append(trace, "remove "+destination)
			if destination == "nix/go.nix" {
				return injected
			}
			return nil
		},
	}

	results, err := applyManagedFilesWithHooks(dir, plan, hooks)
	if !errors.Is(err, injected) {
		t.Fatalf("apply error = %v, want injected failure", err)
	}
	if got := managedResultPaths(results); !reflect.DeepEqual(got, []string{"just/loader.just"}) {
		t.Fatalf("partial result paths = %v", got)
	}
	if want := []string{"write just/loader.just", "remove nix/go.nix"}; !reflect.DeepEqual(trace, want) {
		t.Fatalf("operation trace = %v, want %v", trace, want)
	}
	assertManagedAppliedReport(t, planned, results, []string{"just/loader.just"})
	for _, destination := range []string{"nix/go.nix", "nix/pages.nix"} {
		assertManagedBytes(t, filepath.Join(dir, destination), managedContent(destination, "old"))
	}

	retry, err := applyManagedFiles(dir, plan)
	if err != nil {
		t.Fatal(err)
	}
	if got := managedResultPaths(retry); !reflect.DeepEqual(got, []string{"nix/go.nix", "nix/pages.nix"}) {
		t.Fatalf("retry result paths = %v", got)
	}
	assertManagedMissing(t, filepath.Join(dir, "nix/go.nix"))
	assertManagedMissing(t, filepath.Join(dir, "nix/pages.nix"))
}

func managedFailureWritePlan() managedPlan {
	return managedPlan{Files: []managedPlanFile{
		managedTestPlanFile("just/tailor.just", managedPolicyCore, managedCapabilityNone, true, managedOperationWrite, managedContent("just/tailor.just", "new")),
		managedTestPlanFile("nix/loader.nix", managedPolicyLoader, managedCapabilityNone, true, managedOperationWrite, managedContent("nix/loader.nix", "new")),
		managedTestPlanFile("just/loader.just", managedPolicyLoader, managedCapabilityNone, true, managedOperationWrite, managedContent("just/loader.just", "new")),
	}}
}

func tracingManagedHooks(failureStage string, injected error, trace *[]string) managedApplyHooks {
	hook := func(stage string) func(string) error {
		return func(destination string) error {
			*trace = append(*trace, stage+" "+destination)
			if destination == "nix/loader.nix" && stage == failureStage {
				return injected
			}
			return nil
		}
	}
	return managedApplyHooks{
		beforeTempCreate:    hook("create"),
		beforeTempWrite:     hook("write"),
		beforeTempChmod:     hook("chmod"),
		beforeTempSync:      hook("sync"),
		beforeTempClose:     hook("close"),
		beforeRename:        hook("rename"),
		beforeDirectorySync: hook("directory-sync"),
	}
}

func managedFailureTrace(failureStage string) []string {
	stages := []string{"create", "write", "chmod", "sync", "close", "rename", "directory-sync"}
	trace := make([]string, 0, len(stages)*2)
	for _, destination := range []string{"just/loader.just", "nix/loader.nix"} {
		for _, stage := range stages {
			trace = append(trace, stage+" "+destination)
			if destination == "nix/loader.nix" && stage == failureStage {
				return trace
			}
		}
	}
	return trace
}

func assertManagedAppliedReport(t *testing.T, planned []SwatchResult, confirmed []managedApplyResult, want []string) {
	t.Helper()
	results, err := managedConfirmedResults(planned, confirmed)
	if err != nil {
		t.Fatal(err)
	}
	report := buildReport("alter", "", nil, nil, nil, results, Apply)
	var got []string
	for _, item := range report.Document.Items {
		if item.Outcome == output.Applied {
			got = append(got, item.Name)
		}
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("reported applied paths = %v, want %v", got, want)
	}
}

func assertNoManagedTemporaryFiles(t *testing.T, dir string) {
	t.Helper()
	for _, directory := range []string{"just", "nix"} {
		matches, err := filepath.Glob(filepath.Join(dir, directory, "*.tmp-*"))
		if err != nil || len(matches) != 0 {
			t.Fatalf("temporary files in %q = %v, error = %v", directory, matches, err)
		}
	}
}

func assertManagedBytes(t *testing.T, name string, want []byte) {
	t.Helper()
	got, err := os.ReadFile(name)
	if err != nil || !reflect.DeepEqual(got, want) {
		t.Fatalf("file %q content=%q error=%v, want %q", name, got, err, want)
	}
}

func assertManagedMissing(t *testing.T, name string) {
	t.Helper()
	if _, err := os.Lstat(name); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("file %q exists or cannot be checked: %v", name, err)
	}
}
