package goproject_test

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/wimpysworld/tailor/internal/goproject"
)

func TestDiscover(t *testing.T) {
	const main = "package main\nfunc main() {}\n"
	tests := []struct {
		name  string
		files map[string]string
		want  []goproject.Build
		err   string
	}{
		{name: "root module version", files: map[string]string{"go.mod": "module example.com/tool/v2\n", "main.go": main}, want: []goproject.Build{{ID: "tool", Binary: "tool", Main: "."}}},
		{name: "multiple and duplicate basenames", files: map[string]string{"cmd/tool/main.go": main, "other/tool/main.go": main}, want: []goproject.Build{{ID: "tool", Binary: "tool", Main: "./cmd/tool"}, {ID: "tool-2", Binary: "tool-2", Main: "./other/tool"}}},
		{name: "ignored directories", files: map[string]string{"cmd/app/main.go": main, "vendor/bad/main.go": "broken", "testdata/main.go": "broken", ".git/main.go": "broken", "_ignored/main.go": "broken", "nested/go.mod": "module nested", "nested/main.go": "broken", "cmd/app/main_test.go": "broken"}, want: []goproject.Build{{ID: "app", Binary: "app", Main: "./cmd/app"}}},
		{name: "library", files: map[string]string{"lib.go": "package lib\n"}, err: "no main packages"},
		{name: "imported library main helper", files: map[string]string{"main.go": "package main\nimport \"example.com/project/helper\"\nfunc main() { _ = helper.Count(nil) }\n", "helper/helper.go": "package helper\nfunc main(args []string) int { return len(args) }\nfunc Count(args []string) int { return main(args) }\n"}, want: []goproject.Build{{ID: "project", Binary: "project", Main: "."}}},
		{name: "method is not main", files: map[string]string{"main.go": "package main\ntype T struct{}\nfunc (T) main() {}\n"}, err: "no main packages"},
		{name: "platform main", files: map[string]string{"main_linux.go": main}, err: "darwin/amd64"},
		{name: "complementary platforms", files: map[string]string{"main_linux.go": main, "main_darwin.go": main}, want: []goproject.Build{{ID: "project", Binary: "project", Main: "."}}},
		{name: "custom build tag", files: map[string]string{"main.go": "//go:build custom\n\n" + main}, err: "no main packages"},
		{name: "ignored generator alone", files: map[string]string{"generator/main.go": "//go:build ignore\n\n" + main}, err: "no main packages"},
		{name: "ignored generator directory", files: map[string]string{"main.go": main, "generator/main.go": "//go:build ignore\n\n" + main}, want: []goproject.Build{{ID: "project", Binary: "project", Main: "."}}},
		{name: "ignored generator beside main", files: map[string]string{"main.go": main, "generate.go": "//go:build ignore\n\n" + main}, want: []goproject.Build{{ID: "project", Binary: "project", Main: "."}}},
		{name: "excluded platform directory", files: map[string]string{"main.go": main, "windows/main_windows.go": main}, want: []goproject.Build{{ID: "project", Binary: "project", Main: "."}}},
		{name: "excluded invalid signature", files: map[string]string{"main.go": main, "generate.go": "//go:build ignore\n\npackage main\nfunc main(args []string) int { return len(args) }\n"}, want: []goproject.Build{{ID: "project", Binary: "project", Main: "."}}},
		{name: "complementary build tags", files: map[string]string{"one.go": "//go:build linux\n\n" + main, "two.go": "//go:build darwin\n\n" + main}, want: []goproject.Build{{ID: "project", Binary: "project", Main: "."}}},
		{name: "cgo", files: map[string]string{"main.go": "package main\nimport \"C\"\nfunc main() {}"}, err: "uses cgo"},
		{name: "duplicate main", files: map[string]string{"one.go": main, "two.go": main}, err: "2 main functions"},
		{name: "mixed packages", files: map[string]string{"main.go": main, "lib.go": "package lib"}, err: "mixed package names"},
		{name: "invalid signature", files: map[string]string{"main.go": "package main\nfunc main(x int) {}"}, err: "unsupported main signature"},
		{name: "too large", files: map[string]string{"main.go": main + strings.Repeat(" ", 1<<20)}, err: "exceeds 1 MiB"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			root := t.TempDir()
			write(t, root, "go.mod", "module example.com/project\n")
			for name, data := range test.files {
				write(t, root, name, data)
			}
			got, err := goproject.Discover(root)
			if test.err != "" {
				if err == nil || !strings.Contains(err.Error(), test.err) {
					t.Fatalf("Discover() error = %v, want %q", err, test.err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(got, test.want) {
				t.Fatalf("Discover() = %#v, want %#v", got, test.want)
			}
		})
	}
}

func TestDiscoverSkipsSymlinks(t *testing.T) {
	root, outside := t.TempDir(), t.TempDir()
	write(t, root, "go.mod", "module example.com/project\n")
	write(t, root, "main.go", "package main\nfunc main() {}\n")
	write(t, outside, "bad.go", "this is not Go")
	for link, target := range map[string]string{"linked": outside, "bad.go": filepath.Join(outside, "bad.go")} {
		if err := os.Symlink(target, filepath.Join(root, link)); err != nil {
			t.Fatal(err)
		}
	}
	if builds, err := goproject.Discover(root); err != nil || len(builds) != 1 {
		t.Fatalf("Discover() = %v, %v", builds, err)
	}
}

func TestDiscoverRequiresRootModule(t *testing.T) {
	root := t.TempDir()
	write(t, root, "main.go", "package main\nfunc main() {}\n")
	if _, err := goproject.Discover(root); err == nil || !strings.Contains(err.Error(), "root go.mod") {
		t.Fatalf("Discover() error = %v", err)
	}
	outside := filepath.Join(t.TempDir(), "go.mod")
	if err := os.WriteFile(outside, []byte("module example.com/project\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(root, "go.mod")); err != nil {
		t.Fatal(err)
	}
	if _, err := goproject.Discover(root); err == nil || !strings.Contains(err.Error(), "regular file") {
		t.Fatalf("Discover() error = %v", err)
	}
}

func write(t *testing.T, root, name, data string) {
	t.Helper()
	destination := filepath.Join(root, filepath.FromSlash(name))
	if err := os.MkdirAll(filepath.Dir(destination), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(destination, []byte(data), 0o600); err != nil {
		t.Fatal(err)
	}
}
