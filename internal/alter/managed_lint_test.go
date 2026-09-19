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

	output, err := runManagedLint(t, just, root, "lint", bin, log, nil)
	if err != nil {
		t.Fatalf("just lint: %v\n%s", err, output)
	}
	want := canonicalManagedLintCalls(t, []string{
		"actionlint|" + root + "|0",
		"golangci-lint|" + root + "|1",
	})
	if got := canonicalManagedLintCalls(t, managedLintLog(t, log)); !reflect.DeepEqual(got, want) {
		t.Fatalf("lint calls = %v, want %v", got, want)
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

	got := canonicalManagedLintCalls(t, []string{"actionlint|" + alias + "|0"})
	want := canonicalManagedLintCalls(t, []string{"actionlint|" + root + "|0"})
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
		{name: "golangci-lint", statusEnv: "TAILOR_GOLANGCI_STATUS=19", wantCalls: []string{"actionlint", "golangci-lint"}},
	} {
		t.Run(test.name, func(t *testing.T) {
			root := t.TempDir()
			writeManagedLintFixture(t, root, renderSelectedManagedFiles(t, managedRenderConfig(true, nil)))
			log := filepath.Join(root, "lint.log")
			bin := filepath.Join(root, "bin")
			writeManagedLintStub(t, bin, "actionlint", "TAILOR_ACTIONLINT_STATUS")
			writeManagedLintStub(t, bin, "golangci-lint", "TAILOR_GOLANGCI_STATUS")

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
		{name: "actionlint", wantTool: "actionlint"},
		{name: "golangci-lint", available: []string{"actionlint"}, wantTool: "golangci-lint"},
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

	if output, err := runManagedLint(t, just, root, "lint", bin, log, nil); err != nil {
		t.Fatalf("just lint: %v\n%s", err, output)
	}
	if got := managedLintLog(t, log); len(got) != 1 || !strings.HasPrefix(got[0], "actionlint|") {
		t.Fatalf("aggregate calls = %v, want actionlint only", got)
	}
	if output, err := runManagedLint(t, just, root, "lint-go", bin, log, nil); err != nil {
		t.Fatalf("just lint-go: %v\n%s", err, output)
	}
	if got := managedLintLog(t, log); len(got) != 2 || !strings.HasPrefix(got[1], "golangci-lint|") {
		t.Fatalf("standalone calls = %v, want retained lint-go", got)
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
	writeManagedTestFile(t, root, "justfile", []byte("set shell := [\"/bin/sh\", \"-cu\"]\nimport 'just/loader.just'\n"))
}

func writeManagedLintStub(t *testing.T, bin, name, statusVariable string) {
	t.Helper()
	content := fmt.Sprintf("#!/bin/sh\nprintf '%%s|%%s|%%s\\n' %s \"$PWD\" \"$#\" >> \"$TAILOR_LINT_LOG\"\nexit \"${%s:-0}\"\n", name, statusVariable)
	writeManagedExecutable(t, filepath.Join(bin, name), content)
}

func runManagedLint(t *testing.T, just, root, recipe, bin, log string, extraEnvironment []string) ([]byte, error) {
	t.Helper()
	nested := filepath.Join(root, "nested", "work")
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatal(err)
	}
	command := exec.CommandContext(t.Context(), just, "--justfile", filepath.Join(root, "justfile"), recipe) // #nosec G204 -- The executable and fixture are controlled by the test.
	command.Dir = nested
	command.Env = append(managedEnvironmentWithout("PATH", "TAILOR_LINT_LOG", "TAILOR_ACTIONLINT_STATUS", "TAILOR_GOLANGCI_STATUS", "TAILOR_UNUSED_STATUS"), "PATH="+bin, "TAILOR_LINT_LOG="+log)
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
		if len(fields) != 3 {
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
