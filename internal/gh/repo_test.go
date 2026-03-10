package gh

import (
	"errors"
	"os"
	"testing"

	"github.com/cli/go-gh/v2/pkg/repository"
)

func TestRepoContext(t *testing.T) {
	tests := []struct {
		name      string
		repo      repository.Repository
		repoErr   error
		wantOwner string
		wantName  string
		wantOK    bool
	}{
		{
			name: "detects repo from remote",
			repo: repository.Repository{
				Host:  "github.com",
				Owner: "wimpysworld",
				Name:  "tailor",
			},
			wantOwner: "wimpysworld",
			wantName:  "tailor",
			wantOK:    true,
		},
		{
			name:    "no remote returns ok false",
			repoErr: errors.New("unable to determine current repository"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			restore := SetCurrentRepoFunc(func() (repository.Repository, error) {
				return tt.repo, tt.repoErr
			})
			t.Cleanup(restore)

			owner, name, ok := RepoContext()

			if owner != tt.wantOwner {
				t.Errorf("RepoContext() owner = %q, want %q", owner, tt.wantOwner)
			}
			if name != tt.wantName {
				t.Errorf("RepoContext() name = %q, want %q", name, tt.wantName)
			}
			if ok != tt.wantOK {
				t.Errorf("RepoContext() ok = %v, want %v", ok, tt.wantOK)
			}
		})
	}
}

func TestRepoContextAt(t *testing.T) {
	restore := SetCurrentRepoFunc(func() (repository.Repository, error) {
		return repository.Repository{Host: "github.com", Owner: "testowner", Name: "testrepo"}, nil
	})
	t.Cleanup(restore)

	dir := t.TempDir()

	owner, name, ok, err := RepoContextAt(dir)
	if err != nil {
		t.Fatalf("RepoContextAt() error = %v", err)
	}
	if !ok {
		t.Fatal("RepoContextAt() ok = false, want true")
	}
	if owner != "testowner" {
		t.Errorf("RepoContextAt() owner = %q, want %q", owner, "testowner")
	}
	if name != "testrepo" {
		t.Errorf("RepoContextAt() name = %q, want %q", name, "testrepo")
	}

	// Verify working directory is restored.
	cwd, _ := os.Getwd()
	if cwd == dir {
		t.Error("RepoContextAt() did not restore working directory")
	}
}

func TestRepoContextAtNoRepo(t *testing.T) {
	restore := SetCurrentRepoFunc(func() (repository.Repository, error) {
		return repository.Repository{}, errors.New("no repo")
	})
	t.Cleanup(restore)

	dir := t.TempDir()

	_, _, ok, err := RepoContextAt(dir)
	if err != nil {
		t.Fatalf("RepoContextAt() error = %v", err)
	}
	if ok {
		t.Error("RepoContextAt() ok = true, want false")
	}
}

func TestRepoContextAtBadDir(t *testing.T) {
	_, _, _, err := RepoContextAt("/nonexistent/path/that/does/not/exist")
	if err == nil {
		t.Fatal("RepoContextAt() expected error for non-existent dir, got nil")
	}
}
