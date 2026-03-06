package gh

import (
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
