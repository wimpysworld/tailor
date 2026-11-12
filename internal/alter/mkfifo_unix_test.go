//go:build unix

package alter_test

import (
	"syscall"
	"testing"
)

func testMkfifo(t *testing.T, path string) {
	t.Helper()
	if err := syscall.Mkfifo(path, 0o644); err != nil {
		t.Skipf("mkfifo unavailable: %v", err)
	}
}
