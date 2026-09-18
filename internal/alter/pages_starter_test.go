package alter

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/wimpysworld/tailor/internal/config"
	"github.com/wimpysworld/tailor/internal/model"
	"github.com/wimpysworld/tailor/internal/swatch"
)

func TestPagesStarterLifecycle(t *testing.T) {
	for _, sitePath := range []string{"pages", "web/site"} {
		for _, emptyDirectory := range []bool{false, true} {
			dir := t.TempDir()
			if emptyDirectory {
				if err := os.MkdirAll(filepath.Join(dir, sitePath), 0o755); err != nil {
					t.Fatal(err)
				}
			}
			cfg := pagesTestConfig("static")
			cfg.Pages.Path = &sitePath
			cfg.Repository = &model.RepositorySettings{Description: new(`A "useful" <project>`), HasWiki: new(true), HasDiscussions: new(true)}
			cfg.Pages.Links = &map[string]string{"website": "https://example.com"}
			cfg.License = "MIT"
			p, err := preparePagesSource(cfg, dir, DryRun)
			if err != nil || !p.Starter {
				t.Fatalf("starter = %v, error = %v", p, err)
			}
			p.ProjectName, p.RepoURL = "sample", "https://github.com/owner/sample"
			results, err := processPagesFiles(cfg, dir, DryRun, p)
			if err != nil || len(results) < 4 {
				t.Fatalf("preview = %v, %v", results, err)
			}
			if _, err := os.Stat(filepath.Join(dir, sitePath, "index.html")); !os.IsNotExist(err) {
				t.Fatal("preview wrote starter")
			}
			if _, err := processPagesFiles(cfg, dir, Apply, p); err != nil {
				t.Fatal(err)
			}
			for _, source := range swatch.PagesStarterPaths {
				if _, err := os.Stat(filepath.Join(dir, sitePath, filepath.Base(source))); err != nil {
					t.Fatal(err)
				}
			}
			data, err := os.ReadFile(filepath.Join(dir, sitePath, "index.html"))
			if err != nil {
				t.Fatal(err)
			}
			for _, want := range []string{"<title>sample</title>", "A &#34;useful&#34; &lt;project&gt;", "/wiki", "/discussions", "?tab=MIT-1-ov-file", "https://example.com", "@digicreon/mucss@1.4.9/dist/mu.css", `<main id="main" class="container" tabindex="-1">`} {
				if !strings.Contains(string(data), want) {
					t.Fatalf("missing %s", want)
				}
			}
			if strings.Contains(string(data), "{{") || strings.Contains(string(data), "wimpysworld") || strings.Contains(string(data), "picocss") {
				t.Fatal("unresolved or obsolete project content")
			}
			style, err := os.ReadFile(filepath.Join(dir, sitePath, "style.css"))
			if err != nil || !strings.Contains(string(style), "var(--mu-primary)") || strings.Contains(string(style), "Catppuccin") {
				t.Fatalf("starter theme CSS is incorrect: %v", err)
			}
			icon, err := os.ReadFile(filepath.Join(dir, sitePath, "icon.svg"))
			if err != nil || !strings.Contains(string(icon), `fill="#0172ad"`) {
				t.Fatalf("starter icon is not azure: %v", err)
			}
			pagesTestFile(t, dir, sitePath+"/style.css", "/* user style */")
			pagesTestFile(t, dir, sitePath+"/index.html", "<!-- user edit -->\n"+string(data))
			for _, mode := range []ApplyMode{Apply, Recut} {
				p, err = preparePagesSource(cfg, dir, mode)
				if err != nil || p.Starter {
					t.Fatalf("existing site: %v", err)
				}
				p.RepoURL = "https://github.com/owner/sample"
				if _, err := processPagesFiles(cfg, dir, mode, p); err != nil {
					t.Fatal(err)
				}
				style, err := os.ReadFile(filepath.Join(dir, sitePath, "style.css"))
				if err != nil || string(style) != "/* user style */" {
					t.Fatal("changed user style")
				}
				got, err := os.ReadFile(filepath.Join(dir, sitePath, "index.html"))
				if err != nil || string(got) != "<!-- user edit -->\n"+string(data) {
					t.Fatal("changed user content")
				}
			}
		}
	}
}

func TestPagesStarterCustomStyles(t *testing.T) {
	indexData, err := swatch.Content("pages/index.html")
	if err != nil {
		t.Fatal(err)
	}
	index := string(indexData)
	if count := strings.Count(index, `<span class="brand-name">{{PROJECT_NAME}}</span>`); count != 2 {
		t.Fatalf("starter index has %d project-name brand spans, want 2", count)
	}
	for _, want := range []string{
		`<a class="primary-action" href="{{REPO_URL}}/releases">`,
		`<meta name="theme-color" content="#fff" media="(prefers-color-scheme: light)">`,
		`<meta name="theme-color" content="rgb(19, 22.5, 30.5)" media="(prefers-color-scheme: dark)">`,
		`<script src="theme.js"></script>`,
		`<label id="theme-picker" class="theme-picker" hidden>`,
		`<select id="theme-select" title="Theme">`,
		`<option value="system">System</option>`,
		`<option value="light">Light</option>`,
		`<option value="dark">Dark</option>`,
		`<!-- tailor:navigation:start -->`,
		`<!-- tailor:navigation:end -->`,
	} {
		if !strings.Contains(index, want) {
			t.Fatalf("starter index is missing %q", want)
		}
	}
	if strings.Contains(index, `role="button"`) {
		t.Fatal("starter navigation action overrides link semantics")
	}
	if strings.Index(index, `<script src="theme.js"></script>`) > strings.Index(index, `@digicreon/mucss@1.4.9/dist/mu.css`) {
		t.Fatal("starter theme script must load before the stylesheet")
	}

	styleData, err := swatch.Content("pages/style.css")
	if err != nil {
		t.Fatal(err)
	}
	style := string(styleData)
	for _, want := range []string{
		".brand-name {\n  min-width: 0;\n  overflow-wrap: anywhere;",
		"#hero-title,\n#install-title {\n  overflow-wrap: anywhere;",
		"flex: 0 0 33px;",
		"background-color: var(--mu-primary);",
		"color: var(--mu-inverted-color);",
		".primary-action:is(:hover, :focus)",
		".primary-action:focus-visible",
		"select:focus-visible",
		"body > main:focus-visible {\n  outline: none;",
		"body > main:focus-visible h1:first-of-type::before",
		".theme-picker select {",
		".terminal figcaption {\n  display: flex;\n  align-items: center;\n  gap: 1.1rem;\n  border-bottom: 1px solid var(--mu-muted-border-color);\n  padding: 0.8rem 1.25rem;\n  color: var(--mu-code-color);",
		".terminal-comment {\n  color: var(--mu-code-color);",
		"@media (max-width: 1023px) {\n  .site-hero {\n    grid-template-columns: 1fr;",
	} {
		if !strings.Contains(style, want) {
			t.Fatalf("starter style is missing %q", want)
		}
	}

	themeData, err := swatch.Content("pages/theme.js")
	if err != nil {
		t.Fatal(err)
	}
	theme := string(themeData)
	for _, want := range []string{
		`const backgrounds = { light: "#fff", dark: "rgb(19, 22.5, 30.5)" };`,
		`if (saved === "light" || saved === "dark") preference = saved;`,
		`if (preference === "system") localStorage.removeItem(storageKey);`,
		`select.addEventListener("change", () => chooseTheme(select.value));`,
		`if (preference === "system") {`,
	} {
		if !strings.Contains(theme, want) {
			t.Fatalf("starter theme script is missing %q", want)
		}
	}
}

func TestPagesStarterConflicts(t *testing.T) {
	for _, mode := range []ApplyMode{DryRun, Apply, Recut} {
		dir := t.TempDir()
		cfg := pagesTestConfig("static")
		cfg.Swatches = []config.SwatchEntry{{Path: "pages/style.css", Alteration: swatch.Never}}
		if _, err := preparePagesSource(cfg, dir, mode); err == nil {
			t.Fatal("accepted incomplete starter")
		}
		cfg.Swatches = nil
		p, err := preparePagesSource(cfg, dir, mode)
		if err != nil {
			t.Fatal(err)
		}
		pagesTestFile(t, dir, "pages/style.css", "keep")
		if _, err := processPagesFiles(cfg, dir, mode, p); err == nil {
			t.Fatal("accepted changed destination")
		}
		if _, err := os.Stat(filepath.Join(dir, "pages/index.html")); !os.IsNotExist(err) {
			t.Fatal("wrote into existing directory")
		}
	}
}

func TestPagesStarterNotGeneric(t *testing.T) {
	dir := t.TempDir()
	cfg := &config.Config{}
	for _, source := range swatch.PagesStarterPaths {
		cfg.Swatches = append(cfg.Swatches, config.SwatchEntry{Path: source, Alteration: swatch.Always})
	}
	if _, err := processSwatches(cfg, dir, Apply, nil, managedExcludedPaths()); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "pages")); !os.IsNotExist(err) {
		t.Fatal("generic processing wrote Pages files")
	}
}

func TestPagesStarterWriteFailure(t *testing.T) {
	dir := t.TempDir()
	cfg := pagesTestConfig("static")
	p, err := preparePagesSource(cfg, dir, Apply)
	if err != nil {
		t.Fatal(err)
	}
	original := swatch.PagesStarterPaths
	t.Cleanup(func() { swatch.PagesStarterPaths = original })
	// A duplicate destination fails exclusive creation after the first file succeeds.
	swatch.PagesStarterPaths = []string{"pages/index.html", "pages/index.html"}
	results, err := processPagesStarter(cfg, dir, Apply, p)
	if !os.IsExist(err) || len(results) != 0 {
		t.Fatalf("results = %v, error = %v, want no results and an existing-file error", results, err)
	}
	entries, err := os.ReadDir(filepath.Join(dir, "pages"))
	if err != nil || len(entries) != 0 {
		t.Fatalf("partial starter remains: %v, %v", entries, err)
	}
	swatch.PagesStarterPaths = original
	if _, err := processPagesStarter(cfg, dir, Apply, p); err != nil {
		t.Fatalf("retry failed: %v", err)
	}
}
