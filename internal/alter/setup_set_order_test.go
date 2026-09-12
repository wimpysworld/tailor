package alter

import (
	"reflect"
	"slices"
	"testing"
)

func TestResultComparerSetDisplayOrder(t *testing.T) {
	for _, tt := range []struct {
		name     string
		declared *[]string
		live     *[]string
		category RepoSettingCategory
		value    string
		before   string
	}{
		{name: "omitted", live: &[]string{"squash"}},
		{name: "equal reordered", declared: &[]string{"squash", "merge"}, live: &[]string{"merge", "squash"}, category: RepoNoChange, value: "squash, merge", before: "squash, merge"},
		{name: "equal ordered", declared: &[]string{"squash", "merge"}, live: &[]string{"squash", "merge"}, category: RepoNoChange, value: "squash, merge", before: "squash, merge"},
		{name: "different", declared: &[]string{"squash", "merge"}, live: &[]string{"rebase", "squash"}, category: WouldSet, value: "squash, merge", before: "rebase, squash"},
		{name: "unavailable", declared: &[]string{"squash", "merge"}, category: WouldSet, value: "squash, merge"},
		{name: "known empty", declared: &[]string{"squash"}, live: &[]string{}, category: WouldSet, value: "squash", before: "(none)"},
		{name: "clear", declared: &[]string{}, live: &[]string{"squash"}, category: WouldSet, value: "(none)", before: "squash"},
		{name: "equal empty", declared: &[]string{}, live: new([]string), category: RepoNoChange, value: "(none)", before: "(none)"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			var declared, live []string
			if tt.declared != nil {
				declared = slices.Clone(*tt.declared)
			}
			if tt.live != nil {
				live = slices.Clone(*tt.live)
			}
			comparer := resultComparer{section: "ruleset"}
			comparer.set("allowed_merge_methods", tt.declared, tt.live)
			if tt.declared == nil {
				if len(comparer.results) != 0 {
					t.Fatalf("omitted set produced results: %#v", comparer.results)
				}
				return
			}
			want := []RepoSettingResult{{Section: "ruleset", Field: "allowed_merge_methods", Category: tt.category, Value: tt.value, Before: tt.before}}
			if !reflect.DeepEqual(comparer.results, want) {
				t.Errorf("results = %#v, want %#v", comparer.results, want)
			}
			if !slices.Equal(*tt.declared, declared) || (tt.live != nil && !slices.Equal(*tt.live, live)) {
				t.Error("set comparison changed an input list")
			}
			if tt.name == "equal reordered" {
				const plain = "no change:                           ruleset.allowed_merge_methods (already squash, merge)\n"
				for _, mode := range []ApplyMode{DryRun, Apply, Recut} {
					if got := FormatOutput(comparer.results, nil, nil, nil, mode); got != plain {
						t.Errorf("plain output = %q, want %q", got, plain)
					}
				}
			}
		})
	}
}
