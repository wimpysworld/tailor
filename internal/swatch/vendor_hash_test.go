package swatch_test

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/wimpysworld/tailor/internal/swatch"
	"gopkg.in/yaml.v3"
)

type vendorHashWorkflow struct {
	On          map[string]yaml.Node `yaml:"on"`
	Permissions map[string]string    `yaml:"permissions"`
	Jobs        map[string]struct {
		If          string            `yaml:"if"`
		Permissions map[string]string `yaml:"permissions"`
		Steps       []struct {
			ID  string `yaml:"id"`
			Run string `yaml:"run"`
		} `yaml:"steps"`
	} `yaml:"jobs"`
}

func readVendorHashWorkflow(t *testing.T) ([]byte, vendorHashWorkflow) {
	t.Helper()
	content, err := swatch.Content(swatch.GoVendorHashDestination)
	if err != nil {
		t.Fatal(err)
	}
	var workflow vendorHashWorkflow
	if err := yaml.Unmarshal(content, &workflow); err != nil {
		t.Fatal(err)
	}
	return content, workflow
}

func vendorHashScript(t *testing.T, workflow vendorHashWorkflow, job, id string) string {
	t.Helper()
	steps := workflow.Jobs[job].Steps
	for _, step := range steps {
		if step.Run != "" && (id == "" || step.ID == id) {
			return step.Run
		}
	}
	t.Fatalf("job %s has no script %q", job, id)
	return ""
}

func TestVendorHashWorkflowContract(t *testing.T) {
	content, workflow := readVendorHashWorkflow(t)
	if len(workflow.On) != 1 || workflow.On["pull_request"].Kind == 0 {
		t.Fatalf("triggers = %v, want pull_request only", workflow.On)
	}
	if workflow.Permissions == nil || len(workflow.Permissions) != 0 {
		t.Fatalf("top-level permissions = %v, want {}", workflow.Permissions)
	}
	for job, want := range map[string]map[string]string{"compute": {"contents": "read"}, "push": {"contents": "write"}} {
		got := workflow.Jobs[job].Permissions
		if len(got) != len(want) || got["contents"] != want["contents"] {
			t.Errorf("%s permissions = %v, want %v", job, got, want)
		}
	}
	for _, guard := range []string{
		"github.event.pull_request.user.login == 'dependabot[bot]'",
		"github.event.pull_request.head.repo.full_name == github.repository",
		"startsWith(github.head_ref, 'dependabot/go_modules/')",
	} {
		if !strings.Contains(workflow.Jobs["compute"].If, guard) {
			t.Errorf("compute guard lacks %q", guard)
		}
	}
	for _, forbidden := range []string{"pull_request_target", "secrets.", "--force", "[[TAILOR_"} {
		if bytes.Contains(content, []byte(forbidden)) {
			t.Errorf("workflow contains %q", forbidden)
		}
	}
	own, err := os.ReadFile(filepath.Join("..", "..", ".github", "workflows", "tailor-vendor-hash.yml"))
	if err != nil || !bytes.Equal(own, content) {
		t.Errorf("Tailor's own workflow differs from the swatch: %v", err)
	}
}

func TestVendorHashWorkflowFindsDeclaredHash(t *testing.T) {
	_, workflow := readVendorHashWorkflow(t)
	script := vendorHashScript(t, workflow, "compute", "declared")
	const hash = "sha256-Qn3XxM9KybGXE92RT26ldj+x8HIReXDGPBkAqv1NnNQ="
	for _, tt := range []struct {
		name, body, want string
	}{
		{"literal", "  vendorHash = \"" + hash + "\";\n", "old=" + hash + "\n"},
		{"null", "  vendorHash = null;\n", ""},
		{"two", "  vendorHash = \"" + hash + "\";\n  vendorHash = \"" + hash + "\";\n", ""},
		{"short", "  vendorHash = \"sha256-AAAA=\";\n", ""},
	} {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			writeVendorHashFile(t, filepath.Join(dir, "flake.nix"), "{ }\n")
			writeVendorHashFile(t, filepath.Join(dir, "nix", "package.nix"), "{\n"+tt.body+"}\n")
			output := filepath.Join(dir, "output")
			cmd := exec.CommandContext(t.Context(), "bash", "-euo", "pipefail", "-c", script)
			cmd.Dir = dir
			cmd.Env = append(os.Environ(), "GITHUB_OUTPUT="+output)
			if out, err := cmd.CombinedOutput(); err != nil {
				t.Fatalf("script failed: %v\n%s", err, out)
			}
			got, _ := os.ReadFile(output)
			if string(got) != tt.want {
				t.Fatalf("output = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestVendorHashWorkflowPushesFix(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git is not available")
	}
	_, workflow := readVendorHashWorkflow(t)
	script := vendorHashScript(t, workflow, "push", "")
	const (
		old    = "sha256-Qn3XxM9KybGXE92RT26ldj+x8HIReXDGPBkAqv1NnNQ="
		fixed  = "sha256-B/b+0123456789abcdefghijklmnopqrstuvwxyzABC="
		branch = "dependabot/go_modules/gomod-1"
	)
	env := append(os.Environ(), "GIT_CONFIG_NOSYSTEM=1", "GIT_CONFIG_GLOBAL="+os.DevNull,
		"GIT_AUTHOR_NAME=Test", "GIT_AUTHOR_EMAIL=test@example.invalid",
		"GIT_COMMITTER_NAME=Test", "GIT_COMMITTER_EMAIL=test@example.invalid")
	run := func(dir string, extra []string, name string, args ...string) (string, error) {
		cmd := exec.CommandContext(t.Context(), name, args...)
		cmd.Dir = dir
		cmd.Env = append(append([]string{}, env...), extra...)
		out, err := cmd.CombinedOutput()
		return string(out), err
	}
	setup := func(t *testing.T) (string, string) {
		t.Helper()
		root := t.TempDir()
		remote, work := filepath.Join(root, "remote.git"), filepath.Join(root, "work")
		for _, args := range [][]string{{"init", "--bare", remote}, {"init", "-b", branch, work}} {
			if out, err := run(root, nil, "git", args...); err != nil {
				t.Fatalf("git %v: %v\n%s", args, err, out)
			}
		}
		writeVendorHashFile(t, filepath.Join(work, "nix", "package.nix"), "{\n  vendorHash = \""+old+"\";\n}\n")
		for _, args := range [][]string{{"add", "."}, {"commit", "-m", "deps"}, {"remote", "add", "origin", remote}, {"push", "origin", "HEAD:refs/heads/" + branch}} {
			if out, err := run(work, nil, "git", args...); err != nil {
				t.Fatalf("git %v: %v\n%s", args, err, out)
			}
		}
		return remote, work
	}

	t.Run("fix", func(t *testing.T) {
		remote, work := setup(t)
		if out, err := run(work, []string{"OLD=" + old, "NEW=" + fixed, "HEAD_REF=" + branch}, "bash", "-euo", "pipefail", "-c", script); err != nil {
			t.Fatalf("script failed: %v\n%s", err, out)
		}
		body, err := run(remote, nil, "git", "show", branch+":nix/package.nix")
		if err != nil || !strings.Contains(body, "\""+fixed+"\"") || strings.Contains(body, old) {
			t.Fatalf("remote package.nix = %q, %v", body, err)
		}
		message, err := run(remote, nil, "git", "log", "-1", "--format=%B", branch)
		if err != nil || !strings.Contains(message, "[dependabot skip]") {
			t.Fatalf("commit message = %q, %v", message, err)
		}
	})
	for _, tt := range []struct{ name, old, new string }{
		{"malformed", old, "sha256-$(touch pwned)AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA="},
		{"absent", fixed, old},
	} {
		t.Run(tt.name, func(t *testing.T) {
			remote, work := setup(t)
			before, _ := run(remote, nil, "git", "rev-parse", branch)
			if out, err := run(work, []string{"OLD=" + tt.old, "NEW=" + tt.new, "HEAD_REF=" + branch}, "bash", "-euo", "pipefail", "-c", script); err == nil {
				t.Fatalf("script succeeded, want failure\n%s", out)
			}
			after, _ := run(remote, nil, "git", "rev-parse", branch)
			if before != after {
				t.Fatal("script pushed after a failed check")
			}
		})
	}
}

func writeVendorHashFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}
