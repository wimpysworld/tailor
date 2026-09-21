//go:build darwin

package alter

import (
	"os"

	"golang.org/x/sys/unix"
)

func publishDirectoryNoReplace(directory *os.File, temporary, destination string) error {
	return unix.RenameatxNp(int(directory.Fd()), temporary, int(directory.Fd()), destination, unix.RENAME_EXCL)
}
