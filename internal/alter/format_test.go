package alter

import (
	"fmt"
	"testing"
)

func TestFormatOutputSwatchesOnly(t *testing.T) {
	swatches := []SwatchResult{
		{Path: ".github/FUNDING.yml", Category: WouldOverwrite},
		{Path: "CONTRIBUTING.md", Category: WouldCopy},
		{Path: "LICENSE", Category: NoChange},
		{Path: ".tailor.yml", Category: SkippedFirstFit},
	}

	got := FormatOutput(nil, nil, swatches)
	want := "would copy:                          CONTRIBUTING.md\n" +
		"would overwrite:                     .github/FUNDING.yml\n" +
		"no change:                           LICENSE\n" +
		"skipped (first-fit, exists):         .tailor.yml\n"

	if got != want {
		t.Errorf("FormatOutput swatches only:\ngot:\n%s\nwant:\n%s", got, want)
	}
}

func TestFormatOutputRepoSettingsOnly(t *testing.T) {
	repos := []RepoSettingResult{
		{Field: "has_wiki", Category: WouldSet, Value: "false"},
		{Field: "has_issues", Category: RepoNoChange, Value: "true"},
		{Field: "description", Category: WouldSet, Value: "My project"},
	}

	got := FormatOutput(repos, nil, nil)
	want := "would set:                           repository.description = My project\n" +
		"would set:                           repository.has_wiki = false\n" +
		"no change:                           repository.has_issues (already true)\n"

	if got != want {
		t.Errorf("FormatOutput repo settings only:\ngot:\n%s\nwant:\n%s", got, want)
	}
}

func TestFormatOutputCombined(t *testing.T) {
	repos := []RepoSettingResult{
		{Field: "has_wiki", Category: WouldSet, Value: "false"},
		{Field: "has_issues", Category: RepoNoChange, Value: "true"},
	}

	swatches := []SwatchResult{
		{Path: "CONTRIBUTING.md", Category: WouldCopy},
		{Path: "LICENSE", Category: NoChange},
	}

	got := FormatOutput(repos, nil, swatches)
	want := "would set:                           repository.has_wiki = false\n" +
		"no change:                           repository.has_issues (already true)\n" +
		"would copy:                          CONTRIBUTING.md\n" +
		"no change:                           LICENSE\n"

	if got != want {
		t.Errorf("FormatOutput combined:\ngot:\n%s\nwant:\n%s", got, want)
	}
}

func TestFormatOutputEmpty(t *testing.T) {
	got := FormatOutput(nil, nil, nil)
	if got != "" {
		t.Errorf("FormatOutput empty: got %q, want %q", got, "")
	}
}

func TestFormatOutputEmptySlices(t *testing.T) {
	got := FormatOutput([]RepoSettingResult{}, nil, []SwatchResult{})
	if got != "" {
		t.Errorf("FormatOutput empty slices: got %q, want %q", got, "")
	}
}

func TestFormatOutputSwatchSorting(t *testing.T) {
	swatches := []SwatchResult{
		{Path: "Z-file.md", Category: NoChange},
		{Path: "A-file.md", Category: SkippedFirstFit},
		{Path: "B-file.md", Category: WouldCopy},
		{Path: "A-file.md", Category: WouldOverwrite},
		{Path: "C-file.md", Category: WouldCopy},
		{Path: "M-file.md", Category: NoChange},
	}

	got := FormatOutput(nil, nil, swatches)
	want := "would copy:                          B-file.md\n" +
		"would copy:                          C-file.md\n" +
		"would overwrite:                     A-file.md\n" +
		"no change:                           M-file.md\n" +
		"no change:                           Z-file.md\n" +
		"skipped (first-fit, exists):         A-file.md\n"

	if got != want {
		t.Errorf("FormatOutput swatch sorting:\ngot:\n%s\nwant:\n%s", got, want)
	}
}

func TestFormatOutputRepoSettingSorting(t *testing.T) {
	repos := []RepoSettingResult{
		{Field: "has_wiki", Category: RepoNoChange, Value: "false"},
		{Field: "has_issues", Category: WouldSet, Value: "true"},
		{Field: "description", Category: RepoNoChange, Value: "A project"},
		{Field: "allow_squash_merge", Category: WouldSet, Value: "true"},
	}

	got := FormatOutput(repos, nil, nil)
	want := "would set:                           repository.allow_squash_merge = true\n" +
		"would set:                           repository.has_issues = true\n" +
		"no change:                           repository.description (already A project)\n" +
		"no change:                           repository.has_wiki (already false)\n"

	if got != want {
		t.Errorf("FormatOutput repo sorting:\ngot:\n%s\nwant:\n%s", got, want)
	}
}

func TestFormatOutputColumnAlignment(t *testing.T) {
	labels := []string{
		"would copy:",
		"would overwrite:",
		"would remove:",
		"removed:",
		"no change:",
		"skipped (first-fit, exists):",
		"skip (never):",
		"would set:",
		"would skip (insufficient scope):",
		"would skip (insufficient role):",
	}

	for _, label := range labels {
		padded := fmt.Sprintf("%-*s", defaultLabelWidth, label)
		if len(padded) != defaultLabelWidth {
			t.Errorf("label %q padded to %d chars, want %d", label, len(padded), defaultLabelWidth)
		}
	}
}

func TestFormatOutputActionableBeforeInformational(t *testing.T) {
	// All informational first in input, actionable should appear first in output.
	swatches := []SwatchResult{
		{Path: "info1.md", Category: NoChange},
		{Path: "info2.md", Category: SkippedFirstFit},
		{Path: "action1.md", Category: WouldCopy},
		{Path: "action2.md", Category: WouldOverwrite},
	}

	got := FormatOutput(nil, nil, swatches)
	want := "would copy:                          action1.md\n" +
		"would overwrite:                     action2.md\n" +
		"no change:                           info1.md\n" +
		"skipped (first-fit, exists):         info2.md\n"

	if got != want {
		t.Errorf("FormatOutput actionable before informational:\ngot:\n%s\nwant:\n%s", got, want)
	}
}

func TestFormatOutputRepoSettingsBeforeSwatches(t *testing.T) {
	repos := []RepoSettingResult{
		{Field: "has_wiki", Category: WouldSet, Value: "false"},
	}
	swatches := []SwatchResult{
		{Path: "CONTRIBUTING.md", Category: WouldCopy},
	}

	got := FormatOutput(repos, nil, swatches)

	// Repo settings line must appear before swatch line.
	repoIdx := 0
	swatchIdx := len("would set:                           repository.has_wiki = false\n")
	if got[:swatchIdx] != "would set:                           repository.has_wiki = false\n" {
		t.Errorf("repo settings not first in output:\ngot:\n%s", got)
	}
	_ = repoIdx
}

func TestFormatOutputNoTrailingBlankLine(t *testing.T) {
	swatches := []SwatchResult{
		{Path: "file.md", Category: WouldCopy},
	}

	got := FormatOutput(nil, nil, swatches)
	if got[len(got)-1] != '\n' {
		t.Error("output should end with newline")
	}
	if len(got) > 1 && got[len(got)-2] == '\n' {
		t.Error("output should not have trailing blank line")
	}
}

func TestFormatOutputNewCategories(t *testing.T) {
	swatches := []SwatchResult{
		{Path: "removed.yml", Category: Removed},
		{Path: "ignored.yml", Category: SkippedNever},
		{Path: "would-remove.yml", Category: WouldRemove},
		{Path: "copied.md", Category: WouldCopy},
	}

	got := FormatOutput(nil, nil, swatches)
	want := "would copy:                          copied.md\n" +
		"would remove:                        would-remove.yml\n" +
		"removed:                             removed.yml\n" +
		"skip (never):                        ignored.yml\n"

	if got != want {
		t.Errorf("FormatOutput new categories:\ngot:\n%s\nwant:\n%s", got, want)
	}
}

func TestFormatOutputNewCategorySorting(t *testing.T) {
	swatches := []SwatchResult{
		{Path: "z-ignored.yml", Category: SkippedNever},
		{Path: "a-removed.yml", Category: Removed},
		{Path: "b-would-remove.yml", Category: WouldRemove},
		{Path: "c-skipped.md", Category: SkippedFirstFit},
		{Path: "d-no-change.md", Category: NoChange},
		{Path: "e-would-copy.md", Category: WouldCopy},
		{Path: "f-would-overwrite.md", Category: WouldOverwrite},
		{Path: "a-would-remove.yml", Category: WouldRemove},
	}

	got := FormatOutput(nil, nil, swatches)
	want := "would copy:                          e-would-copy.md\n" +
		"would overwrite:                     f-would-overwrite.md\n" +
		"would remove:                        a-would-remove.yml\n" +
		"would remove:                        b-would-remove.yml\n" +
		"removed:                             a-removed.yml\n" +
		"no change:                           d-no-change.md\n" +
		"skipped (first-fit, exists):         c-skipped.md\n" +
		"skip (never):                        z-ignored.yml\n"

	if got != want {
		t.Errorf("FormatOutput new category sorting:\ngot:\n%s\nwant:\n%s", got, want)
	}
}

func TestFormatOutputSkipCategories(t *testing.T) {
	repos := []RepoSettingResult{
		{Field: "has_wiki", Category: WouldSet, Value: "false"},
		{Field: "has_issues", Category: RepoNoChange, Value: "true"},
		{Field: "enable private vulnerability reporting", Category: WouldSkipRole, Value: "insufficient role"},
		{Field: "patch repo settings", Category: WouldSkipScope, Value: "insufficient scope"},
	}

	got := FormatOutput(repos, nil, nil)
	want := "would set:                           repository.has_wiki = false\n" +
		"no change:                           repository.has_issues (already true)\n" +
		"would skip (insufficient role):      enable private vulnerability reporting\n" +
		"would skip (insufficient scope):     patch repo settings\n"

	if got != want {
		t.Errorf("FormatOutput skip categories:\ngot:\n%s\nwant:\n%s", got, want)
	}
}

func TestFormatOutputSkipSorting(t *testing.T) {
	repos := []RepoSettingResult{
		{Field: "enable private vulnerability reporting", Category: WouldSkipRole, Value: "role error"},
		{Field: "has_wiki", Category: RepoNoChange, Value: "false"},
		{Field: "description", Category: WouldSet, Value: "My project"},
		{Field: "patch repo settings", Category: WouldSkipScope, Value: "scope error"},
	}

	got := FormatOutput(repos, nil, nil)
	// Order: WouldSet (0), RepoNoChange (1), WouldSkipScope (2), WouldSkipRole (2) - alpha within same order.
	want := "would set:                           repository.description = My project\n" +
		"no change:                           repository.has_wiki (already false)\n" +
		"would skip (insufficient role):      enable private vulnerability reporting\n" +
		"would skip (insufficient scope):     patch repo settings\n"

	if got != want {
		t.Errorf("FormatOutput skip sorting:\ngot:\n%s\nwant:\n%s", got, want)
	}
}

func TestFormatOutputAnnotations(t *testing.T) {
	swatches := []SwatchResult{
		{Path: ".github/workflows/tailor-automerge.yml", Category: WouldCopy, Annotation: "triggered: allow_auto_merge"},
		{Path: "LICENSE", Category: NoChange},
	}

	got := FormatOutput(nil, nil, swatches)
	// Annotated label "would copy (triggered: allow_auto_merge):" is 41 chars,
	// plus 1 space = 42 column width.
	want := "would copy (triggered: allow_auto_merge): .github/workflows/tailor-automerge.yml\n" +
		"no change:                                LICENSE\n"

	if got != want {
		t.Errorf("FormatOutput annotations:\ngot:\n%s\nwant:\n%s", got, want)
	}
}

func TestFormatOutputAnnotationWouldRemove(t *testing.T) {
	swatches := []SwatchResult{
		{Path: ".github/workflows/tailor-automerge.yml", Category: WouldRemove, Annotation: "triggered: allow_auto_merge"},
	}

	got := FormatOutput(nil, nil, swatches)
	want := "would remove (triggered: allow_auto_merge): .github/workflows/tailor-automerge.yml\n"

	if got != want {
		t.Errorf("FormatOutput annotation would remove:\ngot:\n%s\nwant:\n%s", got, want)
	}
}

func TestFormatOutputAnnotationSkippedNever(t *testing.T) {
	swatches := []SwatchResult{
		{Path: ".github/workflows/tailor-automerge.yml", Category: SkippedNever, Annotation: "triggered: allow_auto_merge"},
		{Path: "CONTRIBUTING.md", Category: WouldCopy},
	}

	got := FormatOutput(nil, nil, swatches)
	// "skip (never) (triggered: allow_auto_merge):" = 43 chars + 1 space = 44 width
	want := "would copy:                                 CONTRIBUTING.md\n" +
		"skip (never) (triggered: allow_auto_merge): .github/workflows/tailor-automerge.yml\n"

	if got != want {
		t.Errorf("FormatOutput annotation ignored:\ngot:\n%s\nwant:\n%s", got, want)
	}
}

func TestFormatOutputAnnotationMixedWithRepo(t *testing.T) {
	repos := []RepoSettingResult{
		{Field: "allow_auto_merge", Category: WouldSet, Value: "true"},
	}
	swatches := []SwatchResult{
		{Path: ".github/workflows/tailor-automerge.yml", Category: WouldCopy, Annotation: "triggered: allow_auto_merge"},
	}

	got := FormatOutput(repos, nil, swatches)
	// Column width widens to 42 to fit the annotated swatch label.
	want := "would set:                                repository.allow_auto_merge = true\n" +
		"would copy (triggered: allow_auto_merge): .github/workflows/tailor-automerge.yml\n"

	if got != want {
		t.Errorf("FormatOutput annotation mixed with repo:\ngot:\n%s\nwant:\n%s", got, want)
	}
}

func TestFormatOutputSkipAnnotationScope(t *testing.T) {
	repos := []RepoSettingResult{
		{Field: "vulnerability_alerts_enabled", Category: WouldSkipScope, Annotation: "token missing required scope"},
	}

	got := FormatOutput(repos, nil, nil)
	// "would skip (insufficient scope: token missing required scope):" = 62 chars + 1 space = 63 width.
	want := "would skip (insufficient scope: token missing required scope): vulnerability_alerts_enabled\n"

	if got != want {
		t.Errorf("FormatOutput skip annotation scope:\ngot:\n%s\nwant:\n%s", got, want)
	}
}

func TestFormatOutputSkipAnnotationRole(t *testing.T) {
	repos := []RepoSettingResult{
		{Field: "vulnerability_alerts_enabled", Category: WouldSkipRole, Annotation: "admin required"},
	}

	got := FormatOutput(repos, nil, nil)
	// "would skip (insufficient role: admin required):" = 48 chars + 1 space = 49 width.
	want := "would skip (insufficient role: admin required): vulnerability_alerts_enabled\n"

	if got != want {
		t.Errorf("FormatOutput skip annotation role:\ngot:\n%s\nwant:\n%s", got, want)
	}
}

func TestFormatOutputSkipAnnotationMixed(t *testing.T) {
	repos := []RepoSettingResult{
		{Field: "has_wiki", Category: WouldSet, Value: "false"},
		{Field: "has_issues", Category: RepoNoChange, Value: "true"},
		{Field: "vulnerability_alerts_enabled", Category: WouldSkipScope, Annotation: "token missing required scope"},
		{Field: "private_vulnerability_reporting_enabled", Category: WouldSkipRole, Annotation: "admin required"},
	}

	got := FormatOutput(repos, nil, nil)
	// Widest label is "would skip (insufficient scope: token missing required scope):" = 62 chars + 1 = 63.
	want := "would set:                                                     repository.has_wiki = false\n" +
		"no change:                                                     repository.has_issues (already true)\n" +
		"would skip (insufficient role: admin required):                private_vulnerability_reporting_enabled\n" +
		"would skip (insufficient scope: token missing required scope): vulnerability_alerts_enabled\n"

	if got != want {
		t.Errorf("FormatOutput skip annotation mixed:\ngot:\n%s\nwant:\n%s", got, want)
	}
}

func TestFormatOutputLabelSkipAnnotations(t *testing.T) {
	labels := []LabelResult{
		{Name: "bug", Category: WouldCreate, Value: "#d73a4a"},
		{Name: "enhancement", Category: LabelSkipScope, Annotation: "token missing required scope"},
	}

	got := FormatOutput(nil, labels, nil)
	// Widest label is "would skip (insufficient scope: token missing required scope):" = 62 + 1 = 63.
	want := "would create:                                                  label.bug = #d73a4a\n" +
		"would skip (insufficient scope: token missing required scope): enhancement\n"

	if got != want {
		t.Errorf("FormatOutput label skip annotations:\ngot:\n%s\nwant:\n%s", got, want)
	}
}

func TestFormatOutputSkipAnnotationColumnWidth(t *testing.T) {
	// Verify annotated skip labels widen the column correctly.
	repos := []RepoSettingResult{
		{Field: "vuln", Category: WouldSkipScope, Annotation: "token missing required scope"},
	}
	got := FormatOutput(repos, nil, nil)

	// "would skip (insufficient scope: token missing required scope):" is 62 chars.
	// Column width = 63 (62 + 1 space). The field starts at position 63.
	label := "would skip (insufficient scope: token missing required scope): "
	if len(label) != 63 {
		t.Fatalf("expected label+space to be 63 chars, got %d", len(label))
	}
	want := label + "vuln\n"
	if got != want {
		t.Errorf("FormatOutput skip annotation column width:\ngot:\n%s\nwant:\n%s", got, want)
	}
}

func TestFormatOutputSkipWithoutAnnotation(t *testing.T) {
	// Skip results without annotations still render with the base label.
	repos := []RepoSettingResult{
		{Field: "patch repo settings", Category: WouldSkipScope},
	}

	got := FormatOutput(repos, nil, nil)
	want := "would skip (insufficient scope):     patch repo settings\n"

	if got != want {
		t.Errorf("FormatOutput skip without annotation:\ngot:\n%s\nwant:\n%s", got, want)
	}
}
