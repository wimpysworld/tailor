package gh

import (
	"errors"
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
