package swatch_test

import (
	"testing"

	"github.com/wimpysworld/tailor/internal/swatch"
)

// TestContentAvailableForAllRegisteredSwatches verifies that the embed FS
// contains a file for every swatch in the registry. This is the wiring test
// connecting Task 1.1 (embed) to Task 1.2 (registry).
func TestContentAvailableForAllRegisteredSwatches(t *testing.T) {
	all := swatch.All()
	if len(all) == 0 {
		t.Fatal("All() returned no swatches")
	}

	for _, s := range all {
		t.Run(s.Source, func(t *testing.T) {
			data, err := swatch.Content(s.Source)
			if err != nil {
				t.Fatalf("Content(%q) returned error: %v", s.Source, err)
			}
			if len(data) == 0 {
				t.Fatalf("Content(%q) returned empty bytes", s.Source)
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
