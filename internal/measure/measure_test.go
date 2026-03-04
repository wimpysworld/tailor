package measure

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/wimpysworld/tailor/internal/config"
	"github.com/wimpysworld/tailor/internal/swatch"
)

// writeConfig writes a .tailor/config.yml file in dir with the given content.
func writeConfig(t *testing.T, dir, content string) {
	t.Helper()
	configDir := filepath.Join(dir, ".tailor")
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(filepath.Join(configDir, "config.yml"), []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
}

// buildDefaultConfigYAML builds a config YAML string containing all 16 default
// swatches at their default alteration modes.
func buildDefaultConfigYAML() string {
	var b strings.Builder
	b.WriteString("license: MIT\n")
	b.WriteString("swatches:\n")
	for _, s := range swatch.All() {
		b.WriteString("  - source: " + s.Source + "\n")
		b.WriteString("    destination: " + s.Destination + "\n")
		b.WriteString("    alteration: " + string(s.DefaultAlteration) + "\n")
	}
	return b.String()
}

// TestIntegrationEmptyDirNoConfig exercises the full measure pipeline against
// an empty directory with no config file. All 11 health files are missing and
// the advisory message is printed after a blank line.
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
		"\n" +
		"No .tailor/config.yml found. Run `tailor fit <path>` to initialise, or create `.tailor/config.yml` manually to enable configuration alignment checks.\n"

	if got != want {
		t.Errorf("empty dir, no config:\ngot:\n%s\nwant:\n%s", got, want)
	}
}

// TestIntegrationSomeHealthFilesNoConfig exercises the full pipeline with a
// subset of health files present and no config. The advisory message appears.
func TestIntegrationSomeHealthFilesNoConfig(t *testing.T) {
	dir := t.TempDir()

	// Create three health files matching the spec example (lines 258-270).
	createFile(t, dir, "CODE_OF_CONDUCT.md")
	createFile(t, dir, "LICENSE")
	createFile(t, dir, "SECURITY.md")

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
		"present:        CODE_OF_CONDUCT.md\n" +
		"present:        LICENSE\n" +
		"present:        SECURITY.md\n" +
		"\n" +
		"No .tailor/config.yml found. Run `tailor fit <path>` to initialise, or create `.tailor/config.yml` manually to enable configuration alignment checks.\n"

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
	createFile(t, dir, "LICENSE")
	createFile(t, dir, "SECURITY.md")

	// Write a config that matches all 16 defaults exactly.
	writeConfig(t, dir, buildDefaultConfigYAML())

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
		"present:        LICENSE\n" +
		"present:        SECURITY.md\n"

	if got != want {
		t.Errorf("config matches defaults:\ngot:\n%s\nwant:\n%s", got, want)
	}
}

// TestIntegrationConfigWithAllDiffCategories exercises the full pipeline with
// a config that produces entries in all five output categories: missing,
// present, not-configured, config-only, and mode-differs.
func TestIntegrationConfigWithAllDiffCategories(t *testing.T) {
	dir := t.TempDir()

	// Create a subset of health files.
	createFile(t, dir, "LICENSE")
	createFile(t, dir, "SECURITY.md")

	// Write a config that:
	// - omits .github/dependabot.yml (not-configured)
	// - adds some-custom-swatch.yml (config-only)
	// - changes SECURITY.md alteration from always to first-fit (mode-differs)
	// All other defaults are present at their default modes.
	configYAML := "license: MIT\nswatches:\n"
	for _, s := range swatch.All() {
		// Skip .github/dependabot.yml to produce not-configured.
		if s.Destination == ".github/dependabot.yml" {
			continue
		}
		alt := string(s.DefaultAlteration)
		// Override SECURITY.md mode to produce mode-differs.
		if s.Destination == "SECURITY.md" {
			alt = "first-fit"
		}
		configYAML += "  - source: " + s.Source + "\n"
		configYAML += "    destination: " + s.Destination + "\n"
		configYAML += "    alteration: " + alt + "\n"
	}
	// Add a custom swatch not in defaults to produce config-only.
	configYAML += "  - source: some-custom-swatch.yml\n"
	configYAML += "    destination: some-custom-swatch.yml\n"
	configYAML += "    alteration: always\n"

	writeConfig(t, dir, configYAML)

	health := CheckHealth(dir)

	cfg, err := config.Load(dir)
	if err != nil {
		t.Fatalf("config.Load: %v", err)
	}
	diff := CheckConfigDiff(cfg, swatch.All())
	hasConfig := true

	got := FormatOutput(health, diff, hasConfig)

	// This output matches the spec example format (lines 275-281) adapted
	// to the specific files present in our temp directory.
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
// correct category order (missing, present, not-configured, config-only,
// mode-differs), that labels are padded to exactly 16 characters, and that
// entries within each category are sorted lexicographically by destination.
func TestIntegrationOutputOrderAndPadding(t *testing.T) {
	dir := t.TempDir()

	// Create specific health files to get a mix of missing and present.
	createFile(t, dir, "CONTRIBUTING.md")
	createFile(t, dir, "LICENSE")
	createFile(t, dir, "SECURITY.md")

	// Config with all three diff categories, multiple entries per category
	// to verify lexicographic sorting.
	configYAML := "license: MIT\nswatches:\n"
	for _, s := range swatch.All() {
		// Omit two defaults to produce two not-configured entries.
		if s.Destination == ".github/dependabot.yml" || s.Destination == ".envrc" {
			continue
		}
		alt := string(s.DefaultAlteration)
		// Override two modes to produce two mode-differs entries.
		if s.Destination == "SECURITY.md" {
			alt = "first-fit"
		}
		if s.Destination == "CODE_OF_CONDUCT.md" {
			alt = "first-fit"
		}
		configYAML += "  - source: " + s.Source + "\n"
		configYAML += "    destination: " + s.Destination + "\n"
		configYAML += "    alteration: " + alt + "\n"
	}
	// Add two config-only entries.
	configYAML += "  - source: beta-custom.yml\n"
	configYAML += "    destination: beta-custom.yml\n"
	configYAML += "    alteration: always\n"
	configYAML += "  - source: alpha-custom.yml\n"
	configYAML += "    destination: alpha-custom.yml\n"
	configYAML += "    alteration: first-fit\n"

	writeConfig(t, dir, configYAML)

	health := CheckHealth(dir)

	cfg, err := config.Load(dir)
	if err != nil {
		t.Fatalf("config.Load: %v", err)
	}
	diff := CheckConfigDiff(cfg, swatch.All())
	hasConfig := true

	got := FormatOutput(health, diff, hasConfig)

	// Build expected output verifying:
	// 1. Category order: missing, present, not-configured, config-only, mode-differs
	// 2. Lexicographic sort within each category
	// 3. 16-char fixed-width label padding
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

