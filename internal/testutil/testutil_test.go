package testutil

import (
	"os"
	"path/filepath"
	"testing"
)

func TestWriteConfigCreatesFile(t *testing.T) {
	dir := t.TempDir()
	content := "license: BlueOak-1.0.0\nswatches: []\n"

	WriteConfig(t, dir, content)

	path := filepath.Join(dir, ".tailor.yml")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if string(data) != content {
		t.Errorf("file content = %q, want %q", string(data), content)
	}
}
