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
	"github.com/wimpysworld/tailor/internal/output"
)

var managedPlaywrightPaths = []string{
	"nix/playwright.nix",
	".mcp.json",
	".codex/config.toml",
	"opencode.json",
	".pi/mcp.json",
}

func TestManagedPlaywrightTrueCreatesFiveFilesWithoutPages(t *testing.T) {
	for _, mode := range []ApplyMode{DryRun, Apply, Recut} {
		t.Run(managedModeName(mode), func(t *testing.T) {
			dir := t.TempDir()
			cfg := managedPlaywrightConfig(true)
			for range 2 {
				before := snapshotManagedTree(t, dir)
				report, err := Execute(cfg, dir, mode, managedProductionClient(t), io.Discard, Options{})
				if err != nil {
					t.Fatal(err)
				}
				for _, path := range managedPlaywrightPaths {
					if !managedReportHasPath(report, path) {
						t.Errorf("report lacks Playwright path %q", path)
					}
					if mode == DryRun {
						assertManagedMissing(t, filepath.Join(dir, filepath.FromSlash(path)))
					} else if _, err := os.Lstat(filepath.Join(dir, filepath.FromSlash(path))); err != nil {
						t.Errorf("Playwright path %q is absent: %v", path, err)
					}
				}
				if managedReportHasPath(report, "just/pages.just") || managedReportHasPath(report, "nix/pages.nix") {
					t.Fatal("Playwright enabled Pages fragments")
				}
				if mode == DryRun {
					if after := snapshotManagedTree(t, dir); !reflect.DeepEqual(after, before) {
						t.Fatalf("preview changed files: before=%v after=%v", before, after)
					}
				}
			}
		})
	}
}

func TestManagedPlaywrightFalseWarnsWhenFragmentIsMissing(t *testing.T) {
	for _, mode := range []ApplyMode{DryRun, Apply, Recut} {
		t.Run(managedModeName(mode), func(t *testing.T) {
			dir := t.TempDir()
			report, err := Execute(managedPlaywrightConfig(false), dir, mode, managedProductionClient(t), io.Discard, Options{})
			if err != nil {
				t.Fatal(err)
			}
			assertManagedMissing(t, filepath.Join(dir, "nix/playwright.nix"))
			if !strings.Contains(report.Plain, "missing `playwright-mcp` executable") {
				t.Fatalf("report lacks retained-settings warning: %s", report.Plain)
			}
		})
	}
}

func TestManagedPlaywrightPreservesSharedRegularAndFinalLinks(t *testing.T) {
	for _, mode := range []ApplyMode{Apply, Recut} {
		t.Run(managedModeName(mode), func(t *testing.T) {
			dir := t.TempDir()
			regular := map[string][]byte{
				".mcp.json":     []byte("custom Claude settings\n"),
				"opencode.json": []byte("custom OpenCode settings\n"),
			}
			for path, content := range regular {
				writeManagedTestFile(t, dir, path, content)
			}
			outside := filepath.Join(t.TempDir(), "codex")
			if err := os.WriteFile(outside, []byte("custom Codex settings\n"), 0o600); err != nil {
				t.Fatal(err)
			}
			if err := os.MkdirAll(filepath.Join(dir, ".codex"), 0o755); err != nil {
				t.Fatal(err)
			}
			managedSymlinkOrSkip(t, outside, filepath.Join(dir, ".codex/config.toml"))
			danglingTarget := filepath.Join(t.TempDir(), "missing")
			if err := os.MkdirAll(filepath.Join(dir, ".pi"), 0o755); err != nil {
				t.Fatal(err)
			}
			managedSymlinkOrSkip(t, danglingTarget, filepath.Join(dir, ".pi/mcp.json"))
			before := snapshotManagedTree(t, dir)

			report, err := Execute(managedPlaywrightConfig(true), dir, mode, managedProductionClient(t), io.Discard, Options{})
			if err != nil {
				t.Fatal(err)
			}
			for path, content := range regular {
				assertManagedBytes(t, filepath.Join(dir, path), content)
			}
			assertManagedBytes(t, outside, []byte("custom Codex settings\n"))
			for _, path := range managedPlaywrightPaths[1:] {
				if !containsGuidancePath(report.Document.Guidance, path) {
					t.Errorf("report lacks manual merge guidance for %q", path)
				}
			}
			after := snapshotManagedTree(t, dir)
			for _, path := range []string{".mcp.json", ".codex/config.toml", "opencode.json", ".pi/mcp.json"} {
				if !reflect.DeepEqual(after[path], before[path]) {
					t.Errorf("protected path %q changed: got=%#v want=%#v", path, after[path], before[path])
				}
			}
		})
	}
}

func TestManagedPlaywrightAbsentDoesNotInspectReservedPaths(t *testing.T) {
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, ".mcp.json"), 0o755); err != nil {
		t.Fatal(err)
	}
	managedSymlinkOrSkip(t, t.TempDir(), filepath.Join(dir, ".codex"))
	managedFIFOOrSkip(t, filepath.Join(dir, "opencode.json"))
	writeManagedTestFile(t, dir, "nix/playwright.nix", []byte("unowned\n"))
	before := snapshotManagedTree(t, dir)

	if _, err := Execute(&config.Config{License: "none"}, dir, Apply, managedProductionClient(t), io.Discard, Options{}); err != nil {
		t.Fatal(err)
	}
	after := snapshotManagedTree(t, dir)
	for _, path := range []string{".mcp.json", ".codex", "opencode.json", "nix/playwright.nix"} {
		if !reflect.DeepEqual(after[path], before[path]) {
			t.Errorf("absent Playwright changed %q", path)
		}
	}
}

func TestManagedPlaywrightRejectsUnsafeActiveDestinationsReadOnly(t *testing.T) {
	tests := []struct {
		name, want string
		setup      func(*testing.T, string)
	}{
		{name: "linked parent", want: `managed destination parent ".codex" is a symlink`, setup: func(t *testing.T, dir string) {
			managedSymlinkOrSkip(t, t.TempDir(), filepath.Join(dir, ".codex"))
		}},
		{name: "directory", want: `managed destination ".mcp.json" is a directory`, setup: func(t *testing.T, dir string) {
			if err := os.Mkdir(filepath.Join(dir, ".mcp.json"), 0o755); err != nil {
				t.Fatal(err)
			}
		}},
		{name: "special", want: `managed destination "opencode.json" is not a regular file or symlink`, setup: func(t *testing.T, dir string) {
			managedFIFOOrSkip(t, filepath.Join(dir, "opencode.json"))
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			tt.setup(t, dir)
			before := snapshotManagedTree(t, dir)
			_, err := prepareManagedExecution(managedPlaywrightConfig(true), dir, func(selections []managedSelection) (managedRenderedFiles, error) {
				return renderManagedFiles(managedPlaywrightConfig(true), selections)
			}, true)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("preflight error = %v, want %q", err, tt.want)
			}
			if after := snapshotManagedTree(t, dir); !reflect.DeepEqual(after, before) {
				t.Fatalf("failed preflight changed files: before=%v after=%v", before, after)
			}
		})
	}
}

func TestManagedPlaywrightWriteFailureRetries(t *testing.T) {
	dir := t.TempDir()
	cfg := managedPlaywrightConfig(true)
	execution, err := prepareManagedExecution(cfg, dir, func(selections []managedSelection) (managedRenderedFiles, error) {
		return renderManagedFiles(cfg, selections)
	}, true)
	if err != nil {
		t.Fatal(err)
	}
	injected := errors.New("injected Playwright write failure")
	fragmentPath := managedPlaywrightPaths[0]
	results, err := applyManagedFilesWithHooks(dir, execution.plan, managedApplyHooks{beforeTempCreate: func(path string) error {
		if path == fragmentPath {
			return injected
		}
		return nil
	}})
	if !errors.Is(err, injected) || containsManagedApplyPath(results, fragmentPath) {
		t.Fatalf("failed apply results=%v error=%v", results, err)
	}
	assertManagedMissing(t, filepath.Join(dir, fragmentPath))

	retry, err := applyManagedFiles(dir, execution.plan)
	if err != nil {
		t.Fatal(err)
	}
	if !containsManagedApplyPath(retry, fragmentPath) {
		t.Fatalf("retry results omit Playwright fragment: %v", retry)
	}
}

func managedPlaywrightConfig(enabled bool) *config.Config {
	return &config.Config{License: "none", MCP: &config.MCPSettings{Playwright: &enabled}}
}

func managedModeName(mode ApplyMode) string {
	switch mode {
	case DryRun:
		return "baste"
	case Apply:
		return "apply"
	case Recut:
		return "recut"
	default:
		return "unknown"
	}
}

func managedReportHasPath(report Report, path string) bool {
	for _, item := range report.Document.Items {
		if item.Name == path {
			return true
		}
	}
	return false
}

func containsGuidancePath(guidance []output.Guidance, path string) bool {
	for _, item := range guidance {
		if strings.Contains(item.Text, "`"+path+"`") {
			return true
		}
	}
	return false
}
