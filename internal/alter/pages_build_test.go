package alter

import (
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/wimpysworld/tailor/internal/swatch"
	"gopkg.in/yaml.v3"
)

func TestPagesRepresentativeSources(t *testing.T) {
	for _, generator := range []string{"static", "hugo", "jekyll"} {
		t.Run(generator, func(t *testing.T) {
			cfg := pagesTestConfig(generator)
			cfg.Pages.Path = &generator
			if _, err := preparePagesSource(cfg, "testdata/pages", Apply); err != nil {
				t.Fatal(err)
			}
		})
	}
}

// TestPagesGeneratorBuilds is opt-in because generator installation is external
// to the Go suite. Install the workflow's pinned tools and the fixture's locked
// gems, then run TAILOR_TEST_PAGES_BUILDS=1 go test ./internal/alter -run TestPagesGeneratorBuilds -v.
func TestPagesGeneratorBuilds(t *testing.T) {
	if os.Getenv("TAILOR_TEST_PAGES_BUILDS") != "1" {
		t.Skip("set TAILOR_TEST_PAGES_BUILDS=1 with Hugo Extended 0.165.0, Ruby 3.3.12 and the fixture's locked gems installed")
	}
	for _, tool := range []struct {
		name string
		cmd  *exec.Cmd
		want string
	}{
		{"hugo", exec.CommandContext(t.Context(), "hugo", "version"), "v0.165.0"},
		{"ruby", exec.CommandContext(t.Context(), "ruby", "--version"), "ruby 3.3.12 "},
	} {
		output, err := tool.cmd.CombinedOutput()
		if err != nil || !strings.Contains(string(output), tool.want) || tool.name == "hugo" && !strings.Contains(string(output), "+extended") {
			t.Fatalf("wrong %s version: %v\n%s", tool.name, err, output)
		}
		t.Log(strings.TrimSpace(string(output)))
	}
	for _, generator := range []string{"static", "hugo", "jekyll"} {
		for _, siteURL := range []string{"https://owner.github.io/repo/", "https://docs.example.com/"} {
			t.Run(generator+"/"+siteURL, func(t *testing.T) {
				dir := t.TempDir()
				source := "site docs"
				if err := os.CopyFS(filepath.Join(dir, source), os.DirFS(filepath.Join("testdata", "pages", generator))); err != nil {
					t.Fatal(err)
				}
				content, err := swatch.PagesContent(generator, source, "release/docs")
				if err != nil {
					t.Fatal(err)
				}
				var workflow struct {
					Jobs map[string]struct {
						Steps []struct {
							Name string            `yaml:"name"`
							Run  string            `yaml:"run"`
							With map[string]string `yaml:"with"`
						} `yaml:"steps"`
					} `yaml:"jobs"`
				}
				if err := yaml.Unmarshal(content, &workflow); err != nil {
					t.Fatal(err)
				}
				base, err := url.Parse(siteURL)
				if err != nil {
					t.Fatal(err)
				}
				env := append(os.Environ(), "SITE_PATH="+source, "RUNNER_TEMP="+dir,
					"HUGO_BASEURL="+siteURL, "HUGO_ENVIRONMENT=production", "HUGO_CACHEDIR="+filepath.Join(dir, "hugo_cache"),
					"JEKYLL_ENV=production", "BUNDLE_FROZEN=true", "PAGES_BASE_PATH="+strings.TrimSuffix(base.Path, "/"),
					"PAGES_ORIGIN="+base.Scheme+"://"+base.Host)
				artifact, built := "", false
				for _, step := range workflow.Jobs["build"].Steps {
					if step.Name == "Build Hugo" || step.Name == "Build Jekyll" {
						cmd := exec.CommandContext(t.Context(), "bash", "--noprofile", "--norc", "-e", "-o", "pipefail", "-s")
						cmd.Stdin = strings.NewReader(step.Run)
						cmd.Dir, cmd.Env = dir, env
						if output, err := cmd.CombinedOutput(); err != nil {
							t.Fatalf("%s: %v\n%s", step.Name, err, output)
						}
						built = true
					}
					if step.Name == "Upload site" {
						artifact = step.With["path"]
					}
				}
				if artifact == "" || generator != "static" && !built {
					t.Fatal("missing build command or artifact path")
				}
				data, err := os.ReadFile(filepath.Join(dir, artifact, "index.html"))
				if err != nil {
					t.Fatal(err)
				}
				// Hugo minification removes quotes from simple attribute values.
				html := strings.ReplaceAll(string(data), `"`, "")
				if generator != "static" && !strings.Contains(html, "href="+siteURL) {
					t.Fatalf("canonical URL does not use Pages metadata: %s", data)
				}
				for _, target := range []string{"guide/", "style.css"} {
					href := target
					if generator != "static" {
						href = base.Path + target
					}
					if !regexp.MustCompile(`href=` + regexp.QuoteMeta(href) + `[ >]`).MatchString(html) {
						t.Fatalf("missing deployment-relative link %q: %s", href, data)
					}
					resolved := base.ResolveReference(&url.URL{Path: href})
					if resolved.Host != base.Host || !strings.HasPrefix(resolved.Path, base.Path) {
						t.Fatalf("link escapes deployment URL: %s", resolved)
					}
					local := strings.TrimPrefix(resolved.Path, base.Path)
					if strings.HasSuffix(local, "/") {
						local += "index.html"
					}
					if _, err := os.Stat(filepath.Join(dir, artifact, filepath.FromSlash(local))); err != nil {
						t.Fatalf("broken artifact link: %v", err)
					}
				}
				if generator == "static" {
					original, err := os.ReadFile("testdata/pages/static/index.html")
					if err != nil || string(original) != string(data) {
						t.Fatal("static HTML changed")
					}
				}
			})
		}
	}
}
