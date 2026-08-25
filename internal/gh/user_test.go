package gh

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
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

func TestFetchUsernameHTTPErrorBoundsRenderedLiveResponse(t *testing.T) {
	details := make([]string, 4)
	for i := range details {
		details[i] = fmt.Sprintf("detail-%d-\x00\x1b[31m\t\r%s-PRIVATE-TAIL-%d", i, strings.Repeat("x", 300), i)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		if err := json.NewEncoder(w).Encode(map[string]any{
			"message": "Bad credentials\x1b[2J",
			"errors":  details,
		}); err != nil {
			t.Errorf("encode response: %v", err)
		}
	}))
	t.Cleanup(server.Close)

	_, err := FetchUsername(newTestClient(t, server))
	if err == nil {
		t.Fatal("FetchUsername() error = nil, want HTTP error")
	}
	var httpErr *api.HTTPError
	if !errors.As(err, &httpErr) {
		t.Fatalf("FetchUsername() error type = %T, want *api.HTTPError", err)
	}
	if httpErr.StatusCode != http.StatusUnauthorized {
		t.Errorf("status = %d, want %d", httpErr.StatusCode, http.StatusUnauthorized)
	}
	if len(httpErr.Errors) != 3 {
		t.Errorf("detail count = %d, want 3", len(httpErr.Errors))
	}
	rendered := err.Error()
	if strings.ContainsAny(rendered, "\x00\x1b\r\t") {
		t.Errorf("error contains terminal control characters: %q", rendered)
	}
	for i := range details {
		if strings.Contains(rendered, details[i]) || strings.Contains(rendered, fmt.Sprintf("PRIVATE-TAIL-%d", i)) {
			t.Errorf("error contains unbounded detail %d: %q", i, rendered)
		}
	}
	if strings.Contains(rendered, "detail-3-") {
		t.Errorf("error contains fourth detail: %q", rendered)
	}
	if len(rendered) > 1200 {
		t.Errorf("rendered error length = %d, want at most 1200", len(rendered))
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
