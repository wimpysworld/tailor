package swatch_test

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/wimpysworld/tailor/internal/swatch"
)

func TestJavaScriptStarterContents(t *testing.T) {
	for name, want := range map[string]string{
		".oxlintrc.json":   `{"plugins":["typescript","unicorn","oxc"],"categories":{"correctness":"error"}}`,
		".prettierrc.json": `{"$schema":"https://www.schemastore.org/prettierrc.json","semi":true,"singleQuote":false,"trailingComma":"all","bracketSpacing":true,"arrowParens":"always"}`,
	} {
		body, err := swatch.Content(name)
		if err != nil {
			t.Fatal(err)
		}
		var got, expected map[string]any
		if err := json.Unmarshal(body, &got); err != nil {
			t.Fatal(err)
		}
		if err := json.Unmarshal([]byte(want), &expected); err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(got, expected) {
			t.Fatalf("%s = %s", name, body)
		}
		if name == ".prettierrc.json" {
			for _, key := range []string{"printWidth", "tabWidth", "useTabs", "endOfLine"} {
				if _, exists := got[key]; exists {
					t.Errorf("starter overrides EditorConfig with %s", key)
				}
			}
		}
		if !strings.HasSuffix(string(body), "\n") {
			t.Fatalf("%s lacks final newline", name)
		}
	}
	body, err := swatch.Content(".prettierignore")
	want := "node_modules/\nvendor/\nthird_party/\nthird-party/\ngenerated/\ndist/\nbuild/\ncoverage/\n*.min.*\n*.map\n"
	if err != nil || string(body) != want {
		t.Fatalf("ignore = %q, %v", body, err)
	}
}
