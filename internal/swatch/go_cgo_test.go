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
		wantJobs := []string{"lint-code", "lint-actions", "coverage", "test", "security", "sentinel", "snapshot", "release"}
		if len(workflow.Jobs) != len(wantJobs) {
			t.Fatalf("jobs = %d, want %d", len(workflow.Jobs), len(wantJobs))
		}
		for _, job := range wantJobs {
			if _, ok := workflow.Jobs[job]; !ok {
				t.Fatalf("missing job %q", job)
			}
		}
		environments := 0
		for job, settings := range workflow.Jobs {
			want := ""
			wantCGO := policy != nil && (job == "test" || job == "coverage" || job == "lint-code" || job == "security")
			if wantCGO {
				want = "0"
				if *policy {
					want = "1"
				}
			}
			got, present := settings.Env["CGO_ENABLED"]
			if present {
				environments++
			}
			if present != wantCGO || got != want {
				t.Fatalf("%s CGO=%q (present=%t), want %q (present=%t)", job, got, present, want, wantCGO)
			}
		}
		wantEnvironments := 0
		if policy != nil {
			wantEnvironments = 4
		}
		if environments != wantEnvironments {
			t.Fatalf("CGO environments = %d, want %d", environments, wantEnvironments)
		}
		if strings.Contains(string(content), "setup-go-ffmpeg-statigo") || strings.Contains(string(content), "download-lib") {
			t.Fatal("workflow invokes setup")
		}
	}
}
