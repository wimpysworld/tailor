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

func TestIsRegularFile(t *testing.T) {
	tests := []struct {
		name  string
		setup func(t *testing.T, dir string) string
		want  bool
	}{
		{
			name: "regular file",
			setup: func(t *testing.T, dir string) string {
				path := filepath.Join(dir, "file.txt")
				if err := os.WriteFile(path, []byte("hello"), 0o644); err != nil {
					t.Fatalf("WriteFile: %v", err)
				}
				return path
			},
			want: true,
		},
		{
			name: "missing file",
			setup: func(t *testing.T, dir string) string {
				return filepath.Join(dir, "no-such-file.txt")
			},
			want: false,
		},
		{
			name: "directory",
			setup: func(t *testing.T, dir string) string {
				path := filepath.Join(dir, "subdir")
				if err := os.Mkdir(path, 0o755); err != nil {
					t.Fatalf("Mkdir: %v", err)
				}
				return path
			},
			want: false,
		},
		{
			name: "symlink to regular file",
			setup: func(t *testing.T, dir string) string {
				target := filepath.Join(dir, "target.txt")
				if err := os.WriteFile(target, []byte("hello"), 0o644); err != nil {
					t.Fatalf("WriteFile: %v", err)
				}
				path := filepath.Join(dir, "link.txt")
				if err := os.Symlink(target, path); err != nil {
					t.Fatalf("Symlink: %v", err)
				}
				return path
			},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := tt.setup(t, t.TempDir())
			if got := IsRegularFile(path); got != tt.want {
				t.Errorf("IsRegularFile(%q) = %v, want %v", path, got, tt.want)
			}
		})
	}
}
