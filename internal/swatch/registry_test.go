package swatch_test

import (
	"testing"

	"github.com/wimpysworld/tailor/internal/swatch"
)

func TestAllReturns18Swatches(t *testing.T) {
	all := swatch.All()
	if len(all) != 18 {
		t.Fatalf("All() returned %d swatches, want 18", len(all))
	}
}

func TestAllSwatchesHaveRequiredFields(t *testing.T) {
	for _, s := range swatch.All() {
		t.Run(s.Path, func(t *testing.T) {
			if s.Path == "" {
				t.Error("Path is empty")
			}
			if s.DefaultAlteration != swatch.Always && s.DefaultAlteration != swatch.FirstFit && s.DefaultAlteration != swatch.Triggered {
				t.Errorf("DefaultAlteration is %q, want %q, %q, or %q", s.DefaultAlteration, swatch.Always, swatch.FirstFit, swatch.Triggered)
			}
			if s.Category != swatch.Health && s.Category != swatch.Development {
				t.Errorf("Category is %q, want %q or %q", s.Category, swatch.Health, swatch.Development)
			}
		})
	}
}

func TestSwatchAttributes(t *testing.T) {
	tests := []struct {
		path     string
		mode     swatch.AlterationMode
		category swatch.Category
	}{
		{".gitignore", swatch.FirstFit, swatch.Development},
		{".envrc", swatch.FirstFit, swatch.Development},
		{"SECURITY.md", swatch.Always, swatch.Health},
		{"CODE_OF_CONDUCT.md", swatch.Always, swatch.Health},
		{"CONTRIBUTING.md", swatch.Always, swatch.Health},
		{"SUPPORT.md", swatch.Always, swatch.Health},
		{"flake.nix", swatch.FirstFit, swatch.Development},
		{"cubic.yaml", swatch.FirstFit, swatch.Development},
		{"justfile", swatch.FirstFit, swatch.Development},
		{".github/FUNDING.yml", swatch.FirstFit, swatch.Health},
		{".github/dependabot.yml", swatch.FirstFit, swatch.Health},
		{".github/ISSUE_TEMPLATE/bug_report.yml", swatch.Always, swatch.Health},
		{".github/ISSUE_TEMPLATE/feature_request.yml", swatch.Always, swatch.Health},
		{".github/ISSUE_TEMPLATE/config.yml", swatch.FirstFit, swatch.Health},
		{".github/pull_request_template.md", swatch.Always, swatch.Health},
		{".github/workflows/tailor.yml", swatch.Always, swatch.Development},
		{".github/workflows/tailor-automerge.yml", swatch.Triggered, swatch.Development},
		{".tailor.yml", swatch.Always, swatch.Development},
	}

	all := swatch.All()
	byPath := make(map[string]swatch.Swatch, len(all))
	for _, s := range all {
		byPath[s.Path] = s
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			s, ok := byPath[tt.path]
			if !ok {
				t.Fatalf("swatch %q not found in All()", tt.path)
			}
			if s.DefaultAlteration != tt.mode {
				t.Errorf("DefaultAlteration = %q, want %q", s.DefaultAlteration, tt.mode)
			}
			if s.Category != tt.category {
				t.Errorf("Category = %q, want %q", s.Category, tt.category)
			}
		})
	}
}

func TestHealthSwatchesReturnsCorrectSubset(t *testing.T) {
	health := swatch.HealthSwatches()

	// The spec lists 10 health swatches (excluding LICENSE which is not embedded).
	if len(health) != 10 {
		t.Fatalf("HealthSwatches() returned %d swatches, want 10", len(health))
	}

	for _, s := range health {
		if s.Category != swatch.Health {
			t.Errorf("HealthSwatches() included %q with category %q", s.Path, s.Category)
		}
	}
}

func TestPathsReturnsSortedList(t *testing.T) {
	names := swatch.Paths()
	if len(names) != 18 {
		t.Fatalf("Paths() returned %d names, want 18", len(names))
	}
	for i := 1; i < len(names); i++ {
		if names[i] < names[i-1] {
			t.Fatalf("Paths() not sorted: %q comes after %q", names[i], names[i-1])
		}
	}
}

func TestPathsContainsKnownEntries(t *testing.T) {
	names := swatch.Paths()
	want := map[string]bool{
		".gitignore":  false,
		"justfile":    false,
		"SECURITY.md": false,
	}
	for _, n := range names {
		if _, ok := want[n]; ok {
			want[n] = true
		}
	}
	for name, found := range want {
		if !found {
			t.Errorf("Paths() missing %q", name)
		}
	}
}

func TestAllIsACopy(t *testing.T) {
	a := swatch.All()
	b := swatch.All()
	a[0].Path = "modified"
	if b[0].Path == "modified" {
		t.Fatal("All() returned a shared slice, not a copy")
	}
}
