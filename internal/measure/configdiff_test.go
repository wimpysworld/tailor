package measure

import (
	"testing"

	"github.com/wimpysworld/tailor/internal/config"
	"github.com/wimpysworld/tailor/internal/swatch"
)

func TestCheckConfigDiffNotConfigured(t *testing.T) {
	// Config has no swatches, so every default is not-configured.
	cfg := &config.Config{
		Swatches: []config.SwatchEntry{},
	}
	defaults := swatch.All()

	results := CheckConfigDiff(cfg, defaults)

	notConfigured := 0
	for _, r := range results {
		if r.Category == NotConfigured {
			notConfigured++
		}
	}

	if notConfigured != len(defaults) {
		t.Errorf("not-configured count = %d, want %d", notConfigured, len(defaults))
	}
}

func TestCheckConfigDiffConfigOnly(t *testing.T) {
	cfg := &config.Config{
		Swatches: []config.SwatchEntry{
			{Source: "custom.yml", Destination: "custom.yml", Alteration: swatch.Always},
		},
	}
	defaults := []swatch.Swatch{}

	results := CheckConfigDiff(cfg, defaults)

	if len(results) != 1 {
		t.Fatalf("results count = %d, want 1", len(results))
	}
	if results[0].Category != ConfigOnly {
		t.Errorf("category = %q, want %q", results[0].Category, ConfigOnly)
	}
	if results[0].Destination != "custom.yml" {
		t.Errorf("destination = %q, want %q", results[0].Destination, "custom.yml")
	}
}

func TestCheckConfigDiffModeDiffers(t *testing.T) {
	cfg := &config.Config{
		Swatches: []config.SwatchEntry{
			{Source: "SECURITY.md", Destination: "SECURITY.md", Alteration: swatch.FirstFit},
		},
	}
	defaults := []swatch.Swatch{
		{Source: "SECURITY.md", Destination: "SECURITY.md", DefaultAlteration: swatch.Always, Category: swatch.Health},
	}

	results := CheckConfigDiff(cfg, defaults)

	if len(results) != 1 {
		t.Fatalf("results count = %d, want 1", len(results))
	}
	r := results[0]
	if r.Category != ModeDiffers {
		t.Errorf("category = %q, want %q", r.Category, ModeDiffers)
	}
	if r.Annotation != "(config: first-fit, default: always)" {
		t.Errorf("annotation = %q, want %q", r.Annotation, "(config: first-fit, default: always)")
	}
}

func TestCheckConfigDiffExactMatch(t *testing.T) {
	// Config matches defaults exactly, so no diff results.
	defaults := swatch.All()
	entries := make([]config.SwatchEntry, len(defaults))
	for i, s := range defaults {
		entries[i] = config.SwatchEntry{
			Source:      s.Source,
			Destination: s.Destination,
			Alteration:  s.DefaultAlteration,
		}
	}
	cfg := &config.Config{Swatches: entries}

	results := CheckConfigDiff(cfg, defaults)

	if len(results) != 0 {
		t.Errorf("exact match produced %d diff results, want 0", len(results))
		for _, r := range results {
			t.Logf("  %s: %s %s", r.Category, r.Destination, r.Annotation)
		}
	}
}

func TestCheckConfigDiffAllCategories(t *testing.T) {
	defaults := []swatch.Swatch{
		{Source: "a.yml", Destination: "a.yml", DefaultAlteration: swatch.Always, Category: swatch.Health},
		{Source: "b.yml", Destination: "b.yml", DefaultAlteration: swatch.Always, Category: swatch.Development},
	}
	cfg := &config.Config{
		Swatches: []config.SwatchEntry{
			// b.yml present with different mode -> mode-differs
			{Source: "b.yml", Destination: "b.yml", Alteration: swatch.FirstFit},
			// c.yml not in defaults -> config-only
			{Source: "c.yml", Destination: "c.yml", Alteration: swatch.Always},
		},
	}
	// a.yml missing from config -> not-configured

	results := CheckConfigDiff(cfg, defaults)

	if len(results) != 3 {
		t.Fatalf("results count = %d, want 3", len(results))
	}

	// Verify ordering: not-configured, config-only, mode-differs.
	if results[0].Category != NotConfigured || results[0].Destination != "a.yml" {
		t.Errorf("results[0] = {%s, %s}, want {not-configured, a.yml}", results[0].Category, results[0].Destination)
	}
	if results[1].Category != ConfigOnly || results[1].Destination != "c.yml" {
		t.Errorf("results[1] = {%s, %s}, want {config-only, c.yml}", results[1].Category, results[1].Destination)
	}
	if results[2].Category != ModeDiffers || results[2].Destination != "b.yml" {
		t.Errorf("results[2] = {%s, %s}, want {mode-differs, b.yml}", results[2].Category, results[2].Destination)
	}
}

func TestCheckConfigDiffSortWithinCategory(t *testing.T) {
	defaults := []swatch.Swatch{
		{Source: "z.yml", Destination: "z.yml", DefaultAlteration: swatch.Always, Category: swatch.Health},
		{Source: "a.yml", Destination: "a.yml", DefaultAlteration: swatch.Always, Category: swatch.Health},
		{Source: "m.yml", Destination: "m.yml", DefaultAlteration: swatch.Always, Category: swatch.Health},
	}
	cfg := &config.Config{
		Swatches: []config.SwatchEntry{},
	}

	results := CheckConfigDiff(cfg, defaults)

	if len(results) != 3 {
		t.Fatalf("results count = %d, want 3", len(results))
	}

	// All not-configured, sorted lexicographically.
	if results[0].Destination != "a.yml" {
		t.Errorf("results[0].Destination = %q, want %q", results[0].Destination, "a.yml")
	}
	if results[1].Destination != "m.yml" {
		t.Errorf("results[1].Destination = %q, want %q", results[1].Destination, "m.yml")
	}
	if results[2].Destination != "z.yml" {
		t.Errorf("results[2].Destination = %q, want %q", results[2].Destination, "z.yml")
	}
}
