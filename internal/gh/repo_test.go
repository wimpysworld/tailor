package gh

import (
	"errors"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/cli/go-gh/v2/pkg/repository"
)

func TestRepoContext(t *testing.T) {
	restore := SetCurrentRepoFunc(func() (repository.Repository, error) {
		return repository.Repository{Host: "github.com", Owner: "wimpysworld", Name: "tailor"}, nil
	})
	t.Cleanup(restore)

	owner, name, ok := RepoContext()

	if owner != "wimpysworld" || name != "tailor" || !ok {
		t.Errorf("RepoContext() = %q, %q, %v, want %q, %q, true", owner, name, ok, "wimpysworld", "tailor")
	}
}

func TestRepoContextError(t *testing.T) {
	restore := SetCurrentRepoFunc(func() (repository.Repository, error) {
		return repository.Repository{}, errors.New("no repo")
	})
	t.Cleanup(restore)

	owner, name, ok := RepoContext()

	if owner != "" || name != "" || ok {
		t.Errorf("RepoContext() = %q, %q, %v, want empty context", owner, name, ok)
	}
}

func TestRepoContextAtUsesSuppliedDirectory(t *testing.T) {
	configureGitHubHost(t)
	dir := initGitRepository(t, "https://github.com/supplied/project.git")

	owner, name, ok, err := RepoContextAt(dir)
	if err != nil {
		t.Fatalf("RepoContextAt() error = %v", err)
	}
	if owner != "supplied" || name != "project" || !ok {
		t.Errorf("RepoContextAt() = %q, %q, %v, want %q, %q, true", owner, name, ok, "supplied", "project")
	}
}

func TestRepoContextAtIgnoresGHRepo(t *testing.T) {
	configureGitHubHost(t)
	t.Setenv("GH_REPO", "wrong/override")
	dir := initGitRepository(t, "https://github.com/remote/checkout.git")

	owner, name, ok, err := RepoContextAt(dir)
	if err != nil {
		t.Fatalf("RepoContextAt() error = %v", err)
	}
	if owner != "remote" || name != "checkout" || !ok {
		t.Errorf("RepoContextAt() = %q, %q, %v, want %q, %q, true", owner, name, ok, "remote", "checkout")
	}
}

func TestRepoContextAtParsesRemoteURLs(t *testing.T) {
	configureGitHubHost(t)
	tests := []struct {
		name   string
		remote string
	}{
		{name: "HTTPS", remote: "https://github.com/wimpysworld/tailor.git"},
		{name: "SSH", remote: "git@github.com:wimpysworld/tailor.git"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := initGitRepository(t, tt.remote)

			owner, name, ok, err := RepoContextAt(dir)
			if err != nil {
				t.Fatalf("RepoContextAt() error = %v", err)
			}
			if owner != "wimpysworld" || name != "tailor" || !ok {
				t.Errorf("RepoContextAt() = %q, %q, %v, want %q, %q, true", owner, name, ok, "wimpysworld", "tailor")
			}
		})
	}
}

func TestRepoContextAtNoRemote(t *testing.T) {
	configureGitHubHost(t)
	dir := initGitRepository(t, "")

	owner, name, ok, err := RepoContextAt(dir)
	if err != nil {
		t.Fatalf("RepoContextAt() error = %v", err)
	}
	if owner != "" || name != "" || ok {
		t.Errorf("RepoContextAt() = %q, %q, %v, want empty context", owner, name, ok)
	}
}

func TestRepoContextAtBadDir(t *testing.T) {
	_, _, _, err := RepoContextAt(filepath.Join(t.TempDir(), "missing"))
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
