package alter

import (
	"reflect"
	"testing"
)

func TestResultComparerLanguagesBefore(t *testing.T) {
	for _, tt := range []struct {
		name   string
		live   *[]string
		before string
	}{
		{name: "unavailable"},
		{name: "known empty", live: &[]string{}, before: "(none)"},
		{name: "known nil slice", live: new([]string), before: "(none)"},
		{name: "sorted languages", live: &[]string{"ruby", "go"}, before: "go, ruby"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			comparer := resultComparer{section: "code_scanning"}
			declared := []string{"python", "go"}
			comparer.languages(&declared, tt.live)
			want := []RepoSettingResult{{Section: "code_scanning", Field: "languages", Category: WouldSet, Value: "go, python", Before: tt.before}}
			if !reflect.DeepEqual(comparer.results, want) {
				t.Errorf("results = %#v, want %#v", comparer.results, want)
			}
		})
	}
}
