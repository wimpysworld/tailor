package alter

import (
	"bytes"
	"errors"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/wimpysworld/tailor/internal/config"
	"github.com/wimpysworld/tailor/internal/model"
	"gopkg.in/yaml.v3"
)

func TestQualityContributionReadError(t *testing.T) {
	for _, operation := range managedOperations() {
		t.Run(string(operation), func(t *testing.T) {
			body, err := operation.contribution("missing-contribution")
			if body != nil || !errors.Is(err, fs.ErrNotExist) {
				t.Fatalf("contribution = %q, %v, want wrapped missing-file error", body, err)
			}
			if !strings.Contains(err.Error(), `reading managed quality contribution "missing-contribution"`) {
				t.Fatalf("missing contribution context: %v", err)
			}
			var pathErr *fs.PathError
			if !errors.As(err, &pathErr) {
				t.Fatalf("lost embedded read error: %v", err)
			}
			directory := "quality"
			if operation == managedSetup {
				directory = "setup"
			}
			if pathErr.Path != "swatches/"+directory+"/missing-contribution.sh" {
				t.Fatalf("read path = %q", pathErr.Path)
			}
		})
	}
}

func TestQualityMissingContributionStopsBeforeWrites(t *testing.T) {
	root := t.TempDir()
	writeManagedTestFile(t, root, "just/tailor.just", []byte("preserve existing core\n"))
	before := snapshotManagedTree(t, root)
	entry := managedRegistryEntry{
		Path: "just/go.just", Policy: managedPolicyFragment,
		Capability: managedCapabilityGo, BuildBody: "missing-contribution",
	}
	cfg := managedRenderConfig(true, nil)
	body, err := renderManagedQuality(cfg, []byte("[[TAILOR_MANAGED_BUILD_BODY]]"), []managedRegistryEntry{entry}, managedBuild)
	if body != nil || !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("render = %q, %v, want wrapped missing-file error", body, err)
	}
	_, err = preflightManagedFiles(root, []managedSelection{{Entry: entry, Enabled: true}}, managedRenderedFiles{})
	if !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("preflight = %v, want wrapped missing-file error", err)
	}
	if !reflect.DeepEqual(before, snapshotManagedTree(t, root)) {
		t.Fatal("missing contribution changed project files")
	}
}

func TestRepositoryQualityRendering(t *testing.T) {
	root := filepath.Join("..", "..")
	cfg, err := config.Load(root)
	if err != nil {
		t.Fatal(err)
	}
	rendered := renderSelectedManagedFiles(t, cfg)
	for _, name := range []string{"just/tailor.just", "just/go.just"} {
		path := filepath.Join(root, name)
		got, err := os.ReadFile(path)
		if err != nil || !bytes.Equal(got, rendered[name]) {
			t.Fatalf("repository %s differs from production rendering: %v", name, err)
		}
	}
}

func TestRepositoryJustRecipes(t *testing.T) {
	just, err := exec.LookPath("just")
	if err != nil {
		t.Skip("just is required")
	}
	root := filepath.Join("..", "..")
	want := []string{"build", "default", "lint", "pages", "release", "setup", "snapshot", "test"}
	if got := managedJustRecipes(t, just, root); !reflect.DeepEqual(got, want) {
		t.Fatalf("repository recipes = %v, want %v", got, want)
	}
	for _, recipe := range []string{"alter", "measure", "alter-source", "measure-source"} {
		cmd := exec.CommandContext(t.Context(), just, "--dry-run", recipe)
		cmd.Dir = root
		output, err := cmd.CombinedOutput()
		if err == nil || !strings.Contains(string(output), "does not contain recipe") {
			t.Errorf("removed recipe %s: %v\n%s", recipe, err, output)
		}
	}
	project, err := os.ReadFile(filepath.Join(root, "just/project.just"))
	if err != nil || string(project) != "# Build snapshot release packages\nsnapshot:\n    @goreleaser release --snapshot --clean --skip=sign\n" {
		t.Fatalf("snapshot recipe changed: %v\n%s", err, project)
	}
}

func TestQualityCommands(t *testing.T) {
	lint := []string{"actionlint", "go mod tidy -diff", "golangci-lint fmt --config .golangci.yml --diff", "golangci-lint run --config .golangci.yml", "govulncheck -show verbose ./...", "gocyclo -top 20 -avg -ignore _test\\.go$ ."}
	for _, mode := range []string{"check", "correct", "invalid", "$(touch injected)"} {
		t.Run(mode, func(t *testing.T) {
			root, bin, log := qualityFixture(t, true)
			writeManagedTestFile(t, root, "just/project/lint.sh", []byte("printf 'project %s %s\\n' \"$1\" \"$PWD\" >> \"$CALLS\"\n"))
			before := snapshotManagedTree(t, root)
			output, err := qualityRun(t, root, bin, log, "", "lint", mode)
			if _, statErr := os.Lstat(filepath.Join(root, "injected")); !os.IsNotExist(statErr) {
				t.Fatalf("lint expanded the mode argument: %v", statErr)
			}
			if mode != "check" && mode != "correct" {
				if err == nil || !strings.Contains(string(output), "lint mode must be") || len(managedLintLog(t, log)) != 0 {
					t.Fatalf("invalid mode did work: %s %v", output, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("lint: %v %s", err, output)
			}
			want := append([]string{}, lint...)
			if mode == "correct" {
				want = append([]string{"actionlint", "go mod tidy", "golangci-lint fmt --config .golangci.yml"}, want[1:]...)
			}
			want = append(want, "project "+mode+" "+root)
			if got := managedLintLog(t, log); !reflect.DeepEqual(got, want) {
				t.Fatalf("calls %q, want %q", got, want)
			}
			after := snapshotManagedTree(t, root)
			delete(after, "calls")
			if !reflect.DeepEqual(before, after) {
				t.Fatal("lint changed project files")
			}
		})
	}
	for _, fail := range lint {
		t.Run("fail "+fail, func(t *testing.T) {
			root, bin, log := qualityFixture(t, true)
			if output, err := qualityRun(t, root, bin, log, fail, "lint"); err == nil {
				t.Fatalf("expected failure: %s", output)
			}
			for i, call := range lint {
				if call == fail && !reflect.DeepEqual(managedLintLog(t, log), lint[:i+1]) {
					t.Fatal("lint did not stop at first failure")
				}
			}
		})
	}
	t.Run("test coverage once", func(t *testing.T) {
		root, bin, log := qualityFixture(t, true)
		if output, err := qualityRun(t, root, bin, log, "", "test"); err != nil {
			t.Fatalf("test: %v %s", err, output)
		}
		if got := managedLintLog(t, log); !reflect.DeepEqual(got, []string{"go vet ./...", "go test -cover -covermode=atomic ./..."}) {
			t.Fatalf("test calls %q", got)
		}
		if _, err := os.Stat(filepath.Join(root, "coverage.out")); !os.IsNotExist(err) {
			t.Fatal("test wrote coverage.out")
		}
	})
}

func TestQualityWorkflowLintIndependentOfContributions(t *testing.T) {
	registry := []managedRegistryEntry{
		{Path: "just/pages.just", Policy: managedPolicyFragment, Capability: managedCapabilityPages, LintBody: "test-go"},
		{Path: "just/go.just", Policy: managedPolicyFragment, Capability: managedCapabilityGo, LintBody: "lint-go"},
	}
	for _, test := range []struct {
		name  string
		goOn  bool
		pages bool
		want  []string
	}{
		{name: "zero", want: []string{"actionlint"}},
		{name: "one without Go", pages: true, want: []string{"actionlint", "go vet ./...", "go test -cover -covermode=atomic ./..."}},
		{name: "multiple", goOn: true, pages: true, want: []string{"actionlint", "go mod tidy -diff", "golangci-lint fmt --config .golangci.yml --diff", "golangci-lint run --config .golangci.yml", "govulncheck -show verbose ./...", "gocyclo -top 20 -avg -ignore _test\\.go$ .", "go vet ./...", "go test -cover -covermode=atomic ./..."}},
	} {
		t.Run(test.name, func(t *testing.T) {
			root, bin, log := qualityFixture(t, false)
			cfg := managedRenderConfig(test.goOn, &model.PagesSettings{Enabled: new(test.pages)})
			core, err := renderManagedQuality(cfg, []byte("lint $mode=\"check\":\n    #!/usr/bin/env bash\n    set -euo pipefail\n    [[TAILOR_MANAGED_LINT_BODY]]\n"), registry, managedLint)
			if err != nil {
				t.Fatal(err)
			}
			if bytes.Count(core, []byte("\n    check_workflows\n")) != 1 {
				t.Fatalf("workflow invocation count differs from one:\n%s", core)
			}
			writeManagedTestFile(t, root, "just/tailor.just", core)
			if output, err := qualityRun(t, root, bin, log, "", "lint"); err != nil {
				t.Fatalf("lint: %v %s", err, output)
			}
			if got := managedLintLog(t, log); !reflect.DeepEqual(got, test.want) {
				t.Fatalf("calls %q, want %q", got, test.want)
			}
		})
	}
}

func TestQualityPreservesGOFLAGS(t *testing.T) {
	for _, flags := range []string{"", "-mod=readonly"} {
		t.Run(flags, func(t *testing.T) {
			t.Setenv("GOFLAGS", flags)
			for _, operation := range []string{"build", "test", "lint"} {
				root, bin, log := qualityFixture(t, true)
				for _, name := range []string{"go", "golangci-lint"} {
					writeManagedExecutable(t, filepath.Join(bin, name), "#!/bin/sh\n[ \"$GOFLAGS\" = \"$EXPECTED_GOFLAGS\" ] || exit 42\nprintf '%s\\n' \"$*\" >> \"$CALLS\"\n")
				}
				t.Setenv("EXPECTED_GOFLAGS", flags)
				if output, err := qualityRun(t, root, bin, log, "", operation); err != nil {
					t.Fatalf("%s changed GOFLAGS: %v %s", operation, err, output)
				}
				for _, call := range managedLintLog(t, log) {
					if strings.Contains(call, "-cover") && call != "test -cover -covermode=atomic ./..." {
						t.Fatalf("coverage escaped test: %q", call)
					}
				}
			}
		})
	}
}

func TestQualityProjectScripts(t *testing.T) {
	for _, operation := range []string{"build", "test", "lint"} {
		for _, kind := range []string{"missing", "regular", "failure", "symlink", "parent symlink", "directory", "parent file"} {
			t.Run(operation+" "+kind, func(t *testing.T) {
				root, bin, log := qualityFixture(t, false)
				path := "just/project/" + operation + ".sh"
				switch kind {
				case "regular", "failure":
					body := "printf 'extension\\n' >> \"$CALLS\"\n"
					if kind == "failure" {
						body += "exit 23\n"
					}
					writeManagedTestFile(t, root, path, []byte(body))
				case "symlink":
					if err := os.MkdirAll(filepath.Join(root, "just/project"), 0o755); err != nil {
						t.Fatal(err)
					}
					managedSymlinkOrSkip(t, filepath.Join(root, "absent"), filepath.Join(root, path))
				case "parent symlink":
					managedSymlinkOrSkip(t, t.TempDir(), filepath.Join(root, "just/project"))
				case "directory":
					if err := os.MkdirAll(filepath.Join(root, path), 0o755); err != nil {
						t.Fatal(err)
					}
				case "parent file":
					writeManagedTestFile(t, root, "just/project", []byte("not a directory"))
				}
				output, err := qualityRun(t, root, bin, log, "", operation)
				wantFailure := kind != "missing" && kind != "regular"
				if (err != nil) != wantFailure {
					t.Fatalf("%s: %v %s", kind, err, output)
				}
				if wantFailure && kind != "failure" && !strings.Contains(string(output), "project script") {
					t.Fatalf("unclear error: %s", output)
				}
			})
		}
	}
}

func TestQualityOldNamesUnavailable(t *testing.T) {
	root, bin, log := qualityFixture(t, true)
	for _, name := range []string{"build-go", "test-go", "lint-go", "_lint-go", "check-go", "lint-actions", "vet-go", "fmt-go", "fmt-check-go", "tidy-go", "tidy-check-go", "coverage-go", "complexity-go"} {
		if output, err := qualityRun(t, root, bin, log, "", name); err == nil || !strings.Contains(string(output), "does not contain recipe") {
			t.Fatalf("%s remains callable: %v %s", name, err, output)
		}
	}
}

func TestQualityPreflightContribution(t *testing.T) {
	cfg := &config.Config{Languages: &config.LanguageSettings{Go: new(true)}}
	selected, err := selectManagedFiles(cfg)
	if err != nil {
		t.Fatal(err)
	}
	rendered := renderSelectedManagedFiles(t, cfg)
	if _, err := preflightManagedFiles(t.TempDir(), selected, rendered); err != nil {
		t.Fatal(err)
	}
	rendered["just/tailor.just"] = bytes.ReplaceAll(rendered["just/tailor.just"], []byte("go vet ./..."), []byte("missing"))
	if _, err := preflightManagedFiles(t.TempDir(), selected, rendered); err == nil {
		t.Fatal("accepted missing contribution")
	}
}

func TestQualityCGOAndDefaultMode(t *testing.T) {
	for _, policy := range []*bool{nil, new(false), new(true)} {
		root, bin, log := qualityFixture(t, true)
		cfg := managedRenderConfig(true, nil)
		cfg.Languages.GoOptions = &config.GoOptions{CGO: policy}
		writeManagedLintFixture(t, root, renderSelectedManagedFiles(t, cfg))
		writeManagedExecutable(t, filepath.Join(bin, "go"), "#!/bin/sh\nprintf 'go %s\\n' \"$CGO_ENABLED\" >> \"$CALLS\"\n")
		writeManagedTestFile(t, root, "just/project/lint.sh", []byte("printf 'project %s %s\\n' \"$CGO_ENABLED\" \"$1\" >> \"$CALLS\"\n"))
		t.Setenv("CGO_ENABLED", "inherited")
		if output, err := qualityRun(t, root, bin, log, "", "lint"); err != nil {
			t.Fatalf("lint: %v %s", err, output)
		}
		want := "inherited"
		if policy != nil {
			want = "0"
			if *policy {
				want = "1"
			}
		}
		calls := managedLintLog(t, log)
		if calls[0] != "actionlint" || calls[1] != "go "+want || calls[len(calls)-1] != "project "+want+" check" {
			t.Fatalf("CGO or default mode escaped its scope: %q", calls)
		}
	}
}

func TestManagedCGOExport(t *testing.T) {
	for _, test := range []struct {
		name, declaration, want string
	}{
		{"absent", "", "inherited"},
		{"legacy", "languages:\n  go: true\n", "inherited"},
		{"nil", "languages:\n  go:\n    enabled: true\n", "inherited"},
		{"false", "languages:\n  go:\n    enabled: true\n    cgo: false\n", "0"},
		{"true", "languages:\n  go:\n    enabled: true\n    cgo: true\n", "1"},
		{"ffmpeg", "languages:\n  go:\n    enabled: true\n    ffmpeg-statigo: true\n", "1"},
		{"disabled", "languages:\n  go:\n    enabled: false\n    cgo: true\n", "inherited"},
	} {
		t.Run(test.name, func(t *testing.T) {
			var cfg config.Config
			if err := yaml.Unmarshal([]byte(test.declaration), &cfg); err != nil {
				t.Fatal(err)
			}
			files := renderSelectedManagedFiles(t, &cfg)
			encoded, err := yaml.Marshal(&cfg)
			if err != nil {
				t.Fatal(err)
			}
			var decoded config.Config
			if err := yaml.Unmarshal(encoded, &decoded); err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(files, renderSelectedManagedFiles(t, &decoded)) {
				t.Fatal("CGO rendering changed after round trip")
			}
			count := 0
			if test.want != "inherited" {
				count = 1
			}
			if got := bytes.Count(files["just/tailor.just"], []byte("export CGO_ENABLED")); got != count {
				t.Fatalf("CGO export count = %d, want %d", got, count)
			}
			if bytes.Contains(files["just/tailor.just"], []byte("[[TAILOR_")) {
				t.Fatal("unresolved core placeholder")
			}
			root, bin, log := qualityFixture(t, true)
			writeManagedTestFile(t, root, "just/tailor.just", files["just/tailor.just"])
			format := exec.CommandContext(t.Context(), "just", "--unstable", "--fmt", "--check", "-f", "just/tailor.just")
			format.Dir = root
			if output, err := format.CombinedOutput(); err != nil {
				t.Fatalf("core formatting: %v\n%s", err, output)
			}
			retained, err := os.ReadFile(filepath.Join(root, "just/go.just"))
			if err != nil {
				t.Fatal(err)
			}
			for _, name := range []string{"just/tailor.just", "just/go-ffmpeg-statigo.just"} {
				if content, ok := files[name]; ok {
					writeManagedTestFile(t, root, name, content)
				}
			}
			writeManagedTestFile(t, root, "just/go.just", retained)
			writeManagedTestFile(t, root, "justfile", []byte("import 'just/loader.just'\nimport 'just/project.just'\nroot-policy:\n    @printf 'root %s\\n' \"$CGO_ENABLED\" >> \"$CALLS\"\n"))
			writeManagedTestFile(t, root, "just/project.just", []byte("project-policy:\n    @printf 'import %s\\n' \"$CGO_ENABLED\" >> \"$CALLS\"\n"))
			writeManagedTestFile(t, root, "just/project/lint.sh", []byte("printf 'hook %s\\n' \"$CGO_ENABLED\" >> \"$CALLS\"\n"))
			t.Setenv("CGO_ENABLED", "inherited")
			for _, recipe := range []string{"root-policy", "project-policy", "lint"} {
				if output, err := qualityRun(t, root, bin, log, "", recipe); err != nil {
					t.Fatalf("%s: %v %s", recipe, err, output)
				}
			}
			calls := managedLintLog(t, log)
			if calls[0] != "root "+test.want || calls[1] != "import "+test.want || calls[len(calls)-1] != "hook "+test.want {
				t.Fatalf("CGO inheritance: %q", calls)
			}
		})
	}
	for _, template := range []string{"", "[[TAILOR_MANAGED_CGO]][[TAILOR_MANAGED_CGO]]"} {
		if _, err := renderManagedCGO(&config.Config{}, []byte(template)); err == nil {
			t.Fatal("accepted invalid CGO placeholder count")
		}
	}
}

func testTempDir(t *testing.T) string {
	t.Helper()
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	return root
}

func qualityFixture(t *testing.T, enabled bool) (string, string, string) {
	t.Helper()
	root := filepath.Join(testTempDir(t), "project with spaces")
	writeManagedLintFixture(t, root, renderSelectedManagedFiles(t, managedRenderConfig(enabled, nil)))
	bin, log := filepath.Join(root, "stubs"), filepath.Join(root, "calls")
	for _, name := range []string{"go", "golangci-lint", "govulncheck", "gocyclo", "actionlint"} {
		writeManagedExecutable(t, filepath.Join(bin, name), "#!/bin/sh\ncall='"+name+"'\nif [ $# -gt 0 ]; then call=\"$call $*\"; fi\nprintf '%s\\n' \"$call\" >> \"$CALLS\"\n[ \"$call\" != \"$FAIL_CALL\" ]\n")
	}
	return root, bin, log
}

func qualityRun(t *testing.T, root, bin, log, fail string, args ...string) ([]byte, error) {
	t.Helper()
	// #nosec G204 -- The executable, recipe arguments, and temporary fixture are controlled by this test.
	command := exec.CommandContext(t.Context(), managedLintExecutable(t), append([]string{"--justfile", filepath.Join(root, "justfile")}, args...)...)
	command.Dir = filepath.Join(root, "just")
	command.Env = append(managedEnvironmentWithout("PATH", "BASH_ENV", "ENV", "CALLS", "FAIL_CALL"), "PATH="+bin+":"+os.Getenv("PATH"), "CALLS="+log, "FAIL_CALL="+fail)
	return command.CombinedOutput()
}
