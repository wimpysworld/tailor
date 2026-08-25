package docket

import (
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/cli/go-gh/v2/pkg/api"
	"github.com/wimpysworld/tailor/internal/gh"
	"github.com/wimpysworld/tailor/internal/termtext"
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
func Run(client *api.RESTClient) (*Result, error) {
	r := &Result{
		User:       "(none)",
		Repository: "(none)",
		Auth:       "not authenticated",
	}

	repo, ok := gh.RepoContext()
	if ok {
		r.Repository = repo.Owner + "/" + repo.Name
	}

	// Resolve one effective host so the auth check and the REST client
	// cannot disagree when no repository context exists.
	host := gh.ResolveHost(repo.Host)

	if err := gh.CheckAuth(host); err != nil {
		return r, nil
	}

	if client == nil {
		var err error
		client, err = gh.NewRESTClient(host)
		if err != nil {
			return nil, err
		}
	}

	username, err := gh.FetchUsername(client)
	if err != nil {
		var httpErr *api.HTTPError
		if errors.As(err, &httpErr) && httpErr.StatusCode == http.StatusUnauthorized {
			return r, nil
		}
		return nil, err
	}
	r.User = username
	r.Auth = "authenticated"

	return r, nil
}

// labelWidth is the fixed column width for field labels in formatted output.
const labelWidth = 16

// FormatOutput produces the docket command output from a Result.
func FormatOutput(r *Result) string {
	var b strings.Builder
	fmt.Fprintf(&b, "%-*s%s\n", labelWidth, "user:", termtext.EscapeControlText(r.User))
	fmt.Fprintf(&b, "%-*s%s\n", labelWidth, "repository:", termtext.EscapeControlText(r.Repository))
	fmt.Fprintf(&b, "%-*s%s\n", labelWidth, "auth:", termtext.EscapeControlText(r.Auth))
	return b.String()
}
