//go:build unix

package alter

import (
	"os"
	"syscall"
)

func openDependabotFile(root *os.Root, name string) (*os.File, error) {
	return root.OpenFile(name, os.O_RDONLY|syscall.O_NONBLOCK, 0)
}
