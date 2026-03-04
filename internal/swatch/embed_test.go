package swatch_test

import (
	"testing"

	"github.com/wimpysworld/tailor/internal/swatch"
)

func TestContentReadsAllEmbeddedSwatches(t *testing.T) {
	sources := []string{
		".gitignore",
		".envrc",
		"SECURITY.md",
		"CODE_OF_CONDUCT.md",
		"CONTRIBUTING.md",
		"SUPPORT.md",
		"flake.nix",
		"justfile",
		".github/FUNDING.yml",
		".github/dependabot.yml",
		".github/ISSUE_TEMPLATE/bug_report.yml",
		".github/ISSUE_TEMPLATE/feature_request.yml",
		".github/ISSUE_TEMPLATE/config.yml",
		".github/pull_request_template.md",
		".github/workflows/tailor.yml",
		".tailor/config.yml",
	}

	for _, source := range sources {
		t.Run(source, func(t *testing.T) {
			data, err := swatch.Content(source)
			if err != nil {
				t.Fatalf("Content(%q) returned error: %v", source, err)
			}
			if len(data) == 0 {
				t.Fatalf("Content(%q) returned empty bytes", source)
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
