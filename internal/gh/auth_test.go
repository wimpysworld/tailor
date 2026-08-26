package gh

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/cli/go-gh/v2/pkg/api"
)

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

func TestRESTClientOptions(t *testing.T) {
	options := restClientOptions("ghe.example.com")

	if options.Host != "ghe.example.com" {
		t.Errorf("Host = %q, want %q", options.Host, "ghe.example.com")
	}
	if options.Timeout != 30*time.Second {
		t.Errorf("Timeout = %s, want 30s", options.Timeout)
	}
}

func TestVerifyAuth(t *testing.T) {
	tests := []struct {
		name       string
		token      string
		userStatus int
		wantUser   string
		wantErr    string
	}{
		{
			name:       "valid token returns client and username",
			token:      "gho_test",
			userStatus: http.StatusOK,
			wantUser:   "octocat",
		},
		{
			name:    "missing token fails before any request",
			token:   "",
			wantErr: "tailor requires GitHub authentication",
		},
		{
			name:       "rejected token fails verification",
			token:      "gho_invalid",
			userStatus: http.StatusUnauthorized,
			wantErr:    "verifying GitHub authentication",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("GH_CONFIG_DIR", t.TempDir())
			t.Setenv("GH_HOST", "")
			restoreToken := SetTokenForHostFunc(func(string) (string, string) {
				return tt.token, "oauth_token"
			})
			t.Cleanup(restoreToken)

			var requests int
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				requests++
				if tt.userStatus != http.StatusOK {
					w.WriteHeader(tt.userStatus)
					fmt.Fprint(w, `{"message":"Bad credentials"}`)
					return
				}
				fmt.Fprintf(w, `{"login":%q}`, tt.wantUser)
			}))
			t.Cleanup(server.Close)
			client := newTestClient(t, server)
			restoreClient := SetNewRESTClientFunc(func(string) (*api.RESTClient, error) {
				return client, nil
			})
			t.Cleanup(restoreClient)

			gotClient, username, err := VerifyAuth("github.com")

			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("VerifyAuth() error = %v, want substring %q", err, tt.wantErr)
				}
				if tt.token == "" && requests != 0 {
					t.Errorf("requests = %d, want 0 when no token is present", requests)
				}
				return
			}
			if err != nil {
				t.Fatalf("VerifyAuth() error: %v", err)
			}
			if username != tt.wantUser {
				t.Errorf("username = %q, want %q", username, tt.wantUser)
			}
			if gotClient != client {
				t.Error("VerifyAuth() did not return the created client")
			}
			if requests != 1 {
				t.Errorf("requests = %d, want 1", requests)
			}
		})
	}
}
