package alter

import (
	"errors"
	"fmt"
	"os"

	"github.com/wimpysworld/tailor/internal/config"
)

// ProcessRetiredWorkflows checks and optionally removes retired workflow files.
func ProcessRetiredWorkflows(dir string, mode ApplyMode) ([]SwatchResult, error) {
	root, err := os.OpenRoot(dir)
	if err != nil {
		return nil, fmt.Errorf("opening project root %q: %w", dir, err)
	}
	defer root.Close()

	paths := make([]string, 0, len(config.RetiredWorkflowPaths()))
	for _, path := range config.RetiredWorkflowPaths() {
		info, exists, err := lstatRetiredWorkflow(root, path)
		if err != nil {
			return nil, err
		}
		if !exists {
			continue
		}
		if !info.Mode().IsRegular() && info.Mode()&os.ModeSymlink == 0 {
			if info.IsDir() {
				return nil, fmt.Errorf("retired workflow path %q is a directory", path)
			}
			return nil, fmt.Errorf("retired workflow path %q is not a regular file or symlink", path)
		}
		paths = append(paths, path)
	}

	results := make([]SwatchResult, 0, len(paths))
	for _, path := range paths {
		if mode.ShouldWrite() {
			if err := root.Remove(path); err != nil {
				return nil, fmt.Errorf("removing retired workflow %q: %w", path, err)
			}
		}
		results = append(results, SwatchResult{Path: path, Category: WouldRemove})
	}
	return results, nil
}

func lstatRetiredWorkflow(root *os.Root, path string) (os.FileInfo, bool, error) {
	if err := checkParents(root, path, "retired workflow parent"); err != nil {
		return nil, false, err
	}

	info, err := root.Lstat(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, false, nil
		}
		return nil, false, fmt.Errorf("checking retired workflow %q: %w", path, err)
	}
	return info, true, nil
}
