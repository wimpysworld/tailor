package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/wimpysworld/tailor/internal/output"
)

func TestMeasureJavaScriptPolicy(t *testing.T) {
	cases := []struct{ name, body, destination, status string }{
		{"", "", ".oxlintrc.json", "missing"},
		{".oxlintrc.json", "{}", ".oxlintrc.json", "present"},
		{".prettierrc.json", "{}", ".prettierrc.json", "present"},
		{".prettierrc.json", "{}", ".prettierignore", "missing"},
		{".prettierrc.json5", "{semi: false}", ".prettierrc.json", "present"},
		{".prettierrc.json5", "{semi: false}", ".prettierignore", "present"},
		{".prettierrc.yml", "semi: false", ".prettierrc.json", "present"},
		{".prettierrc.yml", "semi: false", ".prettierignore", "present"},
		{"package.json", "{\"prettier\":{}}", ".prettierignore", "present"},
		{"package.yaml", "prettier: {}", ".prettierrc.json", "present"},
		{"oxlint.config.ts", "throw Error('not executed')", ".oxlintrc.json", "warning"},
		{".oxlintrc.jsonc", "{}", ".oxlintrc.json", "warning"},
		{"eslint.config.js", "export default []", ".oxlintrc.json", "warning"},
		{"biome.jsonc", "{}", ".prettierrc.json", "warning"},
		{"package.json", "{\"eslintConfig\":{}}", ".prettierignore", "warning"},
		{"package.json", "{", ".oxlintrc.json", "warning"},
		{"package.yaml", strings.Repeat(" ", (1<<20)+1), ".prettierrc.json", "warning"},
	}
	for _, name := range []string{"package.json", "package.yaml"} {
		for _, value := range []string{"null", "false", "42", "[]", "\"\"", "\"  \\t\"", "{}", "\"@example/prettier-config\""} {
			status := "warning"
			if value == "{}" || value == "\"@example/prettier-config\"" {
				status = "present"
			}
			for _, destination := range []string{".prettierrc.json", ".prettierignore"} {
				cases = append(cases, struct{ name, body, destination, status string }{name, "{\"prettier\":" + value + "}", destination, status})
			}
		}
	}
	for _, tc := range cases {
		for _, key := range []string{"javascript", "typescript"} {
			t.Run(key+"/"+tc.name+"/"+tc.destination+"/"+tc.status, func(t *testing.T) {
				dir := t.TempDir()
				if err := os.WriteFile(filepath.Join(dir, ".tailor.yml"), []byte("languages: {"+key+": true}\nswatches: []\n"), 0o644); err != nil {
					t.Fatal(err)
				}
				if tc.name != "" {
					if err := os.WriteFile(filepath.Join(dir, tc.name), []byte(tc.body), 0o644); err != nil {
						t.Fatal(err)
					}
				}
				t.Chdir(dir)
				var stdout, stderr strings.Builder
				cmd := MeasureCmd{output: output.New(&stdout, &stderr, output.Plain)}
				if err := cmd.Run(); err != nil {
					t.Fatal(err)
				}
				want := fmt.Sprintf("%-16s%s", tc.status+":", tc.destination)
				if !strings.Contains(stdout.String(), want) {
					t.Fatalf("missing %q:\n%s", want, stdout.String())
				}
				if tc.status != "missing" {
					for _, status := range []string{"missing", "not-configured", "present"} {
						if status != tc.status && strings.Contains(stdout.String(), fmt.Sprintf("%-16s%s", status+":", tc.destination)) {
							t.Fatalf("misleading status:\n%s", stdout.String())
						}
					}
				}
				if tc.name != "" {
					body, err := os.ReadFile(filepath.Join(dir, tc.name))
					if err != nil || string(body) != tc.body {
						t.Fatal("measure changed policy")
					}
				}
			})
		}
	}
}

func TestMeasureJavaScriptInactiveOrNever(t *testing.T) {
	for _, declaration := range []string{"", "languages: {javascript: false}\n", "languages: {typescript: false}\n", "languages: {javascript: true}\n"} {
		t.Run(declaration, func(t *testing.T) {
			dir := t.TempDir()
			config := declaration + "swatches:\n  - path: .oxlintrc.json\n    alteration: never\n  - path: .prettierrc.json\n    alteration: never\n  - path: .prettierignore\n    alteration: never\n"
			if err := os.WriteFile(filepath.Join(dir, ".tailor.yml"), []byte(config), 0o644); err != nil {
				t.Fatal(err)
			}
			t.Chdir(dir)
			var stdout, stderr strings.Builder
			cmd := MeasureCmd{output: output.New(&stdout, &stderr, output.Plain)}
			if err := cmd.Run(); err != nil {
				t.Fatal(err)
			}
			for _, status := range []string{"present", "missing", "warning", "not-configured"} {
				for _, name := range []string{".oxlintrc.json", ".prettierrc.json", ".prettierignore"} {
					if strings.Contains(stdout.String(), fmt.Sprintf("%-16s%s", status+":", name)) {
						t.Fatalf("inactive status:\n%s", stdout.String())
					}
				}
			}
		})
	}
}
