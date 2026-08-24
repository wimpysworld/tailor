package swatch_test

import (
	"testing"

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

func TestContentReturnsErrorForUnknownSource(t *testing.T) {
	_, err := swatch.Content("nonexistent.txt")
	if err == nil {
		t.Fatal("Content(\"nonexistent.txt\") expected error, got nil")
	}
}
