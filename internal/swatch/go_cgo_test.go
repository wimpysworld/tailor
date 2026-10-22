package swatch

import (
	"bytes"
	"fmt"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestGoWorkflowCGOHeaderDrift(t *testing.T) {
	original, err := Content(GoWorkflowDestination)
	if err != nil {
		t.Fatal(err)
	}
	for _, job := range []string{"lint-code", "coverage", "test", "security"} {
		header := "  " + job + ":\n"
		for _, test := range []struct {
			name        string
			replacement string
			count       int
		}{
			{"missing", "", 0},
			{"renamed", "  " + job + "-renamed:\n", 0},
			{"duplicate", header + header, 2},
			{"indented", "  " + header, 0},
			{"inline comment", "  " + job + ": # check job\n", 0},
			{"commented out", "# " + header, 0},
		} {
			t.Run(job+"/"+test.name, func(t *testing.T) {
				input := bytes.Replace(original, []byte(header), []byte(test.replacement), 1)
				before := bytes.Clone(input)
				got, err := renderGoWorkflowCGO(input, true)
				want := fmt.Sprintf("rendering Go workflow requires exactly one header for job %q, found %d", job, test.count)
				if err == nil || err.Error() != want || got != nil {
					t.Fatalf("render = %q, %v; want nil, %s", got, err, want)
				}
				if !bytes.Equal(input, before) {
					t.Fatal("render changed input on error")
				}
			})
		}
	}
}

func TestGoWorkflowCGOPreservesOtherBytes(t *testing.T) {
	const original = "# workflow comment\njobs:\n  unrelated: # preserve comment\n    env:\n      test:\n      CGO_ENABLED: inherit\n  lint-code:\n    # lint comment\n  coverage:\n  test:\n  security:\n  release:\n    env:\n      CGO_ENABLED: '0'\n"
	for _, enabled := range []bool{false, true} {
		got, err := renderGoWorkflowCGO([]byte(original), enabled)
		if err != nil {
			t.Fatal(err)
		}
		const unrelated = "  unrelated: # preserve comment\n    env:\n      test:\n      CGO_ENABLED: inherit\n"
		if !bytes.Contains(got, []byte(unrelated)) {
			t.Fatal("render changed an unrelated job with a nested test key")
		}
		value := "0"
		if enabled {
			value = "1"
		}
		insertion := "    env:\n      CGO_ENABLED: \"" + value + "\"\n"
		if count := bytes.Count(got, []byte(insertion)); count != 4 {
			t.Fatalf("CGO insertions = %d, want 4", count)
		}
		if restored := strings.ReplaceAll(string(got), insertion, ""); restored != original {
			t.Fatalf("render changed unrelated bytes:\n%s", got)
		}
	}
}

func TestGoWorkflowCGOJobScope(t *testing.T) {
	for _, policy := range []*bool{nil, new(false), new(true)} {
		content, err := Render(GoWorkflowDestination, Options{GoEnabled: true, GoCGO: policy})
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
