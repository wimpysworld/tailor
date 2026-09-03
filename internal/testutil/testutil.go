package testutil

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/cli/go-gh/v2/pkg/api"
)

// testTransport redirects all requests to the test server, preserving the
// original request path so the test handler can route by path.
type testTransport struct {
	Server *httptest.Server
}

func (t *testTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	clone := req.Clone(req.Context())
	clone.URL.Scheme = "http"
	clone.URL.Host = t.Server.Listener.Addr().String()
	return http.DefaultTransport.RoundTrip(clone)
}

// NewTestClient creates an api.RESTClient that sends all requests to the
// given test server.
func NewTestClient(t *testing.T, server *httptest.Server) *api.RESTClient {
	t.Helper()
	client, err := api.NewRESTClient(api.ClientOptions{
		Host:      "github.com",
		AuthToken: "test-token",
		Transport: &testTransport{Server: server},
	})
	if err != nil {
		t.Fatalf("NewRESTClient: %v", err)
	}
	return client
}

// FailingServer creates an httptest server that replies 500 with a GitHub
// style error body to every request. The server closes when the test ends.
func FailingServer(t *testing.T) *httptest.Server {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprint(w, `{"message":"Internal Server Error"}`)
	}))
	t.Cleanup(server.Close)
	return server
}

// WriteFile writes content to filepath.Join(dir, name) and fails the test
// on error. The parent directory must already exist.
func WriteFile(t *testing.T, dir, name, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
}

// CreateFile creates a file at filepath.Join(dir, name) with dummy content.
// Parent directories are created as needed.
func CreateFile(t *testing.T, dir, name string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(filepath.Join(dir, name)), 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	WriteFile(t, dir, name, "test")
}

// WriteConfig writes a .tailor.yml file in dir with the given content.
func WriteConfig(t *testing.T, dir, content string) {
	t.Helper()
	WriteFile(t, dir, ".tailor.yml", content)
}

// AssertPtrEqual checks a pointer field against a pointer expectation. When
// want is nil, it expects got to be nil. Otherwise it expects got to be
// non-nil with the value that want points to.
func AssertPtrEqual[T comparable](t *testing.T, got, want *T, field string) {
	t.Helper()
	if want == nil {
		if got != nil {
			t.Errorf("%s = %#v, want nil", field, *got)
		}
		return
	}
	if got == nil {
		t.Errorf("%s is nil, want %#v", field, *want)
		return
	}
	if *got != *want {
		t.Errorf("%s = %#v, want %#v", field, *got, *want)
	}
}
