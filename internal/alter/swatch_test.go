package alter_test

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/wimpysworld/tailor/internal/alter"
	"github.com/wimpysworld/tailor/internal/config"
	"github.com/wimpysworld/tailor/internal/ptr"
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
	if results[0].Category != alter.SkippedFirstFit {
		t.Errorf("category = %q, want %q", results[0].Category, alter.SkippedFirstFit)
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

func TestAlwaysSubstitutedSourceNoChangeWhenHashMatches(t *testing.T) {
	dir := t.TempDir()
	// Write identical resolved content; hash comparison now applies to substituted sources too.
	content := mustContent(t, "SECURITY.md")
	writeOnDisk(t, dir, "SECURITY.md", content)

	cfg := newConfig(entry("SECURITY.md", "SECURITY.md", swatch.Always))
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
	// On-disk content differs from resolved swatch content; expect overwrite.
	writeOnDisk(t, dir, "SECURITY.md", []byte("stale on-disk content"))

	cfg := newConfig(entry("SECURITY.md", "SECURITY.md", swatch.Always))
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

	cfg := newConfig(entry(".gitignore", ".gitignore", swatch.FirstFit))
	results, err := alter.ProcessSwatches(cfg, dir, alter.Recut, &alter.TokenContext{})
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
		t.Error("recut did not overwrite file with embedded content")
	}
}

func TestConfigYmlSkippedInProcessSwatches(t *testing.T) {
	dir := t.TempDir()
	writeOnDisk(t, dir, ".tailor/config.yml", []byte("old content"))

	cfg := newConfig(entry(".tailor/config.yml", ".tailor/config.yml", swatch.Always))
	results, err := alter.ProcessSwatches(cfg, dir, alter.Recut, &alter.TokenContext{})
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 0 {
		t.Errorf("expected no results for config.yml swatch, got %d", len(results))
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
			cfg := newConfig(entry(".gitignore", ".gitignore", swatch.Never))
			results, err := alter.ProcessSwatches(cfg, dir, m.mode, &alter.TokenContext{})
			if err != nil {
				t.Fatal(err)
			}
			if len(results) != 1 {
				t.Fatalf("got %d results, want 1", len(results))
			}
			if results[0].Category != alter.SkippedNever {
				t.Errorf("category = %q, want %q", results[0].Category, alter.SkippedNever)
			}
			if _, err := os.Stat(filepath.Join(dir, ".gitignore")); err == nil {
				t.Error("never mode wrote file to disk")
			}
		})

		t.Run(m.name+"/exists", func(t *testing.T) {
			dir := t.TempDir()
			writeOnDisk(t, dir, ".gitignore", []byte("existing"))
			cfg := newConfig(entry(".gitignore", ".gitignore", swatch.Never))
			results, err := alter.ProcessSwatches(cfg, dir, m.mode, &alter.TokenContext{})
			if err != nil {
				t.Fatal(err)
			}
			if len(results) != 1 {
				t.Fatalf("got %d results, want 1", len(results))
			}
			if results[0].Category != alter.SkippedNever {
				t.Errorf("category = %q, want %q", results[0].Category, alter.SkippedNever)
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

// triggeredSource is the swatch source that has a trigger condition.
const triggeredSource = ".github/workflows/tailor-automerge.yml"

func TestTriggeredMetFileAbsentWouldCopy(t *testing.T) {
	dir := t.TempDir()

	cfg := newConfig(entry(triggeredSource, triggeredSource, swatch.Triggered))
	cfg.Repository = &config.RepositorySettings{AllowAutoMerge: ptr.Bool(true)}

	results, err := alter.ProcessSwatches(cfg, dir, alter.DryRun, &alter.TokenContext{})
	if err != nil {
		t.Fatal(err)
	}
	if results[0].Category != alter.WouldCopy {
		t.Errorf("category = %q, want %q", results[0].Category, alter.WouldCopy)
	}
}

func TestTriggeredMetFileExistsDifferentContent(t *testing.T) {
	dir := t.TempDir()
	writeOnDisk(t, dir, triggeredSource, []byte("old content"))

	cfg := newConfig(entry(triggeredSource, triggeredSource, swatch.Triggered))
	cfg.Repository = &config.RepositorySettings{AllowAutoMerge: ptr.Bool(true)}

	results, err := alter.ProcessSwatches(cfg, dir, alter.DryRun, &alter.TokenContext{})
	if err != nil {
		t.Fatal(err)
	}
	if results[0].Category != alter.WouldOverwrite {
		t.Errorf("category = %q, want %q", results[0].Category, alter.WouldOverwrite)
	}
}

func TestTriggeredMetFileExistsSameContent(t *testing.T) {
	dir := t.TempDir()
	// Write resolved content (substitutions applied) so the hash matches.
	raw := mustContent(t, triggeredSource)
	resolved := bytes.ReplaceAll(raw, []byte("{{MERGE_STRATEGY}}"), []byte("--squash"))
	writeOnDisk(t, dir, triggeredSource, resolved)

	cfg := newConfig(entry(triggeredSource, triggeredSource, swatch.Triggered))
	cfg.Repository = &config.RepositorySettings{AllowAutoMerge: ptr.Bool(true)}

	results, err := alter.ProcessSwatches(cfg, dir, alter.DryRun, &alter.TokenContext{})
	if err != nil {
		t.Fatal(err)
	}
	// Resolved content hashes equal; no overwrite needed.
	if results[0].Category != alter.NoChange {
		t.Errorf("category = %q, want %q", results[0].Category, alter.NoChange)
	}
}

func TestTriggeredNotMetFileExistsDryRun(t *testing.T) {
	dir := t.TempDir()
	writeOnDisk(t, dir, triggeredSource, []byte("existing"))

	cfg := newConfig(entry(triggeredSource, triggeredSource, swatch.Triggered))
	cfg.Repository = &config.RepositorySettings{AllowAutoMerge: ptr.Bool(false)}

	results, err := alter.ProcessSwatches(cfg, dir, alter.DryRun, &alter.TokenContext{})
	if err != nil {
		t.Fatal(err)
	}
	if results[0].Category != alter.WouldRemove {
		t.Errorf("category = %q, want %q", results[0].Category, alter.WouldRemove)
	}
	// Dry run: file should still exist.
	if _, err := os.Stat(filepath.Join(dir, triggeredSource)); err != nil {
		t.Error("dry run removed file from disk")
	}
}

func TestTriggeredNotMetFileExistsApply(t *testing.T) {
	dir := t.TempDir()
	writeOnDisk(t, dir, triggeredSource, []byte("existing"))

	cfg := newConfig(entry(triggeredSource, triggeredSource, swatch.Triggered))
	cfg.Repository = &config.RepositorySettings{AllowAutoMerge: ptr.Bool(false)}

	results, err := alter.ProcessSwatches(cfg, dir, alter.Apply, &alter.TokenContext{})
	if err != nil {
		t.Fatal(err)
	}
	if results[0].Category != alter.Removed {
		t.Errorf("category = %q, want %q", results[0].Category, alter.Removed)
	}
	// Apply: file should be removed.
	if _, err := os.Stat(filepath.Join(dir, triggeredSource)); err == nil {
		t.Error("apply mode did not remove file from disk")
	}
}

func TestTriggeredNotMetFileAbsent(t *testing.T) {
	dir := t.TempDir()

	cfg := newConfig(entry(triggeredSource, triggeredSource, swatch.Triggered))
	cfg.Repository = &config.RepositorySettings{AllowAutoMerge: ptr.Bool(false)}

	results, err := alter.ProcessSwatches(cfg, dir, alter.DryRun, &alter.TokenContext{})
	if err != nil {
		t.Fatal(err)
	}
	if results[0].Category != alter.SkippedNever {
		t.Errorf("category = %q, want %q", results[0].Category, alter.SkippedNever)
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
