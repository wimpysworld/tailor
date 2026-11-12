package main

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/wimpysworld/tailor/internal/output"
)

func TestMeasureCompleteHealthOrder(t *testing.T) {
	for _, scenario := range []string{"missing", "mixed", "python-warning"} {
		t.Run(scenario, func(t *testing.T) {
			mixed := scenario != "missing"
			base, err := filepath.EvalSymlinks(t.TempDir())
			if err != nil {
				t.Fatal(err)
			}
			dir := filepath.Join(base, "project")
			for _, name := range []string{"project", "home", "xdg"} {
				if err := os.Mkdir(filepath.Join(base, name), 0o755); err != nil {
					t.Fatal(err)
				}
			}
			t.Setenv("HOME", filepath.Join(base, "home"))
			t.Setenv("XDG_CONFIG_HOME", filepath.Join(base, "xdg"))
			t.Setenv("SHELLCHECK_OPTS", "")
			files := map[string]string{
				".tailor.yml": "languages: {shell: true, javascript: true, python: true}\nswatches: []\n",
				"LICENSE":     "Copyright [year] [fullname]\n",
			}
			if mixed {
				files[".shellcheckrc"] = "disable=SC1091\n"
				files[".oxlintrc.jsonc"] = "{}\n"
				files[".editorconfig"] = "project policy\n"
				files[".prettierrc.json"] = "{}\n"
				files["ruff.toml"] = "line-length = 88\n"
				files["CODE_OF_CONDUCT.md"] = "Project code of conduct\n"
				if scenario == "python-warning" {
					if err := os.Mkdir(filepath.Join(dir, "pyrightconfig.json"), 0o755); err != nil {
						t.Fatal(err)
					}
				} else {
					files["pyrightconfig.json"] = "{}\n"
				}
			}
			for name, body := range files {
				if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			t.Chdir(dir)
			var stdout, stderr strings.Builder
			cmd := MeasureCmd{output: output.New(&stdout, &stderr, output.Plain)}
			if err := cmd.Run(); err != nil {
				t.Fatal(err)
			}
			text := stdout.String()
			want := map[string]string{"LICENSE": "warning", "README.md": "warning", ".oxlintrc.json": "missing", ".prettierignore": "missing"}
			for _, name := range []string{".shellcheckrc", ".editorconfig", ".prettierrc.json", "ruff.toml", "pyrightconfig.json"} {
				want[name] = "missing"
			}
			if runtime.GOOS == "windows" {
				want[".shellcheckrc"] = "warning"
			}
			if mixed {
				want[".shellcheckrc"] = "present"
				want[".oxlintrc.json"] = "warning"
				want[".editorconfig"] = "warning"
				want[".prettierrc.json"] = "present"
				want["ruff.toml"] = "present"
				want["pyrightconfig.json"] = "present"
				want["CODE_OF_CONDUCT.md"] = "present"
			}
			if scenario == "python-warning" {
				want["ruff.toml"] = "warning"
				want["pyrightconfig.json"] = "warning"
			}
			for path, status := range want {
				if !strings.Contains(text, fmt.Sprintf("%-16s%s", status+":", path)) {
					t.Fatalf("missing %s status for %s:\n%s", status, path, text)
				}
			}
			order := map[string]int{"missing:": 0, "warning:": 1, "present:": 2, "not-configured:": 3, "config-only:": 4, "mode-differs:": 5}
			previous, previousPath := -1, ""
			for _, line := range strings.Split(strings.TrimSpace(text), "\n") {
				fields := strings.Fields(line)
				if len(fields) < 2 {
					t.Fatalf("unexpected output line %q", line)
				}
				rank, ok := order[fields[0]]
				if !ok || rank < previous || (rank == previous && fields[1] < previousPath) {
					t.Fatalf("output is not in category/path order at %q:\n%s", line, text)
				}
				previous, previousPath = rank, fields[1]
			}
		})
	}
}
