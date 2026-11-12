package alter

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"

	"github.com/wimpysworld/tailor/internal/config"
	"github.com/wimpysworld/tailor/internal/swatch"
)

func shellTestEnvironment(t *testing.T) (string, string, string) {
	t.Helper()
	base := testTempDir(t)
	project, home, xdg := filepath.Join(base, "project"), filepath.Join(base, "home"), filepath.Join(base, "xdg")
	for _, dir := range []string{project, home, xdg} {
		if err := os.Mkdir(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", xdg)
	t.Setenv("SHELLCHECK_OPTS", "")
	return project, home, xdg
}

func shellStarterConfig(mode swatch.AlterationMode) *config.Config {
	return &config.Config{
		Languages: &config.LanguageSettings{Shell: new(true)},
		Swatches: []config.SwatchEntry{
			{Path: ".shellcheckrc", Alteration: mode},
			{Path: ".editorconfig", Alteration: mode},
		},
	}
}

func TestShellStarterLifecycle(t *testing.T) {
	for _, mode := range []ApplyMode{DryRun, Apply, Recut} {
		for _, alteration := range []swatch.AlterationMode{swatch.FirstFit, swatch.Always, swatch.Never} {
			for _, existing := range []bool{false, true} {
				t.Run(fmt.Sprintf("%v/%s/existing=%t", mode, alteration, existing), func(t *testing.T) {
					root, _, _ := shellTestEnvironment(t)
					cfg := shellStarterConfig(alteration)
					for _, entry := range cfg.Swatches {
						if existing {
							writeManagedTestFile(t, root, entry.Path, []byte("custom policy\n"))
						}
					}
					before := snapshotManagedTree(t, root)
					results, err := processSwatches(cfg, root, mode, nil, nil)
					if err != nil || len(results) != 2 {
						t.Fatalf("results=%v error=%v", results, err)
					}
					for _, result := range results {
						want := WouldCopy
						if existing || alteration == swatch.Never || (runtime.GOOS == "windows" && result.Path == ".shellcheckrc") {
							want = Skipped
						}
						if result.Category != want {
							t.Fatalf("result=%+v want=%s", result, want)
						}
						if want == WouldCopy && mode != DryRun {
							body, err := os.ReadFile(filepath.Join(root, result.Path))
							want, _ := swatch.Content(result.Path)
							if err != nil || string(body) != string(want) {
								t.Fatalf("starter=%q error=%v", body, err)
							}
						}
						if want == Skipped && !existing {
							if _, err := os.Lstat(filepath.Join(root, result.Path)); !os.IsNotExist(err) {
								t.Fatalf("created skipped starter %s: %v", result.Path, err)
							}
						}
					}
					if (existing || alteration == swatch.Never || mode == DryRun) && !reflect.DeepEqual(before, snapshotManagedTree(t, root)) {
						t.Fatal("changed preserved files")
					}
				})
			}
		}
	}
}

func TestShellStarterWorkspaceAlias(t *testing.T) {
	for _, caller := range []string{"workspace-alias", "project-alias", "logical-pwd"} {
		t.Run(caller, func(t *testing.T) {
			if runtime.GOOS == "windows" && caller == "logical-pwd" {
				t.Skip("logical PWD resolution requires unix")
			}
			project, _, _ := shellTestEnvironment(t)
			alias := filepath.Join(testTempDir(t), "workspace-alias")
			target := filepath.Dir(project)
			if caller == "project-alias" {
				target = project
			}
			managedSymlinkOrSkip(t, target, alias)
			if caller != "project-alias" {
				alias = filepath.Join(alias, filepath.Base(project))
			}
			if caller == "logical-pwd" {
				t.Chdir(project)
				t.Setenv("PWD", alias)
				cwd, err := os.Getwd()
				if err != nil || cwd != alias {
					t.Fatalf("working directory=%q want=%q error=%v", cwd, alias, err)
				}
				alias = cwd
			}
			cfg := shellStarterConfig(swatch.FirstFit)
			for _, entry := range cfg.Swatches {
				realPolicy, err := InspectShellPolicy(project, entry.Path)
				wantPolicy := ShellPolicy{}
				if runtime.GOOS == "windows" && entry.Path == ".shellcheckrc" {
					wantPolicy = shellPolicyWarning(entry.Path, "native rc lookup needs manual verification on Windows or with SHELLCHECK_OPTS")
				}
				if err != nil || realPolicy != wantPolicy {
					t.Fatalf("real policy=%+v error=%v", realPolicy, err)
				}
				aliasPolicy, err := InspectShellPolicy(alias, entry.Path)
				if err != nil || aliasPolicy != realPolicy {
					t.Fatalf("alias policy=%+v want=%+v error=%v", aliasPolicy, realPolicy, err)
				}
			}
			before := snapshotManagedTree(t, project)
			for _, dir := range []string{project, alias} {
				results, err := processSwatches(cfg, dir, DryRun, nil, nil)
				if err != nil || len(results) != 2 {
					t.Fatalf("results=%v error=%v", results, err)
				}
				for _, result := range results {
					want := WouldCopy
					if runtime.GOOS == "windows" && result.Path == ".shellcheckrc" {
						want = Skipped
					}
					if result.Category != want {
						t.Fatalf("result=%+v", result)
					}
				}
			}
			if !reflect.DeepEqual(before, snapshotManagedTree(t, project)) {
				t.Fatal("preview changed project files")
			}
			results, err := processSwatches(cfg, alias, Apply, nil, nil)
			if err != nil || len(results) != 2 {
				t.Fatalf("results=%v error=%v", results, err)
			}
			for _, result := range results {
				if runtime.GOOS == "windows" && result.Path == ".shellcheckrc" {
					if result.Category != Skipped {
						t.Fatalf("result=%+v", result)
					}
					if _, err := os.Lstat(filepath.Join(project, result.Path)); !os.IsNotExist(err) {
						t.Fatalf("created %s on Windows: %v", result.Path, err)
					}
					continue
				}
				body, err := os.ReadFile(filepath.Join(project, result.Path))
				want, _ := swatch.Content(result.Path)
				if result.Category != WouldCopy || err != nil || string(body) != string(want) {
					t.Fatalf("result=%+v starter=%q error=%v", result, body, err)
				}
			}
		})
	}
}

func TestShellPolicyWorkspaceAliasPreservesPolicy(t *testing.T) {
	for _, tc := range []struct {
		name, destination, policy string
		linked, dangling          bool
	}{
		{"shellcheck-link", ".shellcheckrc", ".shellcheckrc", true, false},
		{"shellcheck-dangling", ".shellcheckrc", ".shellcheckrc", true, true},
		{"editorconfig-link", ".editorconfig", ".editorconfig", true, false},
		{"editorconfig-dangling", ".editorconfig", ".editorconfig", true, true},
		{"ancestor-dot", ".shellcheckrc", ".shellcheckrc", false, false},
		{"ancestor-plain", ".shellcheckrc", "shellcheckrc", false, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			project, _, _ := shellTestEnvironment(t)
			aliasParent := testTempDir(t)
			alias := filepath.Join(aliasParent, "workspace-alias")
			managedSymlinkOrSkip(t, filepath.Dir(project), alias)
			alias = filepath.Join(alias, filepath.Base(project))
			policyPath := filepath.Join(filepath.Dir(project), tc.policy)
			if tc.linked {
				policyPath = filepath.Join(project, tc.policy)
				target := filepath.Join(project, "target")
				if !tc.dangling {
					writeManagedTestFile(t, project, "target", []byte("custom policy\n"))
				}
				managedSymlinkOrSkip(t, target, policyPath)
			} else {
				writeManagedTestFile(t, filepath.Dir(project), tc.policy, []byte("disable=SC2034\n"))
				writeManagedTestFile(t, aliasParent, ".shellcheckrc", []byte("disable=SC2086\n"))
			}
			policy, err := InspectShellPolicy(alias, tc.destination)
			if err != nil || policy.AdoptionRequired != tc.linked || !strings.Contains(policy.Reason, policyPath) {
				t.Fatalf("policy=%+v error=%v", policy, err)
			}
			if !tc.linked && !policy.Present {
				t.Fatalf("inherited policy not present: %+v", policy)
			}
			cfg := shellStarterConfig(swatch.Always)
			cfg.Swatches = []config.SwatchEntry{{Path: tc.destination, Alteration: swatch.Always}}
			before := snapshotManagedTree(t, filepath.Dir(project))
			aliasBefore := snapshotManagedTree(t, aliasParent)
			for _, mode := range []ApplyMode{DryRun, Apply, Recut} {
				results, err := processSwatches(cfg, alias, mode, nil, nil)
				if err != nil || len(results) != 1 || results[0].Category != Skipped {
					t.Fatalf("results=%v error=%v", results, err)
				}
				if !reflect.DeepEqual(before, snapshotManagedTree(t, filepath.Dir(project))) || !reflect.DeepEqual(aliasBefore, snapshotManagedTree(t, aliasParent)) {
					t.Fatal("changed preserved policy through workspace alias")
				}
			}
		})
	}
}

func TestShellPolicyNativePrecedence(t *testing.T) {
	for _, location := range []string{"root-dot", "root-plain", "ancestor-dot", "ancestor-plain", "home", "xdg", "default-xdg", "relative-xdg"} {
		t.Run(location, func(t *testing.T) {
			if runtime.GOOS == "windows" && (location == "home" || strings.HasSuffix(location, "xdg")) {
				t.Skip("unix home and xdg lookup requires unix")
			}
			root, home, xdg := shellTestEnvironment(t)
			paths := []string{
				filepath.Join(root, ".shellcheckrc"), filepath.Join(root, "shellcheckrc"),
				filepath.Join(filepath.Dir(root), ".shellcheckrc"), filepath.Join(filepath.Dir(root), "shellcheckrc"),
				filepath.Join(home, ".shellcheckrc"), filepath.Join(xdg, "shellcheckrc"),
			}
			index := 0
			switch location {
			case "root-plain":
				index = 1
			case "ancestor-dot":
				index = 2
			case "ancestor-plain":
				index = 3
			case "home":
				index = 4
			case "xdg":
				index = 5
			case "default-xdg", "relative-xdg":
				index = 5
				paths[5] = filepath.Join(home, ".config", "shellcheckrc")
				t.Setenv("XDG_CONFIG_HOME", "")
				if location == "relative-xdg" {
					t.Setenv("XDG_CONFIG_HOME", "relative")
				}
			}
			for _, path := range paths[index:] {
				writeManagedTestFile(t, filepath.Dir(path), filepath.Base(path), []byte("disable=SC2034\n"))
			}
			writeManagedTestFile(t, root, "nested/.shellcheckrc", []byte("nested policy\n"))
			policy, err := InspectShellPolicy(root, ".shellcheckrc")
			if err != nil || policy.AdoptionRequired || !policy.Present || !strings.HasSuffix(policy.Reason, paths[index]) {
				t.Fatalf("policy=%+v error=%v", policy, err)
			}
			before := snapshotManagedTree(t, filepath.Dir(root))
			cfg := shellStarterConfig(swatch.Always)
			cfg.Swatches = cfg.Swatches[:1]
			for _, mode := range []ApplyMode{DryRun, Apply, Recut} {
				results, err := processSwatches(cfg, root, mode, nil, nil)
				if err != nil || len(results) != 1 || results[0].Category != Skipped {
					t.Fatalf("%v %v", results, err)
				}
				if !reflect.DeepEqual(before, snapshotManagedTree(t, filepath.Dir(root))) {
					t.Fatal("changed inherited policy")
				}
			}
		})
	}
}

func TestShellUnsafePolicy(t *testing.T) {
	for _, destination := range []string{".shellcheckrc", ".editorconfig"} {
		for _, location := range []string{"root", "ancestor", "home", "xdg"} {
			if destination == ".editorconfig" && (location == "home" || location == "xdg") {
				continue
			}
			for _, kind := range []string{"symlink", "live-link", "directory", "fifo", "unreadable", "oversized", "linked-parent", "blocked-parent", "inaccessible-parent"} {
				if location != "xdg" && (kind == "linked-parent" || kind == "blocked-parent" || kind == "inaccessible-parent") {
					continue
				}
				t.Run(destination+"/"+location+"/"+kind, func(t *testing.T) {
					if runtime.GOOS == "windows" {
						switch {
						case location == "home" || location == "xdg":
							t.Skip("unix home and xdg lookup requires unix")
						case kind == "fifo":
							t.Skip("POSIX FIFOs require unix")
						case kind == "unreadable" || kind == "inaccessible-parent":
							t.Skip("chmod 0 does not clear read or search permissions on Windows")
						}
					}
					root, home, xdg := shellTestEnvironment(t)
					dir, name := root, destination
					switch location {
					case "ancestor":
						dir = filepath.Dir(root)
					case "home":
						dir = home
					case "xdg":
						dir, name = xdg, "shellcheckrc"
					}
					if kind == "linked-parent" || kind == "blocked-parent" || kind == "inaccessible-parent" {
						t.Setenv("XDG_CONFIG_HOME", filepath.Join(xdg, "parent"))
						switch kind {
						case "linked-parent":
							managedSymlinkOrSkip(t, home, filepath.Join(xdg, "parent"))
						case "inaccessible-parent":
							parent := filepath.Join(xdg, "parent")
							if err := os.Mkdir(parent, 0); err != nil {
								t.Fatal(err)
							}
							t.Cleanup(func() { _ = os.Chmod(parent, 0o755) })
						default:
							writeManagedTestFile(t, xdg, "parent", []byte("not a directory"))
						}
					} else {
						path := filepath.Join(dir, name)
						switch kind {
						case "symlink":
							managedSymlinkOrSkip(t, filepath.Join(dir, "missing"), path)
						case "live-link":
							writeManagedTestFile(t, dir, "target", []byte("keep"))
							managedSymlinkOrSkip(t, filepath.Join(dir, "target"), path)
						case "fifo":
							mkfifoOrSkip(t, path)
						case "directory":
							if err := os.Mkdir(path, 0o755); err != nil {
								t.Fatal(err)
							}
						default:
							body := "custom policy"
							if kind == "oversized" {
								body = strings.Repeat("x", managedInspectionLimit+1)
							}
							writeManagedTestFile(t, dir, name, []byte(body))
							if kind == "unreadable" {
								if err := os.Chmod(path, 0); err != nil {
									t.Fatal(err)
								}
								t.Cleanup(func() { _ = os.Chmod(path, 0o644) })
							}
						}
					}
					policy, err := InspectShellPolicy(root, destination)
					if err != nil || !policy.AdoptionRequired || policy.Reason == "" {
						t.Fatalf("%+v %v", policy, err)
					}
					cfg := shellStarterConfig(swatch.Always)
					cfg.Swatches = []config.SwatchEntry{{Path: destination, Alteration: swatch.Always}}
					for _, mode := range []ApplyMode{DryRun, Apply, Recut} {
						results, err := processSwatches(cfg, root, mode, nil, nil)
						if err != nil || results[0].Category != Skipped {
							t.Fatalf("%v %v", results, err)
						}
					}
				})
			}
		}
	}
}

func TestShellPreservesRootContents(t *testing.T) {
	for _, body := range []string{"", "# comments only\n", strings.Repeat("#", managedInspectionLimit), "$(touch SHOULD_NOT_EXIST)\n", "[*.sh]\nindent_size = 4\n[*]\nindent_size = unset\n"} {
		for _, mode := range []ApplyMode{DryRun, Apply, Recut} {
			for _, alteration := range []swatch.AlterationMode{swatch.FirstFit, swatch.Always, swatch.Never} {
				root, _, _ := shellTestEnvironment(t)
				cfg := shellStarterConfig(alteration)
				for _, entry := range cfg.Swatches {
					content := strings.ReplaceAll(body, "SHOULD_NOT_EXIST", filepath.Join(root, "SHOULD_NOT_EXIST"))
					writeManagedTestFile(t, root, entry.Path, []byte(content))
				}
				before := snapshotManagedTree(t, root)
				results, err := processSwatches(cfg, root, mode, nil, nil)
				if err != nil || len(results) != 2 || !reflect.DeepEqual(before, snapshotManagedTree(t, root)) {
					t.Fatalf("%v %v", results, err)
				}
				for _, result := range results {
					if result.Category != Skipped {
						t.Fatalf("%+v", result)
					}
				}
				policy, err := InspectShellPolicy(root, ".shellcheckrc")
				if err != nil || !policy.Present || policy.AdoptionRequired {
					t.Fatalf("%+v %v", policy, err)
				}
				policy, err = InspectShellPolicy(root, ".editorconfig")
				if err != nil || !policy.Present || !policy.AdoptionRequired {
					t.Fatalf("%+v %v", policy, err)
				}
			}
		}
	}
}

func TestShellPolicyBoundaries(t *testing.T) {
	root, _, _ := shellTestEnvironment(t)
	writeManagedTestFile(t, filepath.Dir(root), ".editorconfig", []byte("[*]\nindent_style = tab\n"))
	writeManagedTestFile(t, root, "nested/.shellcheckrc", []byte("$(touch SHOULD_NOT_EXIST)\n"))
	for _, name := range []string{".shellcheckrc", ".editorconfig"} {
		policy, err := InspectShellPolicy(root, name)
		want := ShellPolicy{}
		if runtime.GOOS == "windows" && name == ".shellcheckrc" {
			want = shellPolicyWarning(name, "native rc lookup needs manual verification on Windows or with SHELLCHECK_OPTS")
		}
		if err != nil || policy != want {
			t.Fatalf("%s: %+v %v", name, policy, err)
		}
	}
	if _, err := processSwatches(shellStarterConfig(swatch.FirstFit), root, Apply, nil, nil); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{".shellcheckrc", ".editorconfig"} {
		policy, err := InspectShellPolicy(root, name)
		if runtime.GOOS == "windows" && name == ".shellcheckrc" {
			if err != nil || !policy.AdoptionRequired || policy.Present {
				t.Fatalf("%s: %+v %v", name, policy, err)
			}
			continue
		}
		if err != nil || policy.AdoptionRequired || !policy.Present {
			t.Fatalf("%s: %+v %v", name, policy, err)
		}
	}
	for _, enabled := range []*bool{nil, new(false)} {
		cfg := shellStarterConfig(swatch.Always)
		cfg.Languages.Shell = enabled
		before := snapshotManagedTree(t, root)
		results, err := processSwatches(cfg, root, Recut, nil, nil)
		if err != nil || len(results) != 0 || !reflect.DeepEqual(before, snapshotManagedTree(t, root)) {
			t.Fatalf("%v %v", results, err)
		}
	}
}

func TestShellPolicyEnvironmentUnknown(t *testing.T) {
	for _, tc := range []struct{ key, value string }{
		{"HOME", ""}, {"HOME", "relative"}, {"SHELLCHECK_OPTS", "--rcfile=custom"}, {"SHELLCHECK_OPTS", "--norc"},
	} {
		t.Run(tc.key+tc.value, func(t *testing.T) {
			root, _, _ := shellTestEnvironment(t)
			t.Setenv(tc.key, tc.value)
			policy, err := InspectShellPolicy(root, ".shellcheckrc")
			if err != nil || !policy.AdoptionRequired {
				t.Fatalf("%+v %v", policy, err)
			}
			cfg := shellStarterConfig(swatch.Always)
			cfg.Swatches = cfg.Swatches[:1]
			before := snapshotManagedTree(t, root)
			results, err := processSwatches(cfg, root, Recut, nil, nil)
			if err != nil || results[0].Category != Skipped || !reflect.DeepEqual(before, snapshotManagedTree(t, root)) {
				t.Fatalf("%v %v", results, err)
			}
		})
	}
}
