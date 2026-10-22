package alter

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestManagedLintExecutesSelectedLintersOnceAtProjectRoot(t *testing.T) {
	just := managedLintExecutable(t)
	root := filepath.Join(t.TempDir(), "project")
	rendered := renderSelectedManagedFiles(t, managedRenderConfig(true, nil))
	writeManagedLintFixture(t, root, rendered)
	log := filepath.Join(root, "lint.log")
	bin := filepath.Join(root, "bin")
	writeManagedLintStub(t, bin, "actionlint", "TAILOR_ACTIONLINT_STATUS")
	writeManagedLintStub(t, bin, "golangci-lint", "TAILOR_GOLANGCI_STATUS")
	writeManagedLintStub(t, bin, "govulncheck", "TAILOR_GOVULNCHECK_STATUS")
	writeManagedLintStub(t, bin, "go", "TAILOR_GO_STATUS")
	writeManagedLintStub(t, bin, "gocyclo", "TAILOR_UNUSED_STATUS")

	output, err := runManagedLint(t, just, root, "lint", bin, log, nil)
	if err != nil {
		t.Fatalf("just lint: %v\n%s", err, output)
	}
	want := canonicalManagedLintCalls(t, []string{
		"actionlint|" + root + "|0|",
		"go|" + root + "|3|mod tidy -diff",
		"golangci-lint|" + root + "|4|fmt --config .golangci.yml --diff",
		"golangci-lint|" + root + "|3|run --config .golangci.yml",
		"govulncheck|" + root + "|3|-show verbose ./...",
		"gocyclo|" + root + "|6|-top 20 -avg -ignore _test\\.go$ .",
	})
	if got := canonicalManagedLintCalls(t, managedLintLog(t, log)); !reflect.DeepEqual(got, want) {
		t.Fatalf("lint calls = %v, want %v", got, want)
	}
}

func TestManagedLintWorkflowDiscovery(t *testing.T) {
	just := managedLintExecutable(t)
	for _, test := range []struct {
		name        string
		files       []string
		directories []string
		wantRun     bool
	}{
		{name: "missing directory"},
		{name: "empty directory", directories: []string{""}},
		{name: "non YAML files", files: []string{"README.md", "ci.yml.disabled"}},
		{name: "YAML directory", directories: []string{"directory.yml"}},
		{name: "yml", files: []string{"ci.yml"}, wantRun: true},
		{name: "yaml", files: []string{"ci.yaml"}, wantRun: true},
		{name: "hidden yml", files: []string{".ci.yml"}, wantRun: true},
		{name: "hidden yaml", files: []string{".ci.yaml"}, wantRun: true},
		{name: "multiple files with spaces", files: []string{"first workflow.yml", "second workflow.yaml"}, wantRun: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			root := filepath.Join(t.TempDir(), "project with spaces")
			writeManagedJustFixture(t, root, renderSelectedManagedFiles(t, managedRenderConfig(true, nil)))
			writeManagedTestFile(t, root, "justfile", []byte("set shell := [\"/bin/sh\", \"-cu\"]\nimport 'just/loader.just'\n"))
			for _, name := range test.directories {
				if err := os.MkdirAll(filepath.Join(root, ".github", "workflows", name), 0o755); err != nil {
					t.Fatal(err)
				}
			}
			for _, name := range test.files {
				writeManagedTestFile(t, root, ".github/workflows/"+name, []byte("invalid: [\n"))
			}
			log := filepath.Join(root, "lint.log")
			bin := filepath.Join(root, "bin")
			if test.wantRun {
				writeManagedLintStub(t, bin, "actionlint", "TAILOR_ACTIONLINT_STATUS")
			}
			writeManagedLintStub(t, bin, "golangci-lint", "TAILOR_GOLANGCI_STATUS")
			writeManagedLintStub(t, bin, "govulncheck", "TAILOR_GOVULNCHECK_STATUS")
			writeManagedLintStub(t, bin, "go", "TAILOR_GO_STATUS")
			writeManagedLintStub(t, bin, "gocyclo", "TAILOR_UNUSED_STATUS")
			output, err := runManagedLint(t, just, root, "lint", bin, log, nil)
			if err != nil {
				t.Fatalf("just lint: %v\n%s", err, output)
			}
			want := []string{"golangci-lint|" + root + "|3|run --config .golangci.yml", "govulncheck|" + root + "|3|-show verbose ./...", "gocyclo|" + root + "|6|-top 20 -avg -ignore _test\\.go$ ."}
			want = append([]string{"go|" + root + "|3|mod tidy -diff", "golangci-lint|" + root + "|4|fmt --config .golangci.yml --diff"}, want...)
			if test.wantRun {
				want = append([]string{"actionlint|" + root + "|0|"}, want...)
			}
			if got := canonicalManagedLintCalls(t, managedLintLog(t, log)); !reflect.DeepEqual(got, canonicalManagedLintCalls(t, want)) {
				t.Fatalf("lint calls = %v, want %v", got, want)
			}
			if skipped := strings.Contains(string(output), "No GitHub Actions workflows found. Skipping actionlint."); skipped == test.wantRun {
				t.Fatalf("unexpected skip message: %s", output)
			}
		})
	}
}

func TestManagedLintWorkflowGateStructure(t *testing.T) {
	rendered := renderSelectedManagedFiles(t, managedRenderConfig(false, nil))
	lines := strings.Split(string(rendered["just/tailor.just"]), "\n")
	for index := range lines {
		lines[index] = strings.TrimSpace(lines[index])
	}
	core := strings.Join(lines, "\n")
	for _, fragment := range []string{
		"check_workflows() {",
		"for workflow in .github/workflows/* .github/workflows/.*; do",
		`case "${workflow}" in *.yml|*.yaml)`,
		`if [[ -f "${workflow}" ]]; then`,
	} {
		if !strings.Contains(core, fragment) {
			t.Fatalf("managed lint omits workflow validation structure %q", fragment)
		}
	}
	definition := strings.Index(core, "check_workflows() {")
	validation := strings.Index(core, "\nactionlint\n")
	invocation := strings.Index(core, "\ncheck_workflows\n")
	project := strings.Index(core, "project_script='just/project/lint.sh'")
	if validation <= definition || invocation <= validation || project <= invocation {
		t.Fatal("managed lint must call actionlint through the workflow gate before the project script")
	}
}

func TestManagedLintInvalidWorkflowFails(t *testing.T) {
	actionlint, err := exec.LookPath("actionlint")
	if err != nil {
		t.Skip("actionlint is unavailable; workflow structure and stubbed failure propagation are checked independently")
	}
	root := t.TempDir()
	writeManagedLintFixture(t, root, renderSelectedManagedFiles(t, managedRenderConfig(false, nil)))
	writeManagedTestFile(t, root, ".github/workflows/ci.yml", []byte("invalid: [\n"))
	bin := filepath.Join(root, "bin")
	if err := os.MkdirAll(bin, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(actionlint, filepath.Join(bin, "actionlint")); err != nil {
		t.Fatal(err)
	}
	command := exec.CommandContext(t.Context(), "git", "init", root)
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("git init: %v\n%s", err, output)
	}
	output, err := runManagedLint(t, managedLintExecutable(t), root, "lint", bin, filepath.Join(root, "lint.log"), nil)
	if err == nil || !strings.Contains(string(output), "syntax-check") {
		t.Fatalf("invalid workflow output = %q, error = %v, want YAML syntax failure", output, err)
	}
}

func TestCanonicalManagedLintCallsResolvesDirectoryAliases(t *testing.T) {
	root := filepath.Join(t.TempDir(), "project")
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
	alias := filepath.Join(t.TempDir(), "project-alias")
	if err := os.Symlink(root, alias); err != nil {
		t.Fatal(err)
	}

	got := canonicalManagedLintCalls(t, []string{"actionlint|" + alias + "|0|"})
	want := canonicalManagedLintCalls(t, []string{"actionlint|" + root + "|0|"})
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("lint calls = %v, want %v", got, want)
	}
}

func TestManagedLintStopsAtFirstFailure(t *testing.T) {
	just := managedLintExecutable(t)
	for _, test := range []struct {
		name      string
		statusEnv string
		wantCalls []string
	}{
		{name: "actionlint", statusEnv: "TAILOR_ACTIONLINT_STATUS=17", wantCalls: []string{"actionlint"}},
		{name: "golangci-lint", statusEnv: "TAILOR_GOLANGCI_STATUS=19", wantCalls: []string{"actionlint", "go", "golangci-lint"}},
		{name: "govulncheck", statusEnv: "TAILOR_GOVULNCHECK_STATUS=23", wantCalls: []string{"actionlint", "go", "golangci-lint", "golangci-lint", "govulncheck"}},
		{name: "mod-tidy", statusEnv: "TAILOR_GO_STATUS=29", wantCalls: []string{"actionlint", "go"}},
	} {
		t.Run(test.name, func(t *testing.T) {
			root := t.TempDir()
			writeManagedLintFixture(t, root, renderSelectedManagedFiles(t, managedRenderConfig(true, nil)))
			log := filepath.Join(root, "lint.log")
			bin := filepath.Join(root, "bin")
			writeManagedLintStub(t, bin, "actionlint", "TAILOR_ACTIONLINT_STATUS")
			writeManagedLintStub(t, bin, "golangci-lint", "TAILOR_GOLANGCI_STATUS")
			writeManagedLintStub(t, bin, "govulncheck", "TAILOR_GOVULNCHECK_STATUS")
			writeManagedLintStub(t, bin, "go", "TAILOR_GO_STATUS")
			writeManagedLintStub(t, bin, "gocyclo", "TAILOR_UNUSED_STATUS")

			output, err := runManagedLint(t, just, root, "lint", bin, log, []string{test.statusEnv})
			if err == nil {
				t.Fatalf("just lint succeeded after %s failure:\n%s", test.name, output)
			}
			calls := managedLintLog(t, log)
			got := make([]string, 0, len(calls))
			for _, call := range calls {
				got = append(got, strings.Split(call, "|")[0])
			}
			if !reflect.DeepEqual(got, test.wantCalls) {
				t.Fatalf("lint calls = %v, want %v", got, test.wantCalls)
			}
		})
	}
}

func TestManagedLintMissingExecutablesFailVisibly(t *testing.T) {
	just := managedLintExecutable(t)
	for _, test := range []struct {
		name      string
		available []string
		wantTool  string
	}{
		{name: "actionlint", available: []string{"go", "golangci-lint"}, wantTool: "actionlint"},
		{name: "golangci-lint", available: []string{"actionlint", "go"}, wantTool: "golangci-lint"},
		{name: "govulncheck", available: []string{"go", "actionlint", "golangci-lint"}, wantTool: "govulncheck"},
		{name: "mod-tidy", available: []string{"actionlint", "golangci-lint", "govulncheck"}, wantTool: "go"},
	} {
		t.Run(test.name, func(t *testing.T) {
			root := t.TempDir()
			writeManagedLintFixture(t, root, renderSelectedManagedFiles(t, managedRenderConfig(true, nil)))
			log := filepath.Join(root, "lint.log")
			bin := filepath.Join(root, "bin")
			if err := os.MkdirAll(bin, 0o755); err != nil {
				t.Fatal(err)
			}
			for _, name := range test.available {
				writeManagedLintStub(t, bin, name, "TAILOR_UNUSED_STATUS")
			}

			output, err := runManagedLint(t, just, root, "lint", bin, log, nil)
			if err == nil || !strings.Contains(string(output), test.wantTool) {
				t.Fatalf("just lint output = %q, error = %v, want visible %q failure", output, err, test.wantTool)
			}
		})
	}
}

func TestManagedLintAbsentGoRemainsCallableButLeavesAggregate(t *testing.T) {
	just := managedLintExecutable(t)
	root := t.TempDir()
	absent := renderSelectedManagedFiles(t, managedRenderConfig(false, nil))
	enabled := renderSelectedManagedFiles(t, managedRenderConfig(true, nil))
	absent["just/go.just"] = enabled["just/go.just"]
	writeManagedLintFixture(t, root, absent)
	log := filepath.Join(root, "lint.log")
	bin := filepath.Join(root, "bin")
	writeManagedLintStub(t, bin, "actionlint", "TAILOR_ACTIONLINT_STATUS")
	writeManagedLintStub(t, bin, "golangci-lint", "TAILOR_GOLANGCI_STATUS")
	writeManagedLintStub(t, bin, "govulncheck", "TAILOR_GOVULNCHECK_STATUS")
	writeManagedLintStub(t, bin, "go", "TAILOR_GO_STATUS")
	writeManagedLintStub(t, bin, "gocyclo", "TAILOR_UNUSED_STATUS")

	if output, err := runManagedLint(t, just, root, "lint", bin, log, nil); err != nil {
		t.Fatalf("just lint: %v\n%s", err, output)
	}
	if got := managedLintLog(t, log); len(got) != 1 || !strings.HasPrefix(got[0], "actionlint|") {
		t.Fatalf("aggregate calls = %v, want actionlint only", got)
	}
	if output, err := runManagedLint(t, just, root, "lint-go", bin, log, nil); err == nil || !strings.Contains(string(output), "does not contain recipe") {
		t.Fatalf("legacy lint-go remains callable: %v %s", err, output)
	}
	if got := managedLintLog(t, log); len(got) != 1 {
		t.Fatalf("legacy command ran tools: %v", got)
	}
}

func managedLintExecutable(t *testing.T) string {
	t.Helper()
	just, err := exec.LookPath("just")
	if err != nil {
		t.Fatalf("required Just executable is unavailable: %v", err)
	}
	return just
}

func writeManagedLintFixture(t *testing.T, root string, rendered managedRenderedFiles) {
	t.Helper()
	writeManagedJustFixture(t, root, rendered)
	writeManagedTestFile(t, root, ".github/workflows/ci.yml", []byte("on: push\njobs:\n  test:\n    runs-on: ubuntu-latest\n    steps:\n      - run: true\n"))
	writeManagedTestFile(t, root, "justfile", []byte("set shell := [\"/bin/sh\", \"-cu\"]\nimport 'just/loader.just'\n"))
}

func writeManagedLintStub(t *testing.T, bin, name, statusVariable string) {
	t.Helper()
	content := fmt.Sprintf("#!/bin/sh\nprintf '%%s|%%s|%%s|%%s\\n' %s \"$PWD\" \"$#\" \"$*\" >> \"$TAILOR_LINT_LOG\"\nexit \"${%s:-0}\"\n", name, statusVariable)
	writeManagedExecutable(t, filepath.Join(bin, name), content)
}

func runManagedLint(t *testing.T, just, root, recipe, bin, log string, extraEnvironment []string) ([]byte, error) {
	t.Helper()
	if err := os.MkdirAll(bin, 0o755); err != nil {
		t.Fatal(err)
	}
	bash, err := exec.LookPath("bash")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(bash, filepath.Join(bin, "bash")); err != nil && !os.IsExist(err) {
		t.Fatal(err)
	}
	nested := filepath.Join(root, "nested", "work")
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatal(err)
	}
	command := exec.CommandContext(t.Context(), just, "--justfile", filepath.Join(root, "justfile"), recipe) // #nosec G204 -- The executable and fixture are controlled by the test.
	command.Dir = nested
	command.Env = append(managedEnvironmentWithout("BASH_ENV", "ENV", "PATH", "TAILOR_LINT_LOG", "TAILOR_ACTIONLINT_STATUS", "TAILOR_GOLANGCI_STATUS", "TAILOR_GOVULNCHECK_STATUS", "TAILOR_GO_STATUS", "TAILOR_UNUSED_STATUS"), "PATH="+bin, "TAILOR_LINT_LOG="+log)
	command.Env = append(command.Env, extraEnvironment...)
	return command.CombinedOutput()
}

func managedLintLog(t *testing.T, name string) []string {
	t.Helper()
	content, err := os.ReadFile(name)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		t.Fatal(err)
	}
	return strings.Split(strings.TrimSpace(string(content)), "\n")
}

func canonicalManagedLintCalls(t *testing.T, calls []string) []string {
	t.Helper()
	canonical := make([]string, len(calls))
	for index, call := range calls {
		fields := strings.Split(call, "|")
		if len(fields) != 4 {
			t.Fatalf("malformed lint call %q", call)
		}
		directory, err := filepath.EvalSymlinks(fields[1])
		if err != nil {
			t.Fatalf("resolve lint call working directory %q: %v", fields[1], err)
		}
		fields[1] = directory
		canonical[index] = strings.Join(fields, "|")
	}
	return canonical
}
