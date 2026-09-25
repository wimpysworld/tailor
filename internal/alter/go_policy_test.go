package alter

import (
	"bytes"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/wimpysworld/tailor/internal/swatch"
)

func TestRepositoryGoPolicy(t *testing.T) {
	root := filepath.Join("..", "..")
	want, err := swatch.Render(swatch.GoLintDestination, swatch.Options{})
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, swatch.GoLintDestination)
	got, err := os.ReadFile(path)
	if err != nil || !bytes.Equal(got, want) {
		t.Fatalf("repository Go policy differs from the shared default: %v", err)
	}
}

func TestManagedGoPolicyOrderAndFailure(t *testing.T) {
	calls := []string{"actionlint", "go mod tidy -diff", "golangci-lint fmt --config .golangci.yml --diff", "golangci-lint run --config .golangci.yml", "govulncheck ./...", "gocyclo -top 20 -avg -ignore _test\\.go$ ."}
	for _, fail := range append([]string{""}, calls...) {
		t.Run("fail="+fail, func(t *testing.T) {
			root := filepath.Join(t.TempDir(), "project with spaces")
			writeManagedLintFixture(t, root, renderSelectedManagedFiles(t, managedRenderConfig(true, nil)))
			for _, path := range []string{"main.go", "go.mod", "go.sum", ".golangci.yml"} {
				writeManagedTestFile(t, root, path, []byte("unchanged\n"))
			}
			bin, log := filepath.Join(root, "bin"), filepath.Join(root, "calls.log")
			for _, name := range []string{"go", "golangci-lint", "govulncheck", "gocyclo", "actionlint"} {
				writeManagedExecutable(t, filepath.Join(bin, name), "#!/bin/sh\ncall='"+name+"'\nif [ $# -gt 0 ]; then call=\"$call $*\"; fi\nprintf '%s\\n' \"$call\" >> \"$TAILOR_LINT_LOG\"\n[ \"$call\" != \"$TAILOR_FAIL_CALL\" ]\n")
			}
			output, err := runManagedLint(t, managedLintExecutable(t), root, "lint", bin, log, []string{"TAILOR_FAIL_CALL=" + fail})
			if (err != nil) != (fail != "") {
				t.Fatalf("lint: %v\n%s", err, output)
			}
			want := calls
			for i, call := range calls {
				if call == fail {
					want = calls[:i+1]
				}
			}
			if got := managedLintLog(t, log); !reflect.DeepEqual(got, want) {
				t.Fatalf("calls = %v, want %v", got, want)
			}
			for _, path := range []string{"main.go", "go.mod", "go.sum", ".golangci.yml"} {
				got, err := os.ReadFile(filepath.Join(root, path))
				if err != nil || string(got) != "unchanged\n" {
					t.Fatalf("check changed %s: %q, %v", path, got, err)
				}
			}
		})
	}
}

func TestManagedGoStandalonePolicyRecipes(t *testing.T) {
	for recipe, want := range map[string][]string{
		"test": {"go|2|vet ./...", "go|4|test -cover -covermode=atomic ./..."},
		"lint": {"actionlint|0|", "go|3|mod tidy -diff", "golangci-lint|4|fmt --config .golangci.yml --diff", "golangci-lint|3|run --config .golangci.yml", "govulncheck|1|./...", "gocyclo|6|-top 20 -avg -ignore _test\\.go$ ."},
	} {
		t.Run(recipe, func(t *testing.T) {
			root := filepath.Join(t.TempDir(), "project with spaces")
			writeManagedLintFixture(t, root, renderSelectedManagedFiles(t, managedRenderConfig(true, nil)))
			bin, log := filepath.Join(root, "bin"), filepath.Join(root, "calls.log")
			for _, name := range []string{"go", "golangci-lint", "gocyclo", "govulncheck", "actionlint"} {
				writeManagedLintStub(t, bin, name, "TAILOR_UNUSED_STATUS")
			}
			if output, err := runManagedLint(t, managedLintExecutable(t), root, recipe, bin, log, nil); err != nil {
				t.Fatalf("%s: %v\n%s", recipe, err, output)
			}
			var expected []string
			for _, call := range want {
				parts := strings.SplitN(call, "|", 2)
				expected = append(expected, parts[0]+"|"+root+"|"+parts[1])
			}
			wantCalls := canonicalManagedLintCalls(t, expected)
			if got := canonicalManagedLintCalls(t, managedLintLog(t, log)); !reflect.DeepEqual(got, wantCalls) {
				t.Fatalf("calls = %v, want %v", got, wantCalls)
			}
		})
	}
}

func TestHasManagedRecipeWithDependencies(t *testing.T) {
	for _, body := range []string{"lint-go:\n", "lint-go: _lint-go && vet-go\n"} {
		if !hasManagedRecipe([]byte(body), "lint-go") {
			t.Fatalf("missing recipe in %q", body)
		}
	}
	for _, body := range []string{" lint-go:\n", "other-lint-go:\n", "lint-go-extra:\n"} {
		if hasManagedRecipe([]byte(body), "lint-go") {
			t.Fatalf("false recipe in %q", body)
		}
	}
}
