//go:build !unix

package alter

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
)

var errManagedUnsafeTarget = errors.New("unsafe managed inspection target")

func openManagedInspectionFile(root *os.Root, name string) (*os.File, error) {
	if root == nil || name == "" {
		return nil, errManagedUnsafeTarget
	}
	parts := strings.Split(filepath.ToSlash(name), "/")
	currentRoot := root
	defer func() {
		if currentRoot != root {
			_ = currentRoot.Close()
		}
	}()
	for _, part := range parts[:len(parts)-1] {
		if part == "" || part == "." || part == ".." {
			return nil, errManagedUnsafeTarget
		}
		parentInfo, err := currentRoot.Lstat(part)
		if err != nil {
			return nil, err
		}
		if parentInfo.Mode()&os.ModeSymlink != 0 || !parentInfo.IsDir() {
			return nil, errManagedUnsafeTarget
		}
		nextRoot, err := currentRoot.OpenRoot(part)
		if err != nil {
			return nil, err
		}
		openedDirInfo, err := nextRoot.Stat(".")
		if err != nil || !openedDirInfo.IsDir() || !os.SameFile(parentInfo, openedDirInfo) {
			_ = nextRoot.Close()
			return nil, errManagedUnsafeTarget
		}
		if currentRoot != root {
			_ = currentRoot.Close()
		}
		currentRoot = nextRoot
	}

	base := parts[len(parts)-1]
	if base == "" || base == "." || base == ".." {
		return nil, errManagedUnsafeTarget
	}
	lstatInfo, err := currentRoot.Lstat(base)
	if err != nil {
		return nil, err
	}
	if lstatInfo.Mode()&os.ModeSymlink != 0 || !lstatInfo.Mode().IsRegular() {
		return nil, errManagedUnsafeTarget
	}
	file, err := currentRoot.Open(base)
	if err != nil {
		return nil, err
	}
	statInfo, err := file.Stat()
	if err != nil {
		_ = file.Close()
		return nil, err
	}
	if !statInfo.Mode().IsRegular() || !os.SameFile(lstatInfo, statInfo) {
		_ = file.Close()
		return nil, errManagedUnsafeTarget
	}
	return file, nil
}
