package swatch_test

import (
	"strings"
	"testing"

	"github.com/wimpysworld/tailor/internal/swatch"
	"gopkg.in/yaml.v3"
)

func TestGoWorkflowCGOJobScope(t *testing.T) {
	for _, policy := range []*bool{nil, new(false), new(true)} {
		content, err := swatch.Render(swatch.GoWorkflowDestination, swatch.Options{GoEnabled: true, GoCGO: policy})
		if err != nil {
			t.Fatal(err)
		}
		var workflow struct {
			Jobs map[string]struct {
				Env map[string]string `yaml:"env"`
			} `yaml:"jobs"`
		}
		if err := yaml.Unmarshal(content, &workflow); err != nil {
			t.Fatal(err)
		}
		for job, settings := range workflow.Jobs {
			want := ""
			if policy != nil && (job == "test" || job == "coverage" || job == "lint-code" || job == "security") {
				want = "0"
				if *policy {
					want = "1"
				}
			}
			if got := settings.Env["CGO_ENABLED"]; got != want {
				t.Fatalf("%s CGO=%q want %q", job, got, want)
			}
		}
		if strings.Contains(string(content), "setup-go-ffmpeg-statigo") || strings.Contains(string(content), "download-lib") {
			t.Fatal("workflow invokes setup")
		}
	}
}
