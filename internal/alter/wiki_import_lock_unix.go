//go:build unix

package alter

import (
	"os"

	"golang.org/x/sys/unix"
)

func lockWikiImport(file *os.File) error {
	return unix.Flock(int(file.Fd()), unix.LOCK_EX|unix.LOCK_NB)
}
