package swatch_test

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"regexp"
	"strings"
	"testing"
	"text/template"

	"github.com/wimpysworld/tailor/internal/goproject"
	"github.com/wimpysworld/tailor/internal/swatch"
	"gopkg.in/yaml.v3"
)

var releaseTemplateFuncs = template.FuncMap{
	"tolower": strings.ToLower,
	"replace": strings.ReplaceAll,
	"split":   strings.Split,
	"filter": func(value, pattern string) (string, error) {
		match, err := regexp.MatchString(pattern, value)
		if match {
			return value, err
		}
		return "", err
	},
}

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

func TestRenderGoReleasePackagesAndContainers(t *testing.T) {
	for _, test := range []struct {
		name     string
		binaries []string
		suffixes []string
	}{
		{name: "single", binaries: []string{"My_App"}, suffixes: []string{""}},
		{name: "multiple", binaries: []string{"Server", "worker"}, suffixes: []string{"-server", "-worker"}},
		{name: "collisions", binaries: []string{"My_App", "my-app", "my-app-2", "--MY__APP--"}, suffixes: []string{"-my-app", "-my-app-2", "-my-app-2-2", "-my-app-3"}},
	} {
		t.Run(test.name, func(t *testing.T) {
			var builds []goproject.Build
			for _, binary := range test.binaries {
				builds = append(builds, goproject.Build{ID: binary, Binary: binary, Main: "./cmd/" + binary})
			}
			data, err := swatch.Render(swatch.GoReleaseDestination, swatch.Options{GoEnabled: true, Builds: builds})
			if err != nil {
				t.Fatal(err)
			}
			var release struct {
				Nfpms []struct {
					IDs, Formats                 []string
					Maintainer, Homepage, Bindir string
					License                      string
				}
				Dockers []struct {
					ID, Dockerfile               string
					IDs, Images, Tags, Platforms []string
					Labels                       map[string]string
					BuildArgs                    map[string]string `yaml:"build_args"`
				} `yaml:"dockers_v2"`
			}
			if err := yaml.Unmarshal(data, &release); err != nil {
				t.Fatal(err)
			}
			if len(release.Nfpms) != 1 || len(release.Dockers) != len(builds) {
				t.Fatalf("expected one package configuration and %d images: %#v", len(builds), release)
			}
			packages := release.Nfpms[0]
			if !reflect.DeepEqual(packages.IDs, test.binaries) || !reflect.DeepEqual(packages.Formats, []string{"deb", "rpm", "apk"}) || packages.Bindir != "/usr/bin" || packages.License != "" {
				t.Fatalf("invalid package configuration: %#v", packages)
			}
			if packages.Maintainer != "{{ .Env.GITHUB_REPOSITORY_OWNER }}" || packages.Homepage != "https://github.com/{{ .Env.GITHUB_REPOSITORY }}" {
				t.Fatalf("package metadata does not use the runtime repository: %#v", packages)
			}
			for index, docker := range release.Dockers {
				if docker.ID != builds[index].ID || !reflect.DeepEqual(docker.IDs, []string{builds[index].ID}) || docker.Dockerfile != "Dockerfile" || docker.BuildArgs["BINARY"] != builds[index].Binary || !reflect.DeepEqual(docker.Platforms, []string{"linux/amd64", "linux/arm64"}) {
					t.Fatalf("invalid container configuration: %#v", docker)
				}
				if docker.Labels["org.opencontainers.image.source"] != "https://github.com/{{ .Env.GITHUB_REPOSITORY }}" {
					t.Fatal("container source label must link the runtime repository")
				}
				if len(docker.Images) != 1 || len(docker.Tags) != 2 {
					t.Fatalf("invalid image names or tags: %#v", docker)
				}
				for _, version := range []struct {
					Version, Prerelease string
					IsSnapshot          bool
					latest              string
				}{
					{Version: "1.2.3", latest: "latest"},
					{Version: "1.2.3-rc.1", Prerelease: "rc.1"},
					{Version: "1.2.3-SNAPSHOT", IsSnapshot: true},
				} {
					context := map[string]any{"Env": map[string]string{"GITHUB_REPOSITORY": "Owner/My_Repo", "GITHUB_REPOSITORY_OWNER": "Owner"}, "Version": version.Version, "Prerelease": version.Prerelease, "IsSnapshot": version.IsSnapshot}
					for _, expression := range []struct{ source, want string }{
						{docker.Images[0], "ghcr.io/owner/my-repo" + test.suffixes[index]},
						{docker.Tags[0], version.Version},
						{docker.Tags[1], version.latest},
					} {
						tmpl, err := template.New("runtime").Funcs(releaseTemplateFuncs).Parse(expression.source)
						if err != nil {
							t.Fatal(err)
						}
						var output bytes.Buffer
						if err := tmpl.Execute(&output, context); err != nil {
							t.Fatal(err)
						}
						if output.String() != expression.want {
							t.Fatalf("template %q = %q, want %q", expression.source, output.String(), expression.want)
						}
					}
				}
			}
			if goreleaser, err := exec.LookPath("goreleaser"); err == nil {
				file := filepath.Join(t.TempDir(), ".goreleaser.yaml")
				if err := os.WriteFile(file, data, 0o600); err != nil {
					t.Fatal(err)
				}
				if output, err := exec.CommandContext(t.Context(), goreleaser, "check", "--config", file).CombinedOutput(); err != nil {
					t.Fatalf("goreleaser check: %v\n%s", err, output)
				}
			}
		})
	}
}

func TestRenderGoContainerRepositoryNames(t *testing.T) {
	data, err := swatch.Render(swatch.GoReleaseDestination, swatch.Options{Builds: []goproject.Build{{ID: "app", Binary: "app", Main: "."}}})
	if err != nil {
		t.Fatal(err)
	}
	var release struct {
		Dockers []struct{ Images []string } `yaml:"dockers_v2"`
	}
	if err := yaml.Unmarshal(data, &release); err != nil {
		t.Fatal(err)
	}
	tmpl, err := template.New("image").Funcs(releaseTemplateFuncs).Parse(release.Dockers[0].Images[0])
	if err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct{ repository, want string }{
		{"app", "app"},
		{"My_Repo", "my-repo"},
		{"_My_Repo", "image-my-repo"},
		{"app.", "app-image"},
		{".app.", "image-app-image"},
		{"app..name", "app--name"},
		{"___", "image---image"},
	} {
		var output bytes.Buffer
		context := map[string]any{"Env": map[string]string{"GITHUB_REPOSITORY": "Owner/" + test.repository, "GITHUB_REPOSITORY_OWNER": "Owner"}}
		if err := tmpl.Execute(&output, context); err != nil {
			t.Fatal(err)
		}
		if output.String() != "ghcr.io/owner/"+test.want {
			t.Fatalf("image name for %q = %q, want %q", test.repository, output.String(), test.want)
		}
	}
}

func TestRenderGoNativePackageNames(t *testing.T) {
	data, err := swatch.Render(swatch.GoReleaseDestination, swatch.Options{Builds: []goproject.Build{{ID: "app", Binary: "app", Main: "."}}})
	if err != nil {
		t.Fatal(err)
	}
	var release struct {
		Nfpms []struct {
			PackageName string `yaml:"package_name"`
		}
	}
	if err := yaml.Unmarshal(data, &release); err != nil {
		t.Fatal(err)
	}
	tmpl, err := template.New("package").Funcs(releaseTemplateFuncs).Parse(release.Nfpms[0].PackageName)
	if err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct{ project, want string }{
		{"app", "app"}, {"My_App", "my-app"}, {".app", "pkg-.app"}, {"-app", "pkg--app"}, {"a", "pkg-a"},
	} {
		var output bytes.Buffer
		if err := tmpl.Execute(&output, map[string]string{"ProjectName": test.project}); err != nil {
			t.Fatal(err)
		}
		if output.String() != test.want {
			t.Fatalf("package name for %q = %q, want %q", test.project, output.String(), test.want)
		}
	}
}

func TestRenderGoReleaseRequiresRuntimeRepository(t *testing.T) {
	data, err := swatch.Render(swatch.GoReleaseDestination, swatch.Options{Builds: []goproject.Build{{ID: "app", Binary: "app", Main: "."}}})
	if err != nil {
		t.Fatal(err)
	}
	var release struct{ Before struct{ Hooks []string } }
	if err := yaml.Unmarshal(data, &release); err != nil {
		t.Fatal(err)
	}
	if len(release.Before.Hooks) != 1 {
		t.Fatal("expected a runtime repository check")
	}
	for _, test := range []struct {
		repository, owner string
		valid             bool
	}{
		{},
		{"owner/repo", "", false},
		{"", "owner", false},
		{"owner", "owner", false},
		{"/repo", "owner", false},
		{"owner/", "owner", false},
		{"owner/repo/extra", "owner", false},
		{"owner/repo", "other", false},
		{"owner/re po", "owner", false},
		{"owner/repo\n", "owner", false},
		{"owner/re\x01po", "owner", false},
		{"own er/repo", "own er", false},
		{"owner/repo", "owner", true},
		{"Owner/My_Repo", "Owner", true},
		{"Owner/_My_Repo", "Owner", true},
		{"Owner/app.", "Owner", true},
		{"Owner/.app.", "Owner", true},
		{"Owner/app..name", "Owner", true},
		{"Owner/___", "Owner", true},
	} {
		command := exec.CommandContext(t.Context(), "sh")
		command.Stdin = strings.NewReader(release.Before.Hooks[0])
		command.Env = []string{"PATH=" + os.Getenv("PATH"), "GITHUB_REPOSITORY=" + test.repository, "GITHUB_REPOSITORY_OWNER=" + test.owner}
		output, err := command.CombinedOutput()
		if test.valid {
			if err != nil {
				t.Fatalf("valid repository %q rejected: %v\n%s", test.repository, err, output)
			}
		} else if err == nil || !bytes.Contains(output, []byte("set GITHUB_REPOSITORY=owner/repository")) {
			t.Fatalf("invalid repository %q with owner %q did not fail clearly: %v\n%s", test.repository, test.owner, err, output)
		}
	}
}

func TestGoWorkflowPackageAndContainerDelivery(t *testing.T) {
	data, err := swatch.Render(swatch.GoWorkflowDestination, swatch.Options{GoEnabled: true})
	if err != nil {
		t.Fatal(err)
	}
	var workflow struct {
		Jobs map[string]struct {
			If          string
			Permissions map[string]string
			Steps       []struct {
				Uses, Run string
				With      map[string]string
				Env       map[string]string
			}
		}
	}
	if err := yaml.Unmarshal(data, &workflow); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"snapshot", "release"} {
		job := workflow.Jobs[name]
		buildx, login, release, upload := -1, -1, -1, -1
		for index, step := range job.Steps {
			if strings.HasPrefix(step.Uses, "docker/setup-buildx-action@") {
				buildx = index
			}
			if strings.Contains(step.Run, "docker login") {
				login = index
				if !strings.Contains(step.Run, "docker login ghcr.io") || !strings.Contains(step.Run, "--password-stdin") || step.Env["GHCR_TOKEN"] != "${{ secrets.GITHUB_TOKEN }}" {
					t.Fatal("GHCR authentication must use the workflow token through standard input")
				}
			}
			if strings.HasPrefix(step.Uses, "goreleaser/goreleaser-action@") {
				release = index
				if name == "snapshot" && (!strings.Contains(step.With["args"], "--snapshot") || strings.Contains(step.With["args"], "--skip")) {
					t.Fatal("snapshots must build packages and local container images")
				}
			}
			if strings.HasPrefix(step.Uses, "actions/upload-artifact@") {
				upload = index
				for _, artifact := range []string{"dist/*.tar.gz", "dist/*.deb", "dist/*.rpm", "dist/*.apk", "dist/*_checksums.txt"} {
					if !strings.Contains(step.With["path"], artifact) {
						t.Fatalf("snapshot uploads lack %s", artifact)
					}
				}
			}
		}
		if buildx < 0 || release <= buildx {
			t.Fatalf("%s must set up Buildx before GoReleaser", name)
		}
		if name == "release" {
			if login < 0 || release <= login || job.Permissions["packages"] != "write" || !strings.Contains(job.If, "startsWith(github.ref, 'refs/tags/v')") {
				t.Fatal("tag releases must authenticate and have package write permission")
			}
		} else if login >= 0 || job.Permissions["packages"] != "" || job.Permissions["contents"] != "read" || upload <= release {
			t.Fatal("snapshots must upload local artifacts without registry credentials or write permissions")
		}
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
