package ghfake

import (
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/cli/go-gh/v2/pkg/api"
	"github.com/cli/go-gh/v2/pkg/repository"
	"github.com/wimpysworld/tailor/internal/gh"
	"github.com/wimpysworld/tailor/internal/testutil"
)

// FakeAuth installs a tokenForHost stub that returns the given token.
func FakeAuth(t *testing.T, token string) {
	t.Helper()
	restore := gh.SetTokenForHostFunc(func(string) (string, string) {
		return token, "oauth_token"
	})
	t.Cleanup(restore)
}

// FakeUserAPI installs a REST client stub backed by a test server. GET /user
// responds with the given status; on http.StatusOK it returns the login.
func FakeUserAPI(t *testing.T, status int, login string) {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/user") {
			if status != http.StatusOK {
				w.WriteHeader(status)
				fmt.Fprint(w, `{"message":"error"}`)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprintf(w, `{"login":%q}`, login)
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	t.Cleanup(server.Close)
	client := testutil.NewTestClient(t, server)
	restore := gh.SetNewRESTClientFunc(func(string) (*api.RESTClient, error) {
		return client, nil
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
	restore := gh.SetCurrentRepoFunc(func(string) (repository.Repository, error) {
		return repo, nil
	})
	t.Cleanup(restore)
}

// FakeNoRepo installs a currentRepo stub that returns an error.
func FakeNoRepo(t *testing.T) {
	t.Helper()
	restore := gh.SetCurrentRepoFunc(func(string) (repository.Repository, error) {
		return repository.Repository{}, errors.New("not a git repository")
	})
	t.Cleanup(restore)
}
