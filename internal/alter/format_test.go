package alter

import (
	"fmt"
	"testing"
)

func TestFormatOutputSwatchesOnly(t *testing.T) {
	swatches := []SwatchResult{
		{Destination: ".github/FUNDING.yml", Category: WouldOverwrite},
		{Destination: "CONTRIBUTING.md", Category: WouldCopy},
		{Destination: "LICENSE", Category: NoChange},
		{Destination: ".tailor/config.yml", Category: SkippedFirstFit},
	}

	got := FormatOutput(nil, swatches)
	want := "would copy:                  CONTRIBUTING.md\n" +
		"would overwrite:             .github/FUNDING.yml\n" +
		"no change:                   LICENSE\n" +
		"skipped (first-fit, exists): .tailor/config.yml\n"

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

	got := FormatOutput(repos, nil)
	want := "would set:                   repository.description = My project\n" +
		"would set:                   repository.has_wiki = false\n" +
		"no change:                   repository.has_issues (already true)\n"

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
		{Destination: "CONTRIBUTING.md", Category: WouldCopy},
		{Destination: "LICENSE", Category: NoChange},
	}

	got := FormatOutput(repos, swatches)
	want := "would set:                   repository.has_wiki = false\n" +
		"no change:                   repository.has_issues (already true)\n" +
		"would copy:                  CONTRIBUTING.md\n" +
		"no change:                   LICENSE\n"

	if got != want {
		t.Errorf("FormatOutput combined:\ngot:\n%s\nwant:\n%s", got, want)
	}
}

func TestFormatOutputEmpty(t *testing.T) {
	got := FormatOutput(nil, nil)
	if got != "" {
		t.Errorf("FormatOutput empty: got %q, want %q", got, "")
	}
}

func TestFormatOutputEmptySlices(t *testing.T) {
	got := FormatOutput([]RepoSettingResult{}, []SwatchResult{})
	if got != "" {
		t.Errorf("FormatOutput empty slices: got %q, want %q", got, "")
	}
}

func TestFormatOutputSwatchSorting(t *testing.T) {
	swatches := []SwatchResult{
		{Destination: "Z-file.md", Category: NoChange},
		{Destination: "A-file.md", Category: SkippedFirstFit},
		{Destination: "B-file.md", Category: WouldCopy},
		{Destination: "A-file.md", Category: WouldOverwrite},
		{Destination: "C-file.md", Category: WouldCopy},
		{Destination: "M-file.md", Category: NoChange},
	}

	got := FormatOutput(nil, swatches)
	want := "would copy:                  B-file.md\n" +
		"would copy:                  C-file.md\n" +
		"would overwrite:             A-file.md\n" +
		"no change:                   M-file.md\n" +
		"no change:                   Z-file.md\n" +
		"skipped (first-fit, exists): A-file.md\n"

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

	got := FormatOutput(repos, nil)
	want := "would set:                   repository.allow_squash_merge = true\n" +
		"would set:                   repository.has_issues = true\n" +
		"no change:                   repository.description (already A project)\n" +
		"no change:                   repository.has_wiki (already false)\n"

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
		{Destination: "info1.md", Category: NoChange},
		{Destination: "info2.md", Category: SkippedFirstFit},
		{Destination: "action1.md", Category: WouldCopy},
		{Destination: "action2.md", Category: WouldOverwrite},
	}

	got := FormatOutput(nil, swatches)
	want := "would copy:                  action1.md\n" +
		"would overwrite:             action2.md\n" +
		"no change:                   info1.md\n" +
		"skipped (first-fit, exists): info2.md\n"

	if got != want {
		t.Errorf("FormatOutput actionable before informational:\ngot:\n%s\nwant:\n%s", got, want)
	}
}

func TestFormatOutputRepoSettingsBeforeSwatches(t *testing.T) {
	repos := []RepoSettingResult{
		{Field: "has_wiki", Category: WouldSet, Value: "false"},
	}
	swatches := []SwatchResult{
		{Destination: "CONTRIBUTING.md", Category: WouldCopy},
	}

	got := FormatOutput(repos, swatches)

	// Repo settings line must appear before swatch line.
	repoIdx := 0
	swatchIdx := len("would set:                   repository.has_wiki = false\n")
	if got[:swatchIdx] != "would set:                   repository.has_wiki = false\n" {
		t.Errorf("repo settings not first in output:\ngot:\n%s", got)
	}
	_ = repoIdx
}

func TestFormatOutputNoTrailingBlankLine(t *testing.T) {
	swatches := []SwatchResult{
		{Destination: "file.md", Category: WouldCopy},
	}

	got := FormatOutput(nil, swatches)
	if got[len(got)-1] != '\n' {
		t.Error("output should end with newline")
	}
	if len(got) > 1 && got[len(got)-2] == '\n' {
		t.Error("output should not have trailing blank line")
	}
}

func TestFormatOutputNewCategories(t *testing.T) {
	swatches := []SwatchResult{
		{Destination: "removed.yml", Category: Removed},
		{Destination: "ignored.yml", Category: SkippedNever},
		{Destination: "would-remove.yml", Category: WouldRemove},
		{Destination: "copied.md", Category: WouldCopy},
	}

	got := FormatOutput(nil, swatches)
	want := "would copy:                  copied.md\n" +
		"would remove:                would-remove.yml\n" +
		"removed:                     removed.yml\n" +
		"skip (never):                ignored.yml\n"

	if got != want {
		t.Errorf("FormatOutput new categories:\ngot:\n%s\nwant:\n%s", got, want)
	}
}

func TestFormatOutputNewCategorySorting(t *testing.T) {
	swatches := []SwatchResult{
		{Destination: "z-ignored.yml", Category: SkippedNever},
		{Destination: "a-removed.yml", Category: Removed},
		{Destination: "b-would-remove.yml", Category: WouldRemove},
		{Destination: "c-skipped.md", Category: SkippedFirstFit},
		{Destination: "d-no-change.md", Category: NoChange},
		{Destination: "e-would-copy.md", Category: WouldCopy},
		{Destination: "f-would-overwrite.md", Category: WouldOverwrite},
		{Destination: "a-would-remove.yml", Category: WouldRemove},
	}

	got := FormatOutput(nil, swatches)
	want := "would copy:                  e-would-copy.md\n" +
		"would overwrite:             f-would-overwrite.md\n" +
		"would remove:                a-would-remove.yml\n" +
		"would remove:                b-would-remove.yml\n" +
		"removed:                     a-removed.yml\n" +
		"no change:                   d-no-change.md\n" +
		"skipped (first-fit, exists): c-skipped.md\n" +
		"skip (never):                z-ignored.yml\n"

	if got != want {
		t.Errorf("FormatOutput new category sorting:\ngot:\n%s\nwant:\n%s", got, want)
	}
}

func TestFormatOutputAnnotations(t *testing.T) {
	swatches := []SwatchResult{
		{Destination: ".github/workflows/tailor-automerge.yml", Category: WouldCopy, Annotation: "triggered: allow_auto_merge"},
		{Destination: "LICENSE", Category: NoChange},
	}

	got := FormatOutput(nil, swatches)
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
		{Destination: ".github/workflows/tailor-automerge.yml", Category: WouldRemove, Annotation: "triggered: allow_auto_merge"},
	}

	got := FormatOutput(nil, swatches)
	want := "would remove (triggered: allow_auto_merge): .github/workflows/tailor-automerge.yml\n"

	if got != want {
		t.Errorf("FormatOutput annotation would remove:\ngot:\n%s\nwant:\n%s", got, want)
	}
}

func TestFormatOutputAnnotationSkippedNever(t *testing.T) {
	swatches := []SwatchResult{
		{Destination: ".github/workflows/tailor-automerge.yml", Category: SkippedNever, Annotation: "triggered: allow_auto_merge"},
		{Destination: "CONTRIBUTING.md", Category: WouldCopy},
	}

	got := FormatOutput(nil, swatches)
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
		{Destination: ".github/workflows/tailor-automerge.yml", Category: WouldCopy, Annotation: "triggered: allow_auto_merge"},
	}

	got := FormatOutput(repos, swatches)
	// Column width widens to 42 to fit the annotated swatch label.
	want := "would set:                                repository.allow_auto_merge = true\n" +
		"would copy (triggered: allow_auto_merge): .github/workflows/tailor-automerge.yml\n"

	if got != want {
		t.Errorf("FormatOutput annotation mixed with repo:\ngot:\n%s\nwant:\n%s", got, want)
	}
}
