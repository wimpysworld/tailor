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
				"authenticated",
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
