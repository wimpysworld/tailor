package alter

import (
	"bytes"
	"encoding/json"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/wimpysworld/tailor"
	"github.com/wimpysworld/tailor/internal/config"
	"github.com/wimpysworld/tailor/internal/model"
	"github.com/wimpysworld/tailor/internal/swatch"
)

func TestManagedTemplatesMatchEmbeddedSources(t *testing.T) {
	managed := make(map[string]bool)
	for _, entry := range fixedManagedRegistry() {
		managed[entry.Path] = true
		content, err := swatch.Content(entry.Path)
		if err != nil {
			t.Fatalf("managed template %q: %v", entry.Path, err)
		}
		if entry.Policy.marked() && !hasManagedMarker(content, entry.Path) {
			t.Errorf("managed template %q lacks its ownership marker", entry.Path)
		}
	}
	if len(managed) != 20 {
		t.Fatalf("managed=%d, want 20", len(managed))
	}

	err := fs.WalkDir(tailor.SwatchFS, "swatches", func(name string, entry fs.DirEntry, err error) error {
		if err != nil || entry.IsDir() {
			return err
		}
		relative := strings.TrimPrefix(name, "swatches/")
		if (strings.HasPrefix(relative, "just/") || strings.HasPrefix(relative, "nix/")) && !managed[relative] {
			t.Errorf("managed source %q is not in the fixed registry", relative)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestManagedNixCanonicalFormatting(t *testing.T) {
	nixfmt, err := exec.LookPath("nixfmt")
	if err != nil {
		t.Skip("nixfmt is required")
	}
	configs := []*config.Config{{}, ffmpegConfig()}
	for _, policy := range []*bool{nil, new(false), new(true)} {
		configs = append(configs, &config.Config{Languages: &config.LanguageSettings{
			Go: new(true), GoOptions: &config.GoOptions{CGO: policy},
		}})
	}
	for index, cfg := range configs {
		t.Run(strconv.Itoa(index), func(t *testing.T) {
			for name, content := range renderSelectedManagedFiles(t, cfg) {
				if !strings.HasSuffix(name, ".nix") {
					continue
				}
				cmd := exec.CommandContext(t.Context(), nixfmt, "--check", "-")
				cmd.Stdin = bytes.NewReader(content)
				if output, err := cmd.CombinedOutput(); err != nil {
					t.Errorf("%s is not canonically formatted: %v\n%s", name, err, output)
				}
			}
		})
	}
}

func TestManagedLoadersUseRegistryFragments(t *testing.T) {
	configs := []*config.Config{
		{},
		{Languages: &config.LanguageSettings{Go: new(true)}},
		{Pages: &model.PagesSettings{Enabled: new(true)}},
	}
	var baseline managedRenderedFiles
	for index, cfg := range configs {
		selections, err := selectManagedFiles(cfg)
		if err != nil {
			t.Fatal(err)
		}
		rendered, err := renderManagedFiles(cfg, selections)
		if err != nil {
			t.Fatal(err)
		}
		if index == 0 {
			baseline = rendered
		} else {
			for _, loader := range []string{"just/loader.just", "nix/loader.nix"} {
				if !bytes.Equal(rendered[loader], baseline[loader]) {
					t.Errorf("%s depends on capability declarations", loader)
				}
			}
		}
	}

	justLoader := string(baseline["just/loader.just"])
	for _, name := range []string{"tailor.just", "go.just", "pages.just"} {
		if !strings.Contains(justLoader, "import? \""+name+"\"") {
			t.Errorf("Just loader lacks relative optional import for %s", name)
		}
	}
	nixLoader := string(baseline["nix/loader.nix"])
	for _, name := range []string{"go.nix", "pages.nix", "playwright.nix"} {
		if !strings.Contains(nixLoader, "builtins.pathExists ./"+name) || !strings.Contains(nixLoader, "import ./"+name) {
			t.Errorf("Nix loader lacks relative optional import for %s", name)
		}
	}
	if strings.Contains(justLoader+nixLoader, managedImportsPlaceholder) {
		t.Fatal("loaders contain an unresolved placeholder")
	}
}

func TestRenderManagedBuildAggregateDependencies(t *testing.T) {
	trueValue, falseValue := true, false
	tests := []struct {
		name        string
		cfg         *config.Config
		wantRecipe  string
		wantCommand string
	}{
		{name: "core only", cfg: &config.Config{}, wantRecipe: "build:", wantCommand: `echo "No build recipes are enabled."`},
		{name: "go true", cfg: &config.Config{Languages: &config.LanguageSettings{Go: &trueValue}}, wantRecipe: "build:", wantCommand: "    go build ./..."},
		{name: "go false", cfg: &config.Config{Languages: &config.LanguageSettings{Go: &falseValue}}, wantRecipe: "build:", wantCommand: `echo "No build recipes are enabled."`},
		{name: "go absent", cfg: &config.Config{Languages: &config.LanguageSettings{}}, wantRecipe: "build:", wantCommand: `echo "No build recipes are enabled."`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			content, err := swatch.Content("just/tailor.just")
			if err != nil {
				t.Fatal(err)
			}
			rendered, err := renderManagedBuildAggregate(tt.cfg, content, fixedManagedRegistry())
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Contains(rendered, []byte(tt.wantRecipe+"\n")) || !bytes.Contains(rendered, []byte("    "+tt.wantCommand+"\n")) {
				t.Fatalf("unexpected build aggregate:\n%s", rendered)
			}
			if bytes.Contains(rendered, []byte(managedBuildPlaceholderPrefix)) {
				t.Fatal("build aggregate contains an unresolved placeholder")
			}
		})
	}
}

func TestRenderManagedBuildAggregateUsesCanonicalCapabilityOrder(t *testing.T) {
	registry := []managedRegistryEntry{
		{Path: "just/pages.just", Policy: managedPolicyFragment, Capability: managedCapabilityPages, BuildBody: "test-go"},
		{Path: "just/go.just", Policy: managedPolicyFragment, Capability: managedCapabilityGo, BuildBody: "build-go"},
	}
	cfg := &config.Config{
		Languages: &config.LanguageSettings{Go: new(true)},
		Pages:     &model.PagesSettings{Enabled: new(true)},
	}
	content := []byte("build:\n    [[TAILOR_MANAGED_BUILD_BODY]]\n")
	rendered, err := renderManagedBuildAggregate(cfg, content, registry)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Index(rendered, []byte("goexe=")) >= bytes.Index(rendered, []byte("go vet ./...")) {
		t.Fatalf("unexpected build aggregate:\n%s", rendered)
	}
}

func TestRenderManagedLintAggregateDependencies(t *testing.T) {
	trueValue, falseValue := true, false
	tests := []struct {
		name string
		cfg  *config.Config
		want string
	}{
		{name: "core only", cfg: &config.Config{}, want: "lint $mode=\"check\":"},
		{name: "go true", cfg: &config.Config{Languages: &config.LanguageSettings{Go: &trueValue}}, want: "lint $mode=\"check\":"},
		{name: "go false", cfg: &config.Config{Languages: &config.LanguageSettings{Go: &falseValue}}, want: "lint $mode=\"check\":"},
		{name: "go absent", cfg: &config.Config{Languages: &config.LanguageSettings{}}, want: "lint $mode=\"check\":"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			content, err := swatch.Content("just/tailor.just")
			if err != nil {
				t.Fatal(err)
			}
			rendered, err := renderManagedLintAggregate(tt.cfg, content, fixedManagedRegistry())
			if err != nil {
				t.Fatal(err)
			}
			var got string
			for line := range strings.SplitSeq(string(rendered), "\n") {
				if strings.HasPrefix(line, "lint $mode") {
					got = line
					break
				}
			}
			if got != tt.want {
				t.Fatalf("lint aggregate = %q, want %q", got, tt.want)
			}
			if strings.Contains(string(rendered), managedLintPlaceholderPrefix) {
				t.Fatal("lint aggregate contains an unresolved placeholder")
			}
		})
	}
}

func TestRenderManagedLintAggregateRejectsMalformedPlaceholders(t *testing.T) {
	placeholder := "[[TAILOR_MANAGED_LINT_BODY]]"
	for _, tt := range []struct {
		name    string
		content string
		want    string
	}{
		{name: "missing", content: "lint: lint-actions\n", want: "one lint body placeholder"},
		{name: "repeated", content: "lint: " + placeholder + " " + placeholder + "\n", want: "one lint body placeholder"},
		{name: "unresolved", content: "lint: " + placeholder + "\n# [[TAILOR_MANAGED_LINT_UNKNOWN]]\n", want: "unresolved lint placeholder"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			_, err := renderManagedLintAggregate(&config.Config{}, []byte(tt.content), fixedManagedRegistry())
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("renderManagedLintAggregate() error = %v, want substring %q", err, tt.want)
			}
		})
	}
}

func TestRenderManagedLintAggregateUsesCanonicalCapabilityOrder(t *testing.T) {
	registry := []managedRegistryEntry{
		{Path: "just/pages.just", Policy: managedPolicyFragment, Capability: managedCapabilityPages, LintBody: "test-go"},
		{Path: "just/go.just", Policy: managedPolicyFragment, Capability: managedCapabilityGo, LintBody: "lint-go"},
		{Path: "just/tailor.just", Policy: managedPolicyCore},
		{Path: "just/loader.just", Policy: managedPolicyLoader},
	}
	cfg := &config.Config{
		Languages: &config.LanguageSettings{Go: new(true)},
		Pages:     &model.PagesSettings{Enabled: new(true)},
	}
	core, err := renderManagedLintAggregate(cfg, []byte("lint $mode=\"check\":\n    #!/usr/bin/env bash\n    set -euo pipefail\n    [[TAILOR_MANAGED_LINT_BODY]]\n"), registry)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Index(core, []byte("go mod tidy -diff")) >= bytes.Index(core, []byte("go vet ./...")) {
		t.Fatalf("unexpected lint aggregate:\n%s", core)
	}

	t.Run("Just recipe dump", func(t *testing.T) {
		just, err := exec.LookPath("just")
		if err != nil {
			t.Skip("just is required")
		}
		loader, err := renderManagedLoader("just/loader.just", []byte(managedImportsPlaceholder+"\n"), registry)
		if err != nil {
			t.Fatal(err)
		}
		root := t.TempDir()
		writeManagedRenderFixture(t, root, "justfile", []byte("import 'just/loader.just'\n"))
		writeManagedRenderFixture(t, root, "just/loader.just", loader)
		writeManagedRenderFixture(t, root, "just/tailor.just", core)
		writeManagedRenderFixture(t, root, "just/go.just", []byte("# compatibility\n"))
		writeManagedRenderFixture(t, root, "just/pages.just", []byte("# synthetic compatibility\n"))
		wantRecipes := []string{"lint"}
		if got := managedJustRecipes(t, just, root); !reflect.DeepEqual(got, wantRecipes) {
			t.Fatalf("synthetic lint recipes = %v, want %v", got, wantRecipes)
		}
	})
}

func TestManagedJustCombinationsParseWithUniqueRecipes(t *testing.T) {
	just, err := exec.LookPath("just")
	if err != nil {
		t.Skip("just is required")
	}
	for _, tt := range []struct {
		name  string
		goOn  bool
		pages *model.PagesSettings
		want  []string
	}{
		{name: "core", want: []string{"alter", "build", "default", "lint", "measure", "release", "setup", "test"}},
		{name: "go", goOn: true, want: []string{"alter", "build", "default", "lint", "measure", "release", "setup", "test"}},
		{name: "static pages", pages: &model.PagesSettings{Enabled: new(true)}, want: []string{"alter", "build", "default", "lint", "measure", "pages", "release", "setup", "test"}},
		{name: "all", goOn: true, pages: &model.PagesSettings{Enabled: new(true)}, want: []string{"alter", "build", "default", "lint", "measure", "pages", "release", "setup", "test"}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			cfg := managedRenderConfig(tt.goOn, tt.pages)
			rendered := renderSelectedManagedFiles(t, cfg)
			dir := t.TempDir()
			for name, content := range rendered {
				if !strings.HasSuffix(name, ".just") && name != "justfile" {
					continue
				}
				writeManagedRenderFixture(t, dir, name, content)
			}
			// #nosec G204 -- The executable is resolved from PATH, and the argument is an isolated fixture.
			output, err := exec.CommandContext(t.Context(), just, "--justfile", filepath.Join(dir, "justfile"), "--dump", "--dump-format", "json").CombinedOutput()
			if err != nil {
				t.Fatalf("just parse: %v\n%s", err, output)
			}
			var dump struct {
				Recipes map[string]json.RawMessage `json:"recipes"`
			}
			if err := json.Unmarshal(output, &dump); err != nil {
				t.Fatal(err)
			}
			got := make([]string, 0, len(dump.Recipes))
			for name := range dump.Recipes {
				got = append(got, name)
			}
			slices.Sort(got)
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("recipes = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestManagedNixFragmentsReturnPackageLists(t *testing.T) {
	nix, err := exec.LookPath("nix")
	if err != nil {
		t.Skip("nix is required")
	}
	for _, tt := range []struct {
		name       string
		goOn       bool
		pages      *model.PagesSettings
		wantValues []string
	}{
		{name: "core", wantValues: []string{}},
		{name: "go", goOn: true, wantValues: []string{"go", "golangci-lint", "govulncheck", "goreleaser", "gocyclo", "goperf"}},
		{name: "pages", pages: &model.PagesSettings{Enabled: new(true)}, wantValues: []string{"miniserve"}},
		{name: "all", goOn: true, pages: &model.PagesSettings{Enabled: new(true)}, wantValues: []string{"go", "golangci-lint", "govulncheck", "goreleaser", "gocyclo", "goperf", "miniserve"}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			rendered := renderSelectedManagedFiles(t, managedRenderConfig(tt.goOn, tt.pages))
			dir := t.TempDir()
			for name, content := range rendered {
				if strings.HasPrefix(name, "nix/") {
					writeManagedRenderFixture(t, dir, name, content)
				}
			}
			loader := filepath.Join(dir, "nix", "loader.nix")
			expression := `let pkgs = { go = "go"; golangci-lint = "golangci-lint"; govulncheck = "govulncheck"; goreleaser = "goreleaser"; gocyclo = "gocyclo"; goperf = "goperf"; miniserve = "miniserve"; }; in import ` + strconv.Quote(loader) + ` { inherit pkgs; }`
			output, err := exec.CommandContext(t.Context(), nix, "eval", "--impure", "--json", "--expr", expression).CombinedOutput()
			if err != nil {
				t.Fatalf("nix eval: %v\n%s", err, output)
			}
			var got []string
			if err := json.Unmarshal(output, &got); err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(got, tt.wantValues) {
				t.Fatalf("packages = %v, want %v", got, tt.wantValues)
			}
		})
	}
}

func TestManagedPagesPreviewPathsAndGuidance(t *testing.T) {
	for _, tt := range []struct {
		generator, source, preview, guidance string
	}{
		{generator: "static", source: "site docs", preview: "site docs", guidance: "Add that file first."},
		{generator: "hugo", source: "site docs", preview: "site docs/public", guidance: `hugo --source \"site docs\"`},
		{generator: "jekyll", source: "site docs", preview: "site docs/_site", guidance: `jekyll build --source \"site docs\" --destination \"site docs/_site\"`},
	} {
		t.Run(tt.generator, func(t *testing.T) {
			settings := &model.PagesSettings{Enabled: new(true), Generator: &tt.generator, Path: &tt.source}
			rendered := renderSelectedManagedFiles(t, managedRenderConfig(false, settings))
			content := string(rendered["just/pages.just"])
			if !strings.Contains(content, "site="+strconv.Quote(tt.preview)) || !strings.Contains(content, tt.guidance) || !strings.Contains(content, "127.0.0.1 --port 18473") {
				t.Fatalf("unexpected Pages recipe:\n%s", content)
			}
			if strings.Contains(content, "[[TAILOR_") {
				t.Fatal("Pages recipe contains an unresolved placeholder")
			}
		})
	}
}

func TestManagedRendererRendersPlaywrightTemplates(t *testing.T) {
	enabled := true
	cfg := &config.Config{MCP: &config.MCPSettings{Playwright: &enabled}}
	rendered := renderSelectedManagedFiles(t, cfg)
	for _, path := range []string{"nix/playwright.nix", ".mcp.json", ".codex/config.toml", "opencode.json", ".pi/mcp.json"} {
		if len(rendered[path]) == 0 {
			t.Errorf("rendered Playwright template %q is empty", path)
		}
	}
}

func managedRenderConfig(goOn bool, pages *model.PagesSettings) *config.Config {
	cfg := &config.Config{
		Swatches: []config.SwatchEntry{
			{Path: "justfile", Alteration: swatch.FirstFit},
			{Path: "flake.nix", Alteration: swatch.FirstFit},
		},
		Pages: pages,
	}
	if goOn {
		cfg.Languages = &config.LanguageSettings{Go: new(true)}
	}
	return cfg
}

func renderSelectedManagedFiles(t *testing.T, cfg *config.Config) managedRenderedFiles {
	t.Helper()
	selections, err := selectManagedFiles(cfg)
	if err != nil {
		t.Fatal(err)
	}
	rendered, err := renderManagedFiles(cfg, selections)
	if err != nil {
		t.Fatal(err)
	}
	return rendered
}

func writeManagedRenderFixture(t *testing.T, root, name string, content []byte) {
	t.Helper()
	destination := filepath.Join(root, filepath.FromSlash(name))
	if err := os.MkdirAll(filepath.Dir(destination), 0o755); err != nil {
		t.Fatal(err)
	}
	// #nosec G703 -- Test paths are controlled and stay in the temporary fixture.
	if err := os.WriteFile(destination, content, 0o600); err != nil {
		t.Fatal(err)
	}
}
