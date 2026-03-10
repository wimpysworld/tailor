package gh

import (
	"fmt"
	"os"

	"github.com/cli/go-gh/v2/pkg/repository"
)

// currentRepo wraps repository.Current for testability.
var currentRepo = repository.Current

// RepoContext detects the GitHub repository for the current directory.
// It returns the owner and name if a GitHub remote is found.
// When no remote is configured, it returns ok=false.
func RepoContext() (owner string, name string, ok bool) {
	repo, repoErr := currentRepo()
	if repoErr != nil {
		return "", "", false
	}
	return repo.Owner, repo.Name, true
}

// RepoContextAt detects the GitHub repository for the given directory.
// It temporarily changes the working directory to dir before querying
// git remotes, then restores the original directory. Returns the owner
// and name if a GitHub remote is found; ok=false otherwise.
func RepoContextAt(dir string) (owner string, name string, ok bool, err error) {
	orig, err := os.Getwd()
	if err != nil {
		return "", "", false, fmt.Errorf("getting working directory: %w", err)
	}
	if err := os.Chdir(dir); err != nil {
		return "", "", false, fmt.Errorf("changing to directory %q: %w", dir, err)
	}
	defer os.Chdir(orig) //nolint:errcheck

	repo, repoErr := currentRepo()
	if repoErr != nil {
		return "", "", false, nil
	}
	return repo.Owner, repo.Name, true, nil
}
