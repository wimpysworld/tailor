package alter

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/wimpysworld/tailor/internal/model"
)

func TestManagedJustLoaderSupportsNestedOptionalFragments(t *testing.T) {
	just := requireManagedExecutable(t, "just")
	registry := []managedRegistryEntry{
		{Path: "justfile", Policy: managedPolicyRoot, Available: true},
		{Path: "just/loader.just", Policy: managedPolicyLoader, Available: true},
		{Path: "just/core.just", Policy: managedPolicyCore, Available: true},
		{Path: "just/ecosystem.just", Policy: managedPolicyFragment, Capability: managedCapabilityGo, Available: true},
	}
	if err := validateManagedRegistry(registry); err != nil {
		t.Fatal(err)
	}
	loader, err := renderManagedLoader("just/loader.just", []byte("# Managed by Tailor: just/loader.just\n"+managedImportsPlaceholder+"\n"), registry)
	if err != nil {
		t.Fatal(err)
	}

	root := filepath.Join(t.TempDir(), "nested", "project")
	rootContent := []byte("# user-owned root\nimport 'just/loader.just'\n\ndefault:\n    @just --list\n\nroot-cwd:\n    @pwd\n")
	writeManagedRenderFixture(t, root, "justfile", rootContent)
	writeManagedRenderFixture(t, root, "just/loader.just", loader)
	writeManagedRenderFixture(t, root, "just/core.just", []byte("core-cwd:\n    @pwd\n"))

	wantWithoutFragment := []string{"core-cwd", "default", "root-cwd"}
	if got := managedJustRecipes(t, just, root); !reflect.DeepEqual(got, wantWithoutFragment) {
		t.Fatalf("recipes without optional fragment = %v, want %v", got, wantWithoutFragment)
	}
	assertManagedJustRecipeCWD(t, just, root, "root-cwd")
	assertManagedJustRecipeCWD(t, just, root, "core-cwd")

	writeManagedRenderFixture(t, root, "just/ecosystem.just", []byte("ecosystem-cwd:\n    @pwd\n"))
	wantWithFragment := []string{"core-cwd", "default", "ecosystem-cwd", "root-cwd"}
	if got := managedJustRecipes(t, just, root); !reflect.DeepEqual(got, wantWithFragment) {
		t.Fatalf("recipes with optional fragment = %v, want %v", got, wantWithFragment)
	}
	assertManagedJustRecipeCWD(t, just, root, "ecosystem-cwd")

	if err := os.Remove(filepath.Join(root, "just", "ecosystem.just")); err != nil {
		t.Fatal(err)
	}
	if got := managedJustRecipes(t, just, root); !reflect.DeepEqual(got, wantWithoutFragment) {
		t.Fatalf("recipes after optional fragment removal = %v, want %v", got, wantWithoutFragment)
	}
	assertManagedBytes(t, filepath.Join(root, "justfile"), rootContent)
}

func TestManagedNixLoaderEvaluatesRelativePackageLists(t *testing.T) {
	nix := requireManagedExecutable(t, "nix")
	cases := []struct {
		name       string
		goOn       bool
		pages      bool
		wantValues []string
	}{
		{name: "core"},
		{name: "go", goOn: true, wantValues: []string{"go", "golangci-lint", "goreleaser"}},
		{name: "pages", pages: true, wantValues: []string{"miniserve"}},
		{name: "all", goOn: true, pages: true, wantValues: []string{"go", "golangci-lint", "goreleaser", "miniserve"}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var pages *model.PagesSettings
			if tc.pages {
				pages = &model.PagesSettings{Enabled: new(true)}
			}
			rendered := renderSelectedManagedFiles(t, managedRenderConfig(tc.goOn, pages))
			root := filepath.Join(t.TempDir(), "nested", "project")
			for name, content := range rendered {
				if strings.HasPrefix(name, "nix/") {
					writeManagedRenderFixture(t, root, name, content)
				}
			}
			loader := filepath.Join(root, "nix", "loader.nix")
			expression := `let pkgs = { go = "go"; golangci-lint = "golangci-lint"; goreleaser = "goreleaser"; miniserve = "miniserve"; }; in import ` + strconv.Quote(loader) + ` { inherit pkgs; }`
			command := exec.CommandContext(t.Context(), nix, "eval", "--impure", "--json", "--expr", expression) // #nosec G204 -- The executable and fixture are controlled by the test.
			command.Dir = t.TempDir()
			output, err := command.CombinedOutput()
			if err != nil {
				t.Fatalf("nix eval: %v\n%s", err, output)
			}
			var got []string
			if err := json.Unmarshal(output, &got); err != nil {
				t.Fatal(err)
			}
			if !slices.Equal(got, tc.wantValues) {
				t.Fatalf("packages = %v, want %v", got, tc.wantValues)
			}
		})
	}
}

func TestManagedNixGitVisibilityUsesIsolatedRepository(t *testing.T) {
	nix := requireManagedExecutable(t, "nix")
	git := requireManagedExecutable(t, "git")
	root := t.TempDir()
	rendered := renderSelectedManagedFiles(t, managedRenderConfig(true, nil))
	writeManagedRenderFixture(t, root, "nix/loader.nix", rendered["nix/loader.nix"])
	writeManagedRenderFixture(t, root, "nix/go.nix", rendered["nix/go.nix"])
	flake := []byte(`{
  outputs = { self }: {
    managedPackages = import ./nix/loader.nix {
      pkgs = {
        go = "go";
        golangci-lint = "golangci-lint";
        goreleaser = "goreleaser";
        miniserve = "miniserve";
      };
    };
  };
}
`)
	writeManagedRenderFixture(t, root, "flake.nix", flake)
	managedGit(t, git, root, "init")
	managedGit(t, git, root, "config", "user.name", "Tailor Tests")
	managedGit(t, git, root, "config", "user.email", "tailor@example.invalid")
	managedGit(t, git, root, "add", "flake.nix", "nix/loader.nix")
	managedGit(t, git, root, "commit", "-m", "track loader")

	flakeURL := "git+file://" + filepath.ToSlash(root) + "#managedPackages"
	if got := managedNixFlakeList(t, nix, root, flakeURL); len(got) != 0 {
		t.Fatalf("packages with untracked fragment = %v, want []", got)
	}

	managedGit(t, git, root, "add", "nix/go.nix")
	managedGit(t, git, root, "commit", "-m", "track Go fragment")
	want := []string{"go", "golangci-lint", "goreleaser"}
	if got := managedNixFlakeList(t, nix, root, flakeURL); !reflect.DeepEqual(got, want) {
		t.Fatalf("packages with tracked fragment = %v, want %v", got, want)
	}
	assertManagedBytes(t, filepath.Join(root, "flake.nix"), flake)
}

func TestManagedPagesRecipeUsesCustomBuiltPathAndStubMiniserve(t *testing.T) {
	just := requireManagedExecutable(t, "just")
	cases := []struct {
		generator string
		preview   string
	}{
		{generator: "static", preview: "custom site"},
		{generator: "hugo", preview: "custom site/public"},
		{generator: "jekyll", preview: "custom site/_site"},
	}
	for _, tc := range cases {
		t.Run(tc.generator, func(t *testing.T) {
			root := t.TempDir()
			argsFile := filepath.Join(root, "miniserve.args")
			miniserve := filepath.Join(root, "bin", "miniserve")
			writeManagedExecutable(t, miniserve, "#!/bin/sh\nprintf '%s\\n' \"$@\" > \"$TAILOR_MINISERVE_ARGS\"\n")
			settings := &model.PagesSettings{Enabled: new(true), Generator: &tc.generator, Path: new("custom site")}
			rendered := renderSelectedManagedFiles(t, managedRenderConfig(false, settings))
			stubManagedMiniserve(t, rendered, miniserve)
			writeManagedJustFixture(t, root, rendered)
			writeManagedTestFile(t, root, filepath.ToSlash(filepath.Join(tc.preview, "index.html")), []byte("built\n"))

			command := exec.CommandContext(t.Context(), just, "--justfile", "justfile", "pages") // #nosec G204 -- The executable and fixture are controlled by the test.
			command.Dir = root
			command.Env = append(managedEnvironmentWithout("TAILOR_MINISERVE_ARGS"), "TAILOR_MINISERVE_ARGS="+argsFile)
			if output, err := command.CombinedOutput(); err != nil {
				t.Fatalf("just pages: %v\n%s", err, output)
			}
			args, err := os.ReadFile(argsFile)
			if err != nil {
				t.Fatal(err)
			}
			want := "--index\nindex.html\n--interfaces\n127.0.0.1\n--port\n18473\n" + tc.preview + "\n"
			if string(args) != want {
				t.Fatalf("miniserve arguments = %q, want %q", args, want)
			}
		})
	}
}

func TestManagedPagesRecipeRejectsMissingNonStaticBuild(t *testing.T) {
	just := requireManagedExecutable(t, "just")
	cases := []struct {
		generator string
		guidance  string
	}{
		{generator: "hugo", guidance: `Run hugo --source "custom site" first.`},
		{generator: "jekyll", guidance: `Run bundle exec jekyll build --source "custom site" --destination "custom site/_site" first.`},
	}
	for _, tc := range cases {
		t.Run(tc.generator, func(t *testing.T) {
			root := t.TempDir()
			called := filepath.Join(root, "miniserve.called")
			miniserve := filepath.Join(root, "bin", "miniserve")
			writeManagedExecutable(t, miniserve, "#!/bin/sh\ntouch \"$TAILOR_MINISERVE_CALLED\"\n")
			settings := &model.PagesSettings{Enabled: new(true), Generator: &tc.generator, Path: new("custom site")}
			rendered := renderSelectedManagedFiles(t, managedRenderConfig(false, settings))
			stubManagedMiniserve(t, rendered, miniserve)
			writeManagedJustFixture(t, root, rendered)

			command := exec.CommandContext(t.Context(), just, "--justfile", "justfile", "pages") // #nosec G204 -- The executable and fixture are controlled by the test.
			command.Dir = root
			command.Env = append(managedEnvironmentWithout("TAILOR_MINISERVE_CALLED"), "TAILOR_MINISERVE_CALLED="+called)
			output, err := command.CombinedOutput()
			if err == nil {
				t.Fatalf("just pages succeeded without a built %s site", tc.generator)
			}
			if !strings.Contains(string(output), tc.guidance) {
				t.Fatalf("missing-build output = %q, want guidance %q", output, tc.guidance)
			}
			if _, err := os.Lstat(called); !os.IsNotExist(err) {
				t.Fatalf("miniserve ran for a missing build: %v", err)
			}
		})
	}
}

func requireManagedExecutable(t *testing.T, name string) string {
	t.Helper()
	path, err := exec.LookPath(name)
	if err != nil {
		t.Fatalf("required test executable %q is unavailable: %v", name, err)
	}
	return path
}

func managedJustRecipes(t *testing.T, just, root string) []string {
	t.Helper()
	command := exec.CommandContext(t.Context(), just, "--justfile", "justfile", "--dump", "--dump-format", "json") // #nosec G204 -- The executable and fixture are controlled by the test.
	command.Dir = root
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("just parse: %v\n%s", err, output)
	}
	var dump struct {
		Recipes map[string]json.RawMessage `json:"recipes"`
	}
	if err := json.Unmarshal(output, &dump); err != nil {
		t.Fatal(err)
	}
	recipes := make([]string, 0, len(dump.Recipes))
	for name := range dump.Recipes {
		recipes = append(recipes, name)
	}
	slices.Sort(recipes)
	return recipes
}

func assertManagedJustRecipeCWD(t *testing.T, just, root, recipe string) {
	t.Helper()
	command := exec.CommandContext(t.Context(), just, "--justfile", "justfile", recipe) // #nosec G204 -- The executable and fixture are controlled by the test.
	command.Dir = root
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("just %s: %v\n%s", recipe, err, output)
	}
	got, err := filepath.EvalSymlinks(strings.TrimSpace(string(output)))
	if err != nil {
		t.Fatal(err)
	}
	want, err := filepath.EvalSymlinks(root)
	if err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Fatalf("recipe %q working directory = %q, want %q", recipe, got, want)
	}
}

func managedGit(t *testing.T, git, root string, args ...string) {
	t.Helper()
	command := exec.CommandContext(t.Context(), git, args...) // #nosec G204 -- The executable, arguments, and isolated repository are controlled by the test.
	command.Dir = root
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, output)
	}
}

func managedNixFlakeList(t *testing.T, nix, root, installable string) []string {
	t.Helper()
	command := exec.CommandContext(t.Context(), nix, "eval", "--json", installable) // #nosec G204 -- The executable and isolated flake are controlled by the test.
	command.Dir = root
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("nix eval %s: %v\n%s", installable, err, output)
	}
	var values []string
	if err := json.Unmarshal(output, &values); err != nil {
		t.Fatalf("decode nix output %q: %v", output, err)
	}
	return values
}

func stubManagedMiniserve(t *testing.T, rendered managedRenderedFiles, executable string) {
	t.Helper()
	const command = "    miniserve "
	content := rendered["just/pages.just"]
	if bytes.Count(content, []byte(command)) != 1 {
		t.Fatalf("Pages recipe contains %d miniserve commands, want 1", bytes.Count(content, []byte(command)))
	}
	rendered["just/pages.just"] = bytes.Replace(content, []byte(command), []byte("    "+strconv.Quote(executable)+" "), 1)
}

func writeManagedJustFixture(t *testing.T, root string, rendered managedRenderedFiles) {
	t.Helper()
	for name, content := range rendered {
		if name == "justfile" || strings.HasPrefix(name, "just/") {
			writeManagedRenderFixture(t, root, name, content)
		}
	}
}

func writeManagedExecutable(t *testing.T, name, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(name), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(name, []byte(content), 0o755); err != nil {
		t.Fatal(err)
	}
}

func managedEnvironmentWithout(names ...string) []string {
	blocked := make(map[string]bool, len(names))
	for _, name := range names {
		blocked[name] = true
	}
	environment := make([]string, 0, len(os.Environ()))
	for _, entry := range os.Environ() {
		name, _, _ := strings.Cut(entry, "=")
		if !blocked[name] {
			environment = append(environment, entry)
		}
	}
	return environment
}
