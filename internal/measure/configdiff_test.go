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
			{Path: "custom.yml", Alteration: swatch.Always},
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
	if results[0].Path != "custom.yml" {
		t.Errorf("destination = %q, want %q", results[0].Path, "custom.yml")
	}
}

func TestCheckConfigDiffModeDiffers(t *testing.T) {
	cfg := &config.Config{
		Swatches: []config.SwatchEntry{
			{Path: "SECURITY.md", Alteration: swatch.FirstFit},
		},
	}
	defaults := []swatch.Swatch{
		{Path: "SECURITY.md", DefaultAlteration: swatch.Always, Category: swatch.Health},
	}

	results := CheckConfigDiff(cfg, defaults)

	if len(results) != 1 {
		t.Fatalf("results count = %d, want 1", len(results))
	}
	r := results[0]
	if r.Category != ModeDiffers {
		t.Errorf("category = %q, want %q", r.Category, ModeDiffers)
	}
	if r.Detail != "(config: first-fit, default: always)" {
		t.Errorf("annotation = %q, want %q", r.Detail, "(config: first-fit, default: always)")
	}
}

func TestCheckConfigDiffExactMatch(t *testing.T) {
	// Config matches defaults exactly, so no diff results.
	defaults := swatch.All()
	entries := make([]config.SwatchEntry, len(defaults))
	for i, s := range defaults {
		entries[i] = config.SwatchEntry{
			Path:       s.Path,
			Alteration: s.DefaultAlteration,
		}
	}
	cfg := &config.Config{Swatches: entries}

	results := CheckConfigDiff(cfg, defaults)

	if len(results) != 0 {
		t.Errorf("exact match produced %d diff results, want 0", len(results))
		for _, r := range results {
			t.Logf("  %s: %s %s", r.Category, r.Path, r.Detail)
		}
	}
}

func TestCheckConfigDiffAllCategories(t *testing.T) {
	defaults := []swatch.Swatch{
		{Path: "a.yml", DefaultAlteration: swatch.Always, Category: swatch.Health},
		{Path: "b.yml", DefaultAlteration: swatch.Always, Category: swatch.Development},
	}
	cfg := &config.Config{
		Swatches: []config.SwatchEntry{
			// b.yml present with different mode -> mode-differs
			{Path: "b.yml", Alteration: swatch.FirstFit},
			// c.yml not in defaults -> config-only
			{Path: "c.yml", Alteration: swatch.Always},
		},
	}
	// a.yml missing from config -> not-configured

	results := CheckConfigDiff(cfg, defaults)

	if len(results) != 3 {
		t.Fatalf("results count = %d, want 3", len(results))
	}

	// Verify ordering: not-configured, config-only, mode-differs.
	if results[0].Category != NotConfigured || results[0].Path != "a.yml" {
		t.Errorf("results[0] = {%s, %s}, want {not-configured, a.yml}", results[0].Category, results[0].Path)
	}
	if results[1].Category != ConfigOnly || results[1].Path != "c.yml" {
		t.Errorf("results[1] = {%s, %s}, want {config-only, c.yml}", results[1].Category, results[1].Path)
	}
	if results[2].Category != ModeDiffers || results[2].Path != "b.yml" {
		t.Errorf("results[2] = {%s, %s}, want {mode-differs, b.yml}", results[2].Category, results[2].Path)
	}
}

func TestCheckConfigDiffSortWithinCategory(t *testing.T) {
	defaults := []swatch.Swatch{
		{Path: "z.yml", DefaultAlteration: swatch.Always, Category: swatch.Health},
		{Path: "a.yml", DefaultAlteration: swatch.Always, Category: swatch.Health},
		{Path: "m.yml", DefaultAlteration: swatch.Always, Category: swatch.Health},
	}
	cfg := &config.Config{
		Swatches: []config.SwatchEntry{},
	}

	results := CheckConfigDiff(cfg, defaults)

	if len(results) != 3 {
		t.Fatalf("results count = %d, want 3", len(results))
	}

	// All not-configured, sorted lexicographically.
	if results[0].Path != "a.yml" {
		t.Errorf("results[0].Path = %q, want %q", results[0].Path, "a.yml")
	}
	if results[1].Path != "m.yml" {
		t.Errorf("results[1].Path = %q, want %q", results[1].Path, "m.yml")
	}
	if results[2].Path != "z.yml" {
		t.Errorf("results[2].Path = %q, want %q", results[2].Path, "z.yml")
	}
}
