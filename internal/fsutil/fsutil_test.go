package fsutil

import (
	"os"
	"path/filepath"
	"testing"
)

func TestFileExistsTrue(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "exists.txt")
	if err := os.WriteFile(path, []byte("hello"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	if !FileExists(path) {
		t.Error("FileExists() = false, want true for existing file")
	}
}

func TestFileExistsMissing(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "no-such-file.txt")

	if FileExists(path) {
		t.Error("FileExists() = true, want false for missing file")
	}
}

func TestFileExistsDirectory(t *testing.T) {
	dir := t.TempDir()
	subdir := filepath.Join(dir, "subdir")
	if err := os.Mkdir(subdir, 0o755); err != nil {
		t.Fatalf("Mkdir: %v", err)
	}

	if FileExists(subdir) {
		t.Error("FileExists() = true, want false for directory")
	}
}
