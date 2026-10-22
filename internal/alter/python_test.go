package alter

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/wimpysworld/tailor/internal/config"
	"github.com/wimpysworld/tailor/internal/swatch"
)

func TestPythonPolicies(t *testing.T) {
	for _, tc := range []struct{ path, body, destination, want string }{
		{"pyproject.toml", "[tool.\"ruff\"]\nline-length=88\n", "ruff.toml", "existing Python policy"},
		{"pyproject.toml", "tool = {basedpyright = {typeCheckingMode='standard'}}\n", "pyrightconfig.json", "existing Python policy"},
		{"pyproject.toml", "[tool.mypy]\nplugins=['custom']\n", "pyrightconfig.json", "adoption required"},
		{"pyproject.toml", "[tool.black]\nline-length=88\n", "ruff.toml", "adoption required"},
		{"nested/pyrightconfig.json", "{}", "pyrightconfig.json", "existing Python policy"},
		{".ruff.toml", "", "ruff.toml", "existing Python policy"},
	} {
		t.Run(tc.path+tc.destination+tc.body, func(t *testing.T) {
			root := testTempDir(t)
			writeManagedTestFile(t, root, tc.path, []byte(tc.body))
			for _, mode := range []ApplyMode{DryRun, Apply, Recut} {
				cfg := &config.Config{Languages: &config.LanguageSettings{Python: new(true)}, Swatches: []config.SwatchEntry{{Path: tc.destination, Alteration: swatch.Always}}}
				results, err := processSwatches(cfg, root, mode, nil, nil)
				if err != nil || len(results) != 1 || !strings.Contains(string(results[0].Reason), tc.want) {
					t.Fatalf("%v %v", results, err)
				}
				if _, err := os.Stat(filepath.Join(root, tc.destination)); !os.IsNotExist(err) {
					t.Fatal("starter shadowed policy")
				}
			}
		})
	}
	root := testTempDir(t)
	writeManagedTestFile(t, root, "pyproject.toml", []byte("[tool.ruff]\n"))
	child := filepath.Join(root, "child")
	if err := os.Mkdir(child, 0o755); err != nil {
		t.Fatal(err)
	}
	if reason, err := PythonPolicyReason(child, "ruff.toml"); err != nil || reason == "" {
		t.Fatalf("ancestor: %s %v", reason, err)
	}
	managedSymlinkOrSkip(t, root, filepath.Join(root, "alias"))
	if _, err := PythonPolicyReason(filepath.Join(root, "alias"), "ruff.toml"); err == nil {
		t.Fatal("accepted unsafe parent")
	}
}

func TestPythonPolicyAncestorAlias(t *testing.T) {
	for _, policy := range []string{"real/.ruff.toml", "real/project/.ruff.toml", "real/project/nested/.ruff.toml"} {
		t.Run(policy, func(t *testing.T) {
			base := testTempDir(t)
			root := filepath.Join(base, "real/project")
			if err := os.MkdirAll(root, 0o755); err != nil {
				t.Fatal(err)
			}
			writeManagedTestFile(t, base, policy, []byte("line-length = 88\n"))
			managedSymlinkOrSkip(t, filepath.Join(base, "real"), filepath.Join(base, "alias"))
			alias := filepath.Join(base, "alias/project")
			before := snapshotManagedTree(t, base)
			want, err := PythonPolicyReason(root, "ruff.toml")
			if err != nil || !strings.Contains(want, filepath.Join(base, policy)) {
				t.Fatalf("physical root: %q, %v", want, err)
			}
			if got, err := PythonPolicyReason(alias, "ruff.toml"); err != nil || got != want {
				t.Fatalf("aliased ancestor: %q, %v; want %q", got, err, want)
			}
			for _, mode := range []ApplyMode{DryRun, Apply, Recut} {
				cfg := &config.Config{Languages: &config.LanguageSettings{Python: new(true)}, Swatches: []config.SwatchEntry{{Path: "ruff.toml", Alteration: swatch.Always}}}
				results, err := processSwatches(cfg, alias, mode, nil, nil)
				if err != nil || len(results) != 1 || string(results[0].Reason) != want {
					t.Fatalf("mode %v: %v, %v", mode, results, err)
				}
			}
			if after := snapshotManagedTree(t, base); !reflect.DeepEqual(before, after) {
				t.Fatalf("policy discovery changed files: before=%v after=%v", before, after)
			}
		})
	}
}

func TestPythonPolicyAliasRejectsLinkedPaths(t *testing.T) {
	for _, linked := range []string{"project", ".ruff.toml", "project/.ruff.toml", "project/nested/.ruff.toml"} {
		t.Run(linked, func(t *testing.T) {
			base := testTempDir(t)
			real := filepath.Join(base, "real")
			if err := os.MkdirAll(filepath.Join(real, "project/nested"), 0o755); err != nil {
				t.Fatal(err)
			}
			managedSymlinkOrSkip(t, real, filepath.Join(base, "alias"))
			root := filepath.Join(base, "alias/project")
			if linked == "project" {
				managedSymlinkOrSkip(t, filepath.Join(real, "project"), filepath.Join(real, "linked-project"))
				root = filepath.Join(base, "alias/linked-project")
			} else {
				writeManagedTestFile(t, base, "policy.toml", []byte("line-length = 88\n"))
				managedSymlinkOrSkip(t, filepath.Join(base, "policy.toml"), filepath.Join(real, linked))
			}
			before := snapshotManagedTree(t, base)
			if _, err := PythonPolicyReason(root, "ruff.toml"); err == nil {
				t.Fatal("accepted linked root or policy")
			}
			if after := snapshotManagedTree(t, base); !reflect.DeepEqual(before, after) {
				t.Fatal("rejected policy changed files")
			}
		})
	}
}

func TestPythonRecipeContract(t *testing.T) {
	for _, operation := range []string{"setup", "build", "test", "check", "correct"} {
		t.Run(operation, func(t *testing.T) {
			root, bin, log := qualityFixture(t, false)
			cfg := &config.Config{Languages: &config.LanguageSettings{Python: new(true)}}
			writeManagedLintFixture(t, root, renderSelectedManagedFiles(t, cfg))
			for _, tool := range []string{"tailor-python", "uv", "ruff", "basedpyright"} {
				writeManagedExecutable(t, filepath.Join(bin, tool), "#!/bin/sh\nif [ \"$1\" = - ]; then cat >/dev/null; exit 0; fi\ncall='"+tool+"'\ncall=\"$call $*\"\nprintf '%s\\n' \"$call\" >> \"$CALLS\"\n[ \"$call\" != \"$FAIL_CALL\" ]\n")
			}
			foreign := filepath.Join(root, "foreign/bin")
			writeManagedExecutable(t, filepath.Join(foreign, "python3"), "#!/bin/sh\necho foreign-python-selected >&2\nexit 99\n")
			t.Setenv("PATH", foreign+":"+os.Getenv("PATH"))
			t.Setenv("VIRTUAL_ENV", filepath.Dir(foreign))
			writeManagedExecutable(t, filepath.Join(root, ".venv/bin/python"), "#!/bin/sh\nprintf 'project-python %s\\n' \"$*\" >> \"$CALLS\"\n")
			args := []string{operation}
			if operation == "check" || operation == "correct" {
				args = []string{"lint", operation}
			}
			before := snapshotManagedTree(t, root)
			output, err := qualityRun(t, root, bin, log, "", args...)
			if err != nil {
				t.Fatalf("%v %s", err, output)
			}
			calls := strings.Join(managedLintLog(t, log), "\n")
			for _, bad := range []string{"uv run", "uv build --locked", "--unsafe-fixes", "--generatebaseline"} {
				if strings.Contains(calls, bad) {
					t.Fatalf("unsafe command %s", calls)
				}
			}
			if !strings.Contains(calls, "uv lock --check") {
				t.Fatal(calls)
			}
			if operation == "setup" {
				if !strings.Contains(calls, "uv sync --locked --python "+filepath.Join(bin, "tailor-python")+" --no-python-downloads") {
					t.Fatal(calls)
				}
			} else if !strings.Contains(calls, "uv sync --check --locked --python "+filepath.Join(root, ".venv/bin/python")) {
				t.Fatal(calls)
			}
			if operation == "test" && !strings.Contains(calls, "project-python -m pytest") {
				t.Fatal(calls)
			}
			if operation == "check" || operation == "correct" {
				for _, want := range []string{"ruff format --check .", "ruff check --no-fix .", "--baselinemode=lock", "--no-emit-project --no-hashes", "--no-deps --disable-pip --strict"} {
					if !strings.Contains(calls, want) {
						t.Fatal(calls)
					}
				}
			}
			after := snapshotManagedTree(t, root)
			delete(after, "calls")
			if !reflect.DeepEqual(before, after) {
				t.Fatalf("quality changed project files: before=%v after=%v", before, after)
			}
			if output, err := qualityRun(t, root, bin, log, "uv lock --check", args...); err == nil {
				t.Fatalf("ignored stale lock: %s", output)
			}
			writeManagedTestFile(t, root, "calls", nil)
			writeManagedExecutable(t, filepath.Join(bin, "tailor-python"), "#!/bin/sh\ncat >/dev/null\necho 'marker environments differ' >&2\nexit 1\n")
			if output, err := qualityRun(t, root, bin, log, "", args...); err == nil {
				t.Fatalf("ignored preflight failure: %s", output)
			}
			if calls := strings.TrimSpace(strings.Join(managedLintLog(t, log), "\n")); calls != "" {
				t.Fatalf("commands ran after preflight failure: %v", calls)
			}
		})
	}
}
