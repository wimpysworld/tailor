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

func pagesTestConfig(generator string) *config.Config {
	enabled := true
	return &config.Config{Pages: &model.PagesSettings{Enabled: &enabled, Generator: &generator}}
}

func pagesTestFile(t *testing.T, dir, name, content string) {
	t.Helper()
	name = filepath.Join(dir, name)
	if err := os.MkdirAll(filepath.Dir(name), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(name, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestPreparePagesSource(t *testing.T) {
	for _, tt := range []struct {
		name, generator string
		files           map[string]string
		wantErr         bool
	}{
		{"static", "static", map[string]string{"index.html": "<h1>site</h1>"}, false},
		{"missing entry", "static", map[string]string{"readme.md": "site"}, true},
		{"hugo", "hugo", map[string]string{"hugo.toml": "title = 'site'"}, false},
		{"local theme", "hugo", map[string]string{"hugo.toml": "theme = 'local'", "themes/local/layouts/baseof.html": "site"}, false},
		{"missing theme", "hugo", map[string]string{"hugo.toml": "theme = 'missing'"}, true},
		{"unpinned module", "hugo", map[string]string{"hugo.toml": "[module]\n[[module.imports]]\npath = 'example.com/theme'", "go.mod": "module site\nrequire example.com/theme latest", "go.sum": ""}, true},
		{"missing jekyll lock", "jekyll", map[string]string{"_config.yml": "title: site", "Gemfile": `gem "jekyll"`}, true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			for name, content := range tt.files {
				pagesTestFile(t, dir, "pages/"+name, content)
			}
			_, err := preparePagesSource(pagesTestConfig(tt.generator), dir, Apply)
			if (err != nil) != tt.wantErr {
				t.Fatalf("error = %v, want error %v", err, tt.wantErr)
			}
			if _, err := os.Stat(filepath.Join(dir, ".github")); !os.IsNotExist(err) {
				t.Fatal("preparation wrote a workflow")
			}
		})
	}
}

func TestPreparePagesSourceRejectsSymlink(t *testing.T) {
	dir := t.TempDir()
	pagesTestFile(t, dir, "pages/index.html", "site")
	if err := os.Symlink(t.TempDir(), filepath.Join(dir, "pages", "escape")); err != nil {
		t.Fatal(err)
	}
	if _, err := preparePagesSource(pagesTestConfig("static"), dir, Apply); err == nil {
		t.Fatal("accepted symlink")
	}
	if p, err := preparePagesSource(&config.Config{}, dir, Apply); err != nil || p != nil {
		t.Fatal("disabled pages inspected source")
	}
}

func TestPreparePagesWorkflow(t *testing.T) {
	for _, tt := range []struct {
		name       string
		alteration swatch.AlterationMode
		mode       ApplyMode
		existing   string
		want       SwatchCategory
		wantErr    bool
	}{
		{"missing always", swatch.Always, Apply, "", WouldCopy, false},
		{"missing never", swatch.Never, Apply, "", "", true},
		{"owned equal", swatch.Always, Apply, "equal", NoChange, false},
		{"unowned recut", swatch.Always, Recut, "name: custom\n", "", true},
		{"owned changed", swatch.Always, Apply, "changed", WouldOverwrite, false},
		{"first fit equal", swatch.FirstFit, Apply, "equal", Skipped, false},
		{"first fit changed", swatch.FirstFit, Apply, "changed", "", true},
		{"first fit recut", swatch.FirstFit, Recut, "changed", WouldOverwrite, false},
		{"never changed", swatch.Never, Recut, "changed", "", true},
		{"never equal", swatch.Never, Recut, "equal", Skipped, false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			existing := tt.existing
			if existing == "equal" || existing == "changed" {
				data, err := swatch.PagesContent("static", "pages", "main")
				if err != nil {
					t.Fatal(err)
				}
				existing = string(data)
				if tt.existing == "changed" {
					existing = strings.Replace(existing, `"main"`, `"old"`, 1)
				}
			}
			if existing != "" {
				pagesTestFile(t, dir, swatch.PagesDestination, existing)
			}
			p := &pagesPreparation{Generator: "static", Path: "pages", Entry: config.SwatchEntry{Path: swatch.PagesDestination, Alteration: tt.alteration}}
			err := preparePagesWorkflow(p, dir, tt.mode, "main")
			if (err != nil) != tt.wantErr {
				t.Fatalf("error = %v, want error %v", err, tt.wantErr)
			}
			if err == nil && p.Result.Category != tt.want {
				t.Fatalf("category = %s, want %s", p.Result.Category, tt.want)
			}
			data, readErr := os.ReadFile(filepath.Join(dir, swatch.PagesDestination))
			if existing == "" && !os.IsNotExist(readErr) || existing != "" && string(data) != existing {
				t.Fatal("preparation changed the destination")
			}
		})
	}
}
