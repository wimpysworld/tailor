package docket

import (
	"net/http"
	"strings"
	"testing"
)

func TestRunFormatOutputIntegration(t *testing.T) {
	tests := []struct {
		name         string
		opts         docketTestOpts
		wantContains []string
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
			wantContains: []string{
				"user:",
				"repository:",
				"auth:",
				"octocat",
				"octocat/my-project",
				"auth:           authenticated",
			},
		},
		{
			name: "token rejected as unauthenticated",
			opts: docketTestOpts{
				token:     "gho_expired",
				repoOwner: "octocat",
				repoName:  "my-project",
				apiStatus: http.StatusUnauthorized,
				apiBody:   `{"message":"Bad credentials"}`,
			},
			wantContains: []string{
				"user:           (none)",
				"repository:     octocat/my-project",
				"auth:           not authenticated",
			},
		},
		{
			name: "not authenticated",
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
			client := setupDocketTest(t, tt.opts)

			result, err := Run(client)
			if err != nil {
				t.Fatalf("Run() error = %v", err)
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
