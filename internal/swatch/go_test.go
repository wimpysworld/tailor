package swatch_test

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/wimpysworld/tailor/internal/goproject"
	"github.com/wimpysworld/tailor/internal/swatch"
	"gopkg.in/yaml.v3"
)

func TestRenderGoVariants(t *testing.T) {
	for _, destination := range []string{"justfile", ".github/dependabot.yml"} {
		base, err := swatch.Content(destination)
		if err != nil {
			t.Fatal(err)
		}
		absent, err := swatch.Render(destination, swatch.Options{})
		if err != nil || !bytes.Equal(base, absent) {
			t.Fatalf("absent selection changes %s: %v", destination, err)
		}
		disabled, err := swatch.Render(destination, swatch.Options{GoDeclared: true})
		if err != nil {
			t.Fatal(err)
		}
		enabled, err := swatch.Render(destination, swatch.Options{GoDeclared: true, GoEnabled: true})
		if err != nil {
			t.Fatal(err)
		}
		if destination == "justfile" {
			if !bytes.Equal(base, disabled) || !strings.Contains(string(enabled), "go build ./...") || !strings.Contains(string(enabled), "go test ./...") {
				t.Fatalf("unexpected justfile variants")
			}
		} else {
			if !bytes.Equal(base, enabled) || bytes.Contains(disabled, []byte("gomod")) || !bytes.Contains(disabled, []byte("github-actions")) || !bytes.Contains(disabled, []byte("nix")) {
				t.Fatalf("unexpected Dependabot variants")
			}
		}
	}
}

func TestRenderGoRelease(t *testing.T) {
	builds := []goproject.Build{{ID: "one", Binary: "one", Main: "./cmd/one"}, {ID: "two", Binary: "two", Main: "./cmd/two"}}
	data, err := swatch.Render(swatch.GoReleaseDestination, swatch.Options{GoEnabled: true, Builds: builds})
	if err != nil {
		t.Fatal(err)
	}
	var rendered struct {
		Builds []struct {
			ID, Main, Binary           string
			Goos, Goarch, Env, Ldflags []string
		}
	}
	if err := yaml.Unmarshal(data, &rendered); err != nil {
		t.Fatal(err)
	}
	if len(rendered.Builds) != 2 {
		t.Fatalf("builds = %v", rendered.Builds)
	}
	for index, build := range rendered.Builds {
		if build.ID != builds[index].ID || build.Main != builds[index].Main || build.Binary != builds[index].Binary || len(build.Goos) != 2 || len(build.Goarch) != 2 || len(build.Env) != 1 || build.Env[0] != "CGO_ENABLED=0" {
			t.Fatalf("invalid build: %#v", build)
		}
	}
	if bytes.Contains(data, []byte("[[TAILOR_")) || !bytes.Contains(data, []byte("{{ .ProjectName }}")) {
		t.Fatalf("release expressions were not preserved")
	}
	if _, err := swatch.Render(swatch.GoReleaseDestination, swatch.Options{GoEnabled: true}); err == nil {
		t.Fatal("expected missing main error")
	}
}

func TestRenderGoWorkflow(t *testing.T) {
	data, err := swatch.Render(swatch.GoWorkflowDestination, swatch.Options{GoEnabled: true, DefaultBranch: "release+next"})
	if err != nil {
		t.Fatal(err)
	}
	var document map[string]any
	if err := yaml.Unmarshal(data, &document); err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(data, []byte("[[TAILOR_")) || !bytes.Contains(data, []byte("${{")) || !bytes.Contains(data, []byte(`release\\+next`)) {
		t.Fatalf("invalid workflow rendering")
	}
	for _, branch := range []string{"bad\nbranch", "${{ github.token }}"} {
		if _, err := swatch.Render(swatch.GoWorkflowDestination, swatch.Options{DefaultBranch: branch}); err == nil {
			t.Fatalf("accepted unsafe branch %q", branch)
		}
	}
}

func TestRenderGoWorkflowWithoutBranch(t *testing.T) {
	data, err := swatch.Render(swatch.GoWorkflowDestination, swatch.Options{GoEnabled: true})
	if err != nil {
		t.Fatal(err)
	}
	var document struct {
		Jobs map[string]struct{ If string }
	}
	if err := yaml.Unmarshal(data, &document); err != nil {
		t.Fatal(err)
	}
	for _, job := range []string{"lint-code", "lint-actions", "coverage", "test", "security"} {
		condition := document.Jobs[job].If
		if !strings.Contains(condition, "github.event.repository.default_branch") || !strings.Contains(condition, "refs/tags/") {
			t.Fatalf("job %q has no dynamic branch guard: %q", job, condition)
		}
	}
	if bytes.Contains(data, []byte("[[TAILOR_")) || !bytes.Contains(data, []byte(`branches: ["**"]`)) {
		t.Fatal("invalid branch fallback")
	}
}

func TestGoWorkflowIsolationAndVulnerabilityChecks(t *testing.T) {
	for _, branch := range []string{"main", ""} {
		t.Run("branch="+branch, func(t *testing.T) {
			data, err := swatch.Render(swatch.GoWorkflowDestination, swatch.Options{GoEnabled: true, DefaultBranch: branch})
			if err != nil {
				t.Fatal(err)
			}
			var workflow struct {
				Name        string
				Concurrency struct{ Group string }
				Jobs        map[string]struct {
					Steps []struct {
						Name, ID, If, Uses, Run string
						With                    map[string]string
						ContinueOnError         bool `yaml:"continue-on-error"`
					}
				}
			}
			if err := yaml.Unmarshal(data, &workflow); err != nil {
				t.Fatal(err)
			}
			if workflow.Name == "Builder 👷" || workflow.Concurrency.Group != "tailor-build-go-${{ github.event.pull_request.number || github.ref }}" {
				t.Fatal("Go workflow can share the existing builder concurrency group")
			}
			steps := workflow.Jobs["security"].Steps
			scanIndex, checkIndex, reportIndex, uploadIndex := -1, -1, -1, -1
			scans := 0
			for index, step := range steps {
				if strings.HasPrefix(step.Uses, "golang/govulncheck-action@") {
					scans++
				}
				switch step.Name {
				case "Run govulncheck":
					scanIndex = index
					if !strings.HasPrefix(step.Uses, "golang/govulncheck-action@") || step.ID != "govulncheck" || step.With["output-format"] != "json" || step.With["output-file"] != "govulncheck.json" || step.ContinueOnError {
						t.Fatal("source scan must produce JSON and propagate failures")
					}
					if step.If != "${{ !cancelled() }}" {
						t.Fatal("vulnerability check must run regardless of upload eligibility or earlier failures")
					}
				case "Check vulnerabilities":
					checkIndex = index
					if step.Run != "govulncheck -mode convert -format text < govulncheck.json" || step.ContinueOnError {
						t.Fatal("text conversion must print findings and propagate vulnerability or JSON errors")
					}
					if step.If != "${{ !cancelled() && steps.govulncheck.outcome == 'success' }}" {
						t.Fatal("text conversion must require a successful scan regardless of upload eligibility or earlier failures")
					}
				case "Generate SARIF":
					reportIndex = index
					if step.Run != "govulncheck -mode convert -format sarif < govulncheck.json > govulncheck.sarif" || step.ContinueOnError {
						t.Fatal("SARIF generation must convert the same JSON and propagate errors")
					}
					if !strings.Contains(step.If, "steps.govulncheck.outcome == 'success'") || strings.Contains(step.If, "success()") {
						t.Fatal("SARIF conversion must require a successful scan and run after vulnerability failures")
					}
				case "Upload SARIF":
					uploadIndex = index
				}
			}
			if scans != 1 || scanIndex < 0 || checkIndex <= scanIndex || reportIndex <= checkIndex || uploadIndex <= reportIndex {
				t.Fatal("expected one source scan, text validation, SARIF conversion and upload in that order")
			}
			for _, index := range []int{reportIndex, uploadIndex} {
				for _, guard := range []string{
					"!cancelled()",
					"!github.event.repository.private",
					"github.actor != 'dependabot[bot]'",
					"github.event_name != 'pull_request' || github.event.pull_request.head.repo.full_name == github.repository",
				} {
					condition := strings.Join(strings.Fields(steps[index].If), " ")
					if !strings.Contains(condition, guard) {
						t.Errorf("%s lacks reporting guard %q", steps[index].Name, guard)
					}
				}
			}
			if actionlint, err := exec.LookPath("actionlint"); err == nil {
				file := filepath.Join(t.TempDir(), "build-go.yml")
				if err := os.WriteFile(file, data, 0o600); err != nil {
					t.Fatal(err)
				}
				if output, err := exec.CommandContext(t.Context(), actionlint, "-shellcheck=", file).CombinedOutput(); err != nil {
					t.Fatalf("actionlint: %v\n%s", err, output)
				}
			}
		})
	}
}
