//go:build !linux && !darwin

package alter

import (
	"errors"
	"os"
)

func publishDirectoryNoReplace(_ *os.File, _, _ string) error {
	return errors.ErrUnsupported
}
