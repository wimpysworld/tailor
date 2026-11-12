package main

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/wimpysworld/tailor/internal/output"
	"github.com/wimpysworld/tailor/internal/swatch"
)

func TestMeasureShellPolicy(t *testing.T) {
	for _, tc := range []struct{ location, name, status string }{
		{"missing", ".shellcheckrc", "missing"},
		{"missing", ".editorconfig", "missing"},
		{"root", ".shellcheckrc", "present"},
		{"root", ".editorconfig", "warning"},
		{"starter", ".editorconfig", "present"},
		{"ancestor", ".shellcheckrc", "present"},
		{"ancestor", ".editorconfig", "missing"},
		{"home", ".shellcheckrc", "present"},
		{"xdg", ".shellcheckrc", "present"},
		{"symlink", ".shellcheckrc", "warning"},
		{"symlink", ".editorconfig", "warning"},
		{"oversized", ".shellcheckrc", "warning"},
		{"oversized", ".editorconfig", "warning"},
		{"environment", ".shellcheckrc", "warning"},
	} {
		t.Run(tc.location+"/"+tc.name, func(t *testing.T) {
			if runtime.GOOS == "windows" && (tc.location == "home" || tc.location == "xdg") {
				t.Skip("unix home and xdg lookup requires unix")
			}
			status := tc.status
			if runtime.GOOS == "windows" && tc.location == "missing" && tc.name == ".shellcheckrc" {
				status = "warning"
			}
			base, err := filepath.EvalSymlinks(t.TempDir())
			if err != nil {
				t.Fatal(err)
			}
			dir, home, xdg := filepath.Join(base, "project"), filepath.Join(base, "home"), filepath.Join(base, "xdg")
			for _, path := range []string{dir, home, xdg} {
				if err := os.Mkdir(path, 0o755); err != nil {
					t.Fatal(err)
				}
			}
			t.Setenv("HOME", home)
			t.Setenv("XDG_CONFIG_HOME", xdg)
			t.Setenv("SHELLCHECK_OPTS", "")
			if err := os.WriteFile(filepath.Join(dir, ".tailor.yml"), []byte("languages: {shell: true}\nswatches: []\n"), 0o644); err != nil {
				t.Fatal(err)
			}
			path := filepath.Join(dir, tc.name)
			body := []byte("project policy\n")
			switch tc.location {
			case "ancestor":
				path = filepath.Join(base, tc.name)
			case "home":
				path = filepath.Join(home, tc.name)
			case "xdg":
				path = filepath.Join(xdg, "shellcheckrc")
			case "starter":
				body, _ = swatch.Content(tc.name)
			case "oversized":
				body = []byte(strings.Repeat("x", (1<<20)+1))
			case "environment":
				t.Setenv("SHELLCHECK_OPTS", "--rcfile=custom")
			}
			if tc.location == "symlink" {
				if err := os.Symlink(filepath.Join(base, "absent"), path); err != nil {
					t.Skip(err)
				}
			} else if tc.location != "missing" && tc.location != "environment" {
				if err := os.WriteFile(path, body, 0o644); err != nil {
					t.Fatal(err)
				}
			}
			t.Chdir(dir)
			var stdout, stderr strings.Builder
			cmd := MeasureCmd{output: output.New(&stdout, &stderr, output.Plain)}
			if err := cmd.Run(); err != nil {
				t.Fatal(err)
			}
			want := fmt.Sprintf("%-16s%s", status+":", tc.name)
			if !strings.Contains(stdout.String(), want) {
				t.Fatalf("missing %q:\n%s", want, stdout.String())
			}
			if status != "missing" {
				for _, other := range []string{"missing", "not-configured", "present", "warning"} {
					if other != status && strings.Contains(stdout.String(), fmt.Sprintf("%-16s%s", other+":", tc.name)) {
						t.Fatalf("misleading status:\n%s", stdout.String())
					}
				}
			}
			if tc.location == "missing" || tc.location == "environment" || tc.location == "ancestor" || tc.location == "home" || tc.location == "xdg" {
				if _, err := os.Lstat(filepath.Join(dir, tc.name)); !os.IsNotExist(err) {
					t.Fatalf("measure created %s: %v", tc.name, err)
				}
			} else if tc.location != "symlink" {
				got, err := os.ReadFile(path)
				if err != nil || string(got) != string(body) {
					t.Fatal("measure changed policy")
				}
			}
		})
	}
}

func TestMeasureShellInactiveOrNever(t *testing.T) {
	for _, declaration := range []string{"", "languages: {shell: false}\n", "languages: {javascript: true}\n", "languages: {typescript: true}\n", "languages: {shell: true}\n"} {
		t.Run(declaration, func(t *testing.T) {
			dir := t.TempDir()
			t.Setenv("HOME", dir)
			t.Setenv("XDG_CONFIG_HOME", dir)
			t.Setenv("SHELLCHECK_OPTS", "")
			config := declaration + "swatches:\n  - path: .shellcheckrc\n    alteration: never\n  - path: .editorconfig\n    alteration: never\n"
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
				for _, name := range []string{".shellcheckrc", ".editorconfig"} {
					if strings.Contains(stdout.String(), fmt.Sprintf("%-16s%s", status+":", name)) {
						t.Fatalf("inactive status:\n%s", stdout.String())
					}
				}
			}
		})
	}
}
