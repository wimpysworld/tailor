//go:build !unix

package alter

import (
	"errors"
	"os"
)

func lockWikiImport(_ *os.File) error {
	return errors.New("safe wiki import locking is unavailable on this platform")
}
