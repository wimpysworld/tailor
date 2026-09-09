package swatch_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/wimpysworld/tailor/internal/config"
	"github.com/wimpysworld/tailor/internal/model"
	"github.com/wimpysworld/tailor/internal/swatch"
	"gopkg.in/yaml.v3"
)

func TestPagesContent(t *testing.T) {
	for _, generator := range []string{"static", "hugo", "jekyll"} {
		for _, source := range []string{"pages", "site docs"} {
			t.Run(generator+"/"+source, func(t *testing.T) {
				content, err := swatch.PagesContent(generator, source, "release/docs")
				if err != nil {
					t.Fatal(err)
				}
				fixture := generator
				if source != "pages" {
					fixture += "-nested"
				}
				golden, err := os.ReadFile(filepath.Join("testdata", "pages", fixture+".golden"))
				if err != nil {
					t.Fatal(err)
				}
				if string(golden) != string(content) {
					t.Fatalf("workflow differs from %s golden", fixture)
				}
				var workflow struct {
					On   map[string]any `yaml:"on"`
					Jobs map[string]struct {
						RunsOn      string            `yaml:"runs-on"`
						Timeout     int               `yaml:"timeout-minutes"`
						Permissions map[string]string `yaml:"permissions"`
						Needs       string            `yaml:"needs"`
					} `yaml:"jobs"`
				}
				if err := yaml.Unmarshal(content, &workflow); err != nil {
					t.Fatal(err)
				}
				for name, job := range workflow.Jobs {
					wantRunner, wantTimeout := "ubuntu-slim", 15
					if name == "build" && generator != "static" {
						wantRunner, wantTimeout = "ubuntu-24.04", 0
					}
					if job.RunsOn != wantRunner || job.Timeout != wantTimeout {
						t.Errorf("%s runner and timeout = %q, %d; want %q, %d", name, job.RunsOn, job.Timeout, wantRunner, wantTimeout)
					}
				}
				if len(workflow.On) != 1 || workflow.On["push"] == nil || workflow.Jobs["deploy"].Needs != "build" {
					t.Fatalf("unsafe workflow events or job dependency: %s", content)
				}
				if len(workflow.Jobs["build"].Permissions) != 1 || workflow.Jobs["build"].Permissions["contents"] != "read" || len(workflow.Jobs["deploy"].Permissions) != 2 {
					t.Fatal("unexpected job permissions")
				}
				if !strings.HasPrefix(string(content), swatch.PagesMarker+"\n") || !strings.Contains(string(content), `"release/docs"`) {
					t.Fatal("missing ownership marker or selected branch")
				}
				if actionlint, err := exec.LookPath("actionlint"); err == nil {
					file := filepath.Join(t.TempDir(), "pages.yml")
					if err := os.WriteFile(file, content, 0o600); err != nil {
						t.Fatal(err)
					}
					if output, err := exec.CommandContext(t.Context(), actionlint, "-shellcheck=", file).CombinedOutput(); err != nil {
						t.Fatalf("actionlint: %v\n%s", err, output)
					}
				}
			})
		}
	}
}

func TestPagesContentLiteralBranchFilters(t *testing.T) {
	for _, tt := range []struct{ branch, filter string }{
		{"!release", `\!release`},
		{"release+", `release\+`},
		{"release]", "release]"},
		{"docs/!release+v2]", `docs/\!release\+v2]`},
		{`release"docs`, `release"docs`},
		{"release/docs", "release/docs"},
	} {
		for _, generator := range []string{"static", "hugo", "jekyll"} {
			t.Run(generator+"/"+tt.branch, func(t *testing.T) {
				cfg := &config.Config{Pages: &model.PagesSettings{Branch: &tt.branch}}
				if err := config.ValidatePages(cfg); err != nil {
					t.Fatalf("branch must pass config validation: %v", err)
				}
				content, err := swatch.PagesContent(generator, "pages", tt.branch)
				if err != nil {
					t.Fatal(err)
				}
				var workflow struct {
					On struct {
						Push struct {
							Branches []string `yaml:"branches"`
						} `yaml:"push"`
					} `yaml:"on"`
				}
				if err := yaml.Unmarshal(content, &workflow); err != nil {
					t.Fatal(err)
				}
				if branches := workflow.On.Push.Branches; len(branches) != 1 || branches[0] != tt.filter {
					t.Fatalf("branch filters = %q, want [%q]", branches, tt.filter)
				}
				if actionlint, err := exec.LookPath("actionlint"); err == nil {
					file := filepath.Join(t.TempDir(), "pages.yml")
					if err := os.WriteFile(file, content, 0o600); err != nil {
						t.Fatal(err)
					}
					if output, err := exec.CommandContext(t.Context(), actionlint, "-shellcheck=", file).CombinedOutput(); err != nil {
						t.Fatalf("actionlint: %v\n%s", err, output)
					}
				}
			})
		}
	}
}

func TestPagesContentRejectsUnsafeInputs(t *testing.T) {
	for _, values := range [][3]string{{"node", "pages", "main"}, {"static", "../pages", "main"}, {"static", "${{ github.token }}", "main"}, {"hugo", "pages", "main\nother"}, {"jekyll", "pages", "*"}} {
		if _, err := swatch.PagesContent(values[0], values[1], values[2]); err == nil {
			t.Errorf("accepted unsafe input %q", values)
		}
	}
}
