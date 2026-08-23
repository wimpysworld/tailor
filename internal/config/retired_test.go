package config

import (
	"slices"
	"testing"

	"github.com/wimpysworld/tailor/internal/swatch"
)

func TestRetiredWorkflowPaths(t *testing.T) {
	want := []string{
		".github/workflows/tailor-automerge.yml",
		".github/workflows/tailor.yml",
	}

	got := RetiredWorkflowPaths()
	if !slices.Equal(got, want) {
		t.Fatalf("RetiredWorkflowPaths() = %v, want %v", got, want)
	}

	got[0] = "changed"
	if next := RetiredWorkflowPaths(); !slices.Equal(next, want) {
		t.Errorf("RetiredWorkflowPaths() returned mutable state: %v", next)
	}
}

func TestIsRetiredWorkflowPath(t *testing.T) {
	tests := []struct {
		path string
		want bool
	}{
		{path: ".github/workflows/tailor-automerge.yml", want: true},
		{path: ".github/workflows/tailor.yml", want: true},
		{path: ".github/workflows/tailor.yaml", want: false},
		{path: "tailor.yml", want: false},
		{path: "", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			if got := IsRetiredWorkflowPath(tt.path); got != tt.want {
				t.Errorf("IsRetiredWorkflowPath(%q) = %t, want %t", tt.path, got, tt.want)
			}
		})
	}
}

func TestRemoveRetiredWorkflowEntriesRemovesEveryDuplicateAndPreservesOrder(t *testing.T) {
	cfg := &Config{Swatches: []SwatchEntry{
		{Path: "justfile", Alteration: swatch.FirstFit},
		{Path: ".github/workflows/tailor.yml", Alteration: swatch.Always},
		{Path: "SECURITY.md", Alteration: swatch.Never},
		{Path: ".github/workflows/tailor-automerge.yml", Alteration: legacyTriggered},
		{Path: ".github/workflows/tailor.yml", Alteration: swatch.FirstFit},
		{Path: ".tailor.yml", Alteration: swatch.Always},
		{Path: ".github/workflows/tailor-automerge.yml", Alteration: swatch.Never},
	}}
	want := []SwatchEntry{
		{Path: "justfile", Alteration: swatch.FirstFit},
		{Path: "SECURITY.md", Alteration: swatch.Never},
		{Path: ".tailor.yml", Alteration: swatch.Always},
	}

	if changed := RemoveRetiredWorkflowEntries(cfg); !changed {
		t.Fatal("RemoveRetiredWorkflowEntries() = false, want true")
	}
	if !slices.Equal(cfg.Swatches, want) {
		t.Errorf("Swatches = %v, want %v", cfg.Swatches, want)
	}
}

func TestRemoveRetiredWorkflowEntriesNoOp(t *testing.T) {
	want := []SwatchEntry{
		{Path: "justfile", Alteration: swatch.FirstFit},
		{Path: "SECURITY.md", Alteration: swatch.Always},
	}
	cfg := &Config{Swatches: slices.Clone(want)}

	if changed := RemoveRetiredWorkflowEntries(cfg); changed {
		t.Fatal("RemoveRetiredWorkflowEntries() = true, want false")
	}
	if !slices.Equal(cfg.Swatches, want) {
		t.Errorf("Swatches = %v, want %v", cfg.Swatches, want)
	}
}
