package gh

import (
	"fmt"
	"net/http"
	"net/http/httptest"
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
