package ghfake

import (
	"errors"
	"testing"

	"github.com/cli/go-gh/v2/pkg/repository"
	"github.com/wimpysworld/tailor/internal/gh"
)

// FakeAuth installs a tokenForHost stub that returns the given token.
func FakeAuth(t *testing.T, token string) {
	t.Helper()
	restore := gh.SetTokenForHostFunc(func(string) (string, string) {
		return token, "oauth_token"
	})
	t.Cleanup(restore)
}

// FakeRepo installs a currentRepo stub that returns the given owner and name.
func FakeRepo(t *testing.T, owner, name string) {
	t.Helper()
	repo, err := repository.Parse(owner + "/" + name)
	if err != nil {
		t.Fatal(err)
	}
	restore := gh.SetCurrentRepoFunc(func() (repository.Repository, error) {
		return repo, nil
	})
	t.Cleanup(restore)
}

// FakeNoRepo installs a currentRepo stub that returns an error.
func FakeNoRepo(t *testing.T) {
	t.Helper()
	restore := gh.SetCurrentRepoFunc(func() (repository.Repository, error) {
		return repository.Repository{}, errors.New("not a git repository")
	})
	t.Cleanup(restore)
}
