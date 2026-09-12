package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestMeasureGoSelectionWithoutTools(t *testing.T) {
	for _, enabled := range []string{"false", "true"} {
		t.Run(enabled, func(t *testing.T) {
			dir := t.TempDir()
			t.Chdir(dir)
			t.Setenv("PATH", t.TempDir())
			data := "languages:\n  go: " + enabled + "\nswatches: []\n"
			if err := os.WriteFile(filepath.Join(dir, ".tailor.yml"), []byte(data), 0o644); err != nil {
				t.Fatal(err)
			}
			var stdout bytes.Buffer
			if err := (&MeasureCmd{stdout: &stdout}).Run(); err != nil {
				t.Fatal(err)
			}
			for _, path := range []string{".golangci.yml", ".goreleaser.yaml", ".github/workflows/build-go.yml"} {
				if strings.Contains(stdout.String(), path) != (enabled == "true") {
					t.Fatalf("unexpected Go path %s in measure output: %s", path, stdout.String())
				}
			}
		})
	}
}
