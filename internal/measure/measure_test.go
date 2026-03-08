package measure

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/wimpysworld/tailor/internal/config"
	"github.com/wimpysworld/tailor/internal/swatch"
	"github.com/wimpysworld/tailor/internal/testutil"
)

// buildDefaultConfigYAML builds a config YAML string containing all 16 default
// swatches at their default alteration modes.
func buildDefaultConfigYAML() string {
	var b strings.Builder
	b.WriteString("license: MIT\n")
	b.WriteString("swatches:\n")
	for _, s := range swatch.All() {
		b.WriteString("  - path: " + s.Path + "\n")
		b.WriteString("    alteration: " + string(s.DefaultAlteration) + "\n")
	}
	return b.String()
}

// TestIntegrationEmptyDirNoConfig exercises the full measure pipeline against
// an empty directory with no config file. All 11 health files are missing,
// README.md appears as a warning, and the advisory message is printed.
func TestIntegrationEmptyDirNoConfig(t *testing.T) {
	dir := t.TempDir()

	health := CheckHealth(dir)
	hasConfig := false
	var diff []DiffResult

	got := FormatOutput(health, diff, hasConfig)

	want := "" +
		"missing:        .github/FUNDING.yml\n" +
		"missing:        .github/ISSUE_TEMPLATE/bug_report.yml\n" +
		"missing:        .github/ISSUE_TEMPLATE/config.yml\n" +
		"missing:        .github/ISSUE_TEMPLATE/feature_request.yml\n" +
		"missing:        .github/dependabot.yml\n" +
		"missing:        .github/pull_request_template.md\n" +
		"missing:        CODE_OF_CONDUCT.md\n" +
		"missing:        CONTRIBUTING.md\n" +
		"missing:        LICENSE\n" +
		"missing:        SECURITY.md\n" +
		"missing:        SUPPORT.md\n" +
		"warning:        README.md (not managed by tailor)\n" +
		"\n" +
		"No .tailor.yml found. Run `tailor fit <path>` to initialise, or create `.tailor.yml` manually to enable configuration alignment checks.\n"

	if got != want {
		t.Errorf("empty dir, no config:\ngot:\n%s\nwant:\n%s", got, want)
	}
}

// TestIntegrationSomeHealthFilesNoConfig exercises the full pipeline with a
// subset of health files present and no config. The advisory message appears.
func TestIntegrationSomeHealthFilesNoConfig(t *testing.T) {
	dir := t.TempDir()

	// Create three health files.
	testutil.CreateFile(t, dir, "CODE_OF_CONDUCT.md")
	testutil.CreateFile(t, dir, "LICENSE")
	testutil.CreateFile(t, dir, "SECURITY.md")

	health := CheckHealth(dir)
	hasConfig := false
	var diff []DiffResult

	got := FormatOutput(health, diff, hasConfig)

	want := "" +
		"missing:        .github/FUNDING.yml\n" +
		"missing:        .github/ISSUE_TEMPLATE/bug_report.yml\n" +
		"missing:        .github/ISSUE_TEMPLATE/config.yml\n" +
		"missing:        .github/ISSUE_TEMPLATE/feature_request.yml\n" +
		"missing:        .github/dependabot.yml\n" +
		"missing:        .github/pull_request_template.md\n" +
		"missing:        CONTRIBUTING.md\n" +
		"missing:        SUPPORT.md\n" +
		"warning:        README.md (not managed by tailor)\n" +
		"present:        CODE_OF_CONDUCT.md\n" +
		"present:        LICENSE\n" +
		"present:        SECURITY.md\n" +
		"\n" +
		"No .tailor.yml found. Run `tailor fit <path>` to initialise, or create `.tailor.yml` manually to enable configuration alignment checks.\n"

	if got != want {
		t.Errorf("some health files, no config:\ngot:\n%s\nwant:\n%s", got, want)
	}
}

// TestIntegrationConfigMatchesDefaults exercises the full pipeline with a
// config that matches the built-in defaults exactly. Health checks are shown
// but no config-diff entries appear in the output.
func TestIntegrationConfigMatchesDefaults(t *testing.T) {
	dir := t.TempDir()

	// Create a few health files so the output is not all-missing.
	testutil.CreateFile(t, dir, "LICENSE")
	testutil.CreateFile(t, dir, "SECURITY.md")

	// Write a config that matches all 16 defaults exactly.
	testutil.WriteConfig(t, dir, buildDefaultConfigYAML())

	health := CheckHealth(dir)

	cfg, err := config.Load(dir)
	if err != nil {
		t.Fatalf("config.Load: %v", err)
	}
	diff := CheckConfigDiff(cfg, swatch.All())
	hasConfig := true

	got := FormatOutput(health, diff, hasConfig)

	// Config matches defaults: no config-diff lines, no advisory.
	want := "" +
		"missing:        .github/FUNDING.yml\n" +
		"missing:        .github/ISSUE_TEMPLATE/bug_report.yml\n" +
		"missing:        .github/ISSUE_TEMPLATE/config.yml\n" +
		"missing:        .github/ISSUE_TEMPLATE/feature_request.yml\n" +
		"missing:        .github/dependabot.yml\n" +
		"missing:        .github/pull_request_template.md\n" +
		"missing:        CODE_OF_CONDUCT.md\n" +
		"missing:        CONTRIBUTING.md\n" +
		"missing:        SUPPORT.md\n" +
		"warning:        README.md (not managed by tailor)\n" +
		"present:        LICENSE\n" +
		"present:        SECURITY.md\n"

	if got != want {
		t.Errorf("config matches defaults:\ngot:\n%s\nwant:\n%s", got, want)
	}
}

// TestIntegrationConfigWithAllDiffCategories exercises the full pipeline with
// a config that produces entries in all six output categories: missing,
// warning, present, not-configured, config-only, and mode-differs.
func TestIntegrationConfigWithAllDiffCategories(t *testing.T) {
	dir := t.TempDir()

	// Create a subset of health files.
	testutil.CreateFile(t, dir, "LICENSE")
	testutil.CreateFile(t, dir, "SECURITY.md")

	// Write a config that:
	// - omits .github/dependabot.yml (not-configured)
	// - adds some-custom-swatch.yml (config-only)
	// - changes SECURITY.md alteration from always to first-fit (mode-differs)
	// All other defaults are present at their default modes.
	var b strings.Builder
	b.WriteString("license: MIT\n")
	b.WriteString("swatches:\n")
	for _, s := range swatch.All() {
		// Skip .github/dependabot.yml to produce not-configured.
		if s.Path == ".github/dependabot.yml" {
			continue
		}
		alt := string(s.DefaultAlteration)
		// Override SECURITY.md mode to produce mode-differs.
		if s.Path == "SECURITY.md" {
			alt = "first-fit"
		}
		b.WriteString("  - path: " + s.Path + "\n")
		b.WriteString("    alteration: " + alt + "\n")
	}
	// Add a custom swatch not in defaults to produce config-only.
	b.WriteString("  - path: some-custom-swatch.yml\n")
	b.WriteString("    alteration: always\n")

	testutil.WriteConfig(t, dir, b.String())

	health := CheckHealth(dir)

	cfg, err := config.Load(dir)
	if err != nil {
		t.Fatalf("config.Load: %v", err)
	}
	diff := CheckConfigDiff(cfg, swatch.All())
	hasConfig := true

	got := FormatOutput(health, diff, hasConfig)

	want := "" +
		"missing:        .github/FUNDING.yml\n" +
		"missing:        .github/ISSUE_TEMPLATE/bug_report.yml\n" +
		"missing:        .github/ISSUE_TEMPLATE/config.yml\n" +
		"missing:        .github/ISSUE_TEMPLATE/feature_request.yml\n" +
		"missing:        .github/dependabot.yml\n" +
		"missing:        .github/pull_request_template.md\n" +
		"missing:        CODE_OF_CONDUCT.md\n" +
		"missing:        CONTRIBUTING.md\n" +
		"missing:        SUPPORT.md\n" +
		"warning:        README.md (not managed by tailor)\n" +
		"present:        LICENSE\n" +
		"present:        SECURITY.md\n" +
		"not-configured: .github/dependabot.yml\n" +
		"config-only:    some-custom-swatch.yml\n" +
		"mode-differs:   SECURITY.md (config: first-fit, default: always)\n"

	if got != want {
		t.Errorf("all diff categories:\ngot:\n%s\nwant:\n%s", got, want)
	}
}

// TestIntegrationOutputOrderAndPadding verifies that entries appear in the
// correct category order (missing, warning, present, not-configured,
// config-only, mode-differs), that labels are padded to exactly 16 characters,
// and that entries within each category are sorted lexicographically.
func TestIntegrationOutputOrderAndPadding(t *testing.T) {
	dir := t.TempDir()

	// Create specific health files to get a mix of missing and present.
	testutil.CreateFile(t, dir, "CONTRIBUTING.md")
	testutil.CreateFile(t, dir, "LICENSE")
	testutil.CreateFile(t, dir, "SECURITY.md")

	// Config with all three diff categories, multiple entries per category
	// to verify lexicographic sorting.
	var b strings.Builder
	b.WriteString("license: MIT\n")
	b.WriteString("swatches:\n")
	for _, s := range swatch.All() {
		// Omit two defaults to produce two not-configured entries.
		if s.Path == ".github/dependabot.yml" || s.Path == ".envrc" {
			continue
		}
		alt := string(s.DefaultAlteration)
		// Override two modes to produce two mode-differs entries.
		if s.Path == "SECURITY.md" {
			alt = "first-fit"
		}
		if s.Path == "CODE_OF_CONDUCT.md" {
			alt = "first-fit"
		}
		b.WriteString("  - path: " + s.Path + "\n")
		b.WriteString("    alteration: " + alt + "\n")
	}
	// Add two config-only entries.
	b.WriteString("  - path: beta-custom.yml\n")
	b.WriteString("    alteration: always\n")
	b.WriteString("  - path: alpha-custom.yml\n")
	b.WriteString("    alteration: first-fit\n")

	testutil.WriteConfig(t, dir, b.String())

	health := CheckHealth(dir)

	cfg, err := config.Load(dir)
	if err != nil {
		t.Fatalf("config.Load: %v", err)
	}
	diff := CheckConfigDiff(cfg, swatch.All())
	hasConfig := true

	got := FormatOutput(health, diff, hasConfig)

	want := "" +
		// missing (sorted lexicographically)
		"missing:        .github/FUNDING.yml\n" +
		"missing:        .github/ISSUE_TEMPLATE/bug_report.yml\n" +
		"missing:        .github/ISSUE_TEMPLATE/config.yml\n" +
		"missing:        .github/ISSUE_TEMPLATE/feature_request.yml\n" +
		"missing:        .github/dependabot.yml\n" +
		"missing:        .github/pull_request_template.md\n" +
		"missing:        CODE_OF_CONDUCT.md\n" +
		"missing:        SUPPORT.md\n" +
		// warning (sorted lexicographically)
		"warning:        README.md (not managed by tailor)\n" +
		// present (sorted lexicographically)
		"present:        CONTRIBUTING.md\n" +
		"present:        LICENSE\n" +
		"present:        SECURITY.md\n" +
		// not-configured (sorted lexicographically)
		"not-configured: .envrc\n" +
		"not-configured: .github/dependabot.yml\n" +
		// config-only (sorted lexicographically)
		"config-only:    alpha-custom.yml\n" +
		"config-only:    beta-custom.yml\n" +
		// mode-differs (sorted lexicographically)
		"mode-differs:   CODE_OF_CONDUCT.md (config: first-fit, default: always)\n" +
		"mode-differs:   SECURITY.md (config: first-fit, default: always)\n"

	if got != want {
		t.Errorf("output order and padding:\ngot:\n%s\nwant:\n%s", got, want)
	}

	// Verify 16-char label padding explicitly by checking that column 16
	// (0-indexed) of every non-empty, non-advisory line is the first
	// character of the destination path.
	lines := strings.FieldsFunc(got, func(r rune) bool { return r == '\n' })
	for _, line := range lines {
		if len(line) < 17 {
			t.Errorf("line too short for padding check: %q", line)
			continue
		}
		// Characters 0-15 are the padded label; character 16 starts the value.
		label := line[:16]
		lastLabelChar := label[len(label)-1]
		if lastLabelChar != ' ' && lastLabelChar != ':' {
			t.Errorf("label padding violated, expected space or colon at position 15: %q", line)
		}
	}
}

// TestIntegrationLicenseWithPlaceholders verifies that a LICENSE containing
// unresolved placeholders appears as warning, not present.
func TestIntegrationLicenseWithPlaceholders(t *testing.T) {
	dir := t.TempDir()

	content := "MIT License\n\nCopyright (c) [year] [fullname]\n"
	if err := os.WriteFile(filepath.Join(dir, "LICENSE"), []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	testutil.CreateFile(t, dir, "README.md")

	health := CheckHealth(dir)
	got := FormatOutput(health, nil, false)

	if !strings.Contains(got, "warning:        LICENSE (contains unresolved placeholders)") {
		t.Errorf("expected LICENSE warning line in output:\n%s", got)
	}
	if strings.Contains(got, "present:        LICENSE") {
		t.Errorf("LICENSE with placeholders should not appear as present:\n%s", got)
	}
}

// TestIntegrationReadmePresent verifies that no README.md warning appears
// when README.md exists.
func TestIntegrationReadmePresent(t *testing.T) {
	dir := t.TempDir()

	testutil.CreateFile(t, dir, "README.md")

	health := CheckHealth(dir)
	got := FormatOutput(health, nil, false)

	if strings.Contains(got, "README.md") {
		t.Errorf("README.md should not appear in output when present:\n%s", got)
	}
}
