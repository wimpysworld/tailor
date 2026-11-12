package alter

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"testing"

	"github.com/wimpysworld/tailor/internal/config"
	"gopkg.in/yaml.v3"
)

func sourceFixture(t *testing.T, declaration string) (string, string, string) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("managed just recipes require unix shell")
	}
	root, bin := testTempDir(t), testTempDir(t)
	log := filepath.Join(bin, "calls")
	cfg := managedRenderConfig(false, nil)
	if err := yaml.Unmarshal([]byte("languages: {"+declaration+"}\n"), cfg); err != nil {
		t.Fatal(err)
	}
	writeManagedLintFixture(t, root, renderSelectedManagedFiles(t, cfg))
	command := exec.CommandContext(t.Context(), "git", "init", "--quiet", root)
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("git init: %v %s", err, output)
	}
	for _, tool := range []string{"node", "oxlint", "prettier", "shellcheck", "shfmt", "actionlint"} {
		writeManagedExecutable(t, filepath.Join(bin, tool), "#!/usr/bin/env bash\nset -eu\ntool="+tool+"\nprintf '%s\\0' \"$tool\" \"$@\" '' >> \"$CALLS\"\n[[ \"$tool $*\" != \"$FAIL_CALL\" ]]\n")
	}
	return root, bin, log
}

func sourceCalls(t *testing.T, log string) []string {
	t.Helper()
	content, err := os.ReadFile(log)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		t.Fatal(err)
	}
	return strings.Split(strings.TrimSuffix(string(content), "\x00"), "\x00")
}

func TestSourceLintSelectionAndModes(t *testing.T) {
	for _, mode := range []string{"", "check", "correct"} {
		t.Run(mode, func(t *testing.T) {
			root, bin, log := sourceFixture(t, "shell: true, typescript: true")
			js := []string{"-entry.js", ".hidden.js", "config/keep.js", "src/config/settings.ts", "src/configs/settings.ts", "src/a space.ts", "src/glob[1].tsx", "src/line\nbreak.mjs", "src/module.cjs", "src/module.cts", "src/module.jsx", "src/module.mts"}
			sh := []string{"-check.sh", ".github/scripts/check.sh", ".tools/script.sh", "scripts/a space.bash", "scripts/glob[1].sh", "scripts/line\nbreak.sh", "scripts/nonexec"}
			sort.Strings(js)
			sort.Strings(sh)
			for _, name := range append(append([]string{}, js...), sh...) {
				writeManagedTestFile(t, root, name, []byte("#!/usr/bin/env bash\nprintf 'source\\n'\n"))
			}
			for _, name := range []string{"vendor/keep.js", "node_modules/keep.ts", "dist/keep.js", "generated/keep.sh", "secrets/keep.js", ".cache/keep.js", ".prettierrc.js", ".eslintrc.cjs", ".babelrc.js", ".env.local.js", "vite.config.ts", "types.d.ts", "bundle.min.js", "a.generated.ts", "data.json", "fragment", "Makefile", "fish.fish", "power.ps1", "package.json", "package-lock.json", "ignored.ts", "tracked.sh"} {
				writeManagedTestFile(t, root, name, []byte("preserve\n"))
			}
			writeManagedTestFile(t, root, "marked.js", []byte("// Code generated. DO NOT EDIT.\n"))
			writeManagedTestFile(t, root, "wrong.sh", []byte("#!/usr/bin/env fish\n"))
			command := exec.CommandContext(t.Context(), "git", "-C", root, "add", "tracked.sh", "vendor/keep.js", "generated/keep.sh", "vite.config.ts")
			if output, err := command.CombinedOutput(); err != nil {
				t.Fatalf("git add: %v %s", err, output)
			}
			writeManagedTestFile(t, root, ".gitignore", []byte("ignored.ts\ntracked.sh\n"))
			before := snapshotManagedTree(t, root)
			args := []string{"lint"}
			if mode != "" {
				args = append(args, mode)
			}
			if output, err := qualityRun(t, root, bin, log, "", args...); err != nil {
				t.Fatalf("lint: %v %s", err, output)
			}
			want := []string{"actionlint", ""}
			add := func(tool string, flags []string, files []string) {
				want = append(want, tool)
				want = append(want, flags...)
				for _, file := range files {
					want = append(want, "./"+file)
				}
				want = append(want, "")
			}
			oxFlags := []string{"--deny-warnings", "--no-error-on-unmatched-pattern", "--"}
			if mode == "correct" {
				add("prettier", []string{"--write", "--"}, js)
			}
			add("oxlint", oxFlags, js)
			add("prettier", []string{"--check", "--"}, js)
			if mode == "correct" {
				add("shfmt", []string{"--apply-ignore", "-w", "--"}, sh)
			}
			add("shellcheck", []string{"--"}, sh)
			add("shfmt", []string{"--apply-ignore", "-d", "--"}, sh)
			if got := sourceCalls(t, log); !reflect.DeepEqual(got, want) {
				t.Fatalf("calls = %q, want %q", got, want)
			}
			if !reflect.DeepEqual(before, snapshotManagedTree(t, root)) {
				t.Fatal("stubbed lint changed project files")
			}
		})
	}
}

func TestSourceLintFailures(t *testing.T) {
	for _, fail := range []string{
		"actionlint ",
		"oxlint --deny-warnings --no-error-on-unmatched-pattern -- ./a.js",
		"prettier --write -- ./a.js", "prettier --check -- ./a.js",
		"shellcheck -- ./a.sh", "shfmt --apply-ignore -w -- ./a.sh", "shfmt --apply-ignore -d -- ./a.sh",
	} {
		t.Run(fail, func(t *testing.T) {
			root, bin, log := sourceFixture(t, "javascript: true, shell: true")
			writeManagedTestFile(t, root, "a.js", []byte("const a = 1;\n"))
			writeManagedTestFile(t, root, "a.sh", []byte("echo hello\n"))
			writeManagedTestFile(t, root, "just/project/lint.sh", []byte("exit 99\n"))
			writeManagedTestFile(t, root, ".gitignore", []byte("just/project/\n"))
			if output, err := qualityRun(t, root, bin, log, fail, "lint", "correct"); err == nil || strings.Contains(string(output), "exit code 99") {
				t.Fatalf("failure did not stop lint: %v %s", err, output)
			}
			calls := sourceCalls(t, log)
			last := len(calls) - 2
			for last > 0 && calls[last-1] != "" {
				last--
			}
			if got := strings.Join(calls[last:len(calls)-1], " "); strings.TrimSpace(got) != strings.TrimSpace(fail) {
				t.Fatalf("continued after failure: %q", calls)
			}
		})
	}
}

func TestSourceRecheckAndProjectHook(t *testing.T) {
	for _, tool := range []string{"", "oxlint", "shellcheck"} {
		t.Run("recheck "+tool, func(t *testing.T) {
			root, bin, log := sourceFixture(t, "javascript: true, shell: true")
			writeManagedTestFile(t, root, "a.js", []byte("const a = 1;\n"))
			writeManagedTestFile(t, root, "a.sh", []byte("echo hello\n"))
			writeManagedTestFile(t, root, "just/project/lint.sh", []byte("printf '%s\\0' project \"$1\" '' >> \"$CALLS\"\n"))
			writeManagedTestFile(t, root, ".gitignore", []byte("just/project/\n"))
			if tool != "" {
				flag := "--write"
				if tool == "shellcheck" {
					flag = "-w"
				}
				writeManagedExecutable(t, filepath.Join(bin, tool), "#!/usr/bin/env bash\nset -eu\nprintf '%s\\0' "+tool+" \"$@\" '' >> \"$CALLS\"\ngrep -aq -- '"+flag+"' \"$CALLS\" || exit 18\nexit 17\n")
			}
			output, err := qualityRun(t, root, bin, log, "", "lint", "correct")
			if (err != nil) != (tool != "") {
				t.Fatalf("recheck: %v %s", err, output)
			}
			calls := sourceCalls(t, log)
			if tool == "" {
				if !reflect.DeepEqual(calls[len(calls)-3:], []string{"project", "correct", ""}) {
					t.Fatalf("hook did not run last: %q", calls)
				}
			} else {
				last := len(calls) - 2
				for last > 0 && calls[last-1] != "" {
					last--
				}
				formatter := "prettier"
				if tool == "shellcheck" {
					formatter = "shfmt"
				}
				if !strings.Contains(strings.Join(calls, " "), formatter) || strings.Count(strings.Join(calls, " "), tool) != 1 {
					t.Fatalf("formatter did not precede the single check: %q", calls)
				}
				if calls[last] != tool || strings.Contains(strings.Join(calls, " "), "project") {
					t.Fatalf("continued after recheck: %q", calls)
				}
			}
		})
	}
	cfg := &config.Config{Languages: &config.LanguageSettings{Go: new(true), JavaScript: new(true), Python: new(true), Rust: new(true), Shell: new(true)}}
	core := string(renderSelectedManagedFiles(t, cfg)["just/tailor.just"])
	_, lint, found := strings.Cut(core, "check_workflows() {")
	if !found {
		t.Fatal("missing workflow lint body")
	}
	previous := -1
	for _, command := range []string{"check_workflows\n", "go mod tidy -diff", "oxlint --deny-warnings", "ruff check", "cargo clippy", "shellcheck --", "project_script='just/project/lint.sh'"} {
		index := strings.Index(lint, command)
		if index <= previous {
			t.Fatalf("incorrect lexical order at %q", command)
		}
		previous = index
	}
}

func TestSourceMissingToolsAndNonLintOperations(t *testing.T) {
	for _, tool := range []string{"node", "oxlint", "prettier", "shellcheck", "shfmt"} {
		t.Run(tool, func(t *testing.T) {
			root, bin, log := sourceFixture(t, "javascript: true, shell: true")
			for _, name := range []string{"bash", "just", "git", "mktemp", "rm", "grep"} {
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
			if output, err := qualityRun(t, root, bin, log, "", "lint", "correct"); err == nil || !strings.Contains(string(output), "requires "+tool+" on PATH") {
				t.Fatalf("missing tool: %v %s", err, output)
			}
			if len(sourceCalls(t, log)) != 0 {
				t.Fatal("missing tool ran checks")
			}
			for _, operation := range []string{"build", "test", "setup"} {
				if output, err := qualityRun(t, root, bin, log, "", operation); err != nil {
					t.Fatalf("%s: %v %s", operation, err, output)
				}
			}
		})
	}
}

func TestSourceGeneratedHeaders(t *testing.T) {
	for _, tc := range []struct {
		name      string
		body      string
		generated bool
	}{
		{"quoted.js", "export const message = \"Please DO NOT EDIT your profile\";\n", false},
		{"late.js", "export const value = 1;\n// DO NOT EDIT\n", false},
		{"after-comment.js", "/* header */ export const message = \"DO NOT EDIT\";\n", false},
		{"line.js", "// Code generated. DO NOT EDIT.\nexport const value = 1;\n", true},
		{"block.js", "/*\n * @generated\n */\nexport const value = 1;\n", true},
		{"no-newline.js", "// @generated", true},
		{"long-comment.js", "// @generated " + strings.Repeat("header ", 100000) + "\n", true},
		{"mixed-case.js", "// CoDe GeNeRaTeD\nexport const value = 1;\n", true},
		{"mixed-block.js", "/* @GeNeRaTeD */\nexport const value = 1;\n", true},
		{"mixed-shell.sh", "#!/bin/sh\n# dO nOt EdIt\necho hello\n", true},
		{"line-50.js", strings.Repeat("// header\n", 49) + "// @generated\n", true},
		{"line-51.js", strings.Repeat("// header\n", 50) + "// @generated\n", false},
		{"script.sh", "#!/bin/sh\n#DO NOT EDIT\necho hello\n", true},
		{"late.sh", "#!/bin/sh\necho hello\n# @generated\n", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root, bin, log := sourceFixture(t, "javascript: true, shell: true")
			writeManagedTestFile(t, root, tc.name, []byte(tc.body))
			writeManagedTestFile(t, root, "control.js", []byte("// Authored source\nexport const value = 1;\n"))
			if output, err := qualityRun(t, root, bin, log, "", "lint", "correct"); err != nil {
				t.Fatalf("lint: %v %s", err, output)
			}
			calls := "\x00" + strings.Join(sourceCalls(t, log), "\x00") + "\x00"
			if !strings.Contains(calls, "\x00./control.js\x00") {
				t.Fatalf("eligible control did not reach tools: %q", calls)
			}
			selected := strings.Contains(calls, "\x00./"+tc.name+"\x00")
			if selected == tc.generated {
				t.Fatalf("selected = %v, generated = %v", selected, tc.generated)
			}
		})
	}
}

func TestSourceRepositoryDirectories(t *testing.T) {
	root, bin, log := sourceFixture(t, "javascript: true, shell: true")
	for _, name := range []string{"engine", "module.js", "line\nbreak.ts"} {
		writeManagedTestFile(t, root, name+"/external.js", []byte("external\n"))
		command := exec.CommandContext(t.Context(), "git", "-C", root, "update-index", "--add", "--cacheinfo", "160000", "1111111111111111111111111111111111111111", name)
		if output, err := command.CombinedOutput(); err != nil {
			t.Fatalf("gitlink: %v %s", err, output)
		}
	}
	for _, name := range []string{"nested", "nested.js"} {
		dir := filepath.Join(root, name)
		command := exec.CommandContext(t.Context(), "git", "init", "--quiet", dir)
		if output, err := command.CombinedOutput(); err != nil {
			t.Fatalf("nested repository: %v %s", err, output)
		}
		writeManagedTestFile(t, dir, "external.sh", []byte("echo external\n"))
	}
	writeManagedTestFile(t, root, "a.js", []byte("const a = 1;\n"))
	writeManagedTestFile(t, root, "a.sh", []byte("echo hello\n"))
	if output, err := qualityRun(t, root, bin, log, "", "lint", "correct"); err != nil {
		t.Fatalf("lint: %v %s", err, output)
	}
	calls := sourceCalls(t, log)
	for _, call := range calls {
		if strings.HasPrefix(call, "./") && call != "./a.js" && call != "./a.sh" {
			t.Fatalf("external repository reached tools: %q", calls)
		}
	}
	if !strings.Contains(strings.Join(calls, " "), "./a.js") || !strings.Contains(strings.Join(calls, " "), "./a.sh") {
		t.Fatalf("authored sources did not reach tools: %q", calls)
	}
}

func TestSourceLogicalWorkspacePath(t *testing.T) {
	root, bin, log := sourceFixture(t, "javascript: true, shell: true")
	alias := filepath.Join(testTempDir(t), "alias")
	managedSymlinkOrSkip(t, filepath.Dir(root), alias)
	logicalRoot := filepath.Join(alias, filepath.Base(root))
	t.Setenv("PWD", logicalRoot)
	t.Setenv("EXPECTED_PWD", logicalRoot)
	for _, name := range []string{"src/a.js", "src/a.sh"} {
		writeManagedTestFile(t, root, name, []byte("source\n"))
	}
	for _, tool := range []string{"prettier", "shfmt"} {
		writeManagedExecutable(t, filepath.Join(bin, tool), "#!/usr/bin/env bash\nset -eu\n[[ \"$PWD\" == \"$EXPECTED_PWD\" ]]\nprintf '%s\\0' \"$PWD\" '' >> \"$CALLS\"\n")
	}
	if output, err := qualityRun(t, logicalRoot, bin, log, "", "lint", "correct"); err != nil {
		t.Fatalf("logical workspace: %v %s", err, output)
	}
	if count := strings.Count(strings.Join(sourceCalls(t, log), "\x00"), logicalRoot); count != 4 {
		t.Fatalf("formatter calls through logical PWD = %d, want 4", count)
	}
}

func TestSourceUnsafePaths(t *testing.T) {
	for _, kind := range []string{"symlink", "parent", "directory"} {
		t.Run(kind, func(t *testing.T) {
			root, bin, log := sourceFixture(t, "javascript: true, shell: true")
			writeManagedTestFile(t, root, "src/a.js", []byte("const a = 1;\n"))
			command := exec.CommandContext(t.Context(), "git", "-C", root, "add", "src/a.js")
			if output, err := command.CombinedOutput(); err != nil {
				t.Fatalf("git add: %v %s", err, output)
			}
			destination := filepath.Join(root, "src/a.js")
			if kind == "parent" {
				destination = filepath.Join(root, "src")
			}
			if err := os.Rename(destination, destination+".saved"); err != nil {
				t.Fatal(err)
			}
			if kind == "directory" {
				if err := os.Mkdir(destination, 0o755); err != nil {
					t.Fatal(err)
				}
			} else {
				managedSymlinkOrSkip(t, destination+".saved", destination)
			}
			before := snapshotManagedTree(t, root)
			if output, err := qualityRun(t, root, bin, log, "", "lint", "correct"); err == nil || !strings.Contains(string(output), "must be a") {
				t.Fatalf("unsafe path: %v %s", err, output)
			}
			if len(sourceCalls(t, log)) != 0 || !reflect.DeepEqual(before, snapshotManagedTree(t, root)) {
				t.Fatal("unsafe path reached tools or changed files")
			}
		})
	}
}

func TestSourceEmptyAndDisabled(t *testing.T) {
	for _, declaration := range []string{"javascript: true, shell: true", "javascript: false, shell: false", ""} {
		root, bin, log := sourceFixture(t, declaration)
		output, err := qualityRun(t, root, bin, log, "", "lint")
		if err != nil {
			t.Fatalf("lint: %v %s", err, output)
		}
		if got := sourceCalls(t, log); !reflect.DeepEqual(got, []string{"actionlint", ""}) {
			t.Fatalf("empty calls: %q", got)
		}
		if strings.Contains(declaration, "true") && (!strings.Contains(string(output), "No eligible JavaScript/TypeScript") || !strings.Contains(string(output), "Shell checks skipped")) {
			t.Fatalf("missing empty guidance: %s", output)
		}
	}
}

func TestSourceAliasAndFragmentLifecycle(t *testing.T) {
	var canonical managedRenderedFiles
	for _, key := range []string{"javascript", "typescript", "shell"} {
		for _, value := range []string{"", "false", "true"} {
			cfg := &config.Config{}
			if value != "" {
				if err := yaml.Unmarshal([]byte("languages: {"+key+": "+value+"}"), cfg); err != nil {
					t.Fatal(err)
				}
			}
			rendered := renderSelectedManagedFiles(t, cfg)
			name := key
			if name == "typescript" {
				name = "javascript"
			}
			if value == "true" && key == "javascript" {
				canonical = rendered
			}
			if value == "true" && key == "typescript" && !reflect.DeepEqual(canonical, rendered) {
				t.Fatal("alias changes rendering")
			}
			for _, path := range []string{"just/" + name + ".just", "nix/" + name + ".nix"} {
				for _, owned := range []bool{false, true} {
					root := t.TempDir()
					content := []byte("user content\n")
					if owned {
						content = managedContent(path, "old")
					}
					writeManagedTestFile(t, root, path, content)
					execution, err := prepareManagedExecution(cfg, root, func(selections []managedSelection) (managedRenderedFiles, error) {
						return renderManagedFiles(cfg, selections)
					})
					if value != "" && !owned {
						if err == nil {
							t.Fatal("accepted unowned fragment")
						}
						continue
					}
					if err != nil {
						t.Fatal(err)
					}
					if _, err := applyManagedFiles(root, execution.plan); err != nil {
						t.Fatal(err)
					}
					switch value {
					case "":
						assertManagedBytes(t, filepath.Join(root, path), content)
					case "false":
						assertManagedMissing(t, filepath.Join(root, path))
					case "true":
						assertManagedBytes(t, filepath.Join(root, path), rendered[path])
					}
				}
			}
		}
	}
}

func TestSourceNixPackages(t *testing.T) {
	nix, err := exec.LookPath("nix")
	if err != nil {
		t.Skip("nix is required")
	}
	cfg := &config.Config{Languages: &config.LanguageSettings{JavaScript: new(true), Shell: new(true)}}
	root := t.TempDir()
	for name, content := range renderSelectedManagedFiles(t, cfg) {
		if strings.HasPrefix(name, "nix/") {
			writeManagedTestFile(t, root, name, content)
		}
	}
	expression := `let pkgs = { nodejs = "node"; oxlint = "oxlint"; prettier = "prettier"; shellcheck = "shellcheck"; shfmt = "shfmt"; }; in import ` + strconv.Quote(filepath.Join(root, "nix/loader.nix")) + ` { inherit pkgs; }`
	output, err := exec.CommandContext(t.Context(), nix, "eval", "--impure", "--json", "--expr", expression).CombinedOutput()
	if err != nil {
		t.Fatalf("nix eval: %v %s", err, output)
	}
	var got []string
	if err := json.Unmarshal(output, &got); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, []string{"node", "oxlint", "prettier", "shellcheck", "shfmt"}) {
		t.Fatalf("packages: %q", got)
	}
}

func TestSourceCorrectionBoundary(t *testing.T) {
	root, bin, log := sourceFixture(t, "javascript: true, shell: true")
	for _, name := range []string{"a.js", "a.sh", "vendor/a.js", "package.json", "package-lock.json", ".editorconfig", ".shellcheckrc", "prettier.config.js"} {
		writeManagedTestFile(t, root, name, []byte("original\n"))
	}
	for _, tool := range []string{"prettier", "shfmt"} {
		writeManagedExecutable(t, filepath.Join(bin, tool), "#!/usr/bin/env bash\nset -eu\nwrite=false\nfor arg; do\n case \"$arg\" in --write|-w) write=true;; ./*) if $write; then printf 'formatted\\n' > \"$arg\"; fi;; esac\ndone\n")
	}
	before := snapshotManagedTree(t, root)
	if output, err := qualityRun(t, root, bin, log, "", "lint"); err != nil {
		t.Fatalf("check: %v %s", err, output)
	}
	if !reflect.DeepEqual(before, snapshotManagedTree(t, root)) {
		t.Fatal("check changed source")
	}
	for range 2 {
		if output, err := qualityRun(t, root, bin, log, "", "lint", "correct"); err != nil {
			t.Fatalf("correct: %v %s", err, output)
		}
		for _, name := range []string{"a.js", "a.sh"} {
			assertManagedBytes(t, filepath.Join(root, name), []byte("formatted\n"))
		}
		after := snapshotManagedTree(t, root)
		delete(before, "a.js")
		delete(before, "a.sh")
		delete(after, "a.js")
		delete(after, "a.sh")
		if !reflect.DeepEqual(before, after) {
			t.Fatal("correction changed excluded files")
		}
	}
	core := renderSelectedManagedFiles(t, &config.Config{Languages: &config.LanguageSettings{JavaScript: new(true), Shell: new(true)}})["just/tailor.just"]
	for _, forbidden := range []string{"oxlint --fix", "--type-aware", "--type-check", "--tsconfig", "shellcheck -x", "shfmt -s", "shfmt -mn", "prettier .", "npm "} {
		if bytes.Contains(core, []byte(forbidden)) {
			t.Fatalf("unexpected policy %q", forbidden)
		}
	}
}
