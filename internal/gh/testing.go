package gh

import (
	"errors"

	"github.com/cli/go-gh/v2/pkg/api"
	"github.com/cli/go-gh/v2/pkg/repository"
)

// These test hooks change package globals. Callers must restore them and must not run in parallel.

// SetInspectWikiFunc replaces wiki inspection for tests and returns a restore function.
func SetInspectWikiFunc(fn func(string, string, string) error) func() {
	old := inspectWiki
	oldRead, oldRecheck := readWikiSnapshot, recheckWikiHead
	inspectWiki = fn
	readWikiSnapshot = func(string, string, string) (*WikiSnapshot, error) {
		return nil, errors.New("wiki snapshot test hook is required while inspection is overridden")
	}
	recheckWikiHead = func(string, string) error {
		return errors.New("wiki HEAD test hook is required while inspection is overridden")
	}
	return func() {
		inspectWiki = old
		readWikiSnapshot, recheckWikiHead = oldRead, oldRecheck
	}
}

// SetReadWikiSnapshotFunc replaces snapshot reads and returns a restore function.
// Install this hook after SetInspectWikiFunc when a test needs both hooks.
func SetReadWikiSnapshotFunc(fn func(string, string, string) (*WikiSnapshot, error)) func() {
	old := readWikiSnapshot
	readWikiSnapshot = fn
	return func() { readWikiSnapshot = old }
}

// SetRecheckWikiHeadFunc replaces HEAD rechecks and returns a restore function.
// Install this hook after SetInspectWikiFunc when a test needs both hooks.
func SetRecheckWikiHeadFunc(fn func(string, string) error) func() {
	old := recheckWikiHead
	recheckWikiHead = fn
	return func() { recheckWikiHead = old }
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
