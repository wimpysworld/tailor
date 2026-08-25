package fsutil

import "os"

// FileExists reports whether the given path exists as a file (not a directory).
func FileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}
