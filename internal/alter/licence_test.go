package alter_test

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"

	"github.com/wimpysworld/tailor/internal/alter"
	"github.com/wimpysworld/tailor/internal/config"
	"github.com/wimpysworld/tailor/internal/testutil"
)

func licenceServer(body string) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, `{"key":"mit","name":"MIT License","body":%q}`, body)
	}))
}

func failingLicenceServer() *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		fmt.Fprint(w, `{"message":"Not Found"}`)
	}))
}

func TestProcessLicenceWrittenWhenAbsent(t *testing.T) {
	dir := t.TempDir()
	body := "MIT License text"
	server := licenceServer(body)
	t.Cleanup(server.Close)
	client := testutil.NewTestClient(t, server)

	cfg := &config.Config{License: "mit"}
	result, err := alter.ProcessLicence(cfg, dir, alter.Apply, client, io.Discard)
	if err != nil {
		t.Fatalf("ProcessLicence() error: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if result.Category != alter.WouldCopy {
		t.Errorf("category = %q, want %q", result.Category, alter.WouldCopy)
	}

	data, err := os.ReadFile(filepath.Join(dir, "LICENSE"))
	if err != nil {
		t.Fatalf("LICENSE not written: %v", err)
	}
	if string(data) != body {
		t.Errorf("LICENSE content = %q, want %q", string(data), body)
	}
}

func TestProcessLicenceDryRunDoesNotWrite(t *testing.T) {
	dir := t.TempDir()
	server := licenceServer("MIT License text")
	t.Cleanup(server.Close)
	client := testutil.NewTestClient(t, server)

	cfg := &config.Config{License: "mit"}
	result, err := alter.ProcessLicence(cfg, dir, alter.DryRun, client, io.Discard)
	if err != nil {
		t.Fatalf("ProcessLicence() error: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if result.Category != alter.WouldCopy {
		t.Errorf("category = %q, want %q", result.Category, alter.WouldCopy)
	}

	if _, err := os.Stat(filepath.Join(dir, "LICENSE")); err == nil {
		t.Error("dry run wrote LICENSE to disk")
	}
}

func TestProcessLicenceSkippedWhenPresent(t *testing.T) {
	dir := t.TempDir()
	existing := []byte("Existing licence content")
	writeOnDisk(t, dir, "LICENSE", existing)

	server := licenceServer("should not be used")
	t.Cleanup(server.Close)
	client := testutil.NewTestClient(t, server)

	cfg := &config.Config{License: "mit"}
	result, err := alter.ProcessLicence(cfg, dir, alter.Apply, client, io.Discard)
	if err != nil {
		t.Fatalf("ProcessLicence() error: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if result.Category != alter.Skipped || result.Reason != alter.SkipFirstFitExists {
		t.Errorf("result = %+v, want skipped because first-fit destination exists", result)
	}

	// Apply leaves the existing file unchanged.
	data, err := os.ReadFile(filepath.Join(dir, "LICENSE"))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(data, existing) {
		t.Error("existing LICENSE was modified")
	}
}

func TestProcessLicenceExemptFromRecut(t *testing.T) {
	dir := t.TempDir()
	existing := []byte("Original licence")
	writeOnDisk(t, dir, "LICENSE", existing)

	server := licenceServer("should not overwrite")
	t.Cleanup(server.Close)
	client := testutil.NewTestClient(t, server)

	cfg := &config.Config{License: "mit"}
	result, err := alter.ProcessLicence(cfg, dir, alter.Recut, client, io.Discard)
	if err != nil {
		t.Fatalf("ProcessLicence() error: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if result.Category != alter.Skipped || result.Reason != alter.SkipFirstFitExists {
		t.Errorf("result = %+v, want skipped because first-fit destination exists", result)
	}

	data, err := os.ReadFile(filepath.Join(dir, "LICENSE"))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(data, existing) {
		t.Error("Recut overwrote exempt LICENSE file")
	}
}

func TestProcessLicenceWarningWhenNoneAndNoFile(t *testing.T) {
	dir := t.TempDir()
	server := licenceServer("unused")
	t.Cleanup(server.Close)
	client := testutil.NewTestClient(t, server)

	cfg := &config.Config{License: "none"}

	var stderr strings.Builder
	result, err := alter.ProcessLicence(cfg, dir, alter.DryRun, client, &stderr)
	if err != nil {
		t.Fatalf("ProcessLicence() error: %v", err)
	}
	if result != nil {
		t.Errorf("expected nil result, got %+v", result)
	}
	if stderr.Len() == 0 {
		t.Error("expected warning on stderr, got nothing")
	}
	want := "No licence file found and no licence configured."
	if !strings.Contains(stderr.String(), want) {
		t.Errorf("stderr = %q, want substring %q", stderr.String(), want)
	}
}

func TestProcessLicenceWarningWhenEmptyAndNoFile(t *testing.T) {
	dir := t.TempDir()
	server := licenceServer("unused")
	t.Cleanup(server.Close)
	client := testutil.NewTestClient(t, server)

	cfg := &config.Config{License: ""}

	var stderr strings.Builder
	result, err := alter.ProcessLicence(cfg, dir, alter.DryRun, client, &stderr)
	if err != nil {
		t.Fatalf("ProcessLicence() error: %v", err)
	}
	if result != nil {
		t.Errorf("expected nil result, got %+v", result)
	}
	if stderr.Len() == 0 {
		t.Error("expected warning on stderr for empty licence, got nothing")
	}
}

func TestProcessLicenceNoWarningWhenConfigured(t *testing.T) {
	dir := t.TempDir()
	server := licenceServer("MIT text")
	t.Cleanup(server.Close)
	client := testutil.NewTestClient(t, server)

	cfg := &config.Config{License: "mit"}

	var stderr strings.Builder
	_, err := alter.ProcessLicence(cfg, dir, alter.DryRun, client, &stderr)
	if err != nil {
		t.Fatalf("ProcessLicence() error: %v", err)
	}
	if stderr.Len() != 0 {
		t.Errorf("expected no stderr output, got %q", stderr.String())
	}
}

func TestProcessLicenceNoWarningWhenFileExistsAndNone(t *testing.T) {
	dir := t.TempDir()
	writeOnDisk(t, dir, "LICENSE", []byte("existing"))

	server := licenceServer("unused")
	t.Cleanup(server.Close)
	client := testutil.NewTestClient(t, server)

	cfg := &config.Config{License: "none"}

	var stderr strings.Builder
	result, err := alter.ProcessLicence(cfg, dir, alter.DryRun, client, &stderr)
	if err != nil {
		t.Fatalf("ProcessLicence() error: %v", err)
	}
	if result != nil {
		t.Errorf("expected nil result, got %+v", result)
	}
	if stderr.Len() != 0 {
		t.Errorf("expected no stderr when LICENSE exists, got %q", stderr.String())
	}
}

func TestProcessLicenceAPIErrorPropagated(t *testing.T) {
	dir := t.TempDir()
	server := failingLicenceServer()
	t.Cleanup(server.Close)
	client := testutil.NewTestClient(t, server)

	cfg := &config.Config{License: "mit"}
	_, err := alter.ProcessLicence(cfg, dir, alter.Apply, client, io.Discard)
	if err == nil {
		t.Fatal("expected error from API failure, got nil")
	}
}

func TestProcessLicenceNilResultWhenNone(t *testing.T) {
	dir := t.TempDir()
	// Put LICENSE on disk so no warning is emitted.
	writeOnDisk(t, dir, "LICENSE", []byte("existing"))

	server := licenceServer("unused")
	t.Cleanup(server.Close)
	client := testutil.NewTestClient(t, server)

	for _, licence := range []string{"", "none"} {
		t.Run(fmt.Sprintf("license=%q", licence), func(t *testing.T) {
			cfg := &config.Config{License: licence}
			result, err := alter.ProcessLicence(cfg, dir, alter.DryRun, client, io.Discard)
			if err != nil {
				t.Fatalf("ProcessLicence() error: %v", err)
			}
			if result != nil {
				t.Errorf("expected nil result for licence %q, got %+v", licence, result)
			}
		})
	}
}

func TestProcessLicenceDestinationPolicy(t *testing.T) {
	body := "new licence"
	tests := []struct {
		name     string
		setup    func(t *testing.T, dir string)
		wantErr  string
		wantSkip bool
		check    func(t *testing.T, dir string)
	}{
		{
			name: "regular file skipped",
			setup: func(t *testing.T, dir string) {
				writeOnDisk(t, dir, "LICENSE", []byte("existing"))
			},
			wantSkip: true,
			check: func(t *testing.T, dir string) {
				data, err := os.ReadFile(filepath.Join(dir, "LICENSE"))
				if err != nil {
					t.Fatal(err)
				}
				if string(data) != "existing" {
					t.Errorf("existing LICENSE changed to %q", data)
				}
			},
		},
		{
			name: "directory rejected",
			setup: func(t *testing.T, dir string) {
				if err := os.Mkdir(filepath.Join(dir, "LICENSE"), 0o755); err != nil {
					t.Fatal(err)
				}
			},
			wantErr: `licence destination "LICENSE" is a directory`,
		},
		{
			name: "dangling symlink replaced",
			setup: func(t *testing.T, dir string) {
				symlinkOrSkip(t, "missing", filepath.Join(dir, "LICENSE"))
			},
			check: func(t *testing.T, dir string) {
				if _, err := os.Lstat(filepath.Join(dir, "missing")); !os.IsNotExist(err) {
					t.Errorf("dangling target exists or returned an unexpected error: %v", err)
				}
			},
		},
		{
			name: "in-root symlink replaced without following",
			setup: func(t *testing.T, dir string) {
				writeOnDisk(t, dir, "target", []byte("unchanged"))
				symlinkOrSkip(t, "target", filepath.Join(dir, "LICENSE"))
			},
			check: func(t *testing.T, dir string) {
				data, err := os.ReadFile(filepath.Join(dir, "target"))
				if err != nil {
					t.Fatal(err)
				}
				if string(data) != "unchanged" {
					t.Errorf("symlink target changed to %q", data)
				}
			},
		},
		{
			name: "escaping symlink replaced without following",
			setup: func(t *testing.T, dir string) {
				outside := filepath.Join(t.TempDir(), "LICENSE")
				if err := os.WriteFile(outside, []byte("outside"), 0o644); err != nil {
					t.Fatal(err)
				}
				symlinkOrSkip(t, outside, filepath.Join(dir, "LICENSE"))
				t.Cleanup(func() {
					data, err := os.ReadFile(outside)
					if err != nil {
						t.Fatal(err)
					}
					if string(data) != "outside" {
						t.Errorf("outside LICENSE changed to %q", data)
					}
				})
			},
		},
		{
			name: "special file rejected",
			setup: func(t *testing.T, dir string) {
				if err := syscall.Mkfifo(filepath.Join(dir, "LICENSE"), 0o644); err != nil {
					t.Skipf("mkfifo unavailable: %v", err)
				}
			},
			wantErr: `licence destination "LICENSE" is not a regular file`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			tt.setup(t, dir)

			server := licenceServer(body)
			t.Cleanup(server.Close)
			client := testutil.NewTestClient(t, server)

			cfg := &config.Config{License: "mit"}
			result, err := alter.ProcessLicence(cfg, dir, alter.Apply, client, io.Discard)
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("error = %v, want substring %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("ProcessLicence() error: %v", err)
			}
			if result == nil {
				t.Fatal("expected non-nil result")
			}

			if tt.wantSkip {
				if result.Category != alter.Skipped || result.Reason != alter.SkipFirstFitExists {
					t.Errorf("result = %+v, want skipped because first-fit destination exists", result)
				}
			} else {
				if result.Category != alter.WouldCopy {
					t.Errorf("category = %q, want %q", result.Category, alter.WouldCopy)
				}
				info, err := os.Lstat(filepath.Join(dir, "LICENSE"))
				if err != nil {
					t.Fatal(err)
				}
				if !info.Mode().IsRegular() {
					t.Errorf("LICENSE mode = %v, want regular file", info.Mode())
				}
				data, err := os.ReadFile(filepath.Join(dir, "LICENSE"))
				if err != nil {
					t.Fatal(err)
				}
				if string(data) != body {
					t.Errorf("LICENSE content = %q, want %q", data, body)
				}
			}

			if tt.check != nil {
				tt.check(t, dir)
			}
		})
	}
}
