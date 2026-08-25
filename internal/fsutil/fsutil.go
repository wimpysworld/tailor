package fsutil

import "os"

// FileExists reports whether the given path exists as a file (not a directory).
func FileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

// IsRegularFile reports whether the given path is a regular file. Symlinks
// are not followed, so a symlink is never a regular file.
func IsRegularFile(path string) bool {
	info, err := os.Lstat(path)
	return err == nil && info.Mode().IsRegular()
}
