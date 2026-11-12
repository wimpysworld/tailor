//go:build windows

package alter

import (
	"errors"
	"os"
	"syscall"
	"unsafe"
)

const (
	lockfileFailImmediately = 0x00000001
	lockfileExclusiveLock   = 0x00000002
	errorLockViolation      = syscall.Errno(33)
)

var (
	errWikiImportBusy = errors.New("wiki import lock is held")
	kernel32          = syscall.NewLazyDLL("kernel32.dll")
	procLockFileEx    = kernel32.NewProc("LockFileEx")
	procUnlockFileEx  = kernel32.NewProc("UnlockFileEx")
)

func init() {
	unlockWikiImport = unlockFile
}

func lockWikiImport(file *os.File) error {
	return lockFile(file)
}

func lockFile(file *os.File) error {
	const allBytes = uintptr(^uint32(0))
	var overlapped syscall.Overlapped
	r1, _, err := procLockFileEx.Call(
		file.Fd(),
		lockfileExclusiveLock|lockfileFailImmediately,
		0,
		allBytes,
		allBytes,
		uintptr(unsafe.Pointer(&overlapped)), // #nosec G103 -- LockFileEx requires an OVERLAPPED structure pointer.
	)
	if r1 != 0 {
		return nil
	}
	if errors.Is(err, errorLockViolation) {
		return errWikiImportBusy
	}
	return err
}

func unlockFile(file *os.File) error {
	const allBytes = uintptr(^uint32(0))
	var overlapped syscall.Overlapped
	r1, _, err := procUnlockFileEx.Call(
		file.Fd(),
		0,
		allBytes,
		allBytes,
		uintptr(unsafe.Pointer(&overlapped)), // #nosec G103 -- UnlockFileEx requires an OVERLAPPED structure pointer.
	)
	if r1 != 0 {
		return nil
	}
	return err
}
