package gh

import "testing"

func TestResolveHost(t *testing.T) {
	tests := []struct {
		name   string
		host   string
		ghHost string
		want   string
	}{
		{name: "non-empty host passes through", host: "ghe.example.com", ghHost: "", want: "ghe.example.com"},
		{name: "empty host falls back to github.com", host: "", ghHost: "", want: "github.com"},
		{name: "empty host honours GH_HOST", host: "", ghHost: "ghe.example.com", want: "ghe.example.com"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("GH_CONFIG_DIR", t.TempDir())
			t.Setenv("GH_HOST", tt.ghHost)

			if got := ResolveHost(tt.host); got != tt.want {
				t.Errorf("ResolveHost(%q) = %q, want %q", tt.host, got, tt.want)
			}
		})
	}
}

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
			t.Setenv("GH_CONFIG_DIR", t.TempDir())
			t.Setenv("GH_HOST", "")
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
