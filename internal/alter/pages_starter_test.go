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
	if _, err := ProcessSwatches(cfg, dir, Apply, nil); err != nil {
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
