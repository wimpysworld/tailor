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

var javaScriptStarters = []string{".oxlintrc.json", ".prettierrc.json", ".prettierignore"}

func javaScriptStarterConfig(mode swatch.AlterationMode) *config.Config {
	cfg := &config.Config{Languages: &config.LanguageSettings{JavaScript: new(true)}}
	for _, name := range javaScriptStarters {
		cfg.Swatches = append(cfg.Swatches, config.SwatchEntry{Path: name, Alteration: mode})
	}
	return cfg
}

func TestJavaScriptStarterLifecycle(t *testing.T) {
	for _, mode := range []ApplyMode{DryRun, Apply, Recut} {
		t.Run(fmt.Sprint(mode), func(t *testing.T) {
			root := testTempDir(t)
			cfg := javaScriptStarterConfig(swatch.FirstFit)
			results, err := processSwatches(cfg, root, mode, nil, nil)
			if err != nil || len(results) != 3 {
				t.Fatalf("results=%v error=%v", results, err)
			}
			for _, result := range results {
				if result.Category != WouldCopy {
					t.Fatalf("fresh starter: %+v", result)
				}
				body, err := os.ReadFile(filepath.Join(root, result.Path))
				if mode == DryRun {
					if !os.IsNotExist(err) {
						t.Fatalf("preview wrote %s", result.Path)
					}
				} else {
					want, _ := swatch.Content(result.Path)
					if err != nil || string(body) != string(want) {
						t.Fatalf("starter %s: %v", result.Path, err)
					}
				}
			}
		})
	}
	for _, mode := range []ApplyMode{Apply, Recut} {
		for _, alteration := range []swatch.AlterationMode{swatch.FirstFit, swatch.Always, swatch.Never} {
			root := testTempDir(t)
			cfg := javaScriptStarterConfig(alteration)
			for _, name := range javaScriptStarters {
				writeManagedTestFile(t, root, name, []byte("custom policy\n"))
			}
			if _, err := processSwatches(cfg, root, mode, nil, nil); err != nil {
				t.Fatal(err)
			}
			for _, name := range javaScriptStarters {
				body, err := os.ReadFile(filepath.Join(root, name))
				want := "custom policy\n"
				if alteration != swatch.Never && (mode == Recut || alteration == swatch.Always) {
					content, _ := swatch.Content(name)
					want = string(content)
				}
				if err != nil || string(body) != want {
					t.Fatalf("%s %v %s: %q, %v", name, mode, alteration, body, err)
				}
			}
		}
	}
}

func TestJavaScriptIgnoreRestoration(t *testing.T) {
	root := testTempDir(t)
	cfg := javaScriptStarterConfig(swatch.FirstFit)
	writeManagedTestFile(t, root, ".prettierrc.json", []byte("{\"semi\":false}\n"))
	for run := range 3 {
		if _, err := processSwatches(cfg, root, Apply, nil, nil); err != nil {
			t.Fatal(err)
		}
		for _, name := range javaScriptStarters {
			if _, err := os.Stat(filepath.Join(root, name)); err != nil {
				t.Fatalf("run %d: %s: %v", run, name, err)
			}
		}
		body, _ := os.ReadFile(filepath.Join(root, ".prettierrc.json"))
		if string(body) != "{\"semi\":false}\n" {
			t.Fatal("changed existing canonical policy")
		}
		if run == 1 {
			if err := os.Remove(filepath.Join(root, ".prettierignore")); err != nil {
				t.Fatal(err)
			}
		}
	}
}

func TestJavaScriptAlternatePolicyPreservation(t *testing.T) {
	for name, kind := range javaScriptPolicyFiles {
		if name == ".prettierignore" || name == ".oxlintrc.json" || name == ".prettierrc.json" {
			continue
		}
		t.Run(name, func(t *testing.T) {
			root := testTempDir(t)
			body := "project policy\n"
			if name == ".prettierrc.json5" {
				body = "{semi: false, singleQuote: true,}\n"
			}
			if name == "package.json" {
				body = "{\"prettier\":{}}"
			}
			if name == "package.yaml" {
				body = "prettier: {}\n"
			}
			writeManagedTestFile(t, root, name, []byte(body))
			for _, destination := range javaScriptStarters {
				blocked := kind == "conflict" || (destination == ".oxlintrc.json" && kind == "unverified") || (destination != ".oxlintrc.json" && (kind == "prettier" || kind == "metadata"))
				policy, err := InspectJavaScriptPolicy(root, destination)
				if err != nil || (policy.Reason != "") != blocked {
					t.Fatalf("%s: %+v %v", destination, policy, err)
				}
				if !blocked {
					continue
				}
				if policy.AdoptionRequired != (kind == "conflict" || kind == "unverified") {
					t.Fatalf("wrong adoption status: %+v", policy)
				}
				for _, mode := range []ApplyMode{DryRun, Apply, Recut} {
					cfg := javaScriptStarterConfig(swatch.FirstFit)
					if mode != Recut {
						cfg.Swatches = []config.SwatchEntry{{Path: destination, Alteration: swatch.FirstFit}}
						before := snapshotManagedTree(t, root)
						results, err := processSwatches(cfg, root, mode, nil, nil)
						if err != nil || len(results) != 1 || results[0].Category != Skipped || !reflect.DeepEqual(before, snapshotManagedTree(t, root)) {
							t.Fatalf("first-fit changed preserved policy: %v %v", results, err)
						}
					}
					cfg.Swatches = []config.SwatchEntry{{Path: destination, Alteration: swatch.Always}}
					before := snapshotManagedTree(t, root)
					results, err := processSwatches(cfg, root, mode, nil, nil)
					if err != nil || len(results) != 1 || results[0].Category != Skipped {
						t.Fatalf("%v %v", results, err)
					}
					if !reflect.DeepEqual(before, snapshotManagedTree(t, root)) {
						t.Fatal("changed preserved policy")
					}
				}
			}
		})
	}
}

func TestJavaScriptPolicyAncestorsAndMetadata(t *testing.T) {
	for _, tc := range []struct {
		name, body string
		warning    bool
	}{
		{".prettierrc.json", "{}", false},
		{".oxlintrc.json", "{}", false},
		{"package.json", "{\"eslintConfig\":{}}", true},
		{"package.yaml", "eslintConfig: {}", true},
		{"package.json", "{\"scripts\":{\"prettier\":\"yes\"}}", false},
		{"package.json", "{bad", true},
		{"package.json", "{\"prettier\":{},\"prettier\":{}}", true},
		{"package.json", "{} {}", true},
		{"package.json", "[]", true},
		{"package.yaml", "prettier: [", true},
		{"package.yaml", "prettier: {}\nprettier: {}", true},
		{"package.yaml", "{}\n---\n{}", true},
		{"package.yaml", "null", true},
		{"package.json", strings.Repeat(" ", (1<<20)+1), true},
	} {
		t.Run(tc.name+tc.body[:min(len(tc.body), 20)], func(t *testing.T) {
			root := testTempDir(t)
			writeManagedTestFile(t, root, tc.name, []byte(tc.body))
			child := filepath.Join(root, "project")
			if err := os.Mkdir(child, 0o755); err != nil {
				t.Fatal(err)
			}
			for _, destination := range javaScriptStarters {
				policy, err := InspectJavaScriptPolicy(child, destination)
				if err != nil || policy.AdoptionRequired != tc.warning {
					t.Fatalf("%s: %+v %v", destination, policy, err)
				}
				if tc.warning && policy.Reason == "" {
					t.Fatal("warning claimed absence")
				}
				if (tc.name == ".prettierrc.json" && destination != ".oxlintrc.json" || tc.name == ".oxlintrc.json" && destination == tc.name) && policy.Reason == "" {
					t.Fatal("missed ancestor")
				}
			}
		})
	}
	root := testTempDir(t)
	writeManagedTestFile(t, root, "nested/eslint.config.js", []byte("throw Error('never execute')"))
	writeManagedTestFile(t, root, ".editorconfig", []byte("[*]\nindent_size = 4\n"))
	for _, destination := range javaScriptStarters {
		policy, err := InspectJavaScriptPolicy(root, destination)
		if err != nil || policy.Reason != "" {
			t.Fatalf("inspected descendant or EditorConfig: %+v %v", policy, err)
		}
	}
}

func TestJavaScriptUnsafePolicy(t *testing.T) {
	for _, kind := range []string{"symlink", "directory", "unreadable", "ambiguous"} {
		t.Run(kind, func(t *testing.T) {
			if kind == "unreadable" && runtime.GOOS == "windows" {
				t.Skip("chmod 0 does not clear read permissions on Windows")
			}
			root := testTempDir(t)
			path := filepath.Join(root, ".prettierrc.json")
			switch kind {
			case "symlink":
				managedSymlinkOrSkip(t, filepath.Join(root, "missing"), path)
			case "directory":
				if err := os.Mkdir(path, 0o755); err != nil {
					t.Fatal(err)
				}
			default:
				writeManagedTestFile(t, root, ".prettierrc.json", []byte("{}"))
				if kind == "unreadable" {
					if err := os.Chmod(path, 0); err != nil {
						t.Fatal(err)
					}
				} else {
					writeManagedTestFile(t, root, ".prettierrc.yaml", []byte("{}"))
				}
			}
			for _, name := range []string{".prettierrc.json", ".prettierignore"} {
				policy, err := InspectJavaScriptPolicy(root, name)
				if err != nil || !policy.AdoptionRequired || policy.Reason == "" {
					t.Fatalf("%+v %v", policy, err)
				}
			}
			if kind == "unreadable" {
				cfg := javaScriptStarterConfig(swatch.Always)
				results, err := processSwatches(cfg, root, Recut, nil, nil)
				if err != nil {
					t.Fatal(err)
				}
				for _, result := range results {
					if result.Category != Skipped {
						t.Fatalf("unsafe write: %+v", result)
					}
				}
				if err := os.Chmod(path, 0o644); err != nil {
					t.Fatal(err)
				}
				body, err := os.ReadFile(path)
				if err != nil || string(body) != "{}" {
					t.Fatal("changed unreadable policy")
				}
			} else {
				before := snapshotManagedTree(t, root)
				cfg := javaScriptStarterConfig(swatch.Always)
				cfg.Swatches = cfg.Swatches[1:]
				if _, err := processSwatches(cfg, root, Recut, nil, nil); err != nil {
					t.Fatal(err)
				}
				if !reflect.DeepEqual(before, snapshotManagedTree(t, root)) {
					t.Fatal("changed unsafe policy")
				}
			}
		})
	}
}

func TestJavaScriptCanonicalPrecedence(t *testing.T) {
	for _, tc := range []struct {
		name       string
		mode       ApplyMode
		alteration swatch.AlterationMode
	}{
		{"first-fit", Apply, swatch.FirstFit},
		{"always", Apply, swatch.Always},
		{"recut", Recut, swatch.FirstFit},
	} {
		for _, present := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/ignore=%t", tc.name, present), func(t *testing.T) {
				parent := testTempDir(t)
				root := filepath.Join(parent, "project")
				for _, name := range javaScriptStarters[:2] {
					writeManagedTestFile(t, parent, name, []byte("{}\n"))
					writeManagedTestFile(t, root, name, []byte("{\"custom\":true}\n"))
				}
				if present {
					writeManagedTestFile(t, root, ".prettierignore", []byte("custom/\n"))
				}
				for _, name := range javaScriptStarters {
					policy, err := InspectJavaScriptPolicy(root, name)
					if err != nil || policy.Reason != "" || policy.AdoptionRequired {
						t.Fatalf("%s: %+v %v", name, policy, err)
					}
				}
				if _, err := processSwatches(javaScriptStarterConfig(tc.alteration), root, tc.mode, nil, nil); err != nil {
					t.Fatal(err)
				}
				for _, name := range javaScriptStarters {
					want, _ := swatch.Content(name)
					if tc.name == "first-fit" {
						if name != ".prettierignore" {
							want = []byte("{\"custom\":true}\n")
						} else if present {
							want = []byte("custom/\n")
						}
					}
					body, err := os.ReadFile(filepath.Join(root, name))
					if err != nil || string(body) != string(want) {
						t.Fatalf("%s: %q, %v", name, body, err)
					}
				}
			})
		}
	}
}

func TestJavaScriptRootIgnoreWithInheritedConfig(t *testing.T) {
	for _, mode := range []ApplyMode{Apply, Recut} {
		for _, alteration := range []swatch.AlterationMode{swatch.FirstFit, swatch.Always} {
			parent := testTempDir(t)
			root := filepath.Join(parent, "project")
			writeManagedTestFile(t, parent, ".prettierrc.json", []byte("{}"))
			writeManagedTestFile(t, root, ".prettierignore", []byte("custom/\n"))
			policy, err := InspectJavaScriptPolicy(root, ".prettierrc.json")
			if err != nil || policy.Reason == "" || policy.AdoptionRequired || policy.Present {
				t.Fatalf("lost inherited config guard: %+v %v", policy, err)
			}
			if _, err := processSwatches(javaScriptStarterConfig(alteration), root, mode, nil, nil); err != nil {
				t.Fatal(err)
			}
			if _, err := os.Stat(filepath.Join(root, ".prettierrc.json")); !os.IsNotExist(err) {
				t.Fatalf("created competing config: %v", err)
			}
			want := "custom/\n"
			if mode == Recut || alteration == swatch.Always {
				body, _ := swatch.Content(".prettierignore")
				want = string(body)
			}
			body, err := os.ReadFile(filepath.Join(root, ".prettierignore"))
			if err != nil || string(body) != want {
				t.Fatalf("root ignore: %q %v", body, err)
			}
		}
	}
}

func TestJavaScriptCanonicalPrecedenceRetainsGuards(t *testing.T) {
	for _, tc := range []struct {
		name, body string
		ancestor   bool
	}{
		{"package.json", "{\"prettier\":{}}", false},
		{"package.yaml", "prettier: {}", false},
		{".prettierrc.json5", "{semi: false}", false},
		{"eslint.config.js", "export default []", true},
		{"biome.json", "{}", true},
		{"package.json", "{\"prettier\":null}", true},
		{".prettierrc.json", "unsafe", true},
	} {
		t.Run(tc.name+tc.body, func(t *testing.T) {
			parent := testTempDir(t)
			root := filepath.Join(parent, "project")
			for _, name := range javaScriptStarters {
				writeManagedTestFile(t, root, name, []byte("{}\n"))
			}
			dir := root
			if tc.ancestor {
				dir = parent
			}
			if tc.body == "unsafe" {
				managedSymlinkOrSkip(t, filepath.Join(parent, "missing"), filepath.Join(dir, tc.name))
			} else {
				writeManagedTestFile(t, dir, tc.name, []byte(tc.body))
			}
			before := snapshotManagedTree(t, root)
			cfg := javaScriptStarterConfig(swatch.Always)
			cfg.Swatches = cfg.Swatches[1:]
			for _, mode := range []ApplyMode{Apply, Recut} {
				results, err := processSwatches(cfg, root, mode, nil, nil)
				if err != nil {
					t.Fatal(err)
				}
				for _, result := range results {
					if result.Category != Skipped {
						t.Fatalf("lost guard: %+v", result)
					}
				}
				if !reflect.DeepEqual(before, snapshotManagedTree(t, root)) {
					t.Fatal("changed guarded policy")
				}
			}
		})
	}
}

func TestJavaScriptAncestorIgnore(t *testing.T) {
	for _, unsafe := range []bool{false, true} {
		t.Run(fmt.Sprint(unsafe), func(t *testing.T) {
			parent := testTempDir(t)
			root := filepath.Join(parent, "project")
			if err := os.Mkdir(root, 0o755); err != nil {
				t.Fatal(err)
			}
			if unsafe {
				managedSymlinkOrSkip(t, filepath.Join(parent, "missing"), filepath.Join(parent, ".prettierignore"))
			} else {
				writeManagedTestFile(t, parent, ".prettierignore", []byte("parent-only/\n"))
			}
			for _, name := range javaScriptStarters {
				policy, err := InspectJavaScriptPolicy(root, name)
				if err != nil || policy != (JavaScriptPolicy{}) {
					t.Fatalf("ancestor ignore affected %s: %+v %v", name, policy, err)
				}
			}
			if _, err := processSwatches(javaScriptStarterConfig(swatch.FirstFit), root, Apply, nil, nil); err != nil {
				t.Fatal(err)
			}
			body, err := os.ReadFile(filepath.Join(root, ".prettierignore"))
			want, _ := swatch.Content(".prettierignore")
			if err != nil || string(body) != string(want) {
				t.Fatalf("root ignore: %q %v", body, err)
			}
		})
	}
}

func TestJavaScriptPackagePrettierValues(t *testing.T) {
	for _, name := range []string{"package.json", "package.yaml"} {
		for _, value := range []string{"{}", "{\"semi\":false}", "\"@example/prettier-config\"", "null", "true", "42", "[]", "[{}]", "\"\"", "\"  \\t\""} {
			t.Run(name+value, func(t *testing.T) {
				valid := value == "{}" || value == "{\"semi\":false}" || value == "\"@example/prettier-config\""
				root := testTempDir(t)
				writeManagedTestFile(t, root, name, []byte("{\"prettier\":"+value+"}"))
				for _, destination := range javaScriptStarters {
					policy, err := InspectJavaScriptPolicy(root, destination)
					if err != nil || policy.AdoptionRequired == valid || policy.Present {
						t.Fatalf("%s: %+v %v", destination, policy, err)
					}
					if (!valid || destination != ".oxlintrc.json") && policy.Reason == "" {
						t.Fatal("missing policy reason")
					}
				}
				if valid {
					return
				}
				before := snapshotManagedTree(t, root)
				for _, mode := range []ApplyMode{DryRun, Apply, Recut} {
					results, err := processSwatches(javaScriptStarterConfig(swatch.Always), root, mode, nil, nil)
					if err != nil {
						t.Fatal(err)
					}
					for _, result := range results {
						if result.Category != Skipped {
							t.Fatalf("invalid metadata allowed write: %+v", result)
						}
					}
					if !reflect.DeepEqual(before, snapshotManagedTree(t, root)) {
						t.Fatal("changed files with invalid metadata")
					}
				}
			})
		}
	}
}

func TestJavaScriptInactiveStarters(t *testing.T) {
	for _, enabled := range []*bool{nil, new(false)} {
		root := testTempDir(t)
		cfg := javaScriptStarterConfig(swatch.Always)
		cfg.Languages.JavaScript = enabled
		for _, name := range javaScriptStarters {
			writeManagedTestFile(t, root, name, []byte("keep\n"))
		}
		before := snapshotManagedTree(t, root)
		results, err := processSwatches(cfg, root, Recut, nil, nil)
		if err != nil || len(results) != 0 || !reflect.DeepEqual(before, snapshotManagedTree(t, root)) {
			t.Fatalf("inactive: %v %v", results, err)
		}
	}
}
