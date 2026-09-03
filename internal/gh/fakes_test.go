package gh

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

// recordedRequest holds the method, path, and decoded JSON body of the last
// request that a recordingServer answered.
type recordedRequest struct {
	Method string
	Path   string
	Body   map[string]any
}

// recordingServer answers every request with 204 and records the method,
// path, and decoded JSON body of the last request.
func recordingServer(t *testing.T) (*httptest.Server, *recordedRequest) {
	t.Helper()
	got := &recordedRequest{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got.Method = r.Method
		got.Path = r.URL.Path
		data, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(data, &got.Body)
		w.WriteHeader(http.StatusNoContent)
	}))
	t.Cleanup(server.Close)
	return server, got
}

// statusServer answers every request with one status and JSON body.
func statusServer(t *testing.T, status int, body string) *httptest.Server {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		fmt.Fprint(w, body)
	}))
	t.Cleanup(server.Close)
	return server
}
