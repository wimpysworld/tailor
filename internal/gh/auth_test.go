package gh

import "testing"

func TestCheckAuth(t *testing.T) {
	tests := []struct {
		name    string
		token   string
		wantErr string
	}{
		{
			name:  "valid token returns nil",
			token: "test-valid-token",
		},
		{
			name:    "empty token returns error",
			token:   "",
			wantErr: "tailor requires GitHub authentication. Set the GH_TOKEN or GITHUB_TOKEN environment variable, or run 'gh auth login'",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			restore := SetTokenForHostFunc(func(string) (string, string) {
				return tt.token, "oauth_token"
			})
			t.Cleanup(restore)

			err := CheckAuth()

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
