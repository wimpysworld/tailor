package docket

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
)

// testTransport redirects all requests to the test server, preserving the
// original request path so the test handler can route by path.
type testTransport struct {
	server *httptest.Server
}

func (t *testTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	req.URL.Scheme = "http"
	req.URL.Host = t.server.Listener.Addr().String()
	return http.DefaultTransport.RoundTrip(req)
}

// newTestClient creates an api.RESTClient that sends all requests to the
// given test server.
func newTestClient(t *testing.T, server *httptest.Server) *api.RESTClient {
	t.Helper()
	client, err := api.NewRESTClient(api.ClientOptions{
		Host:      "github.com",
		AuthToken: "test-token",
		Transport: &testTransport{server: server},
	})
	if err != nil {
		t.Fatalf("NewRESTClient: %v", err)
	}
	return client
}

// fakeAuth installs a tokenForHost stub that returns the given token.
func fakeAuth(t *testing.T, token string) {
	t.Helper()
	restore := gh.SetTokenForHostFunc(func(string) (string, string) {
		return token, "oauth_token"
	})
	t.Cleanup(restore)
}

// fakeRepo installs a currentRepo stub that returns the given owner and name.
func fakeRepo(t *testing.T, owner, name string) {
	t.Helper()
	restore := gh.SetCurrentRepoFunc(func() (repository.Repository, error) {
		return repository.Repository{Owner: owner, Name: name}, nil
	})
	t.Cleanup(restore)
}

// fakeNoRepo installs a currentRepo stub that returns an error.
func fakeNoRepo(t *testing.T) {
	t.Helper()
	restore := gh.SetCurrentRepoFunc(func() (repository.Repository, error) {
		return repository.Repository{}, errors.New("not a git repository")
	})
	t.Cleanup(restore)
}

// setupDocketTest configures auth and repo fakes, optionally starts an
// httptest server, and returns a *api.RESTClient (nil when token is empty).
func setupDocketTest(t *testing.T, token string, hasRepo bool, repoOwner, repoName string, apiStatus int, apiBody string) *api.RESTClient {
	t.Helper()
	fakeAuth(t, token)
	if hasRepo {
		fakeRepo(t, repoOwner, repoName)
	} else {
		fakeNoRepo(t)
	}

	if token == "" {
		return nil
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/user" {
			http.NotFound(w, r)
			return
		}
		w.WriteHeader(apiStatus)
		fmt.Fprint(w, apiBody)
	}))
	t.Cleanup(server.Close)
	return newTestClient(t, server)
}

func TestRun(t *testing.T) {
	tests := []struct {
		name      string
		token     string
		hasRepo   bool
		repoOwner string
		repoName  string
		apiStatus int
		apiBody   string
		wantUser  string
		wantRepo  string
		wantAuth  string
	}{
		{
			name:      "authenticated with repo",
			token:     "gho_test",
			hasRepo:   true,
			repoOwner: "octocat",
			repoName:  "my-project",
			apiStatus: http.StatusOK,
			apiBody:   `{"login":"octocat"}`,
			wantUser:  "octocat",
			wantRepo:  "octocat/my-project",
			wantAuth:  "authenticated",
		},
		{
			name:      "authenticated without repo",
			token:     "gho_test",
			hasRepo:   false,
			apiStatus: http.StatusOK,
			apiBody:   `{"login":"octocat"}`,
			wantUser:  "octocat",
			wantRepo:  "(none)",
			wantAuth:  "authenticated",
		},
		{
			name:     "not authenticated",
			token:    "",
			hasRepo:  false,
			wantUser: "(none)",
			wantRepo: "(none)",
			wantAuth: "not authenticated",
		},
		{
			name:      "not authenticated but has repo",
			token:     "",
			hasRepo:   true,
			repoOwner: "octocat",
			repoName:  "my-project",
			wantUser:  "(none)",
			wantRepo:  "octocat/my-project",
			wantAuth:  "not authenticated",
		},
		{
			name:      "authenticated but API failure",
			token:     "gho_test",
			hasRepo:   false,
			apiStatus: http.StatusInternalServerError,
			apiBody:   `{"message":"Internal Server Error"}`,
			wantUser:  "(none)",
			wantRepo:  "(none)",
			wantAuth:  "authenticated",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := setupDocketTest(t, tt.token, tt.hasRepo, tt.repoOwner, tt.repoName, tt.apiStatus, tt.apiBody)

			result := Run(client)

			if result.User != tt.wantUser {
				t.Errorf("User = %q, want %q", result.User, tt.wantUser)
			}
			if result.Repository != tt.wantRepo {
				t.Errorf("Repository = %q, want %q", result.Repository, tt.wantRepo)
			}
			if result.Auth != tt.wantAuth {
				t.Errorf("Auth = %q, want %q", result.Auth, tt.wantAuth)
			}
		})
	}
}

func TestFormatOutput(t *testing.T) {
	tests := []struct {
		name   string
		result *Result
		want   string
	}{
		{
			name: "all fields populated",
			result: &Result{
				User:       "octocat",
				Repository: "octocat/my-project",
				Auth:       "authenticated",
			},
			want: "user:           octocat\n" +
				"repository:     octocat/my-project\n" +
				"auth:           authenticated\n",
		},
		{
			name: "all none",
			result: &Result{
				User:       "(none)",
				Repository: "(none)",
				Auth:       "not authenticated",
			},
			want: "user:           (none)\n" +
				"repository:     (none)\n" +
				"auth:           not authenticated\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := FormatOutput(tt.result)
			if got != tt.want {
				t.Errorf("FormatOutput() =\n%s\nwant:\n%s", got, tt.want)
			}
		})
	}

	t.Run("column alignment", func(t *testing.T) {
		got := FormatOutput(&Result{
			User:       "u",
			Repository: "r",
			Auth:       "a",
		})
		lines := strings.Split(strings.TrimSuffix(got, "\n"), "\n")
		if len(lines) != 3 {
			t.Fatalf("expected 3 lines, got %d", len(lines))
		}
		labels := []string{"user:", "repository:", "auth:"}
		for i, line := range lines {
			prefix := fmt.Sprintf("%-16s", labels[i])
			if !strings.HasPrefix(line, prefix) {
				t.Errorf("line %d: label not padded to 16 chars\ngot:  %q\nwant prefix: %q", i, line, prefix)
			}
		}
	})
}
