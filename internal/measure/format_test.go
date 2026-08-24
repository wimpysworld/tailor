package measure

import (
	"testing"
)

func TestFormatOutputWithoutConfig(t *testing.T) {
	health := []HealthResult{
		{Path: ".github/FUNDING.yml", Status: Missing},
		{Path: ".github/ISSUE_TEMPLATE/bug_report.yml", Status: Missing},
		{Path: ".github/ISSUE_TEMPLATE/feature_request.yml", Status: Missing},
		{Path: ".github/dependabot.yml", Status: Missing},
		{Path: ".github/pull_request_template.md", Status: Missing},
		{Path: "CONTRIBUTING.md", Status: Missing},
		{Path: "SUPPORT.md", Status: Missing},
		{Path: "LICENSE", Status: Warning, Detail: "(contains unresolved placeholders)"},
		{Path: "README.md", Status: Warning, Detail: "(not managed by tailor)"},
		{Path: "CODE_OF_CONDUCT.md", Status: Present},
		{Path: "SECURITY.md", Status: Present},
	}

	got := FormatOutput(health, nil, false)

	want := "missing:        .github/FUNDING.yml\n" +
		"missing:        .github/ISSUE_TEMPLATE/bug_report.yml\n" +
		"missing:        .github/ISSUE_TEMPLATE/feature_request.yml\n" +
		"missing:        .github/dependabot.yml\n" +
		"missing:        .github/pull_request_template.md\n" +
		"missing:        CONTRIBUTING.md\n" +
		"missing:        SUPPORT.md\n" +
		"warning:        LICENSE (contains unresolved placeholders)\n" +
		"warning:        README.md (not managed by tailor)\n" +
		"present:        CODE_OF_CONDUCT.md\n" +
		"present:        SECURITY.md\n" +
		"\n" +
		"No .tailor.yml found. Run `tailor fit <path>` to initialise, or create `.tailor.yml` manually to enable configuration alignment checks.\n"

	if got != want {
		t.Errorf("FormatOutput without config:\ngot:\n%s\nwant:\n%s", got, want)
	}
}

func TestFormatOutputWithConfig(t *testing.T) {
	health := []HealthResult{
		{Path: "CONTRIBUTING.md", Status: Missing},
		{Path: "LICENSE", Status: Present},
		{Path: "SECURITY.md", Status: Present},
	}

	diff := []DiffResult{
		{Path: ".github/dependabot.yml", Category: NotConfigured},
		{Path: "some-custom-swatch.yml", Category: ConfigOnly},
		{Path: "SECURITY.md", Category: ModeDiffers, Detail: "(config: first-fit, default: always)"},
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

func TestFormatOutputEmptyResults(t *testing.T) {
	got := FormatOutput(nil, nil, true)
	if got != "" {
		t.Errorf("FormatOutput with no results and config present:\ngot: %q\nwant: %q", got, "")
	}
}

func TestFormatOutputHealthOnlyWithConfig(t *testing.T) {
	health := []HealthResult{
		{Path: "LICENSE", Status: Present},
	}

	got := FormatOutput(health, nil, true)
	want := "present:        LICENSE\n"

	if got != want {
		t.Errorf("got: %q\nwant: %q", got, want)
	}
}
