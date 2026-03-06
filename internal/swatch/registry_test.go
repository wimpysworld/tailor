package swatch_test

import (
	"testing"

	"github.com/wimpysworld/tailor/internal/swatch"
)

func TestAllReturns16Swatches(t *testing.T) {
	all := swatch.All()
	if len(all) != 16 {
		t.Fatalf("All() returned %d swatches, want 16", len(all))
	}
}

func TestAllSwatchesHaveRequiredFields(t *testing.T) {
	for _, s := range swatch.All() {
		t.Run(s.Source, func(t *testing.T) {
			if s.Source == "" {
				t.Error("Source is empty")
			}
			if s.Destination == "" {
				t.Error("Destination is empty")
			}
			if s.DefaultAlteration != swatch.Always && s.DefaultAlteration != swatch.FirstFit {
				t.Errorf("DefaultAlteration is %q, want %q or %q", s.DefaultAlteration, swatch.Always, swatch.FirstFit)
			}
			if s.Category != swatch.Health && s.Category != swatch.Development {
				t.Errorf("Category is %q, want %q or %q", s.Category, swatch.Health, swatch.Development)
			}
		})
	}
}

func TestSwatchAttributes(t *testing.T) {
	tests := []struct {
		source   string
		dest     string
		mode     swatch.AlterationMode
		category swatch.Category
	}{
		{".gitignore", ".gitignore", swatch.FirstFit, swatch.Development},
		{".envrc", ".envrc", swatch.FirstFit, swatch.Development},
		{"SECURITY.md", "SECURITY.md", swatch.Always, swatch.Health},
		{"CODE_OF_CONDUCT.md", "CODE_OF_CONDUCT.md", swatch.Always, swatch.Health},
		{"CONTRIBUTING.md", "CONTRIBUTING.md", swatch.Always, swatch.Health},
		{"SUPPORT.md", "SUPPORT.md", swatch.Always, swatch.Health},
		{"flake.nix", "flake.nix", swatch.FirstFit, swatch.Development},
		{"justfile", "justfile", swatch.FirstFit, swatch.Development},
		{".github/FUNDING.yml", ".github/FUNDING.yml", swatch.FirstFit, swatch.Health},
		{".github/dependabot.yml", ".github/dependabot.yml", swatch.FirstFit, swatch.Health},
		{".github/ISSUE_TEMPLATE/bug_report.yml", ".github/ISSUE_TEMPLATE/bug_report.yml", swatch.Always, swatch.Health},
		{".github/ISSUE_TEMPLATE/feature_request.yml", ".github/ISSUE_TEMPLATE/feature_request.yml", swatch.Always, swatch.Health},
		{".github/ISSUE_TEMPLATE/config.yml", ".github/ISSUE_TEMPLATE/config.yml", swatch.FirstFit, swatch.Health},
		{".github/pull_request_template.md", ".github/pull_request_template.md", swatch.Always, swatch.Health},
		{".github/workflows/tailor.yml", ".github/workflows/tailor.yml", swatch.Always, swatch.Development},
		{".tailor/config.yml", ".tailor/config.yml", swatch.FirstFit, swatch.Development},
	}

	for _, tt := range tests {
		t.Run(tt.source, func(t *testing.T) {
			s, err := swatch.BySource(tt.source)
			if err != nil {
				t.Fatalf("BySource(%q) returned error: %v", tt.source, err)
			}
			if s.Destination != tt.dest {
				t.Errorf("Destination = %q, want %q", s.Destination, tt.dest)
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

func TestBySourceUnknownReturnsError(t *testing.T) {
	_, err := swatch.BySource("nonexistent.txt")
	if err == nil {
		t.Fatal("BySource(\"nonexistent.txt\") expected error, got nil")
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
			t.Errorf("HealthSwatches() included %q with category %q", s.Source, s.Category)
		}
	}
}

func TestSourceNamesReturnsSortedList(t *testing.T) {
	names := swatch.SourceNames()
	if len(names) != 16 {
		t.Fatalf("SourceNames() returned %d names, want 16", len(names))
	}
	for i := 1; i < len(names); i++ {
		if names[i] < names[i-1] {
			t.Fatalf("SourceNames() not sorted: %q comes after %q", names[i], names[i-1])
		}
	}
}

func TestSourceNamesContainsKnownEntries(t *testing.T) {
	names := swatch.SourceNames()
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
			t.Errorf("SourceNames() missing %q", name)
		}
	}
}

func TestAllIsACopy(t *testing.T) {
	a := swatch.All()
	b := swatch.All()
	a[0].Source = "modified"
	if b[0].Source == "modified" {
		t.Fatal("All() returned a shared slice, not a copy")
	}
}
