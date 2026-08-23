package gh

import (
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/cli/go-gh/v2/pkg/api"
)

func TestFetchUsernameSuccess(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/user" {
			http.NotFound(w, r)
			return
		}
		fmt.Fprint(w, `{"login": "testuser"}`)
	}))
	t.Cleanup(server.Close)

	client := newTestClient(t, server)
	username, err := FetchUsername(client)
	if err != nil {
		t.Fatalf("FetchUsername() error: %v", err)
	}

	if username != "testuser" {
		t.Errorf("username = %q, want %q", username, "testuser")
	}
}

func TestFetchUsernameAPIError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		fmt.Fprint(w, `{"message": "Bad credentials"}`)
	}))
	t.Cleanup(server.Close)

	client := newTestClient(t, server)
	_, err := FetchUsername(client)
	if err == nil {
		t.Fatal("FetchUsername() expected error, got nil")
	}
}

func TestFetchUsernameGitHubActionsDoesNotMaskAPIError(t *testing.T) {
	t.Setenv("GITHUB_ACTIONS", "true")
	t.Setenv("GITHUB_REPOSITORY_OWNER", "actions-owner")

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/user" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusForbidden)
		fmt.Fprint(w, `{"message": "Resource not accessible by integration"}`)
	}))
	t.Cleanup(server.Close)

	client := newTestClient(t, server)
	username, err := FetchUsername(client)
	if err == nil {
		t.Fatalf("FetchUsername() = %q, nil; want /user API error", username)
	}

	var httpErr *api.HTTPError
	if !errors.As(err, &httpErr) {
		t.Fatalf("FetchUsername() error = %T, want *api.HTTPError", err)
	}
	if httpErr.StatusCode != http.StatusForbidden {
		t.Errorf("FetchUsername() error status = %d, want %d", httpErr.StatusCode, http.StatusForbidden)
	}
	if httpErr.Message != "Resource not accessible by integration" {
		t.Errorf("FetchUsername() error message = %q, want %q", httpErr.Message, "Resource not accessible by integration")
	}
}
