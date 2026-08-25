package alter

import (
	"fmt"
	"strings"
	"testing"
	"unicode"

	"github.com/wimpysworld/tailor/internal/gh"
)

func TestFormatOutputSwatchesOnly(t *testing.T) {
	swatches := []SwatchResult{
		{Path: ".github/FUNDING.yml", Category: WouldOverwrite},
		{Path: "CONTRIBUTING.md", Category: WouldCopy},
		{Path: "LICENSE", Category: NoChange},
		{Path: ".tailor.yml", Category: Skipped, Reason: SkipFirstFitExists},
	}

	got := FormatOutput(nil, nil, swatches, DryRun)
	want := "would copy:                          CONTRIBUTING.md\n" +
		"would overwrite:                     .github/FUNDING.yml\n" +
		"no change:                           LICENSE\n" +
		"skipped:                             .tailor.yml (first-fit, exists)\n"

	if got != want {
		t.Errorf("FormatOutput swatches only:\ngot:\n%s\nwant:\n%s", got, want)
	}
}

func TestFormatOutputSkippedSwatchesAllModes(t *testing.T) {
	swatches := []SwatchResult{
		{Path: ".github/pull_request_template.md", Category: Skipped, Reason: SkipModeNever},
		{Path: ".envrc", Category: Skipped, Reason: SkipFirstFitExists},
	}
	want := "skipped:                             .envrc (first-fit, exists)\n" +
		"skipped:                             .github/pull_request_template.md (mode never)\n"

	for _, mode := range []ApplyMode{DryRun, Apply, Recut} {
		t.Run(fmt.Sprint(mode), func(t *testing.T) {
			if got := FormatOutput(nil, nil, swatches, mode); got != want {
				t.Errorf("FormatOutput() =\n%s\nwant:\n%s", got, want)
			}
		})
	}
}

func TestFormatOutputRepoSettingsOnly(t *testing.T) {
	repos := []RepoSettingResult{
		{Field: "has_wiki", Category: WouldSet, Value: "false"},
		{Field: "has_issues", Category: RepoNoChange, Value: "true"},
		{Field: "description", Category: WouldSet, Value: "My project"},
	}

	got := FormatOutput(repos, nil, nil, DryRun)
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

	got := FormatOutput(repos, nil, swatches, DryRun)
	want := "would set:                           repository.has_wiki = false\n" +
		"no change:                           repository.has_issues (already true)\n" +
		"would copy:                          CONTRIBUTING.md\n" +
		"no change:                           LICENSE\n"

	if got != want {
		t.Errorf("FormatOutput combined:\ngot:\n%s\nwant:\n%s", got, want)
	}
}

func TestFormatOutputEmpty(t *testing.T) {
	got := FormatOutput(nil, nil, nil, DryRun)
	if got != "" {
		t.Errorf("FormatOutput empty: got %q, want %q", got, "")
	}
}

func TestFormatOutputEmptySlices(t *testing.T) {
	got := FormatOutput([]RepoSettingResult{}, nil, []SwatchResult{}, DryRun)
	if got != "" {
		t.Errorf("FormatOutput empty slices: got %q, want %q", got, "")
	}
}

func TestFormatOutputSwatchSorting(t *testing.T) {
	swatches := []SwatchResult{
		{Path: "Z-file.md", Category: NoChange},
		{Path: "A-file.md", Category: Skipped, Reason: SkipFirstFitExists},
		{Path: "B-file.md", Category: WouldCopy},
		{Path: "A-file.md", Category: WouldOverwrite},
		{Path: "C-file.md", Category: WouldCopy},
		{Path: "M-file.md", Category: NoChange},
	}

	got := FormatOutput(nil, nil, swatches, DryRun)
	want := "would copy:                          B-file.md\n" +
		"would copy:                          C-file.md\n" +
		"would overwrite:                     A-file.md\n" +
		"no change:                           M-file.md\n" +
		"no change:                           Z-file.md\n" +
		"skipped:                             A-file.md (first-fit, exists)\n"

	if got != want {
		t.Errorf("FormatOutput swatch sorting:\ngot:\n%s\nwant:\n%s", got, want)
	}
}

func TestFormatOutputMixedSwatchActionOrder(t *testing.T) {
	swatches := []SwatchResult{
		{Path: "existing.md", Category: WouldOverwrite},
		{Path: "new.md", Category: WouldCopy},
		{Path: ".github/workflows/tailor.yml", Category: WouldRemove},
		{Path: ".tailor.yml", Category: WouldUpdateConfig},
	}

	got := FormatOutput(nil, nil, swatches, DryRun)
	want := "would update:                        .tailor.yml\n" +
		"would remove:                        .github/workflows/tailor.yml\n" +
		"would copy:                          new.md\n" +
		"would overwrite:                     existing.md\n"

	if got != want {
		t.Errorf("FormatOutput mixed swatch actions:\ngot:\n%s\nwant:\n%s", got, want)
	}
}

func TestFormatOutputRetiredWorkflowRemovalByMode(t *testing.T) {
	swatches := []SwatchResult{
		{Path: ".github/workflows/tailor.yml", Category: WouldRemove},
		{Path: ".github/workflows/tailor-automerge.yml", Category: WouldRemove},
	}
	tests := []struct {
		name string
		mode ApplyMode
		want string
	}{
		{
			name: "dry run",
			mode: DryRun,
			want: "would remove:                        .github/workflows/tailor-automerge.yml\n" +
				"would remove:                        .github/workflows/tailor.yml\n",
		},
		{
			name: "apply",
			mode: Apply,
			want: "removed:                             .github/workflows/tailor-automerge.yml\n" +
				"removed:                             .github/workflows/tailor.yml\n",
		},
		{
			name: "recut",
			mode: Recut,
			want: "removed:                             .github/workflows/tailor-automerge.yml\n" +
				"removed:                             .github/workflows/tailor.yml\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := FormatOutput(nil, nil, swatches, tt.mode); got != tt.want {
				t.Errorf("FormatOutput() =\n%s\nwant:\n%s", got, tt.want)
			}
		})
	}
}

func TestFormatOutputRepoSettingSorting(t *testing.T) {
	repos := []RepoSettingResult{
		{Field: "has_wiki", Category: RepoNoChange, Value: "false"},
		{Field: "has_issues", Category: WouldSet, Value: "true"},
		{Field: "description", Category: RepoNoChange, Value: "A project"},
		{Field: "allow_squash_merge", Category: WouldSet, Value: "true"},
	}

	got := FormatOutput(repos, nil, nil, DryRun)
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
		"skipped:",
		"would set:",
		"would skip (insufficient scope):",
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
		{Path: "info2.md", Category: Skipped, Reason: SkipFirstFitExists},
		{Path: "action1.md", Category: WouldCopy},
		{Path: "action2.md", Category: WouldOverwrite},
	}

	got := FormatOutput(nil, nil, swatches, DryRun)
	want := "would copy:                          action1.md\n" +
		"would overwrite:                     action2.md\n" +
		"no change:                           info1.md\n" +
		"skipped:                             info2.md (first-fit, exists)\n"

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

	got := FormatOutput(repos, nil, swatches, DryRun)

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

	got := FormatOutput(nil, nil, swatches, DryRun)
	if got[len(got)-1] != '\n' {
		t.Error("output should end with newline")
	}
	if len(got) > 1 && got[len(got)-2] == '\n' {
		t.Error("output should not have trailing blank line")
	}
}

func TestFormatOutputSkipCategories(t *testing.T) {
	repos := []RepoSettingResult{
		{Field: "has_wiki", Category: WouldSet, Value: "false"},
		{Field: "has_issues", Category: RepoNoChange, Value: "true"},
		{Operation: gh.Op(gh.OpPatchRepoSettings), Category: WouldSkipScope, Value: "insufficient scope"},
	}

	got := FormatOutput(repos, nil, nil, DryRun)
	want := "would set:                           repository.has_wiki = false\n" +
		"no change:                           repository.has_issues (already true)\n" +
		"would skip (insufficient scope):     patch repo settings\n"

	if got != want {
		t.Errorf("FormatOutput skip categories:\ngot:\n%s\nwant:\n%s", got, want)
	}
}

func TestFormatOutputActionsSkipOperationHasNoSectionPrefix(t *testing.T) {
	repos := []RepoSettingResult{
		{Section: "actions", Field: "enabled", Category: WouldSkipScope},
		{Section: "actions", Operation: gh.Op(gh.OpDisableActionsForPolicyUpdate), Category: WouldSkipScope},
	}

	got := FormatOutput(repos, nil, nil, DryRun)
	want := "would skip (insufficient scope):     disable actions for selected policy update\n" +
		"would skip (insufficient scope):     actions.enabled\n"
	if got != want {
		t.Errorf("FormatOutput actions skip operations:\ngot:\n%s\nwant:\n%s", got, want)
	}
}

func TestFormatOutputSkipSorting(t *testing.T) {
	repos := []RepoSettingResult{
		{Field: "has_wiki", Category: RepoNoChange, Value: "false"},
		{Field: "description", Category: WouldSet, Value: "My project"},
		{Operation: gh.Op(gh.OpPatchRepoSettings), Category: WouldSkipScope, Value: "scope error"},
	}

	got := FormatOutput(repos, nil, nil, DryRun)
	want := "would set:                           repository.description = My project\n" +
		"no change:                           repository.has_wiki (already false)\n" +
		"would skip (insufficient scope):     patch repo settings\n"

	if got != want {
		t.Errorf("FormatOutput skip sorting:\ngot:\n%s\nwant:\n%s", got, want)
	}
}

func TestFormatOutputSkipAnnotationScope(t *testing.T) {
	repos := []RepoSettingResult{
		{Field: "default_workflow_permissions", Category: WouldSkipScope, Annotation: "token missing required scope"},
	}

	got := FormatOutput(repos, nil, nil, DryRun)
	// "would skip (insufficient scope: token missing required scope):" = 62 chars + 1 space = 63 width.
	want := "would skip (insufficient scope: token missing required scope): default_workflow_permissions\n"

	if got != want {
		t.Errorf("FormatOutput skip annotation scope:\ngot:\n%s\nwant:\n%s", got, want)
	}
}

func TestFormatOutputSkipAnnotationMixed(t *testing.T) {
	repos := []RepoSettingResult{
		{Field: "has_wiki", Category: WouldSet, Value: "false"},
		{Field: "has_issues", Category: RepoNoChange, Value: "true"},
		{Field: "default_workflow_permissions", Category: WouldSkipScope, Annotation: "token missing required scope"},
		{Field: "can_approve_pull_request_reviews", Category: WouldSkipScope, Annotation: "token missing required scope"},
	}

	got := FormatOutput(repos, nil, nil, DryRun)
	// Widest label is "would skip (insufficient scope: token missing required scope):" = 62 chars + 1 = 63.
	want := "would set:                                                     repository.has_wiki = false\n" +
		"no change:                                                     repository.has_issues (already true)\n" +
		"would skip (insufficient scope: token missing required scope): can_approve_pull_request_reviews\n" +
		"would skip (insufficient scope: token missing required scope): default_workflow_permissions\n"

	if got != want {
		t.Errorf("FormatOutput skip annotation mixed:\ngot:\n%s\nwant:\n%s", got, want)
	}
}

func TestFormatOutputDynamicWidthWithSkippedSwatches(t *testing.T) {
	repos := []RepoSettingResult{
		{Field: "default_workflow_permissions", Category: WouldSkipScope, Annotation: "token missing required scope"},
	}
	swatches := []SwatchResult{
		{Path: ".github/pull_request_template.md", Category: Skipped, Reason: SkipModeNever},
		{Path: ".envrc", Category: Skipped, Reason: SkipFirstFitExists},
	}

	got := FormatOutput(repos, nil, swatches, DryRun)
	want := "would skip (insufficient scope: token missing required scope): default_workflow_permissions\n" +
		"skipped:                                                       .envrc (first-fit, exists)\n" +
		"skipped:                                                       .github/pull_request_template.md (mode never)\n"

	if got != want {
		t.Errorf("FormatOutput dynamic width with skipped swatches:\ngot:\n%s\nwant:\n%s", got, want)
	}
}

func TestFormatOutputLabelSkipAnnotations(t *testing.T) {
	labels := []LabelResult{
		{Name: "bug", Category: WouldCreate, Value: "#d73a4a"},
		{Operation: gh.CreateLabelOp("enhancement"), Category: LabelSkipScope, Annotation: "token missing required scope"},
	}

	got := FormatOutput(nil, labels, nil, DryRun)
	// Widest label is "would skip (insufficient scope: token missing required scope):" = 62 + 1 = 63.
	want := "would create:                                                  label.bug = #d73a4a\n" +
		"would skip (insufficient scope: token missing required scope): create label \"enhancement\"\n"

	if got != want {
		t.Errorf("FormatOutput label skip annotations:\ngot:\n%s\nwant:\n%s", got, want)
	}
}

func TestFormatOutputSkipAnnotationColumnWidth(t *testing.T) {
	// Annotated skip labels widen the column correctly.
	repos := []RepoSettingResult{
		{Field: "vuln", Category: WouldSkipScope, Annotation: "token missing required scope"},
	}
	got := FormatOutput(repos, nil, nil, DryRun)

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
		{Operation: gh.Op(gh.OpPatchRepoSettings), Category: WouldSkipScope},
	}

	got := FormatOutput(repos, nil, nil, DryRun)
	want := "would skip (insufficient scope):     patch repo settings\n"

	if got != want {
		t.Errorf("FormatOutput skip without annotation:\ngot:\n%s\nwant:\n%s", got, want)
	}
}

func TestFormatOutputApplyModes(t *testing.T) {
	repos := []RepoSettingResult{
		{Field: "has_wiki", Category: WouldSet, Value: "false"},
	}
	labels := []LabelResult{
		{Name: "bug", Category: WouldCreate, Value: "#d73a4a \"A problem\""},
		{Name: "docs", Category: WouldUpdate, Value: "#0075ca"},
	}
	swatches := []SwatchResult{
		{Path: "LICENSE", Category: WouldCopy},
		{Path: "CONTRIBUTING.md", Category: WouldOverwrite},
		{Path: ".tailor.yml", Category: WouldUpdateConfig},
	}

	tests := []struct {
		name string
		mode ApplyMode
		want string
	}{
		{
			name: "dry run",
			mode: DryRun,
			want: "would set:                           repository.has_wiki = false\n" +
				"would create:                        label.bug = #d73a4a \"A problem\"\n" +
				"would update:                        label.docs = #0075ca\n" +
				"would update:                        .tailor.yml\n" +
				"would copy:                          LICENSE\n" +
				"would overwrite:                     CONTRIBUTING.md\n",
		},
		{
			name: "apply",
			mode: Apply,
			want: "set:                                 repository.has_wiki = false\n" +
				"created:                             label.bug = #d73a4a \"A problem\"\n" +
				"updated:                             label.docs = #0075ca\n" +
				"updated:                             .tailor.yml\n" +
				"copied:                              LICENSE\n" +
				"overwritten:                         CONTRIBUTING.md\n",
		},
		{
			name: "recut",
			mode: Recut,
			want: "set:                                 repository.has_wiki = false\n" +
				"created:                             label.bug = #d73a4a \"A problem\"\n" +
				"updated:                             label.docs = #0075ca\n" +
				"updated:                             .tailor.yml\n" +
				"copied:                              LICENSE\n" +
				"overwritten:                         CONTRIBUTING.md\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := FormatOutput(repos, labels, swatches, tt.mode)
			if got != tt.want {
				t.Errorf("FormatOutput() =\n%s\nwant:\n%s", got, tt.want)
			}
		})
	}
}

func TestFormatOutputWriteModesOmitSkippedActions(t *testing.T) {
	repos := []RepoSettingResult{
		{Field: "topics", Category: WouldSet, Value: "go, cli"},
		{Operation: gh.Op(gh.OpSetTopics), Category: WouldSkipScope, Annotation: "token missing required scope"},
	}
	labels := []LabelResult{
		{Name: "bug", Category: WouldCreate, Value: "#d73a4a"},
		{Operation: gh.CreateLabelOp("bug"), Category: LabelSkipScope, Annotation: "token missing required scope"},
	}
	want := "would skip (insufficient scope: token missing required scope): set topics\n" +
		"would skip (insufficient scope: token missing required scope): create label \"bug\"\n"

	for _, mode := range []ApplyMode{Apply, Recut} {
		t.Run(fmt.Sprint(mode), func(t *testing.T) {
			got := FormatOutput(repos, labels, nil, mode)
			if got != want {
				t.Errorf("FormatOutput() =\n%s\nwant:\n%s", got, want)
			}
		})
	}
}

func TestFormatOutputWriteModesOmitSkippedSecuritySettings(t *testing.T) {
	repos := []RepoSettingResult{
		{Field: "private_vulnerability_reporting_enabled", Category: WouldSet, Value: "true"},
		{Operation: gh.SecurityFeatureOp(true, gh.OpSetPrivateVulnerabilityReporting), Category: WouldSkipScope, Annotation: "token missing required scope"},
		{Field: "vulnerability_alerts_enabled", Category: WouldSet, Value: "false"},
		{Operation: gh.SecurityFeatureOp(false, gh.OpSetVulnerabilityAlerts), Category: WouldSkipScope, Annotation: "token missing required scope"},
		{Field: "automated_security_fixes_enabled", Category: WouldSet, Value: "true"},
		{Operation: gh.SecurityFeatureOp(true, gh.OpSetAutomatedSecurityFixes), Category: WouldSkipScope, Annotation: "token missing required scope"},
	}
	want := "would skip (insufficient scope: token missing required scope): disable vulnerability alerts\n" +
		"would skip (insufficient scope: token missing required scope): enable automated security fixes\n" +
		"would skip (insufficient scope: token missing required scope): enable private vulnerability reporting\n"

	for _, mode := range []ApplyMode{Apply, Recut} {
		t.Run(fmt.Sprint(mode), func(t *testing.T) {
			if got := FormatOutput(repos, nil, nil, mode); got != want {
				t.Errorf("FormatOutput() =\n%s\nwant:\n%s", got, want)
			}
		})
	}
}

func TestFormatOutputEscapesControlCharacters(t *testing.T) {
	tests := []struct {
		name         string
		repos        []RepoSettingResult
		labels       []LabelResult
		wantContains string
	}{
		{
			name:         "ANSI CSI in repository description",
			repos:        []RepoSettingResult{{Field: "description", Category: WouldSet, Value: "\x1b[31mred\x1b[0m"}},
			wantContains: `repository.description = \x1b[31mred\x1b[0m`,
		},
		{
			name:         "OSC 8 hyperlink in label name",
			labels:       []LabelResult{{Name: "\x1b]8;;https://evil.example\x07bug", Category: WouldCreate, Value: "#d73a4a"}},
			wantContains: `label.\x1b]8;;https://evil.example\x07bug = #d73a4a`,
		},
		{
			name:         "OSC 52 clipboard write in label name",
			labels:       []LabelResult{{Name: "\x1b]52;c;Zm9v\x07bug", Category: WouldCreate, Value: "#d73a4a"}},
			wantContains: `label.\x1b]52;c;Zm9v\x07bug = #d73a4a`,
		},
		{
			name:         "carriage return in repository description",
			repos:        []RepoSettingResult{{Field: "description", Category: WouldSet, Value: "safe\rspoofed"}},
			wantContains: `repository.description = safe\x0dspoofed`,
		},
		{
			name:         "line feed in repository description",
			repos:        []RepoSettingResult{{Field: "description", Category: WouldSet, Value: "safe\nwould set: injected"}},
			wantContains: `repository.description = safe\x0awould set: injected`,
		},
		{
			name:         "C1 CSI in label name",
			labels:       []LabelResult{{Name: "\u009b31mbug", Category: WouldCreate, Value: "#d73a4a"}},
			wantContains: `label.\u009b31mbug = #d73a4a`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := FormatOutput(tt.repos, tt.labels, nil, DryRun)
			if !strings.Contains(got, tt.wantContains) {
				t.Errorf("FormatOutput() =\n%s\nwant substring %q", got, tt.wantContains)
			}
			for _, r := range got {
				if r != '\n' && unicode.IsControl(r) {
					t.Errorf("FormatOutput() contains raw control character %U:\n%q", r, got)
				}
			}
		})
	}
}
