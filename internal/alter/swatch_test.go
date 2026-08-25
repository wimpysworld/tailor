package alter_test

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/wimpysworld/tailor/internal/alter"
	"github.com/wimpysworld/tailor/internal/config"
	"github.com/wimpysworld/tailor/internal/swatch"
)

func newConfig(entries ...config.SwatchEntry) *config.Config {
	return &config.Config{Swatches: entries}
}

func entry(path string, mode swatch.AlterationMode) config.SwatchEntry {
	return config.SwatchEntry{Path: path, Alteration: mode}
}

func mustContent(t *testing.T, source string) []byte {
	t.Helper()
	data, err := swatch.Content(source)
	if err != nil {
		t.Fatalf("swatch.Content(%q): %v", source, err)
	}
	return data
}

func writeOnDisk(t *testing.T, dir, rel string, data []byte) {
	t.Helper()
	path := filepath.Join(dir, rel)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
}

func symlinkOrSkip(t *testing.T, oldname, newname string) {
	t.Helper()
	if err := os.Symlink(oldname, newname); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
}

func TestFirstFitSkipWhenExists(t *testing.T) {
	dir := t.TempDir()
	writeOnDisk(t, dir, ".gitignore", []byte("existing"))

	cfg := newConfig(entry(".gitignore", swatch.FirstFit))
	results, err := alter.ProcessSwatches(cfg, dir, alter.DryRun, &alter.TokenContext{})
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 {
		t.Fatalf("got %d results, want 1", len(results))
	}
	if results[0].Category != alter.Skipped || results[0].Reason != alter.SkipFirstFitExists {
		t.Errorf("result = %+v, want skipped because first-fit destination exists", results[0])
	}
}

func TestFirstFitCopyWhenAbsent(t *testing.T) {
	dir := t.TempDir()

	cfg := newConfig(entry(".gitignore", swatch.FirstFit))
	results, err := alter.ProcessSwatches(cfg, dir, alter.DryRun, &alter.TokenContext{})
	if err != nil {
		t.Fatal(err)
	}
	if results[0].Category != alter.WouldCopy {
		t.Errorf("category = %q, want %q", results[0].Category, alter.WouldCopy)
	}
	// Dry-run reports the copy without creating the file.
	if _, err := os.Stat(filepath.Join(dir, ".gitignore")); err == nil {
		t.Error("dry run wrote file to disk")
	}
}

func TestFirstFitApplyWritesFile(t *testing.T) {
	dir := t.TempDir()

	cfg := newConfig(entry(".gitignore", swatch.FirstFit))
	results, err := alter.ProcessSwatches(cfg, dir, alter.Apply, &alter.TokenContext{})
	if err != nil {
		t.Fatal(err)
	}
	if results[0].Category != alter.WouldCopy {
		t.Errorf("category = %q, want %q", results[0].Category, alter.WouldCopy)
	}
	// Apply writes the missing file.
	data, err := os.ReadFile(filepath.Join(dir, ".gitignore"))
	if err != nil {
		t.Fatalf("file not written: %v", err)
	}
	want := mustContent(t, ".gitignore")
	if string(data) != string(want) {
		t.Error("written content does not match embedded swatch")
	}
}

func TestFirstFitRejectsDirectoryDestination(t *testing.T) {
	tests := []struct {
		name string
		mode alter.ApplyMode
	}{
		{name: "dry-run", mode: alter.DryRun},
		{name: "apply", mode: alter.Apply},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			dir := t.TempDir()
			path := filepath.Join(dir, ".gitignore")
			if err := os.Mkdir(path, 0o755); err != nil {
				t.Fatal(err)
			}

			cfg := newConfig(entry(".gitignore", swatch.FirstFit))
			_, err := alter.ProcessSwatches(cfg, dir, test.mode, &alter.TokenContext{})
			if err == nil {
				t.Fatal("ProcessSwatches() error = nil, want directory error")
			}
			if !strings.Contains(err.Error(), `swatch destination ".gitignore" is a directory`) {
				t.Errorf("error = %q, want directory error", err)
			}
			info, statErr := os.Stat(path)
			if statErr != nil {
				t.Fatal(statErr)
			}
			if !info.IsDir() {
				t.Error("destination is not a directory")
			}
		})
	}
}

func TestAlwaysNoChangeWhenSHA256Matches(t *testing.T) {
	dir := t.TempDir()
	content := mustContent(t, "CODE_OF_CONDUCT.md")
	writeOnDisk(t, dir, "CODE_OF_CONDUCT.md", content)

	cfg := newConfig(entry("CODE_OF_CONDUCT.md", swatch.Always))
	results, err := alter.ProcessSwatches(cfg, dir, alter.DryRun, &alter.TokenContext{})
	if err != nil {
		t.Fatal(err)
	}
	if results[0].Category != alter.NoChange {
		t.Errorf("category = %q, want %q", results[0].Category, alter.NoChange)
	}
}

func TestAlwaysWouldOverwriteWhenSHA256Differs(t *testing.T) {
	dir := t.TempDir()
	writeOnDisk(t, dir, "CODE_OF_CONDUCT.md", []byte("old content"))

	cfg := newConfig(entry("CODE_OF_CONDUCT.md", swatch.Always))
	results, err := alter.ProcessSwatches(cfg, dir, alter.DryRun, &alter.TokenContext{})
	if err != nil {
		t.Fatal(err)
	}
	if results[0].Category != alter.WouldOverwrite {
		t.Errorf("category = %q, want %q", results[0].Category, alter.WouldOverwrite)
	}
}

func TestAlwaysRejectsDirectoryDestination(t *testing.T) {
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, "CODE_OF_CONDUCT.md"), 0o755); err != nil {
		t.Fatal(err)
	}

	cfg := newConfig(entry("CODE_OF_CONDUCT.md", swatch.Always))
	_, err := alter.ProcessSwatches(cfg, dir, alter.DryRun, &alter.TokenContext{})
	if err == nil {
		t.Fatal("ProcessSwatches() error = nil, want directory error")
	}
	if !strings.Contains(err.Error(), `swatch destination "CODE_OF_CONDUCT.md" is a directory`) {
		t.Errorf("error = %q, want directory error", err)
	}
}

func TestAlwaysSubstitutedSourceNoChangeWhenHashMatches(t *testing.T) {
	dir := t.TempDir()
	// Identical resolved content exercises the substituted-source hash comparison.
	content := mustContent(t, "SECURITY.md")
	writeOnDisk(t, dir, "SECURITY.md", content)

	cfg := newConfig(entry("SECURITY.md", swatch.Always))
	results, err := alter.ProcessSwatches(cfg, dir, alter.DryRun, &alter.TokenContext{})
	if err != nil {
		t.Fatal(err)
	}
	if results[0].Category != alter.NoChange {
		t.Errorf("category = %q, want %q", results[0].Category, alter.NoChange)
	}
}

func TestAlwaysSubstitutedSourceOverwritesWhenDifferent(t *testing.T) {
	dir := t.TempDir()
	// Different on-disk content reports an overwrite.
	writeOnDisk(t, dir, "SECURITY.md", []byte("stale on-disk content"))

	cfg := newConfig(entry("SECURITY.md", swatch.Always))
	results, err := alter.ProcessSwatches(cfg, dir, alter.DryRun, &alter.TokenContext{})
	if err != nil {
		t.Fatal(err)
	}
	if results[0].Category != alter.WouldOverwrite {
		t.Errorf("category = %q, want %q", results[0].Category, alter.WouldOverwrite)
	}
}

func TestRecutOverwritesExisting(t *testing.T) {
	dir := t.TempDir()
	writeOnDisk(t, dir, ".gitignore", []byte("old"))

	cfg := newConfig(entry(".gitignore", swatch.FirstFit))
	results, err := alter.ProcessSwatches(cfg, dir, alter.Recut, &alter.TokenContext{})
	if err != nil {
		t.Fatal(err)
	}
	if results[0].Category != alter.WouldOverwrite {
		t.Errorf("category = %q, want %q", results[0].Category, alter.WouldOverwrite)
	}
	data, err := os.ReadFile(filepath.Join(dir, ".gitignore"))
	if err != nil {
		t.Fatalf("file not found: %v", err)
	}
	want := mustContent(t, ".gitignore")
	if string(data) != string(want) {
		t.Error("recut did not overwrite file with embedded content")
	}
}

func TestConfigYmlSkippedInProcessSwatches(t *testing.T) {
	dir := t.TempDir()
	writeOnDisk(t, dir, ".tailor.yml", []byte("old content"))

	cfg := newConfig(entry(".tailor.yml", swatch.Always))
	results, err := alter.ProcessSwatches(cfg, dir, alter.Recut, &alter.TokenContext{})
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 0 {
		t.Errorf("expected no results for config swatch, got %d", len(results))
	}
}

func TestWouldCopyWhenAbsentRegardlessOfMode(t *testing.T) {
	modes := []struct {
		name string
		mode alter.ApplyMode
	}{
		{"DryRun", alter.DryRun},
		{"Apply", alter.Apply},
		{"Recut", alter.Recut},
	}

	for _, m := range modes {
		t.Run(m.name, func(t *testing.T) {
			dir := t.TempDir()
			cfg := newConfig(entry(".gitignore", swatch.FirstFit))
			results, err := alter.ProcessSwatches(cfg, dir, m.mode, &alter.TokenContext{})
			if err != nil {
				t.Fatal(err)
			}
			if results[0].Category != alter.WouldCopy {
				t.Errorf("category = %q, want %q", results[0].Category, alter.WouldCopy)
			}
		})
	}
}

func TestAlwaysApplyWritesOnOverwrite(t *testing.T) {
	dir := t.TempDir()
	writeOnDisk(t, dir, "CODE_OF_CONDUCT.md", []byte("old"))

	cfg := newConfig(entry("CODE_OF_CONDUCT.md", swatch.Always))
	results, err := alter.ProcessSwatches(cfg, dir, alter.Apply, &alter.TokenContext{})
	if err != nil {
		t.Fatal(err)
	}
	if results[0].Category != alter.WouldOverwrite {
		t.Errorf("category = %q, want %q", results[0].Category, alter.WouldOverwrite)
	}
	data, err := os.ReadFile(filepath.Join(dir, "CODE_OF_CONDUCT.md"))
	if err != nil {
		t.Fatal(err)
	}
	want := mustContent(t, "CODE_OF_CONDUCT.md")
	if string(data) != string(want) {
		t.Error("Apply mode did not write file on overwrite")
	}
}

func TestNeverSkipsRegardlessOfFileExistence(t *testing.T) {
	modes := []struct {
		name string
		mode alter.ApplyMode
	}{
		{"DryRun", alter.DryRun},
		{"Apply", alter.Apply},
		{"Recut", alter.Recut},
	}

	for _, m := range modes {
		t.Run(m.name+"/absent", func(t *testing.T) {
			dir := t.TempDir()
			cfg := newConfig(entry(".gitignore", swatch.Never))
			results, err := alter.ProcessSwatches(cfg, dir, m.mode, &alter.TokenContext{})
			if err != nil {
				t.Fatal(err)
			}
			if len(results) != 1 {
				t.Fatalf("got %d results, want 1", len(results))
			}
			if results[0].Category != alter.Skipped || results[0].Reason != alter.SkipModeNever {
				t.Errorf("result = %+v, want skipped because mode is never", results[0])
			}
			if _, err := os.Stat(filepath.Join(dir, ".gitignore")); err == nil {
				t.Error("never mode wrote file to disk")
			}
		})

		t.Run(m.name+"/exists", func(t *testing.T) {
			dir := t.TempDir()
			writeOnDisk(t, dir, ".gitignore", []byte("existing"))
			cfg := newConfig(entry(".gitignore", swatch.Never))
			results, err := alter.ProcessSwatches(cfg, dir, m.mode, &alter.TokenContext{})
			if err != nil {
				t.Fatal(err)
			}
			if len(results) != 1 {
				t.Fatalf("got %d results, want 1", len(results))
			}
			if results[0].Category != alter.Skipped || results[0].Reason != alter.SkipModeNever {
				t.Errorf("result = %+v, want skipped because mode is never", results[0])
			}
			data, err := os.ReadFile(filepath.Join(dir, ".gitignore"))
			if err != nil {
				t.Fatal(err)
			}
			if string(data) != "existing" {
				t.Error("never mode modified existing file")
			}
		})
	}
}

func TestNestedDestinationCreatesDirectories(t *testing.T) {
	dir := t.TempDir()

	cfg := newConfig(entry(".github/ISSUE_TEMPLATE/bug_report.yml", swatch.Always))
	_, err := alter.ProcessSwatches(cfg, dir, alter.Apply, &alter.TokenContext{})
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, ".github/ISSUE_TEMPLATE/bug_report.yml")
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("nested file not created: %v", err)
	}
}

func TestSwatchSymlinkParentEscapeRejectsWrite(t *testing.T) {
	dir := t.TempDir()
	outside := t.TempDir()
	writeOnDisk(t, outside, "ISSUE_TEMPLATE/bug_report.yml", []byte("outside"))
	symlinkOrSkip(t, outside, filepath.Join(dir, ".github"))

	cfg := newConfig(entry(".github/ISSUE_TEMPLATE/bug_report.yml", swatch.Always))
	_, err := alter.ProcessSwatches(cfg, dir, alter.Apply, &alter.TokenContext{})
	if err == nil {
		t.Fatal("expected symlink escape write error, got nil")
	}

	data, err := os.ReadFile(filepath.Join(outside, "ISSUE_TEMPLATE/bug_report.yml"))
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "outside" {
		t.Fatalf("outside file changed to %q", string(data))
	}
}

func TestSwatchRejectsInRootRelativeParentSymlink(t *testing.T) {
	tests := []struct {
		name       string
		alteration swatch.AlterationMode
		mode       alter.ApplyMode
	}{
		{name: "always", alteration: swatch.Always, mode: alter.Apply},
		{name: "first-fit", alteration: swatch.FirstFit, mode: alter.Apply},
		{name: "recut", alteration: swatch.FirstFit, mode: alter.Recut},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			dir := t.TempDir()
			target := "managed/ISSUE_TEMPLATE/bug_report.yml"
			writeOnDisk(t, dir, target, []byte("unchanged"))
			symlinkOrSkip(t, "managed", filepath.Join(dir, ".github"))

			cfg := newConfig(entry(".github/ISSUE_TEMPLATE/bug_report.yml", test.alteration))
			_, err := alter.ProcessSwatches(cfg, dir, test.mode, &alter.TokenContext{})
			if err == nil {
				t.Fatal("ProcessSwatches() error = nil, want parent symlink error")
			}
			if !strings.Contains(err.Error(), `swatch parent ".github" is a symlink`) {
				t.Errorf("error = %q, want parent symlink error", err)
			}

			data, readErr := os.ReadFile(filepath.Join(dir, target))
			if readErr != nil {
				t.Fatal(readErr)
			}
			if string(data) != "unchanged" {
				t.Fatalf("target file changed to %q", data)
			}
		})
	}
}

func TestSwatchReplacesDestinationSymlinkWithoutFollowingIt(t *testing.T) {
	modes := []struct {
		name       string
		alteration swatch.AlterationMode
		mode       alter.ApplyMode
	}{
		{name: "always", alteration: swatch.Always, mode: alter.Apply},
		{name: "first-fit", alteration: swatch.FirstFit, mode: alter.Apply},
		{name: "recut", alteration: swatch.FirstFit, mode: alter.Recut},
	}
	targets := []struct {
		name   string
		exists bool
	}{
		{name: "existing", exists: true},
		{name: "dangling", exists: false},
	}

	for _, mode := range modes {
		for _, target := range targets {
			t.Run(mode.name+"/"+target.name, func(t *testing.T) {
				dir := t.TempDir()
				managed := ".github/ISSUE_TEMPLATE/bug_report.yml"
				targetPath := ".github/ISSUE_TEMPLATE/target.yml"
				if target.exists {
					writeOnDisk(t, dir, targetPath, []byte("unchanged"))
				} else if err := os.MkdirAll(filepath.Join(dir, filepath.Dir(managed)), 0o755); err != nil {
					t.Fatal(err)
				}
				symlinkOrSkip(t, filepath.Base(targetPath), filepath.Join(dir, managed))

				cfg := newConfig(entry(managed, mode.alteration))
				results, err := alter.ProcessSwatches(cfg, dir, mode.mode, &alter.TokenContext{})
				if err != nil {
					t.Fatal(err)
				}
				if len(results) != 1 || results[0].Category != alter.WouldCopy {
					t.Errorf("results = %+v, want one would-copy result", results)
				}

				info, err := os.Lstat(filepath.Join(dir, managed))
				if err != nil {
					t.Fatal(err)
				}
				if !info.Mode().IsRegular() {
					t.Errorf("managed path mode = %v, want regular file", info.Mode())
				}
				data, err := os.ReadFile(filepath.Join(dir, managed))
				if err != nil {
					t.Fatal(err)
				}
				if want := mustContent(t, managed); !bytes.Equal(data, want) {
					t.Error("managed path content does not match embedded swatch")
				}

				if target.exists {
					data, err := os.ReadFile(filepath.Join(dir, targetPath))
					if err != nil {
						t.Fatal(err)
					}
					if string(data) != "unchanged" {
						t.Errorf("target content = %q, want unchanged", data)
					}
				} else if _, err := os.Lstat(filepath.Join(dir, targetPath)); !os.IsNotExist(err) {
					t.Errorf("dangling target exists or returned an unexpected error: %v", err)
				}
			})
		}
	}
}
