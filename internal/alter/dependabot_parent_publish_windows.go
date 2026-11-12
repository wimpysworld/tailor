//go:build windows

package alter

import (
	"errors"
	"os"
	"path/filepath"
	"syscall"
	"unsafe"
)

var procMoveFileExW = syscall.NewLazyDLL("kernel32.dll").NewProc("MoveFileExW")

func publishDirectoryNoReplace(directory *os.File, temporary, destination string) error {
	if directory == nil {
		return os.ErrInvalid
	}
	return renameDirectoryNoReplace(
		filepath.Join(directory.Name(), temporary),
		filepath.Join(directory.Name(), destination),
	)
}

func renameDirectoryNoReplace(from, to string) error {
	fromPtr, err := syscall.UTF16PtrFromString(from)
	if err != nil {
		return err
	}
	toPtr, err := syscall.UTF16PtrFromString(to)
	if err != nil {
		return err
	}
	// #nosec G103 -- MoveFileExW requires UTF-16 string pointers passed via LazyProc.Call.
	r1, _, callErr := procMoveFileExW.Call(
		uintptr(unsafe.Pointer(fromPtr)),
		uintptr(unsafe.Pointer(toPtr)),
		0,
	)
	if r1 != 0 {
		return nil
	}
	var errno syscall.Errno
	if errors.As(callErr, &errno) && errno != 0 {
		return errno
	}
	return syscall.EINVAL
}
