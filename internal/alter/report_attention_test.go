package alter

import "testing"

func TestBuildReportDryRunAttentionGuidance(t *testing.T) {
	const attentionGuidance = "No changes made. Resolve the items that need attention, then run `tailor baste` again."
	for _, tt := range []struct {
		name      string
		repo      []RepoSettingResult
		labels    []LabelResult
		variables []VariableResult
		attention int
		guidance  string
	}{
		{
			name:      "repository scope",
			repo:      []RepoSettingResult{{Field: "has_wiki", Category: WouldSkipScope, Annotation: "scope"}},
			attention: 1,
			guidance:  attentionGuidance,
		},
		{
			name:      "repository setup",
			repo:      []RepoSettingResult{{Section: "code_scanning", Field: "state", Category: WouldSkipSetup, Annotation: "not available"}},
			attention: 1,
			guidance:  attentionGuidance,
		},
		{
			name:      "label scope",
			labels:    []LabelResult{{Name: "bug", Category: LabelSkipScope, Annotation: "scope"}},
			attention: 1,
			guidance:  attentionGuidance,
		},
		{
			name:      "variable scope",
			variables: []VariableResult{{Name: "VERSION", Category: LabelSkipScope, Annotation: "scope"}},
			attention: 1,
			guidance:  attentionGuidance,
		},
		{
			name:     "owner policy",
			repo:     []RepoSettingResult{{Section: "immutable_releases", Field: "enabled", Category: WouldSkipSetup, Annotation: "enforced by owner"}},
			guidance: "No changes needed.",
		},
		{
			name:     "matching",
			repo:     []RepoSettingResult{{Field: "has_wiki", Category: RepoNoChange, Value: "true"}},
			guidance: "No changes needed.",
		},
		{
			name:     "empty",
			guidance: "No changes needed.",
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			report := buildReport("baste", "owner/repo", tt.repo, tt.labels, tt.variables, nil, DryRun)
			if got := report.Document.Summary; got.Alterations != 0 || got.Attention != tt.attention {
				t.Fatalf("summary = %#v, want zero alterations and %d attention", got, tt.attention)
			}
			if got := report.Document.Guidance; len(got) != 1 || got[0].Text != tt.guidance || got[0].Order != 1000 {
				t.Errorf("guidance = %#v, want %q at order 1000", got, tt.guidance)
			}
			if want := FormatOutput(tt.repo, tt.labels, tt.variables, nil, DryRun); report.Plain != want {
				t.Errorf("plain output = %q, want legacy output %q", report.Plain, want)
			}
		})
	}
}
