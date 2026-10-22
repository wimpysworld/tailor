package alter

import (
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"strings"
	"testing"

	"github.com/wimpysworld/tailor/internal/swatch"
	"gopkg.in/yaml.v3"
)

func TestRepositoryGoPolicy(t *testing.T) {
	root := filepath.Join("..", "..")
	want, err := swatch.Render(swatch.GoLintDestination, swatch.Options{})
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, swatch.GoLintDestination)
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var repository, shared map[string]any
	if err := yaml.Unmarshal(got, &repository); err != nil {
		t.Fatal(err)
	}
	if err := yaml.Unmarshal(want, &shared); err != nil {
		t.Fatal(err)
	}
	linters, ok := shared["linters"].(map[string]any)
	if !ok {
		t.Fatal("shared Go policy has no linters mapping")
	}
	exclusions, ok := linters["exclusions"].(map[string]any)
	if !ok {
		t.Fatal("shared Go policy has no exclusions mapping")
	}
	rules, ok := exclusions["rules"].([]any)
	if !ok {
		t.Fatal("shared Go policy has no exclusion rules")
	}
	// This ordinary first-fit policy omits only the two shared G703 exceptions.
	// Tailor does not use the dev tools or audio engine that require them.
	for _, path := range []string{`tools/(rasterise|placement-score)/main\.go`, `internal/engine/audio_linux\.go`} {
		rule := map[string]any{"path": path, "text": "G703", "linters": []any{"gosec"}}
		var retained []any
		matches := 0
		for _, candidate := range rules {
			if reflect.DeepEqual(candidate, rule) {
				matches++
			} else {
				retained = append(retained, candidate)
			}
		}
		if matches != 1 {
			t.Fatalf("shared Go policy has %d exact G703 exceptions for %q, want 1", matches, path)
		}
		rules = retained
	}
	exclusions["rules"] = rules
	if !reflect.DeepEqual(repository, shared) {
		t.Fatal("repository Go policy differs from the shared default beyond the two approved G703 omissions")
	}
}

func TestGoPolicyDirectoryExclusions(t *testing.T) {
	data, err := swatch.Render(swatch.GoLintDestination, swatch.Options{})
	if err != nil {
		t.Fatal(err)
	}
	type section struct {
		Exclusions struct {
			Paths []string `yaml:"paths"`
		} `yaml:"exclusions"`
	}
	var policy struct {
		Linters    section `yaml:"linters"`
		Formatters section `yaml:"formatters"`
	}
	if err := yaml.Unmarshal(data, &policy); err != nil {
		t.Fatal(err)
	}
	for name, section := range map[string]section{"linters": policy.Linters, "formatters": policy.Formatters} {
		t.Run(name, func(t *testing.T) {
			patterns := section.Exclusions.Paths
			for _, name := range []string{"third_party", "builtin", "examples"} {
				for path, want := range map[string]bool{
					name + "/bad.go":                true,
					name + "/nested/bad.go":         true,
					"internal/" + name + "/bad.go":  true,
					name + "_test.go":               false,
					"internal/" + name + "_test.go": false,
					"not_" + name + "/bad.go":       false,
					name + "_extra/bad.go":          false,
					"good.go":                       false,
				} {
					matched := false
					for _, pattern := range patterns {
						matched = matched || regexp.MustCompile(pattern).MatchString(path)
					}
					if matched != want {
						t.Errorf("path %q excluded = %v, want %v", path, matched, want)
					}
				}
			}
		})
	}
}

func TestManagedGoPolicyOrderAndFailure(t *testing.T) {
	calls := []string{"actionlint", "go mod tidy -diff", "golangci-lint fmt --config .golangci.yml --diff", "golangci-lint run --config .golangci.yml", "govulncheck -show verbose ./...", "gocyclo -top 20 -avg -ignore _test\\.go$ ."}
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
