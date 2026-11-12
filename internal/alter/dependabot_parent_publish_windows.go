//go:build windows

package alter

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"unsafe"

	"golang.org/x/sys/windows"
)

const fileRenameInformationEx = 65

type fileRenameInformation struct {
	Flags          uint32
	RootDirectory  windows.Handle
	FileNameLength uint32
	FileName       [windows.MAX_PATH]uint16
}

func publishDirectoryNoReplace(directory *os.File, temporary, destination string) error {
	if directory == nil || !isSingleEntryName(temporary) || !isSingleEntryName(destination) {
		return os.ErrInvalid
	}
	dirHandle := windows.Handle(directory.Fd())
	if dirHandle == 0 || dirHandle == windows.InvalidHandle {
		return os.ErrInvalid
	}
	defer runtime.KeepAlive(directory)

	sourceName, err := windows.NewNTUnicodeString(temporary)
	if err != nil {
		return err
	}
	oa := windows.OBJECT_ATTRIBUTES{
		Length:        uint32(unsafe.Sizeof(windows.OBJECT_ATTRIBUTES{})),
		RootDirectory: dirHandle,
		ObjectName:    sourceName,
	}
	var (
		sourceHandle windows.Handle
		iosb         windows.IO_STATUS_BLOCK
	)
	err = windows.NtCreateFile(
		&sourceHandle,
		windows.SYNCHRONIZE|windows.DELETE,
		&oa,
		&iosb,
		nil,
		0,
		windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE|windows.FILE_SHARE_DELETE,
		windows.FILE_OPEN,
		windows.FILE_DIRECTORY_FILE|windows.FILE_OPEN_REPARSE_POINT|windows.FILE_OPEN_FOR_BACKUP_INTENT|windows.FILE_SYNCHRONOUS_IO_NONALERT,
		0,
		0,
	)
	runtime.KeepAlive(sourceName)
	if err != nil {
		return ntStatusError(err)
	}
	defer func() { _ = windows.CloseHandle(sourceHandle) }()

	destUTF16, err := windows.UTF16FromString(destination)
	if err != nil {
		return err
	}
	var renameInfo fileRenameInformation
	if len(destUTF16) < 2 || len(destUTF16) > len(renameInfo.FileName) {
		return os.ErrInvalid
	}
	renameInfo.RootDirectory = dirHandle
	copy(renameInfo.FileName[:], destUTF16)
	// #nosec G115 -- len(destUTF16) is bounded between 2 and windows.MAX_PATH (260).
	renameInfo.FileNameLength = uint32((len(destUTF16) - 1) * 2)

	// #nosec G103 -- NtSetInformationFile requires a byte pointer to the FILE_RENAME_INFORMATION buffer.
	renameBuf := (*byte)(unsafe.Pointer(&renameInfo))
	renameLen := uint32(unsafe.Sizeof(renameInfo))
	err = windows.NtSetInformationFile(
		sourceHandle,
		&iosb,
		renameBuf,
		renameLen,
		fileRenameInformationEx,
	)
	if err == nil {
		return nil
	}
	if errors.Is(err, windows.STATUS_INVALID_INFO_CLASS) ||
		errors.Is(err, windows.STATUS_INVALID_PARAMETER) ||
		errors.Is(err, windows.STATUS_NOT_SUPPORTED) {
		err = windows.NtSetInformationFile(
			sourceHandle,
			&iosb,
			renameBuf,
			renameLen,
			windows.FileRenameInformation,
		)
	}
	return ntStatusError(err)
}

func isSingleEntryName(name string) bool {
	return name != "" &&
		name != "." &&
		name != ".." &&
		filepath.IsLocal(name) &&
		!strings.ContainsAny(name, `/\:`)
}

func ntStatusError(err error) error {
	if err == nil {
		return nil
	}
	if status, ok := errors.AsType[windows.NTStatus](err); ok {
		if errno := status.Errno(); errno != 0 {
			return errno
		}
	}
	return err
}
