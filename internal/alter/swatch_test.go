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

// captureStderr calls fn while redirecting os.Stderr to a pipe and returns
// whatever was written.
func captureStderr(t *testing.T, fn func()) string {
	t.Helper()
	oldStderr := os.Stderr
	r, w, _ := os.Pipe()
	os.Stderr = w

	fn()

	w.Close()
	os.Stderr = oldStderr

	var buf bytes.Buffer
	_, _ = buf.ReadFrom(r)
	return buf.String()
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

func TestAlwaysNoChangeWhenMD5Matches(t *testing.T) {
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

func TestAlwaysWouldOverwriteWhenMD5Differs(t *testing.T) {
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

func TestAlwaysReturnsOnDiskReadError(t *testing.T) {
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, "CODE_OF_CONDUCT.md"), 0o755); err != nil {
		t.Fatal(err)
	}

	cfg := newConfig(entry("CODE_OF_CONDUCT.md", swatch.Always))
	_, err := alter.ProcessSwatches(cfg, dir, alter.DryRun, &alter.TokenContext{})
	if err == nil {
		t.Fatal("ProcessSwatches() error = nil, want read error")
	}
	if !strings.Contains(err.Error(), `hashing on-disk file "CODE_OF_CONDUCT.md"`) {
		t.Errorf("error = %q, want on-disk hash context", err)
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
