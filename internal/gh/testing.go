package gh

import "github.com/cli/go-gh/v2/pkg/repository"

// SetTokenForHostFunc replaces the tokenForHost function for testing.
// It returns a restore function for t.Cleanup.
func SetTokenForHostFunc(fn func(string) (string, string)) func() {
	old := tokenForHost
	tokenForHost = fn
	return func() { tokenForHost = old }
}

// SetCurrentRepoFunc replaces the currentRepo function for testing.
// It returns a restore function for t.Cleanup.
func SetCurrentRepoFunc(fn func() (repository.Repository, error)) func() {
	old := currentRepo
	currentRepo = fn
	return func() { currentRepo = old }
}
