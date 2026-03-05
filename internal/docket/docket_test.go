package docket

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/cli/go-gh/v2/pkg/api"
	"github.com/wimpysworld/tailor/internal/ghfake"
	"github.com/wimpysworld/tailor/internal/testutil"
)

// docketTestOpts configures the test environment for setupDocketTest.
type docketTestOpts struct {
	token     string
	repoOwner string
	repoName  string
	apiStatus int
	apiBody   string
}

// setupDocketTest configures auth and repo fakes, optionally starts an
// httptest server, and returns a *api.RESTClient (nil when token is empty).
func setupDocketTest(t *testing.T, opts docketTestOpts) *api.RESTClient {
	t.Helper()
	ghfake.FakeAuth(t, opts.token)
	if opts.repoOwner != "" {
		ghfake.FakeRepo(t, opts.repoOwner, opts.repoName)
	} else {
		ghfake.FakeNoRepo(t)
	}

	if opts.token == "" {
		return nil
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/user" {
			http.NotFound(w, r)
			return
		}
		w.WriteHeader(opts.apiStatus)
		fmt.Fprint(w, opts.apiBody)
	}))
	t.Cleanup(server.Close)
	return testutil.NewTestClient(t, server)
}

func TestRun(t *testing.T) {
	tests := []struct {
		name     string
		opts     docketTestOpts
		wantUser string
		wantRepo string
		wantAuth string
	}{
		{
			name: "authenticated with repo",
			opts: docketTestOpts{
				token:     "gho_test",
				repoOwner: "octocat",
				repoName:  "my-project",
				apiStatus: http.StatusOK,
				apiBody:   `{"login":"octocat"}`,
			},
			wantUser: "octocat",
			wantRepo: "octocat/my-project",
			wantAuth: "authenticated",
		},
		{
			name: "authenticated without repo",
			opts: docketTestOpts{
				token:     "gho_test",
				apiStatus: http.StatusOK,
				apiBody:   `{"login":"octocat"}`,
			},
			wantUser: "octocat",
			wantRepo: "(none)",
			wantAuth: "authenticated",
		},
		{
			name:     "not authenticated",
			wantUser: "(none)",
			wantRepo: "(none)",
			wantAuth: "not authenticated",
		},
		{
			name: "not authenticated but has repo",
			opts: docketTestOpts{
				repoOwner: "octocat",
				repoName:  "my-project",
			},
			wantUser: "(none)",
			wantRepo: "octocat/my-project",
			wantAuth: "not authenticated",
		},
		{
			name: "authenticated but API failure",
			opts: docketTestOpts{
				token:     "gho_test",
				apiStatus: http.StatusInternalServerError,
				apiBody:   `{"message":"Internal Server Error"}`,
			},
			wantUser: "(none)",
			wantRepo: "(none)",
			wantAuth: "authenticated",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := setupDocketTest(t, tt.opts)

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
