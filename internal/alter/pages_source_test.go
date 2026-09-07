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
	root, err := os.OpenRoot(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	if err := root.MkdirAll(filepath.Dir(name), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := root.WriteFile(name, []byte(content), 0o644); err != nil {
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

func TestPreparePagesSourceHugoModuleTheme(t *testing.T) {
	for _, tt := range []struct {
		name, theme, requirement, checksum, wantErr string
	}{
		{"matching module", "github.com/owner/theme", "github.com/owner/theme", "github.com/owner/theme v1.2.3 h1:test\n", ""},
		{"unrelated module", "github.com/owner/theme", "github.com/owner/other", "github.com/owner/other v1.2.3 h1:test\n", "must contain local files or a pinned submodule"},
		{"module path prefix", "github.com/owner/theme/subtheme", "github.com/owner/theme", "github.com/owner/theme v1.2.3 h1:test\n", "must contain local files or a pinned submodule"},
		{"missing checksum", "github.com/owner/theme", "github.com/owner/theme", "", "matching go.sum entries"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			pagesTestFile(t, dir, "pages/hugo.toml", "theme = '"+tt.theme+"'\n")
			pagesTestFile(t, dir, "pages/go.mod", "module site\nrequire "+tt.requirement+" v1.2.3\n")
			pagesTestFile(t, dir, "pages/go.sum", tt.checksum)
			_, err := preparePagesSource(pagesTestConfig("hugo"), dir, Apply)
			if tt.wantErr == "" {
				if err != nil {
					t.Fatal(err)
				}
			} else if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("error = %v, want %q", err, tt.wantErr)
			}
		})
	}
}

func TestPreparePagesSourceJekyllRequirements(t *testing.T) {
	for _, tt := range []struct {
		name, declaration string
		wantErr           bool
	}{
		{"unconstrained", `gem "jekyll"`, false},
		{"exact", `gem "jekyll", "4.4.1"`, false},
		{"explicit exact", `gem "jekyll", "= 4.4.1"`, false},
		{"incompatible exact", `gem "jekyll", "3.9.0"`, true},
		{"incompatible explicit exact", `gem "jekyll", "= 4.4.0"`, true},
		{"pessimistic major", `gem "jekyll", "~> 4"`, false},
		{"pessimistic minor", `gem "jekyll", "~> 4.0"`, false},
		{"pessimistic patch", `gem "jekyll", "~> 4.4.0"`, false},
		{"incompatible pessimistic minor", `gem "jekyll", "~> 3.9"`, true},
		{"incompatible pessimistic patch", `gem "jekyll", "~> 4.3.0"`, true},
		{"pessimistic lower bound", `gem "jekyll", "~> 4.4.2"`, true},
		{"range", `gem 'jekyll', '>= 4.0', '< 5.0'`, false},
		{"inclusive range", `gem "jekyll", ">= 4.4.1", "<= 4.4.1"`, false},
		{"incompatible lower bound", `gem "jekyll", "> 4.4.1"`, true},
		{"incompatible upper bound", `gem "jekyll", ">= 4.0", "< 4.4.1"`, true},
		{"excluded version", `gem "jekyll", "!= 4.4.1"`, true},
		{"other excluded version", `gem "jekyll", "!= 4.4.0"`, false},
		{"numeric comparison", `gem "jekyll", "< 4.10"`, false},
		{"trailing zero", `gem "jekyll", "4.4.1.0"`, false},
		{"comment", `gem "jekyll", "~> 4.4" # pinned in the lockfile`, false},
		{"options", `gem "jekyll", "~> 4.4", require: false`, false},
		{"quoted option punctuation", `gem "jekyll", "~> 4.4", require: "jekyll#,", group: 'site#,'`, false},
		{"comment punctuation", `gem "jekyll", "~> 4.4" # no continuation,`, false},
		{"continued first constraint", "gem \"jekyll\",\n  \"~> 3.9\"", true},
		{"continued second constraint", "gem \"jekyll\", \">= 4.0\",\n  \"< 4.4.1\"", true},
		{"continued constraint after comment", "gem \"jekyll\", # version follows\n  \"~> 3.9\"", true},
		{"continued second constraint after comment", "gem \"jekyll\", \">= 4.0\", # upper bound follows\n  \"< 4.4.1\"", true},
		{"backslash continuation", "gem \"jekyll\" \\\n  , \"~> 3.9\"", true},
		{"backslash second constraint", "gem \"jekyll\", \">= 4.0\" \\\n  , \"< 4.4.1\"", true},
		{"continued compatible constraint", "gem \"jekyll\",\n  \"~> 4.4\"", true},
		{"unsupported literal", `gem "jekyll", "latest"`, true},
		{"oversized component", `gem "jekyll", "> 9999999999999999999999999"`, true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			if err := os.CopyFS(filepath.Join(dir, "pages"), os.DirFS("testdata/pages/jekyll")); err != nil {
				t.Fatal(err)
			}
			pagesTestFile(t, dir, "pages/Gemfile", "source 'https://rubygems.org'\n"+tt.declaration+"\n")
			_, err := preparePagesSource(pagesTestConfig("jekyll"), dir, Apply)
			if (err != nil) != tt.wantErr {
				t.Fatalf("error = %v, want error %v", err, tt.wantErr)
			}
			if err != nil && !strings.Contains(err.Error(), "jekyll version requirement") {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}

func TestPreparePagesSourceJekyllDottedDependency(t *testing.T) {
	for _, tt := range []struct {
		name    string
		missing bool
	}{
		{"complete lockfile", false},
		{"missing dotted dependency", true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			if err := os.CopyFS(filepath.Join(dir, "pages"), os.DirFS("testdata/pages/jekyll")); err != nil {
				t.Fatal(err)
			}
			if tt.missing {
				lock, err := os.ReadFile(filepath.Join(dir, "pages", "Gemfile.lock"))
				if err != nil {
					t.Fatal(err)
				}
				const spec = "    http_parser.rb (0.8.1)\n"
				if !strings.Contains(string(lock), spec) {
					t.Fatal("fixture lacks dotted dependency")
				}
				pagesTestFile(t, dir, "pages/Gemfile.lock", strings.Replace(string(lock), spec, "", 1))
			}
			_, err := preparePagesSource(pagesTestConfig("jekyll"), dir, Apply)
			if tt.missing {
				if err == nil || !strings.Contains(err.Error(), `missing transitive dependency "http_parser.rb" in Gemfile.lock`) {
					t.Fatalf("error = %v, want missing dotted dependency", err)
				}
			} else if err != nil {
				t.Fatal(err)
			}
		})
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
		{"owned crlf", swatch.Always, Apply, "crlf", WouldOverwrite, false},
		{"unowned recut", swatch.Always, Recut, "name: custom\n", "", true},
		{"marker suffix", swatch.Always, Apply, swatch.PagesMarker + " extra\n", "", true},
		{"marker bare cr", swatch.Always, Apply, swatch.PagesMarker + "\rname: custom\n", "", true},
		{"marker repeated cr", swatch.Always, Apply, swatch.PagesMarker + "\r\r\n", "", true},
		{"marker missing newline", swatch.Always, Apply, swatch.PagesMarker, "", true},
		{"owned changed", swatch.Always, Apply, "changed", WouldOverwrite, false},
		{"first fit equal", swatch.FirstFit, Apply, "equal", Skipped, false},
		{"first fit crlf", swatch.FirstFit, Apply, "crlf", Skipped, false},
		{"first fit changed", swatch.FirstFit, Apply, "changed", "", true},
		{"first fit recut", swatch.FirstFit, Recut, "changed", WouldOverwrite, false},
		{"never changed", swatch.Never, Recut, "changed", "", true},
		{"never equal", swatch.Never, Recut, "equal", Skipped, false},
		{"never crlf", swatch.Never, Apply, "crlf", Skipped, false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			existing := tt.existing
			if existing == "equal" || existing == "changed" || existing == "crlf" {
				data, err := swatch.PagesContent("static", "pages", "main")
				if err != nil {
					t.Fatal(err)
				}
				existing = string(data)
				if tt.existing == "changed" {
					existing = strings.Replace(existing, `"main"`, `"old"`, 1)
				}
				if tt.existing == "crlf" {
					existing = strings.ReplaceAll(existing, "\n", "\r\n")
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
