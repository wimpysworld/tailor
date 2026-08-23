package alter_test

import (
	"errors"
	"net"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/wimpysworld/tailor/internal/alter"
	"github.com/wimpysworld/tailor/internal/config"
)

func TestProcessRetiredWorkflowsPresentInEveryMode(t *testing.T) {
	want := []alter.SwatchResult{
		{Path: ".github/workflows/tailor-automerge.yml", Category: alter.WouldRemove},
		{Path: ".github/workflows/tailor.yml", Category: alter.WouldRemove},
	}
	tests := []struct {
		name        string
		mode        alter.ApplyMode
		wantPresent bool
	}{
		{name: "dry run", mode: alter.DryRun, wantPresent: true},
		{name: "apply", mode: alter.Apply, wantPresent: false},
		{name: "recut", mode: alter.Recut, wantPresent: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			for _, path := range config.RetiredWorkflowPaths() {
				writeOnDisk(t, dir, path, []byte(path))
			}

			got, err := alter.ProcessRetiredWorkflows(dir, tt.mode)
			if err != nil {
				t.Fatalf("ProcessRetiredWorkflows() error: %v", err)
			}
			if !slices.Equal(got, want) {
				t.Errorf("ProcessRetiredWorkflows() = %v, want %v", got, want)
			}

			for _, path := range config.RetiredWorkflowPaths() {
				_, err := os.Lstat(filepath.Join(dir, path))
				if tt.wantPresent && err != nil {
					t.Errorf("%s was removed in %s mode: %v", path, tt.name, err)
				}
				if !tt.wantPresent && !errors.Is(err, os.ErrNotExist) {
					t.Errorf("%s still exists in %s mode: %v", path, tt.name, err)
				}
			}
		})
	}
}

func TestProcessRetiredWorkflowsAbsentInEveryMode(t *testing.T) {
	for name, mode := range map[string]alter.ApplyMode{
		"dry run": alter.DryRun,
		"apply":   alter.Apply,
		"recut":   alter.Recut,
	} {
		t.Run(name, func(t *testing.T) {
			got, err := alter.ProcessRetiredWorkflows(t.TempDir(), mode)
			if err != nil {
				t.Fatalf("ProcessRetiredWorkflows() error: %v", err)
			}
			if len(got) != 0 {
				t.Errorf("ProcessRetiredWorkflows() = %v, want no results", got)
			}
		})
	}
}

func TestProcessRetiredWorkflowsRemovesFinalSymlinkWithoutRemovingTarget(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(t.TempDir(), "workflow.yml")
	if err := os.WriteFile(target, []byte("outside"), 0o644); err != nil {
		t.Fatalf("WriteFile(%q): %v", target, err)
	}
	link := filepath.Join(dir, config.RetiredWorkflowPaths()[0])
	if err := os.MkdirAll(filepath.Dir(link), 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	symlinkOrSkip(t, target, link)

	results, err := alter.ProcessRetiredWorkflows(dir, alter.Apply)
	if err != nil {
		t.Fatalf("ProcessRetiredWorkflows() error: %v", err)
	}
	if len(results) != 1 || results[0].Path != config.RetiredWorkflowPaths()[0] {
		t.Errorf("results = %v, want the final symlink removal", results)
	}
	if _, err := os.Lstat(link); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("final symlink still exists: %v", err)
	}
	if got, err := os.ReadFile(target); err != nil || string(got) != "outside" {
		t.Errorf("target changed or disappeared: content %q, error %v", got, err)
	}
}

func TestProcessRetiredWorkflowsRemovesDanglingFinalSymlink(t *testing.T) {
	dir := t.TempDir()
	link := filepath.Join(dir, config.RetiredWorkflowPaths()[1])
	if err := os.MkdirAll(filepath.Dir(link), 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	symlinkOrSkip(t, filepath.Join(t.TempDir(), "missing.yml"), link)

	results, err := alter.ProcessRetiredWorkflows(dir, alter.Apply)
	if err != nil {
		t.Fatalf("ProcessRetiredWorkflows() error: %v", err)
	}
	if len(results) != 1 || results[0].Path != config.RetiredWorkflowPaths()[1] {
		t.Errorf("results = %v, want the dangling symlink removal", results)
	}
	if _, err := os.Lstat(link); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("dangling symlink still exists: %v", err)
	}
}

func TestProcessRetiredWorkflowsRejectsParentSymlinkEscape(t *testing.T) {
	dir := t.TempDir()
	outside := t.TempDir()
	outsideWorkflow := filepath.Join(outside, "workflows", "tailor.yml")
	if err := os.MkdirAll(filepath.Dir(outsideWorkflow), 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(outsideWorkflow, []byte("outside"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	symlinkOrSkip(t, outside, filepath.Join(dir, ".github"))

	results, err := alter.ProcessRetiredWorkflows(dir, alter.Apply)
	if err == nil {
		t.Fatal("ProcessRetiredWorkflows() error = nil, want parent symlink error")
	}
	if len(results) != 0 {
		t.Errorf("results = %v, want no results", results)
	}
	if !strings.Contains(err.Error(), `retired workflow parent ".github" is a symlink`) {
		t.Errorf("error = %q, want parent symlink error", err)
	}
	if got, readErr := os.ReadFile(outsideWorkflow); readErr != nil || string(got) != "outside" {
		t.Errorf("outside workflow changed or disappeared: content %q, error %v", got, readErr)
	}
}

func TestProcessRetiredWorkflowsRejectsDirectoryBeforeRemovingFiles(t *testing.T) {
	dir := t.TempDir()
	paths := config.RetiredWorkflowPaths()
	writeOnDisk(t, dir, paths[0], []byte("keep until validation succeeds"))
	if err := os.MkdirAll(filepath.Join(dir, paths[1]), 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}

	results, err := alter.ProcessRetiredWorkflows(dir, alter.Apply)
	if err == nil {
		t.Fatal("ProcessRetiredWorkflows() error = nil, want directory error")
	}
	if len(results) != 0 {
		t.Errorf("results = %v, want no results", results)
	}
	if !strings.Contains(err.Error(), `retired workflow path ".github/workflows/tailor.yml" is a directory`) {
		t.Errorf("error = %q, want directory error", err)
	}
	if _, statErr := os.Stat(filepath.Join(dir, paths[0])); statErr != nil {
		t.Errorf("first workflow was removed before validation completed: %v", statErr)
	}
}

func TestProcessRetiredWorkflowsRejectsSpecialFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, config.RetiredWorkflowPaths()[0])
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	listener, err := (&net.ListenConfig{}).Listen(t.Context(), "unix", path)
	if err != nil {
		t.Skipf("Unix sockets unavailable: %v", err)
	}
	defer listener.Close()

	results, err := alter.ProcessRetiredWorkflows(dir, alter.Apply)
	if err == nil {
		t.Fatal("ProcessRetiredWorkflows() error = nil, want special-file error")
	}
	if len(results) != 0 {
		t.Errorf("results = %v, want no results", results)
	}
	if !strings.Contains(err.Error(), "is not a regular file or symlink") {
		t.Errorf("error = %q, want special-file error", err)
	}
	if _, statErr := os.Lstat(path); statErr != nil {
		t.Errorf("special file was removed: %v", statErr)
	}
}

func TestProcessRetiredWorkflowsRetryAfterPartialOrCompletedCleanup(t *testing.T) {
	dir := t.TempDir()
	remaining := config.RetiredWorkflowPaths()[1]
	writeOnDisk(t, dir, remaining, []byte("remaining"))

	first, err := alter.ProcessRetiredWorkflows(dir, alter.Apply)
	if err != nil {
		t.Fatalf("first ProcessRetiredWorkflows() error: %v", err)
	}
	want := []alter.SwatchResult{{Path: remaining, Category: alter.WouldRemove}}
	if !slices.Equal(first, want) {
		t.Errorf("first results = %v, want %v", first, want)
	}

	second, err := alter.ProcessRetiredWorkflows(dir, alter.Apply)
	if err != nil {
		t.Fatalf("retry ProcessRetiredWorkflows() error: %v", err)
	}
	if len(second) != 0 {
		t.Errorf("retry results = %v, want no results", second)
	}
}
