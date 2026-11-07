//go:build unix

package alter

import (
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/sys/unix"
)

func openManagedInspectionFile(root *os.Root, name string) (*os.File, error) {
	directory, err := root.Open(".")
	if err != nil {
		return nil, err
	}
	defer func() { _ = directory.Close() }()
	parts := strings.Split(filepath.ToSlash(name), "/")
	for _, part := range parts[:len(parts)-1] {
		fd, err := unix.Openat(int(directory.Fd()), part, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
		if err != nil {
			return nil, err
		}
		_ = directory.Close()
		directory = os.NewFile(uintptr(fd), part)
	}
	fd, err := unix.Openat(int(directory.Fd()), parts[len(parts)-1], unix.O_RDONLY|unix.O_NOFOLLOW|unix.O_NONBLOCK|unix.O_CLOEXEC, 0)
	if err != nil {
		return nil, err
	}
	return os.NewFile(uintptr(fd), name), nil
}
