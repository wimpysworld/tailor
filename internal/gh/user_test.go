package gh

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
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
	t.Setenv("GITHUB_ACTIONS", "")
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

func TestFetchUsernameGitHubActionsFallback(t *testing.T) {
	// Simulate installation token: GET /user returns 403.
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		fmt.Fprint(w, `{"message": "Resource not accessible by integration"}`)
	}))
	t.Cleanup(server.Close)

	t.Setenv("GITHUB_ACTIONS", "true")
	t.Setenv("GITHUB_REPOSITORY_OWNER", "testowner")

	client := newTestClient(t, server)
	username, err := FetchUsername(client)
	if err != nil {
		t.Fatalf("FetchUsername() error: %v", err)
	}

	if username != "testowner" {
		t.Errorf("username = %q, want %q", username, "testowner")
	}
}

func TestFetchUsernameGitHubActionsPATUsesAPI(t *testing.T) {
	// PAT in GitHub Actions: GET /user succeeds.
	var requestCount atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount.Add(1)
		if r.URL.Path != "/user" {
			http.NotFound(w, r)
			return
		}
		fmt.Fprint(w, `{"login": "patuser"}`)
	}))
	t.Cleanup(server.Close)

	t.Setenv("GITHUB_ACTIONS", "true")
	t.Setenv("GITHUB_REPOSITORY_OWNER", "testowner")

	client := newTestClient(t, server)
	username, err := FetchUsername(client)
	if err != nil {
		t.Fatalf("FetchUsername() error: %v", err)
	}

	if username != "patuser" {
		t.Errorf("username = %q, want %q", username, "patuser")
	}

	if n := requestCount.Load(); n == 0 {
		t.Error("expected at least one HTTP request, got zero")
	}
}

func TestFetchUsernameGitHubActionsNoOwner(t *testing.T) {
	// Installation token with no GITHUB_REPOSITORY_OWNER: error propagated.
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		fmt.Fprint(w, `{"message": "Resource not accessible by integration"}`)
	}))
	t.Cleanup(server.Close)

	t.Setenv("GITHUB_ACTIONS", "true")
	t.Setenv("GITHUB_REPOSITORY_OWNER", "")

	client := newTestClient(t, server)
	_, err := FetchUsername(client)
	if err == nil {
		t.Fatal("FetchUsername() expected error when GITHUB_REPOSITORY_OWNER is empty, got nil")
	}
}

func TestFetchUsernameNotGitHubActions(t *testing.T) {
	var requestCount atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount.Add(1)
		if r.URL.Path != "/user" {
			http.NotFound(w, r)
			return
		}
		fmt.Fprint(w, `{"login": "apiuser"}`)
	}))
	t.Cleanup(server.Close)

	t.Setenv("GITHUB_ACTIONS", "")

	client := newTestClient(t, server)
	username, err := FetchUsername(client)
	if err != nil {
		t.Fatalf("FetchUsername() error: %v", err)
	}

	if username != "apiuser" {
		t.Errorf("username = %q, want %q", username, "apiuser")
	}

	if n := requestCount.Load(); n == 0 {
		t.Error("expected at least one HTTP request, got zero")
	}
}
