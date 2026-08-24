package swatch_test

import (
	"io/fs"
	"strings"
	"testing"

	"github.com/wimpysworld/tailor"
	"github.com/wimpysworld/tailor/internal/swatch"
)

// TestContentAvailableForAllRegisteredSwatches verifies that the embedded
// filesystem contains a file for every swatch in the registry.
func TestContentAvailableForAllRegisteredSwatches(t *testing.T) {
	all := swatch.All()
	if len(all) == 0 {
		t.Fatal("All() returned no swatches")
	}

	for _, s := range all {
		t.Run(s.Path, func(t *testing.T) {
			data, err := swatch.Content(s.Path)
			if err != nil {
				t.Fatalf("Content(%q) returned error: %v", s.Path, err)
			}
			if len(data) == 0 {
				t.Fatalf("Content(%q) returned empty bytes", s.Path)
			}
		})
	}
}

// TestAllEmbeddedFilesAreRegistered verifies the reverse: every file in the
// embedded filesystem has a registry entry, so a stray file added under
// swatches/ without registration fails the suite.
func TestAllEmbeddedFilesAreRegistered(t *testing.T) {
	registered := make(map[string]bool)
	for _, p := range swatch.Paths() {
		registered[p] = true
	}

	err := fs.WalkDir(tailor.SwatchFS, "swatches", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		rel := strings.TrimPrefix(path, "swatches/")
		if !registered[rel] {
			t.Errorf("embedded file %q has no registry entry", rel)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("WalkDir returned error: %v", err)
	}
}

func TestContentReturnsErrorForUnknownSource(t *testing.T) {
	_, err := swatch.Content("nonexistent.txt")
	if err == nil {
		t.Fatal("Content(\"nonexistent.txt\") expected error, got nil")
	}
}
