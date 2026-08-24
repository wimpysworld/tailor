package gh

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestFetchLicenceSuccess(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/licenses/mit" {
			http.NotFound(w, r)
			return
		}
		fmt.Fprint(w, `{"key":"mit","name":"MIT License","body":"MIT License\n\nCopyright (c) [year] [fullname]"}`)
	}))
	t.Cleanup(server.Close)

	client := newTestClient(t, server)
	body, err := FetchLicence(client, "mit")
	if err != nil {
		t.Fatalf("FetchLicence() error: %v", err)
	}

	want := "MIT License\n\nCopyright (c) [year] [fullname]"
	if body != want {
		t.Errorf("body = %q, want %q", body, want)
	}
}

func TestFetchLicenceAPIError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		fmt.Fprint(w, `{"message":"Not Found"}`)
	}))
	t.Cleanup(server.Close)

	client := newTestClient(t, server)
	_, err := FetchLicence(client, "nonexistent")
	if err == nil {
		t.Fatal("FetchLicence() expected error, got nil")
	}
}

func TestFetchLicenceBoundsAPIErrorBody(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		fmt.Fprintf(w, `{"message":"Not Found\u001b[2J\t%s-PRIVATE-TAIL"}`, strings.Repeat("x", 500))
	}))
	t.Cleanup(server.Close)

	client := newTestClient(t, server)
	_, err := FetchLicence(client, "nonexistent")
	if err == nil {
		t.Fatal("FetchLicence() expected error, got nil")
	}
	rendered := err.Error()
	if !strings.Contains(rendered, "Not Found") {
		t.Errorf("error is missing the API message: %q", rendered)
	}
	if strings.ContainsAny(rendered, "\x1b\t") {
		t.Errorf("error contains terminal control characters: %q", rendered)
	}
	if strings.Contains(rendered, "PRIVATE-TAIL") {
		t.Errorf("error contains unbounded body tail: %q", rendered)
	}
	if len(rendered) > 600 {
		t.Errorf("rendered error length = %d, want at most 600", len(rendered))
	}
}
