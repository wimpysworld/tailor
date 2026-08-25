package alter

import (
	"fmt"
	"io"

	"github.com/cli/go-gh/v2/pkg/api"
)

// RepoTarget identifies the GitHub repository that the alter processors act
// on: the API client, the owner and name, and whether a repository context
// exists.
type RepoTarget struct {
	Client  *api.RESTClient
	Owner   string
	Name    string
	HasRepo bool
	Stderr  io.Writer
}

func (t RepoTarget) stderr() io.Writer {
	if t.Stderr == nil {
		return io.Discard
	}
	return t.Stderr
}

// missingRepo reports whether no repository context exists. When the context
// is missing it warns that the named subject is deferred until a remote is
// configured.
func (t RepoTarget) missingRepo(subject string) bool {
	if t.HasRepo {
		return false
	}
	fmt.Fprintf(t.stderr(), "No GitHub repository context found. %s will be applied once a remote is configured.\n", subject)
	return true
}
