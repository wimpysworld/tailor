package testutil

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestNewTestClientRoutesToTestServer(t *testing.T) {
	tests := []struct {
		name string
		path string
	}{
		{name: "root resource", path: "user"},
		{name: "nested resource", path: "repos/wimpysworld/tailor/labels"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var gotPath string
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				gotPath = r.URL.Path
				fmt.Fprint(w, "{}")
			}))
			defer server.Close()

			client := NewTestClient(t, server)
			var out struct{}
			if err := client.Get(tt.path, &out); err != nil {
				t.Fatalf("Get: %v", err)
			}
			want := "/" + tt.path
			if gotPath != want {
				t.Errorf("server saw path %q, want %q", gotPath, want)
			}
		})
	}
}

func TestTestTransportRedirectsArbitraryHost(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, "reached")
	}))
	defer server.Close()

	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, "https://example.invalid/some/path", nil)
	if err != nil {
		t.Fatalf("NewRequestWithContext: %v", err)
	}
	httpClient := &http.Client{Transport: &testTransport{Server: server}}
	resp, err := httpClient.Do(req)
	if err != nil {
		t.Fatalf("Do: %v", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("ReadAll: %v", err)
	}
	if string(body) != "reached" {
		t.Errorf("body = %q, want %q", string(body), "reached")
	}
}

func TestTestTransportDoesNotMutateRequest(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, "{}")
	}))
	defer server.Close()

	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, "https://api.github.com/user", nil)
	if err != nil {
		t.Fatalf("NewRequestWithContext: %v", err)
	}
	req.Header.Set("Authorization", "token test-token")

	transport := &testTransport{Server: server}
	resp, err := transport.RoundTrip(req)
	if err != nil {
		t.Fatalf("RoundTrip: %v", err)
	}
	resp.Body.Close()

	if req.URL.Scheme != "https" {
		t.Errorf("URL.Scheme = %q, want %q", req.URL.Scheme, "https")
	}
	if req.URL.Host != "api.github.com" {
		t.Errorf("URL.Host = %q, want %q", req.URL.Host, "api.github.com")
	}
	if req.Host != "api.github.com" {
		t.Errorf("Host = %q, want %q", req.Host, "api.github.com")
	}
	if got := req.Header.Get("Authorization"); got != "token test-token" {
		t.Errorf("Authorization header = %q, want %q", got, "token test-token")
	}
}

func TestCreateFile(t *testing.T) {
	tests := []struct {
		name     string
		fileName string
	}{
		{name: "file in root", fileName: "LICENSE"},
		{name: "file in nested directory", fileName: filepath.Join(".github", "workflows", "ci.yml")},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()

			CreateFile(t, dir, tt.fileName)

			data, err := os.ReadFile(filepath.Join(dir, tt.fileName))
			if err != nil {
				t.Fatalf("ReadFile: %v", err)
			}
			if string(data) != "test" {
				t.Errorf("file content = %q, want %q", string(data), "test")
			}
		})
	}
}

func TestWriteConfigCreatesFile(t *testing.T) {
	dir := t.TempDir()
	content := "license: BlueOak-1.0.0\nswatches: []\n"

	WriteConfig(t, dir, content)

	path := filepath.Join(dir, ".tailor.yml")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if string(data) != content {
		t.Errorf("file content = %q, want %q", string(data), content)
	}
}

func TestAssertPtrEqual(t *testing.T) {
	val := "read"
	AssertPtrEqual(t, &val, &val, "string_field")
	AssertPtrEqual(t, &val, new("read"), "string_field")
	AssertPtrEqual[string](t, nil, nil, "string_field")
	flag := true
	AssertPtrEqual(t, &flag, new(true), "bool_field")
	AssertPtrEqual[bool](t, nil, nil, "bool_field")
}
