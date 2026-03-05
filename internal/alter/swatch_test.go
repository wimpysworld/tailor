package alter_test

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/wimpysworld/tailor/internal/alter"
	"github.com/wimpysworld/tailor/internal/config"
	"github.com/wimpysworld/tailor/internal/swatch"
)

func newConfig(entries ...config.SwatchEntry) *config.Config {
	return &config.Config{Swatches: entries}
}

func entry(source, dest string, mode swatch.AlterationMode) config.SwatchEntry {
	return config.SwatchEntry{Source: source, Destination: dest, Alteration: mode}
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
	buf.ReadFrom(r)
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

func TestFirstFitSkipWhenExists(t *testing.T) {
	dir := t.TempDir()
	writeOnDisk(t, dir, ".gitignore", []byte("existing"))

	cfg := newConfig(entry(".gitignore", ".gitignore", swatch.FirstFit))
	results, err := alter.ProcessSwatches(cfg, dir, alter.DryRun, &alter.TokenContext{})
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 {
		t.Fatalf("got %d results, want 1", len(results))
	}
	if results[0].Category != alter.Skipped {
		t.Errorf("category = %q, want %q", results[0].Category, alter.Skipped)
	}
}

func TestFirstFitCopyWhenAbsent(t *testing.T) {
	dir := t.TempDir()

	cfg := newConfig(entry(".gitignore", ".gitignore", swatch.FirstFit))
	results, err := alter.ProcessSwatches(cfg, dir, alter.DryRun, &alter.TokenContext{})
	if err != nil {
		t.Fatal(err)
	}
	if results[0].Category != alter.WouldCopy {
		t.Errorf("category = %q, want %q", results[0].Category, alter.WouldCopy)
	}
	// Dry run: file should not exist.
	if _, err := os.Stat(filepath.Join(dir, ".gitignore")); err == nil {
		t.Error("dry run wrote file to disk")
	}
}

func TestFirstFitApplyWritesFile(t *testing.T) {
	dir := t.TempDir()

	cfg := newConfig(entry(".gitignore", ".gitignore", swatch.FirstFit))
	results, err := alter.ProcessSwatches(cfg, dir, alter.Apply, &alter.TokenContext{})
	if err != nil {
		t.Fatal(err)
	}
	if results[0].Category != alter.WouldCopy {
		t.Errorf("category = %q, want %q", results[0].Category, alter.WouldCopy)
	}
	// Apply: file should exist.
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

	cfg := newConfig(entry("CODE_OF_CONDUCT.md", "CODE_OF_CONDUCT.md", swatch.Always))
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

	cfg := newConfig(entry("CODE_OF_CONDUCT.md", "CODE_OF_CONDUCT.md", swatch.Always))
	results, err := alter.ProcessSwatches(cfg, dir, alter.DryRun, &alter.TokenContext{})
	if err != nil {
		t.Fatal(err)
	}
	if results[0].Category != alter.WouldOverwrite {
		t.Errorf("category = %q, want %q", results[0].Category, alter.WouldOverwrite)
	}
}

func TestAlwaysSubstitutedSourceAlwaysOverwrites(t *testing.T) {
	dir := t.TempDir()
	// Write identical content; substituted sources skip MD5, so result is still WouldOverwrite.
	content := mustContent(t, "SECURITY.md")
	writeOnDisk(t, dir, "SECURITY.md", content)

	cfg := newConfig(entry("SECURITY.md", "SECURITY.md", swatch.Always))
	results, err := alter.ProcessSwatches(cfg, dir, alter.DryRun, &alter.TokenContext{})
	if err != nil {
		t.Fatal(err)
	}
	if results[0].Category != alter.WouldOverwrite {
		t.Errorf("category = %q, want %q", results[0].Category, alter.WouldOverwrite)
	}
}

func TestForceApplyOverwritesExisting(t *testing.T) {
	dir := t.TempDir()
	writeOnDisk(t, dir, ".gitignore", []byte("old"))

	cfg := newConfig(entry(".gitignore", ".gitignore", swatch.FirstFit))
	results, err := alter.ProcessSwatches(cfg, dir, alter.ForceApply, &alter.TokenContext{})
	if err != nil {
		t.Fatal(err)
	}
	if results[0].Category != alter.WouldOverwrite {
		t.Errorf("category = %q, want %q", results[0].Category, alter.WouldOverwrite)
	}
	// Verify file was actually overwritten.
	data, err := os.ReadFile(filepath.Join(dir, ".gitignore"))
	if err != nil {
		t.Fatalf("file not found: %v", err)
	}
	want := mustContent(t, ".gitignore")
	if string(data) != string(want) {
		t.Error("force-apply did not overwrite file with embedded content")
	}
}

func TestForceApplyConfigYmlExempt(t *testing.T) {
	dir := t.TempDir()
	writeOnDisk(t, dir, ".tailor/config.yml", []byte("existing config"))

	cfg := newConfig(entry(".tailor/config.yml", ".tailor/config.yml", swatch.FirstFit))
	results, err := alter.ProcessSwatches(cfg, dir, alter.ForceApply, &alter.TokenContext{})
	if err != nil {
		t.Fatal(err)
	}
	if results[0].Category != alter.Skipped {
		t.Errorf("category = %q, want %q", results[0].Category, alter.Skipped)
	}
	// Verify file was NOT overwritten.
	data, err := os.ReadFile(filepath.Join(dir, ".tailor/config.yml"))
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "existing config" {
		t.Error("force-apply overwrote exempt .tailor/config.yml")
	}
}

func TestWouldCopyWhenAbsentRegardlessOfMode(t *testing.T) {
	modes := []struct {
		name string
		mode alter.ApplyMode
	}{
		{"DryRun", alter.DryRun},
		{"Apply", alter.Apply},
		{"ForceApply", alter.ForceApply},
	}

	for _, m := range modes {
		t.Run(m.name, func(t *testing.T) {
			dir := t.TempDir()
			cfg := newConfig(entry(".gitignore", ".gitignore", swatch.FirstFit))
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

	cfg := newConfig(entry("CODE_OF_CONDUCT.md", "CODE_OF_CONDUCT.md", swatch.Always))
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

func TestNestedDestinationCreatesDirectories(t *testing.T) {
	dir := t.TempDir()

	cfg := newConfig(entry(".github/ISSUE_TEMPLATE/bug_report.yml", ".github/ISSUE_TEMPLATE/bug_report.yml", swatch.Always))
	_, err := alter.ProcessSwatches(cfg, dir, alter.Apply, &alter.TokenContext{})
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, ".github/ISSUE_TEMPLATE/bug_report.yml")
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("nested file not created: %v", err)
	}
}
