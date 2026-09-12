package gh

import (
	"github.com/cli/go-gh/v2/pkg/api"
	"github.com/cli/go-gh/v2/pkg/repository"
)

// These test hooks change package globals. Callers must restore them and must not run in parallel.

// SetInspectWikiFunc replaces wiki inspection for tests and returns a restore function.
func SetInspectWikiFunc(fn func(string, string, string) error) func() {
	old := inspectWiki
	inspectWiki = fn
	return func() { inspectWiki = old }
}

// SetTokenForHostFunc replaces the tokenForHost function for testing.
// It returns a restore function for t.Cleanup.
func SetTokenForHostFunc(fn func(string) (string, string)) func() {
	old := tokenForHost
	tokenForHost = fn
	return func() { tokenForHost = old }
}

// SetNewRESTClientFunc replaces REST client construction for testing.
// It returns a restore function for t.Cleanup.
func SetNewRESTClientFunc(fn func(string) (*api.RESTClient, error)) func() {
	old := newRESTClient
	newRESTClient = fn
	return func() { newRESTClient = old }
}

// SetCurrentRepoFunc replaces repository discovery for testing.
// It returns a restore function for t.Cleanup.
func SetCurrentRepoFunc(fn func(string) (repository.Repository, error)) func() {
	old := currentRepoAt
	currentRepoAt = fn
	return func() { currentRepoAt = old }
}
