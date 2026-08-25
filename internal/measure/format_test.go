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

func TestFormatOutputEscapesControlCharacters(t *testing.T) {
	tests := []struct {
		name string
		diff []DiffResult
		want string
	}{
		{
			name: "newline in swatch path",
			diff: []DiffResult{{Path: "safe.yml\nmissing:        forged.yml", Category: ConfigOnly}},
			want: "config-only:    safe.yml\\x0amissing:        forged.yml\n",
		},
		{
			name: "carriage return in swatch path",
			diff: []DiffResult{{Path: "safe.yml\rspoofed.yml", Category: ConfigOnly}},
			want: "config-only:    safe.yml\\x0dspoofed.yml\n",
		},
		{
			name: "ansi csi sequence in swatch path",
			diff: []DiffResult{{Path: "\x1b[31mred.yml\x1b[0m", Category: ConfigOnly}},
			want: "config-only:    \\x1b[31mred.yml\\x1b[0m\n",
		},
		{
			name: "bare escape in swatch path",
			diff: []DiffResult{{Path: "path\x1b.yml", Category: ConfigOnly}},
			want: "config-only:    path\\x1b.yml\n",
		},
		{
			name: "c1 control in swatch path",
			diff: []DiffResult{{Path: "path\u009b.yml", Category: ConfigOnly}},
			want: "config-only:    path\\u009b.yml\n",
		},
		{
			name: "escape in detail",
			diff: []DiffResult{{Path: "safe.yml", Category: ModeDiffers, Detail: "(config: \x1b[31mfirst-fit\x1b[0m, default: always)"}},
			want: "mode-differs:   safe.yml (config: \\x1b[31mfirst-fit\\x1b[0m, default: always)\n",
		},
		{
			name: "benign path unchanged",
			diff: []DiffResult{{Path: ".github/dependabot.yml", Category: NotConfigured}},
			want: "not-configured: .github/dependabot.yml\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := FormatOutput(nil, tt.diff, true); got != tt.want {
				t.Errorf("FormatOutput() = %q, want %q", got, tt.want)
			}
		})
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
