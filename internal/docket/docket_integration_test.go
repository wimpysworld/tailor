package docket

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/cli/go-gh/v2/pkg/api"
)

func TestRunFormatOutputIntegration(t *testing.T) {
	tests := []struct {
		name          string
		token         string
		hasRepo       bool
		repoOwner     string
		repoName      string
		apiStatus     int
		apiBody       string
		wantContains []string
	}{
		{
			name:      "authenticated with repo",
			token:     "gho_test",
			hasRepo:   true,
			repoOwner: "octocat",
			repoName:  "my-project",
			apiStatus: http.StatusOK,
			apiBody:   `{"login":"octocat"}`,
			wantContains: []string{
				"user:",
				"repository:",
				"auth:",
				"octocat",
				"octocat/my-project",
				"authenticated",
			},
		},
		{
			name:  "not authenticated",
			token: "",
			wantContains: []string{
				"user:",
				"repository:",
				"auth:",
				"(none)",
				"not authenticated",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fakeAuth(t, tt.token)
			if tt.hasRepo {
				fakeRepo(t, tt.repoOwner, tt.repoName)
			} else {
				fakeNoRepo(t)
			}

			var client *api.RESTClient
			if tt.token != "" {
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					if r.URL.Path != "/user" {
						http.NotFound(w, r)
						return
					}
					w.WriteHeader(tt.apiStatus)
					fmt.Fprint(w, tt.apiBody)
				}))
				t.Cleanup(server.Close)
				client = newTestClient(t, server)
			}

			result, err := Run(client)
			if err != nil {
				t.Fatalf("Run() error: %v", err)
			}

			output := FormatOutput(result)

			for _, s := range tt.wantContains {
				if !strings.Contains(output, s) {
					t.Errorf("output missing %q\ngot:\n%s", s, output)
				}
			}
		})
	}
}
