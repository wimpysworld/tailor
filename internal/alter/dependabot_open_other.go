//go:build !unix

package alter

import "os"

func openDependabotFile(root *os.Root, name string) (*os.File, error) {
	return root.Open(name)
}
