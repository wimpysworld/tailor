package docket

import (
	"net/http"
	"strings"
	"testing"
)

func TestRunFormatOutputIntegration(t *testing.T) {
	tests := []struct {
		name         string
		token        string
		hasRepo      bool
		repoOwner    string
		repoName     string
		apiStatus    int
		apiBody      string
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
			client := setupDocketTest(t, tt.token, tt.hasRepo, tt.repoOwner, tt.repoName, tt.apiStatus, tt.apiBody)

			result := Run(client)
			output := FormatOutput(result)

			for _, s := range tt.wantContains {
				if !strings.Contains(output, s) {
					t.Errorf("output missing %q\ngot:\n%s", s, output)
				}
			}
		})
	}
}
