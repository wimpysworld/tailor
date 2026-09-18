package alter

import (
	"crypto/rand"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
)

type managedApplyHooks struct {
	beforeTempCreate    func(string) error
	beforeTempWrite     func(string) error
	beforeTempChmod     func(string) error
	beforeTempSync      func(string) error
	beforeTempClose     func(string) error
	beforeRename        func(string) error
	beforeRemove        func(string) error
	beforeDirectorySync func(string) error
}

func writeManagedFile(root *os.Root, destination string, content []byte, hooks managedApplyHooks) (changed bool, retErr error) {
	if err := checkParents(root, destination, "managed destination parent"); err != nil {
		return false, err
	}
	if err := root.MkdirAll(path.Dir(destination), 0o755); err != nil {
		return false, fmt.Errorf("creating directories for managed destination %q: %w", destination, err)
	}

	mode, err := managedDestinationMode(root, destination)
	if err != nil {
		return false, err
	}
	if err := runManagedHook(hooks.beforeTempCreate, destination); err != nil {
		return false, fmt.Errorf("creating temporary managed file for %q: %w", destination, err)
	}

	tempPath := destination + ".tmp-" + rand.Text()
	temp, err := root.OpenFile(tempPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return false, fmt.Errorf("creating temporary managed file for %q: %w", destination, err)
	}
	tempOpen := true
	defer func() {
		if tempOpen {
			if err := temp.Close(); err != nil {
				retErr = errors.Join(retErr, fmt.Errorf("closing temporary managed file for %q: %w", destination, err))
			}
		}
		if tempPath != "" {
			if err := root.Remove(tempPath); err != nil && !errors.Is(err, fs.ErrNotExist) {
				retErr = errors.Join(retErr, fmt.Errorf("removing temporary managed file for %q: %w", destination, err))
			}
		}
	}()

	if err := runManagedHook(hooks.beforeTempWrite, destination); err != nil {
		return false, fmt.Errorf("writing temporary managed file for %q: %w", destination, err)
	}
	written, err := temp.Write(content)
	if err != nil {
		return false, fmt.Errorf("writing temporary managed file for %q: %w", destination, err)
	}
	if written != len(content) {
		return false, fmt.Errorf("writing temporary managed file for %q: %w", destination, io.ErrShortWrite)
	}
	if err := runManagedHook(hooks.beforeTempChmod, destination); err != nil {
		return false, fmt.Errorf("setting temporary managed file permissions for %q: %w", destination, err)
	}
	if err := temp.Chmod(mode); err != nil {
		return false, fmt.Errorf("setting temporary managed file permissions for %q: %w", destination, err)
	}
	if err := runManagedHook(hooks.beforeTempSync, destination); err != nil {
		return false, fmt.Errorf("syncing temporary managed file for %q: %w", destination, err)
	}
	if err := temp.Sync(); err != nil {
		return false, fmt.Errorf("syncing temporary managed file for %q: %w", destination, err)
	}
	if err := runManagedHook(hooks.beforeTempClose, destination); err != nil {
		return false, fmt.Errorf("closing temporary managed file for %q: %w", destination, err)
	}
	if err := temp.Close(); err != nil {
		tempOpen = false
		return false, fmt.Errorf("closing temporary managed file for %q: %w", destination, err)
	}
	tempOpen = false

	if err := runManagedHook(hooks.beforeRename, destination); err != nil {
		return false, fmt.Errorf("replacing managed destination %q: %w", destination, err)
	}
	if err := root.Rename(tempPath, destination); err != nil {
		return false, fmt.Errorf("replacing managed destination %q: %w", destination, err)
	}
	tempPath = ""

	if err := syncManagedDirectory(root, destination, hooks); err != nil {
		return true, err
	}
	return true, nil
}

func removeManagedFile(root *os.Root, destination string, hooks managedApplyHooks) (bool, error) {
	if err := checkParents(root, destination, "managed destination parent"); err != nil {
		return false, err
	}
	if err := runManagedHook(hooks.beforeRemove, destination); err != nil {
		return false, fmt.Errorf("removing managed destination %q: %w", destination, err)
	}
	if err := root.Remove(destination); err != nil {
		return false, fmt.Errorf("removing managed destination %q: %w", destination, err)
	}
	if err := syncManagedDirectory(root, destination, hooks); err != nil {
		return true, err
	}
	return true, nil
}

func managedDestinationMode(root *os.Root, destination string) (os.FileMode, error) {
	info, err := root.Lstat(destination)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return 0o644, nil
		}
		return 0, fmt.Errorf("checking managed destination %q: %w", destination, err)
	}
	switch {
	case info.Mode()&os.ModeSymlink != 0:
		return 0o644, nil
	case info.IsDir():
		return 0, fmt.Errorf("managed destination %q is a directory", destination)
	case !info.Mode().IsRegular():
		return 0, fmt.Errorf("managed destination %q is not a regular file or symlink", destination)
	default:
		return info.Mode().Perm(), nil
	}
}

func syncManagedDirectory(root *os.Root, destination string, hooks managedApplyHooks) error {
	if err := runManagedHook(hooks.beforeDirectorySync, destination); err != nil {
		return fmt.Errorf("syncing directory for managed destination %q: %w", destination, err)
	}
	directory, err := root.Open(path.Dir(destination))
	if err != nil {
		return fmt.Errorf("opening directory for managed destination %q: %w", destination, err)
	}
	if err := directory.Sync(); err != nil {
		closeErr := directory.Close()
		return errors.Join(
			fmt.Errorf("syncing directory for managed destination %q: %w", destination, err),
			wrapManagedDirectoryCloseError(destination, closeErr),
		)
	}
	if err := directory.Close(); err != nil {
		return fmt.Errorf("closing directory for managed destination %q: %w", destination, err)
	}
	return nil
}

func runManagedHook(hook func(string) error, destination string) error {
	if hook == nil {
		return nil
	}
	return hook(destination)
}

func wrapManagedDirectoryCloseError(destination string, err error) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("closing directory for managed destination %q: %w", destination, err)
}
