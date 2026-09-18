package alter_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/wimpysworld/tailor/internal/alter"
	"github.com/wimpysworld/tailor/internal/swatch"
)

var playwrightPaths = []string{
	"nix/playwright.nix",
	".mcp.json",
	".codex/config.toml",
	"opencode.json",
	".pi/mcp.json",
}

func TestMCPDeclarationsControlPlaywrightFilesAcrossAlterModes(t *testing.T) {
	modes := []struct {
		name string
		mode alter.ApplyMode
	}{
		{name: "baste", mode: alter.DryRun},
		{name: "apply", mode: alter.Apply},
		{name: "recut", mode: alter.Recut},
	}

	for _, mode := range modes {
		t.Run(mode.name+"/absent", func(t *testing.T) {
			tc := setupAlterTest(t, "license: none\nswatches: []\n")
			for _, path := range playwrightPaths {
				writeOnDisk(t, tc.Dir, path, []byte("unmanaged "+path+"\n"))
			}

			output := captureAlterRun(t, loadTestConfig(t, tc.Dir), tc.Dir, mode.mode, tc.Client)
			for _, path := range playwrightPaths {
				assertCapabilityBytes(t, tc.Dir, path, []byte("unmanaged "+path+"\n"))
			}
			if strings.Contains(output, "playwright") || strings.Contains(output, "Playwright") {
				t.Fatalf("absent declaration reported Playwright: %s", output)
			}
		})

		t.Run(mode.name+"/true-without-pages", func(t *testing.T) {
			tc := setupAlterTest(t, "license: none\nmcp:\n  playwright: true\nswatches: []\n")
			output := captureAlterRun(t, loadTestConfig(t, tc.Dir), tc.Dir, mode.mode, tc.Client)

			for _, path := range playwrightPaths {
				if !strings.Contains(output, path) {
					t.Errorf("Playwright output lacks %q: %s", path, output)
				}
				if mode.mode == alter.DryRun {
					assertCapabilityMissing(t, tc.Dir, path)
					continue
				}
				want, err := swatch.Content(path)
				if err != nil {
					t.Fatal(err)
				}
				assertCapabilityBytes(t, tc.Dir, path, want)
			}
			if strings.Contains(output, "just/pages.just") || strings.Contains(output, "nix/pages.nix") {
				t.Fatalf("Playwright enabled Pages files: %s", output)
			}
			if calls := tc.MutatingCalls(); len(calls) != 0 {
				t.Fatalf("Playwright declaration caused mutating API calls: %v", calls)
			}
		})

		t.Run(mode.name+"/false", func(t *testing.T) {
			tc := setupAlterTest(t, "license: none\nmcp:\n  playwright: false\nswatches: []\n")
			fragment, err := swatch.Content("nix/playwright.nix")
			if err != nil {
				t.Fatal(err)
			}
			writeOnDisk(t, tc.Dir, "nix/playwright.nix", fragment)
			for _, path := range playwrightPaths[1:] {
				writeOnDisk(t, tc.Dir, path, []byte("retained "+path+"\n"))
			}

			output := captureAlterRun(t, loadTestConfig(t, tc.Dir), tc.Dir, mode.mode, tc.Client)
			for _, path := range playwrightPaths[1:] {
				assertCapabilityBytes(t, tc.Dir, path, []byte("retained "+path+"\n"))
			}
			if mode.mode == alter.DryRun {
				assertCapabilityBytes(t, tc.Dir, "nix/playwright.nix", fragment)
			} else {
				assertCapabilityMissing(t, tc.Dir, "nix/playwright.nix")
			}
			if !strings.Contains(output, "missing `playwright-mcp` executable") {
				t.Fatalf("disabled Playwright output lacks retained-settings warning: %s", output)
			}
		})
	}
}

func assertCapabilityBytes(t *testing.T, root, path string, want []byte) {
	t.Helper()
	got, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(path)))
	if err != nil || string(got) != string(want) {
		t.Fatalf("file %q content=%q error=%v, want %q", path, got, err, want)
	}
}

func assertCapabilityMissing(t *testing.T, root, path string) {
	t.Helper()
	if _, err := os.Lstat(filepath.Join(root, filepath.FromSlash(path))); !os.IsNotExist(err) {
		t.Fatalf("file %q exists or cannot be checked: %v", path, err)
	}
}
