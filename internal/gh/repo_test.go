package gh

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/cli/go-gh/v2/pkg/repository"
)

func TestRepoContext(t *testing.T) {
	restore := SetCurrentRepoFunc(func(dir string) (repository.Repository, error) {
		if dir != "." {
			t.Errorf("repository discovery directory = %q, want %q", dir, ".")
		}
		return repository.Repository{Host: "github.com", Owner: "wimpysworld", Name: "tailor"}, nil
	})
	t.Cleanup(restore)

	repo, ok := RepoContext()

	want := Repo{Host: "github.com", Owner: "wimpysworld", Name: "tailor"}
	if repo != want || !ok {
		t.Errorf("RepoContext() = %+v, %v, want %+v, true", repo, ok, want)
	}
}

func TestRepoContextError(t *testing.T) {
	restore := SetCurrentRepoFunc(func(string) (repository.Repository, error) {
		return repository.Repository{}, errors.New("no repo")
	})
	t.Cleanup(restore)

	repo, ok := RepoContext()

	if repo != (Repo{}) || ok {
		t.Errorf("RepoContext() = %+v, %v, want empty context", repo, ok)
	}
}

func TestRepoContextAtUsesSuppliedDirectory(t *testing.T) {
	configureGitHubHost(t)
	dir := initGitRepository(t, "https://github.com/supplied/project.git")

	repo, ok, err := RepoContextAt(dir)
	if err != nil {
		t.Fatalf("RepoContextAt() error = %v", err)
	}
	want := Repo{Host: "github.com", Owner: "supplied", Name: "project"}
	if repo != want || !ok {
		t.Errorf("RepoContextAt() = %+v, %v, want %+v, true", repo, ok, want)
	}
}

func TestRepoContextAtIgnoresGHRepo(t *testing.T) {
	configureGitHubHost(t)
	t.Setenv("GH_REPO", "wrong/override")
	dir := initGitRepository(t, "https://github.com/remote/checkout.git")

	repo, ok, err := RepoContextAt(dir)
	if err != nil {
		t.Fatalf("RepoContextAt() error = %v", err)
	}
	want := Repo{Host: "github.com", Owner: "remote", Name: "checkout"}
	if repo != want || !ok {
		t.Errorf("RepoContextAt() = %+v, %v, want %+v, true", repo, ok, want)
	}
}

func TestRepoContextAtParsesRemoteURLs(t *testing.T) {
	configureGitHubHost(t)
	tests := []struct {
		name   string
		remote string
	}{
		{name: "HTTPS", remote: "https://github.com/wimpysworld/tailor.git"},
		{name: "scp with user", remote: "git@github.com:wimpysworld/tailor.git"},
		{name: "scp without user", remote: "github.com:wimpysworld/tailor.git"},
		{name: "git+ssh", remote: "git+ssh://git@github.com/wimpysworld/tailor.git"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := initGitRepository(t, tt.remote)

			repo, ok, err := RepoContextAt(dir)
			if err != nil {
				t.Fatalf("RepoContextAt() error = %v", err)
			}
			want := Repo{Host: "github.com", Owner: "wimpysworld", Name: "tailor"}
			if repo != want || !ok {
				t.Errorf("RepoContextAt() = %+v, %v, want %+v, true", repo, ok, want)
			}
		})
	}
}

func TestRepoContextAtReturnsEnterpriseHost(t *testing.T) {
	t.Setenv("GH_CONFIG_DIR", t.TempDir())
	t.Setenv("GH_HOST", "ghe.example.com")
	t.Setenv("GH_TOKEN", "test-token")
	dir := initGitRepository(t, "https://ghe.example.com/acme/widgets.git")

	repo, ok, err := RepoContextAt(dir)
	if err != nil {
		t.Fatalf("RepoContextAt() error = %v", err)
	}
	want := Repo{Host: "ghe.example.com", Owner: "acme", Name: "widgets"}
	if repo != want || !ok {
		t.Errorf("RepoContextAt() = %+v, %v, want %+v, true", repo, ok, want)
	}
}

func TestRepoContextAtResolvesSSHAlias(t *testing.T) {
	configureGitHubHost(t)
	binDir := t.TempDir()
	sshPath := filepath.Join(binDir, "ssh")
	if err := os.WriteFile(sshPath, []byte("#!/bin/sh\nprintf 'hostname github.com\\n'\n"), 0o755); err != nil {
		t.Fatalf("writing fake ssh: %v", err)
	}
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))
	dir := initGitRepository(t, "git@github-work:wimpysworld/tailor.git")

	repo, ok, err := RepoContextAt(dir)
	if err != nil {
		t.Fatalf("RepoContextAt() error = %v", err)
	}
	want := Repo{Host: "github.com", Owner: "wimpysworld", Name: "tailor"}
	if repo != want || !ok {
		t.Errorf("RepoContextAt() = %+v, %v, want %+v, true", repo, ok, want)
	}
}

func TestRepoContextAtNoAuthenticatedHosts(t *testing.T) {
	t.Setenv("GH_CONFIG_DIR", t.TempDir())
	t.Setenv("GH_HOST", "")
	t.Setenv("GH_TOKEN", "")
	t.Setenv("GITHUB_TOKEN", "")
	dir := initGitRepository(t, "https://github.com/no-auth/project.git")

	repo, ok, err := RepoContextAt(dir)
	if err != nil {
		t.Fatalf("RepoContextAt() error = %v", err)
	}
	want := Repo{Host: "github.com", Owner: "no-auth", Name: "project"}
	if repo != want || !ok {
		t.Errorf("RepoContextAt() = %+v, %v, want %+v, true", repo, ok, want)
	}
}

func TestRepoContextAtNoRemote(t *testing.T) {
	configureGitHubHost(t)
	dir := initGitRepository(t, "")

	repo, ok, err := RepoContextAt(dir)
	if err != nil {
		t.Fatalf("RepoContextAt() error = %v", err)
	}
	if repo != (Repo{}) || ok {
		t.Errorf("RepoContextAt() = %+v, %v, want empty context", repo, ok)
	}
}

func TestRepoContextAtBadDir(t *testing.T) {
	_, _, err := RepoContextAt(filepath.Join(t.TempDir(), "missing"))
	if err == nil {
		t.Fatal("RepoContextAt() error = nil, want an error")
	}
}

func configureGitHubHost(t *testing.T) {
	t.Helper()
	t.Setenv("GH_CONFIG_DIR", t.TempDir())
	t.Setenv("GH_TOKEN", "test-token")
}

func initGitRepository(t *testing.T, remote string) string {
	t.Helper()
	dir := t.TempDir()
	runGit(t, dir, "init", "--quiet")
	if remote != "" {
		runGit(t, dir, "remote", "add", "origin", remote)
	}
	return dir
}

func runGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	commandArgs := append([]string{"-C", dir}, args...)
	output, err := exec.CommandContext(t.Context(), "git", commandArgs...).CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, output)
	}
}
