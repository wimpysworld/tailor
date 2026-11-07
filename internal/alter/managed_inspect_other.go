//go:build !unix

package alter

import (
	"errors"
	"os"
)

func openManagedInspectionFile(_ *os.Root, _ string) (*os.File, error) {
	return nil, errors.New("no-follow inspection is unavailable on this platform")
}
