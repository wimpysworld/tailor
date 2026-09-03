package gh

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/cli/go-gh/v2/pkg/api"
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

// assertBoundedHTTPError checks that err is an *api.HTTPError with wantStatus,
// that it keeps at most three of the details, and that the rendered message
// carries no control characters, no unbounded detail, and stays under 1200
// bytes. It returns the HTTP error for further assertions.
func assertBoundedHTTPError(t *testing.T, err error, wantStatus int, details []string) *api.HTTPError {
	t.Helper()
	var httpErr *api.HTTPError
	if !errors.As(err, &httpErr) {
		t.Fatalf("error type = %T, want *api.HTTPError", err)
	}
	if httpErr.StatusCode != wantStatus {
		t.Errorf("status = %d, want %d", httpErr.StatusCode, wantStatus)
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
	return httpErr
}
