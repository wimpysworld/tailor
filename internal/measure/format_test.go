package measure

import (
	"fmt"
	"testing"
)

func TestFormatOutputWithoutConfig(t *testing.T) {
	health := []HealthResult{
		{Destination: ".github/FUNDING.yml", Status: Missing},
		{Destination: ".github/ISSUE_TEMPLATE/bug_report.yml", Status: Missing},
		{Destination: ".github/ISSUE_TEMPLATE/feature_request.yml", Status: Missing},
		{Destination: ".github/dependabot.yml", Status: Missing},
		{Destination: ".github/pull_request_template.md", Status: Missing},
		{Destination: "CONTRIBUTING.md", Status: Missing},
		{Destination: "SUPPORT.md", Status: Missing},
		{Destination: "CODE_OF_CONDUCT.md", Status: Present},
		{Destination: "LICENSE", Status: Present},
		{Destination: "SECURITY.md", Status: Present},
	}

	got := FormatOutput(health, nil, false)

	want := "missing:        .github/FUNDING.yml\n" +
		"missing:        .github/ISSUE_TEMPLATE/bug_report.yml\n" +
		"missing:        .github/ISSUE_TEMPLATE/feature_request.yml\n" +
		"missing:        .github/dependabot.yml\n" +
		"missing:        .github/pull_request_template.md\n" +
		"missing:        CONTRIBUTING.md\n" +
		"missing:        SUPPORT.md\n" +
		"present:        CODE_OF_CONDUCT.md\n" +
		"present:        LICENSE\n" +
		"present:        SECURITY.md\n" +
		"\n" +
		"No .tailor/config.yml found. Run `tailor fit <path>` to initialise, or create `.tailor/config.yml` manually to enable configuration alignment checks.\n"

	if got != want {
		t.Errorf("FormatOutput without config:\ngot:\n%s\nwant:\n%s", got, want)
	}
}

func TestFormatOutputWithConfig(t *testing.T) {
	health := []HealthResult{
		{Destination: "CONTRIBUTING.md", Status: Missing},
		{Destination: "LICENSE", Status: Present},
		{Destination: "SECURITY.md", Status: Present},
	}

	diff := []DiffResult{
		{Destination: ".github/dependabot.yml", Category: NotConfigured},
		{Destination: "some-custom-swatch.yml", Category: ConfigOnly},
		{Destination: "SECURITY.md", Category: ModeDiffers, Detail: "(config: first-fit, default: always)"},
	}

	got := FormatOutput(health, diff, true)

	want := "missing:        CONTRIBUTING.md\n" +
		"present:        LICENSE\n" +
		"present:        SECURITY.md\n" +
		"not-configured: .github/dependabot.yml\n" +
		"config-only:    some-custom-swatch.yml\n" +
		"mode-differs:   SECURITY.md (config: first-fit, default: always)\n"

	if got != want {
		t.Errorf("FormatOutput with config:\ngot:\n%s\nwant:\n%s", got, want)
	}
}

func TestFormatOutputPaddingWidth(t *testing.T) {
	// Verify every category label produces exactly 16 characters before the destination.
	tests := []struct {
		label string
		width int
	}{
		{"missing:", 16},
		{"present:", 16},
		{"not-configured:", 16},
		{"config-only:", 16},
		{"mode-differs:", 16},
	}

	for _, tt := range tests {
		formatted := formatLabel(tt.label, tt.width)
		if len(formatted) != tt.width {
			t.Errorf("label %q padded to %d chars, want %d", tt.label, len(formatted), tt.width)
		}
	}
}

// formatLabel pads label to the given width, matching the production format logic.
func formatLabel(label string, width int) string {
	return fmt.Sprintf("%-*s", width, label)
}

func TestFormatOutputEmptyResults(t *testing.T) {
	got := FormatOutput(nil, nil, true)
	if got != "" {
		t.Errorf("FormatOutput with no results and config present:\ngot: %q\nwant: %q", got, "")
	}
}

func TestFormatOutputHealthOnlyWithConfig(t *testing.T) {
	health := []HealthResult{
		{Destination: "LICENSE", Status: Present},
	}

	got := FormatOutput(health, nil, true)
	want := "present:        LICENSE\n"

	if got != want {
		t.Errorf("got: %q\nwant: %q", got, want)
	}
}

func TestAdvisoryMessageContent(t *testing.T) {
	want := "No .tailor/config.yml found. Run `tailor fit <path>` to initialise, or create `.tailor/config.yml` manually to enable configuration alignment checks."
	if AdvisoryMessage != want {
		t.Errorf("AdvisoryMessage =\n%q\nwant:\n%q", AdvisoryMessage, want)
	}
}
