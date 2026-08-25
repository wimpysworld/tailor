package gh

import "testing"

func TestCheckAuth(t *testing.T) {
	tests := []struct {
		name     string
		host     string
		token    string
		wantHost string
		wantErr  string
	}{
		{
			name:     "valid token returns nil",
			host:     "github.com",
			token:    "test-valid-token",
			wantHost: "github.com",
		},
		{
			name:     "empty host falls back to github.com",
			host:     "",
			token:    "test-valid-token",
			wantHost: "github.com",
		},
		{
			name:     "enterprise host is checked as given",
			host:     "ghe.example.com",
			token:    "test-valid-token",
			wantHost: "ghe.example.com",
		},
		{
			name:     "empty token returns error",
			host:     "github.com",
			token:    "",
			wantHost: "github.com",
			wantErr:  "tailor requires GitHub authentication. Set the GH_TOKEN or GITHUB_TOKEN environment variable, or run 'gh auth login'",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var gotHost string
			restore := SetTokenForHostFunc(func(host string) (string, string) {
				gotHost = host
				return tt.token, "oauth_token"
			})
			t.Cleanup(restore)

			err := CheckAuth(tt.host)

			if gotHost != tt.wantHost {
				t.Errorf("CheckAuth(%q) checked host %q, want %q", tt.host, gotHost, tt.wantHost)
			}

			if tt.wantErr == "" {
				if err != nil {
					t.Errorf("CheckAuth() = %v, want nil", err)
				}
				return
			}

			if err == nil {
				t.Fatalf("CheckAuth() = nil, want error %q", tt.wantErr)
			}
			if err.Error() != tt.wantErr {
				t.Errorf("CheckAuth() error = %q, want %q", err.Error(), tt.wantErr)
			}
		})
	}
}
