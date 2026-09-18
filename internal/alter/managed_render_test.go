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

func TestAvailableManagedTemplatesMatchEmbeddedSources(t *testing.T) {
	available := make(map[string]bool)
	for _, entry := range fixedManagedRegistry() {
		if !entry.Available {
			t.Errorf("managed template %q is unavailable", entry.Path)
			continue
		}
		available[entry.Path] = true
		content, err := swatch.Content(entry.Path)
		if err != nil {
			t.Fatalf("available template %q: %v", entry.Path, err)
		}
		if entry.Policy.marked() && !hasManagedMarker(content, entry.Path) {
			t.Errorf("available template %q lacks its ownership marker", entry.Path)
		}
	}
	if len(available) != 14 {
		t.Fatalf("available=%d, want 14", len(available))
	}

	err := fs.WalkDir(tailor.SwatchFS, "swatches", func(name string, entry fs.DirEntry, err error) error {
		if err != nil || entry.IsDir() {
			return err
		}
		relative := strings.TrimPrefix(name, "swatches/")
		if (strings.HasPrefix(relative, "just/") || strings.HasPrefix(relative, "nix/")) && !available[relative] {
			t.Errorf("managed source %q is not available in the fixed registry", relative)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestManagedLoadersUseAvailableRegistryFragments(t *testing.T) {
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
		{name: "core", want: []string{"alter", "default", "lint", "measure", "release"}},
		{name: "go", goOn: true, want: []string{"alter", "build", "default", "lint", "lint-go", "measure", "release", "test"}},
		{name: "static pages", pages: &model.PagesSettings{Enabled: new(true)}, want: []string{"alter", "default", "lint", "measure", "pages", "release"}},
		{name: "all", goOn: true, pages: &model.PagesSettings{Enabled: new(true)}, want: []string{"alter", "build", "default", "lint", "lint-go", "measure", "pages", "release", "test"}},
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
		{name: "go", goOn: true, wantValues: []string{"go", "golangci-lint", "goreleaser"}},
		{name: "pages", pages: &model.PagesSettings{Enabled: new(true)}, wantValues: []string{"miniserve"}},
		{name: "all", goOn: true, pages: &model.PagesSettings{Enabled: new(true)}, wantValues: []string{"go", "golangci-lint", "goreleaser", "miniserve"}},
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
			expression := `let pkgs = { go = "go"; golangci-lint = "golangci-lint"; goreleaser = "goreleaser"; miniserve = "miniserve"; }; in import ` + strconv.Quote(loader) + ` { inherit pkgs; }`
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
