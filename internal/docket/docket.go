package docket

import (
	"fmt"
	"strings"

	"github.com/cli/go-gh/v2/pkg/api"
	"github.com/wimpysworld/tailor/internal/gh"
)

// Result holds the diagnostic information gathered by Run.
type Result struct {
	User       string
	Repository string
	Auth       string
}

// Run gathers diagnostic context: repository, authentication, and username.
// Missing information is represented as "(none)" or "not authenticated"
// in the returned Result.
func Run(client *api.RESTClient) *Result {
	r := &Result{
		User:       "(none)",
		Repository: "(none)",
		Auth:       "not authenticated",
	}

	owner, name, ok := gh.RepoContext()
	if ok {
		r.Repository = owner + "/" + name
	}

	if err := gh.CheckAuth(); err != nil {
		return r
	}
	r.Auth = "authenticated"

	if client == nil {
		var err error
		client, err = api.DefaultRESTClient()
		if err != nil {
			return r
		}
	}

	username, err := gh.FetchUsername(client)
	if err != nil {
		return r
	}
	r.User = username

	return r
}

// labelWidth is the fixed column width for field labels in formatted output.
const labelWidth = 16

// FormatOutput produces the docket command output from a Result.
func FormatOutput(r *Result) string {
	var b strings.Builder
	fmt.Fprintf(&b, "%-*s%s\n", labelWidth, "user:", r.User)
	fmt.Fprintf(&b, "%-*s%s\n", labelWidth, "repository:", r.Repository)
	fmt.Fprintf(&b, "%-*s%s\n", labelWidth, "auth:", r.Auth)
	return b.String()
}
