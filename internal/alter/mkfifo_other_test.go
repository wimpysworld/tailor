//go:build !unix

package alter_test

import "testing"

func testMkfifo(t *testing.T, _ string) {
	t.Helper()
	t.Skip("mkfifo requires unix")
}
