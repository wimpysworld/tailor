package config

import (
	"testing"

	"github.com/wimpysworld/tailor/internal/swatch"
)

// allNonConfigSwatches returns every registered swatch except .tailor/config.yml.
func allNonConfigSwatches() []swatch.Swatch {
	var out []swatch.Swatch
	for _, s := range swatch.All() {
		if s.Source != ConfigSwatchSource {
			out = append(out, s)
		}
	}
	return out
}

func TestMergeAllPresent(t *testing.T) {
	var entries []SwatchEntry
	for _, s := range allNonConfigSwatches() {
		entries = append(entries, SwatchEntry{
			Source:      s.Source,
			Destination: s.Destination,
			Alteration:  s.DefaultAlteration,
		})
	}
	cfg := &Config{Swatches: entries}
	origLen := len(cfg.Swatches)

	added := MergeDefaultSwatches(cfg)

	if len(added) != 0 {
		t.Fatalf("expected no additions, got %d", len(added))
	}
	if len(cfg.Swatches) != origLen {
		t.Fatalf("swatches length changed from %d to %d", origLen, len(cfg.Swatches))
	}
}

func TestMergeSubset(t *testing.T) {
	cfg := &Config{
		Swatches: []SwatchEntry{
			{Source: ".gitignore", Destination: ".gitignore", Alteration: swatch.FirstFit},
			{Source: "SECURITY.md", Destination: "SECURITY.md", Alteration: swatch.Always},
		},
	}

	added := MergeDefaultSwatches(cfg)

	expected := allNonConfigSwatches()
	wantAdded := len(expected) - 2 // two already present
	if len(added) != wantAdded {
		t.Fatalf("expected %d additions, got %d", wantAdded, len(added))
	}
	if len(cfg.Swatches) != len(expected) {
		t.Fatalf("expected %d total swatches, got %d", len(expected), len(cfg.Swatches))
	}

	// Verify each added entry has the correct alteration mode from the registry.
	addedBySource := make(map[string]SwatchEntry, len(added))
	for _, e := range added {
		addedBySource[e.Source] = e
	}
	for _, s := range expected {
		if s.Source == ".gitignore" || s.Source == "SECURITY.md" {
			continue
		}
		e, ok := addedBySource[s.Source]
		if !ok {
			t.Errorf("missing added entry for source %q", s.Source)
			continue
		}
		if e.Alteration != s.DefaultAlteration {
			t.Errorf("source %q: alteration = %q, want %q", s.Source, e.Alteration, s.DefaultAlteration)
		}
		if e.Destination != s.Destination {
			t.Errorf("source %q: destination = %q, want %q", s.Source, e.Destination, s.Destination)
		}
	}
}

func TestMergeNeverNotDuplicated(t *testing.T) {
	cfg := &Config{
		Swatches: []SwatchEntry{
			{Source: ".gitignore", Destination: ".gitignore", Alteration: swatch.Never},
		},
	}

	added := MergeDefaultSwatches(cfg)

	// .gitignore already present (with never), should not be duplicated.
	count := 0
	for _, e := range cfg.Swatches {
		if e.Source == ".gitignore" {
			count++
		}
	}
	if count != 1 {
		t.Fatalf(".gitignore appears %d times, want 1", count)
	}

	// Should not be in the added slice either.
	for _, e := range added {
		if e.Source == ".gitignore" {
			t.Fatal(".gitignore should not appear in added slice")
		}
	}
}

func TestMergeRemappedDestination(t *testing.T) {
	cfg := &Config{
		Swatches: []SwatchEntry{
			{Source: ".gitignore", Destination: "custom/.gitignore", Alteration: swatch.FirstFit},
		},
	}

	added := MergeDefaultSwatches(cfg)

	// Source matches, so it should not be treated as missing.
	for _, e := range added {
		if e.Source == ".gitignore" {
			t.Fatal(".gitignore with remapped destination should not be added again")
		}
	}
}

func TestMergeEmptyConfig(t *testing.T) {
	cfg := &Config{}

	added := MergeDefaultSwatches(cfg)

	expected := allNonConfigSwatches()
	if len(added) != len(expected) {
		t.Fatalf("expected %d additions, got %d", len(expected), len(added))
	}
	if len(cfg.Swatches) != len(expected) {
		t.Fatalf("expected %d total swatches, got %d", len(expected), len(cfg.Swatches))
	}

	// Verify no config.yml entry was added.
	for _, e := range cfg.Swatches {
		if e.Source == ConfigSwatchSource {
			t.Fatal("config.yml swatch should not be added by merge")
		}
	}
}
