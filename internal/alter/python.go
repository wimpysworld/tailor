package alter

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/pelletier/go-toml/v2"
)

// PythonPolicyReason preserves alternate policies instead of shadowing them with a starter.
func PythonPolicyReason(dir, destination string) (string, error) {
	if destination != "ruff.toml" && destination != "pyrightconfig.json" {
		return "", nil
	}
	root, err := filepath.Abs(dir)
	if err != nil {
		return "", err
	}
	info, err := os.Lstat(root)
	if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return "", fmt.Errorf("unsafe Python policy parent %q", root)
	}
	parent, err := filepath.EvalSymlinks(filepath.Dir(root))
	if err != nil {
		return "", fmt.Errorf("resolving Python policy parent: %w", err)
	}
	root = filepath.Join(parent, filepath.Base(root))
	for current := root; ; current = filepath.Dir(current) {
		info, err := os.Lstat(current)
		if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
			return "", fmt.Errorf("unsafe Python policy parent %q", current)
		}
		if current == filepath.Dir(current) {
			break
		}
	}
	var reason string
	inspect := func(directory string) error {
		names := []string{"pyproject.toml", ".ruff.toml", "ruff.toml", "pyrightconfig.json", "mypy.ini", ".mypy.ini", "setup.cfg"}
		for _, name := range names {
			if directory == root && name == destination {
				continue
			}
			path := filepath.Join(directory, name)
			info, err := os.Lstat(path)
			if os.IsNotExist(err) {
				continue
			}
			if err != nil {
				return err
			}
			if !info.Mode().IsRegular() {
				return fmt.Errorf("python policy %q must be a regular file, not a symlink", path)
			}
			equivalent := destination == "ruff.toml" && (name == ".ruff.toml" || name == "ruff.toml") || destination == "pyrightconfig.json" && name == "pyrightconfig.json"
			conflict := destination == "pyrightconfig.json" && (name == "mypy.ini" || name == ".mypy.ini")
			if name == "setup.cfg" && destination == "pyrightconfig.json" {
				if info.Size() > 1<<20 {
					return fmt.Errorf("python policy %q exceeds 1 MiB", path)
				}
				content, err := os.ReadFile(path)
				if err != nil {
					return err
				}
				for line := range strings.SplitSeq(string(content), "\n") {
					section := strings.TrimSpace(line)
					conflict = conflict || section == "[mypy]" || strings.HasPrefix(section, "[mypy-")
				}
			}
			if name == "pyproject.toml" {
				if info.Size() > 1<<20 {
					return fmt.Errorf("python policy %q exceeds 1 MiB", path)
				}
				content, err := os.ReadFile(path)
				if err != nil {
					return err
				}
				var document struct {
					Tool map[string]any `toml:"tool"`
				}
				if err := toml.Unmarshal(content, &document); err != nil {
					return fmt.Errorf("reading Python policy %q: %w", path, err)
				}
				_, ruff := document.Tool["ruff"]
				_, black := document.Tool["black"]
				_, based := document.Tool["basedpyright"]
				_, pyright := document.Tool["pyright"]
				_, mypy := document.Tool["mypy"]
				equivalent = destination == "ruff.toml" && ruff || destination == "pyrightconfig.json" && (based || pyright)
				conflict = destination == "ruff.toml" && black || destination == "pyrightconfig.json" && mypy
			}
			if equivalent && reason == "" {
				reason = "existing Python policy preserved: " + path
			}
			if conflict {
				reason = "Python adoption required; conflicting policy preserved: " + path
			}
		}
		return nil
	}
	for current := root; ; current = filepath.Dir(current) {
		if err := inspect(current); err != nil {
			return "", err
		}
		if current == filepath.Dir(current) {
			break
		}
	}
	err = filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !entry.IsDir() || path == root {
			return nil
		}
		if strings.HasPrefix(entry.Name(), ".") || entry.Name() == "node_modules" || entry.Name() == "vendor" {
			return filepath.SkipDir
		}
		return inspect(path)
	})
	return reason, err
}
