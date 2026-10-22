package alter

import (
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/wimpysworld/tailor/internal/config"
)

func rustFixture(t *testing.T, goEnabled bool) (string, string, string) {
	t.Helper()
	root, bin, log := qualityFixture(t, goEnabled)
	cfg := managedRenderConfig(goEnabled, nil)
	if cfg.Languages == nil {
		cfg.Languages = &config.LanguageSettings{}
	}
	cfg.Languages.Rust = new(true)
	writeManagedLintFixture(t, root, renderSelectedManagedFiles(t, cfg))
	writeManagedTestFile(t, root, "Cargo.toml", []byte("[workspace]\nmembers = []\n"))
	writeManagedTestFile(t, root, "Cargo.lock", []byte("version = 4\n"))
	for _, name := range []string{"cargo", "rustc", "rustfmt", "cargo-fmt", "cargo-clippy", "clippy-driver", "cargo-audit"} {
		writeManagedExecutable(t, filepath.Join(bin, name), "#!/bin/sh\ncall='"+name+"'\nif [ $# -gt 0 ]; then call=\"$call $*\"; fi\nprintf '%s\\n' \"$call\" >> \"$CALLS\"\n[ \"$call\" != \"$FAIL_CALL\" ]\n")
	}
	return root, bin, log
}

func TestRustQuality(t *testing.T) {
	checks := []string{"cargo fmt --all -- --check", "cargo clippy --workspace --all-targets --locked -- -D warnings", "cargo audit --file Cargo.lock"}
	for _, operation := range []string{"build", "test", "check", "correct", "setup"} {
		t.Run(operation, func(t *testing.T) {
			root, bin, log := rustFixture(t, false)
			args := []string{operation}
			var want []string
			switch operation {
			case "build", "test":
				want = []string{"cargo " + operation + " --workspace --locked"}
			case "check", "correct":
				args = []string{"lint", operation}
				want = []string{"actionlint"}
				if operation == "correct" {
					want = append(want, "cargo fmt --all")
				}
				want = append(want, checks...)
			}
			before := snapshotManagedTree(t, root)
			if output, err := qualityRun(t, root, bin, log, "", args...); err != nil {
				t.Fatalf("%v: %v\n%s", args, err, output)
			}
			if got := managedLintLog(t, log); !reflect.DeepEqual(got, want) {
				t.Fatalf("calls %q, want %q", got, want)
			}
			after := snapshotManagedTree(t, root)
			delete(after, "calls")
			if !reflect.DeepEqual(before, after) {
				t.Fatal("quality commands changed project files or lockfile")
			}
		})
	}
	for _, fail := range append([]string{"cargo fmt --all"}, checks...) {
		t.Run("failure "+fail, func(t *testing.T) {
			root, bin, log := rustFixture(t, false)
			writeManagedTestFile(t, root, "just/project/lint.sh", []byte("exit 99\n"))
			if output, err := qualityRun(t, root, bin, log, fail, "lint", "correct"); err == nil {
				t.Fatalf("failure ignored: %s", output)
			}
			calls := managedLintLog(t, log)
			if calls[len(calls)-1] != fail {
				t.Fatalf("continued after failure: %q", calls)
			}
		})
	}
}

func TestRustMissingTools(t *testing.T) {
	for _, tool := range []string{"cargo", "rustc", "rustfmt", "cargo-fmt", "cargo-clippy", "clippy-driver", "cargo-audit"} {
		t.Run(tool, func(t *testing.T) {
			root, bin, log := rustFixture(t, true)
			for _, name := range []string{"bash", "just"} {
				path, err := exec.LookPath(name)
				if err != nil {
					t.Fatal(err)
				}
				managedSymlinkOrSkip(t, path, filepath.Join(bin, name))
			}
			if err := os.Remove(filepath.Join(bin, tool)); err != nil {
				t.Fatal(err)
			}
			t.Setenv("PATH", bin)
			before := snapshotManagedTree(t, root)
			output, err := qualityRun(t, root, bin, log, "", "lint", "correct")
			if err == nil || !strings.Contains(string(output), "Rust requires "+tool+" on PATH") {
				t.Fatalf("missing tool: %v %s", err, output)
			}
			if len(managedLintLog(t, log)) != 0 || !reflect.DeepEqual(before, snapshotManagedTree(t, root)) {
				t.Fatal("missing tool ran commands or changed files")
			}
		})
	}
}

func TestRustInputs(t *testing.T) {
	for _, name := range []string{"Cargo.toml", "Cargo.lock"} {
		for _, kind := range []string{"missing", "directory", "symlink"} {
			t.Run(name+kind, func(t *testing.T) {
				root, bin, log := rustFixture(t, true)
				path := filepath.Join(root, name)
				if err := os.Remove(path); err != nil {
					t.Fatal(err)
				}
				switch kind {
				case "directory":
					if err := os.Mkdir(path, 0o755); err != nil {
						t.Fatal(err)
					}
				case "symlink":
					managedSymlinkOrSkip(t, "missing", path)
				}
				before := snapshotManagedTree(t, root)
				output, err := qualityRun(t, root, bin, log, "", "lint", "correct")
				want := "Rust requires a regular root " + name + ", not a symlink or non-regular file"
				if kind == "missing" {
					want = "Rust requires root " + name + ", but it is missing"
				}
				if err == nil || !strings.Contains(string(output), want) || len(managedLintLog(t, log)) != 0 {
					t.Fatalf("unsafe input reached commands or wrong diagnostic: %v %s", err, output)
				}
				if !reflect.DeepEqual(before, snapshotManagedTree(t, root)) {
					t.Fatal("unsafe input changed files")
				}
			})
		}
	}
}

func TestRustUnsafeParent(t *testing.T) {
	root, _, _ := rustFixture(t, false)
	parent := filepath.Join(t.TempDir(), "linked")
	managedSymlinkOrSkip(t, filepath.Dir(root), parent)
	logical := filepath.Join(parent, filepath.Base(root))
	body, err := managedQualityBody("rust-preflight")
	if err != nil {
		t.Fatal(err)
	}
	body = []byte(strings.ReplaceAll(string(body), "[[TAILOR_RUST_TOOLS]]", ""))
	command := exec.CommandContext(t.Context(), "bash")
	command.Stdin = strings.NewReader(string(body))
	command.Dir = root
	command.Env = append(managedEnvironmentWithout("PWD", "BASH_ENV", "ENV"), "PWD="+logical)
	before := snapshotManagedTree(t, root)
	output, err := command.CombinedOutput()
	if err == nil || !strings.Contains(string(output), "Rust project parent "+parent) {
		t.Fatalf("unsafe parent: %v %s", err, output)
	}
	if !reflect.DeepEqual(before, snapshotManagedTree(t, root)) {
		t.Fatal("unsafe parent changed files")
	}
}

func TestRustWorkspaceOutputsAndFailures(t *testing.T) {
	for _, manifest := range []string{
		"[package]\nname = 'library'\nversion = '0.1.0'\n[lib]\npath = 'src/lib.rs'\n",
		"[workspace]\nmembers = ['library', 'commands']\ndefault-members = ['library']\n",
	} {
		for _, operation := range []string{"build", "test"} {
			for _, fail := range []bool{false, true} {
				root, bin, log := rustFixture(t, false)
				writeManagedTestFile(t, root, "Cargo.toml", []byte(manifest))
				writeManagedTestFile(t, root, "library/Cargo.toml", []byte("[package]\nname = 'library'\nversion = '0.1.0'\n"))
				writeManagedTestFile(t, root, "library/src/lib.rs", []byte("pub fn library() {}\n"))
				writeManagedTestFile(t, root, "commands/Cargo.toml", []byte("[package]\nname = 'commands'\nversion = '0.1.0'\n[[bin]]\nname = 'one'\npath = 'one.rs'\n[[bin]]\nname = 'two'\npath = 'two.rs'\n"))
				for _, path := range []string{"commands/one.rs", "commands/two.rs", "src/lib.rs"} {
					writeManagedTestFile(t, root, path, []byte("fn main() {}\n"))
				}
				call := "cargo " + operation + " --workspace --locked"
				failure := ""
				if fail {
					failure = call
				}
				before := snapshotManagedTree(t, root)
				output, err := qualityRun(t, root, bin, log, failure, operation)
				if (err != nil) != fail {
					t.Fatalf("%s failure=%t: %v %s", operation, fail, err, output)
				}
				if got := managedLintLog(t, log); !reflect.DeepEqual(got, []string{call}) {
					t.Fatalf("workspace calls: %q", got)
				}
				after := snapshotManagedTree(t, root)
				delete(after, "calls")
				if !reflect.DeepEqual(before, after) {
					t.Fatal("workspace command changed files or copied outputs")
				}
			}
		}
	}
}

func TestRustDisabledQuality(t *testing.T) {
	for _, state := range []*bool{nil, new(false)} {
		for _, operation := range []string{"build", "test", "lint"} {
			root, bin, log := rustFixture(t, false)
			cfg := &config.Config{Languages: &config.LanguageSettings{Rust: state}}
			writeManagedLintFixture(t, root, renderSelectedManagedFiles(t, cfg))
			if err := os.Remove(filepath.Join(root, "Cargo.lock")); err != nil {
				t.Fatal(err)
			}
			before := snapshotManagedTree(t, root)
			output, err := qualityRun(t, root, bin, log, "", operation)
			if err != nil {
				t.Fatalf("disabled Rust: %v %s", err, output)
			}
			var want []string
			if operation == "lint" {
				want = []string{"actionlint"}
			}
			if got := managedLintLog(t, log); !reflect.DeepEqual(got, want) {
				t.Fatalf("disabled calls: %q", got)
			}
			after := snapshotManagedTree(t, root)
			delete(after, "calls")
			if !reflect.DeepEqual(before, after) {
				t.Fatal("disabled Rust changed files")
			}
		}
	}
}

func TestRustFragmentOwnership(t *testing.T) {
	for _, path := range []string{"just/rust.just", "nix/rust.nix"} {
		for _, owned := range []bool{false, true} {
			for _, state := range []*bool{nil, new(false), new(true)} {
				dir := t.TempDir()
				content := []byte("user content\n")
				if owned {
					content = managedContent(path, "old")
				}
				writeManagedTestFile(t, dir, path, content)
				writeManagedTestFile(t, dir, "Cargo.lock", []byte("keep lock\n"))
				cfg := &config.Config{Languages: &config.LanguageSettings{Rust: state}}
				before := snapshotManagedTree(t, dir)
				execution, err := prepareManagedExecution(cfg, dir, func(selections []managedSelection) (managedRenderedFiles, error) {
					return renderManagedFiles(cfg, selections)
				})
				if state != nil && !owned {
					if err == nil || !strings.Contains(err.Error(), path) {
						t.Fatalf("unowned fragment: %v", err)
					}
					if !reflect.DeepEqual(before, snapshotManagedTree(t, dir)) {
						t.Fatal("ownership failure changed files")
					}
					continue
				}
				if err != nil {
					t.Fatal(err)
				}
				if _, err := applyManagedFiles(dir, execution.plan); err != nil {
					t.Fatal(err)
				}
				switch {
				case state == nil:
					assertManagedBytes(t, filepath.Join(dir, path), content)
				case !*state:
					assertManagedMissing(t, filepath.Join(dir, path))
				default:
					assertManagedBytes(t, filepath.Join(dir, path), renderSelectedManagedFiles(t, cfg)[path])
				}
				assertManagedFileContent(t, filepath.Join(dir, "Cargo.lock"), "keep lock\n")
			}
		}
	}
}

func TestRustMixedOrderAndLifecycle(t *testing.T) {
	root, bin, log := rustFixture(t, true)
	writeManagedTestFile(t, root, "just/project/test.sh", []byte("printf 'project\\n' >> \"$CALLS\"\n"))
	if output, err := qualityRun(t, root, bin, log, "", "test"); err != nil {
		t.Fatalf("%v %s", err, output)
	}
	want := []string{"go vet ./...", "go test -cover -covermode=atomic ./...", "cargo test --workspace --locked", "project"}
	if got := managedLintLog(t, log); !reflect.DeepEqual(got, want) {
		t.Fatalf("calls %q, want %q", got, want)
	}
	for _, value := range []*bool{nil, new(false), new(true)} {
		cfg := &config.Config{Languages: &config.LanguageSettings{Rust: value}}
		selected, err := selectManagedFiles(cfg)
		if err != nil {
			t.Fatal(err)
		}
		count := 0
		for _, selection := range selected {
			if selection.Entry.Capability == managedCapabilityRust {
				count++
				if value == nil || selection.Enabled != *value {
					t.Fatal("incorrect Rust lifecycle")
				}
			}
		}
		if (value == nil && count != 0) || (value != nil && count != 2) {
			t.Fatalf("Rust selections: %d", count)
		}
	}
}
