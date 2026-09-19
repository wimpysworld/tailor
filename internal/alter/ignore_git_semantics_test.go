package alter

import (
	"bytes"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

type gitIgnoreCheck struct {
	path       string
	wantSource string
}

type gitIgnoreFixture struct {
	name   string
	files  map[string]string
	paths  []string
	checks []gitIgnoreCheck
}

func TestGitIgnoreSemantics(t *testing.T) {
	fixtures := []gitIgnoreFixture{
		{
			name: "include text is a literal pattern, not an import",
			files: map[string]string{
				".gitignore":   "include:extra.ignore\n",
				"extra.ignore": "imported.log\n",
			},
			paths: []string{"include:extra.ignore", "imported.log"},
			checks: []gitIgnoreCheck{
				{path: "include:extra.ignore", wantSource: ".gitignore:1:include:extra.ignore"},
				{path: "imported.log"},
			},
		},
		{
			name: "root and nested files have different scopes",
			files: map[string]string{
				".gitignore":        "*.tmp\n/root-only.log\n",
				"nested/.gitignore": "local.log\n",
			},
			paths: []string{"deep/cache.tmp", "root-only.log", "nested/root-only.log", "nested/local.log", "local.log"},
			checks: []gitIgnoreCheck{
				{path: "deep/cache.tmp", wantSource: ".gitignore:1:*.tmp"},
				{path: "root-only.log", wantSource: ".gitignore:2:/root-only.log"},
				{path: "nested/root-only.log"},
				{path: "nested/local.log", wantSource: "nested/.gitignore:1:local.log"},
				{path: "local.log"},
			},
		},
		{
			name: "a nested negation overrides a root pattern",
			files: map[string]string{
				".gitignore":        "nested/*.log\n",
				"nested/.gitignore": "!keep.log\n",
			},
			paths: []string{"nested/keep.log", "nested/drop.log"},
			checks: []gitIgnoreCheck{
				{path: "nested/keep.log", wantSource: "nested/.gitignore:1:!keep.log"},
				{path: "nested/drop.log", wantSource: ".gitignore:1:nested/*.log"},
			},
		},
		{
			name:  "a later negation overrides a positive pattern",
			files: map[string]string{".gitignore": "*.log\n!important.log\n"},
			paths: []string{"error.log", "important.log"},
			checks: []gitIgnoreCheck{
				{path: "error.log", wantSource: ".gitignore:1:*.log"},
				{path: "important.log", wantSource: ".gitignore:2:!important.log"},
			},
		},
		{
			name:  "a child cannot be re-included below an ignored ancestor",
			files: map[string]string{".gitignore": "build/\n!build/keep.txt\n"},
			paths: []string{"build/keep.txt"},
			checks: []gitIgnoreCheck{
				{path: "build/keep.txt", wantSource: ".gitignore:1:build/"},
			},
		},
		{
			name:  "ancestor reinclusion permits child reinclusion",
			files: map[string]string{".gitignore": "build/\n!build/\nbuild/*\n!build/keep.txt\n"},
			paths: []string{"build/keep.txt"},
			checks: []gitIgnoreCheck{
				{path: "build/keep.txt", wantSource: ".gitignore:4:!build/keep.txt"},
			},
		},
		{
			name:  "ordered duplicates keep last-match behaviour",
			files: map[string]string{".gitignore": "same.log\n!same.log\nsame.log\nother.log\n!other.log\n"},
			paths: []string{"same.log", "other.log"},
			checks: []gitIgnoreCheck{
				{path: "same.log", wantSource: ".gitignore:3:same.log"},
				{path: "other.log", wantSource: ".gitignore:5:!other.log"},
			},
		},
		{
			name:  "an appended Pages rule overrides an earlier negation",
			files: map[string]string{".gitignore": "!/pages/public/\n/pages/public/\n"},
			paths: []string{"pages/public/index.html"},
			checks: []gitIgnoreCheck{
				{path: "pages/public/index.html", wantSource: ".gitignore:2:/pages/public/"},
			},
		},
		{
			name:  "a later negation overrides an earlier Pages rule",
			files: map[string]string{".gitignore": "/pages/public/\n!/pages/public/\n"},
			paths: []string{"pages/public/index.html"},
			checks: []gitIgnoreCheck{
				{path: "pages/public/index.html"},
			},
		},
	}

	for _, fixture := range fixtures {
		t.Run(fixture.name, func(t *testing.T) {
			repo, env := newGitIgnoreRepository(t)
			for path, content := range fixture.files {
				writeOnDisk(t, repo, path, []byte(content))
			}
			for _, path := range fixture.paths {
				if _, exists := fixture.files[path]; !exists {
					writeOnDisk(t, repo, path, nil)
				}
			}
			for _, check := range fixture.checks {
				checkGitIgnore(t, repo, env, check)
			}
		})
	}
}

func TestGitIgnoreHistoricalDuplicateSurvivesGeneratedRemoval(t *testing.T) {
	repo, env := newGitIgnoreRepository(t)
	const path = "generated/result.txt"
	writeOnDisk(t, repo, path, nil)

	writeOnDisk(t, repo, ".gitignore", []byte("# generated body\n/generated/\n# project history\n/generated/\n"))
	checkGitIgnore(t, repo, env, gitIgnoreCheck{path: path, wantSource: ".gitignore:4:/generated/"})

	writeOnDisk(t, repo, ".gitignore", []byte("# generated body\n# project history\n/generated/\n"))
	checkGitIgnore(t, repo, env, gitIgnoreCheck{path: path, wantSource: ".gitignore:3:/generated/"})
}

func newGitIgnoreRepository(t *testing.T) (string, []string) {
	t.Helper()
	repo := t.TempDir()
	configHome := filepath.Join(repo, "config-home")
	if err := os.Mkdir(configHome, 0o755); err != nil {
		t.Fatal(err)
	}
	env := isolatedGitEnvironment(configHome)
	cmd := exec.CommandContext(t.Context(), "git", "-C", repo, "init", "--quiet")
	cmd.Env = env
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git init: %v: %s", err, output)
	}
	return repo, env
}

func isolatedGitEnvironment(configHome string) []string {
	env := make([]string, 0, len(os.Environ())+5)
	for _, variable := range os.Environ() {
		name, _, _ := strings.Cut(variable, "=")
		if strings.HasPrefix(name, "GIT_") || name == "HOME" || name == "XDG_CONFIG_HOME" {
			continue
		}
		env = append(env, variable)
	}
	return append(env,
		"GIT_CONFIG_NOSYSTEM=1",
		"GIT_CONFIG_SYSTEM="+os.DevNull,
		"GIT_CONFIG_GLOBAL="+os.DevNull,
		"HOME="+configHome,
		"XDG_CONFIG_HOME="+configHome,
	)
}

func checkGitIgnore(t *testing.T, repo string, env []string, check gitIgnoreCheck) {
	t.Helper()
	cmd := exec.CommandContext(t.Context(), "git", "-C", repo, "check-ignore", "--no-index", "-v", "--", check.path)
	cmd.Env = env
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	if stderr.Len() != 0 {
		t.Fatalf("git check-ignore %q stderr = %q", check.path, stderr.String())
	}
	if check.wantSource == "" {
		var exitError *exec.ExitError
		if !errors.As(err, &exitError) || exitError.ExitCode() != 1 || stdout.Len() != 0 {
			t.Fatalf("git check-ignore %q = %q, error = %v, want no match", check.path, stdout.String(), err)
		}
		return
	}
	if err != nil {
		t.Fatalf("git check-ignore %q: %v", check.path, err)
	}
	want := check.wantSource + "\t" + check.path + "\n"
	if stdout.String() != want {
		t.Fatalf("git check-ignore %q = %q, want %q", check.path, stdout.String(), want)
	}
}
