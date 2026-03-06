package alter_test

import (
	"bytes"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
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
	result, err := alter.ProcessLicence(cfg, dir, alter.Apply, client)
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
	result, err := alter.ProcessLicence(cfg, dir, alter.DryRun, client)
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
	result, err := alter.ProcessLicence(cfg, dir, alter.Apply, client)
	if err != nil {
		t.Fatalf("ProcessLicence() error: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if result.Category != alter.SkippedFirstFit {
		t.Errorf("category = %q, want %q", result.Category, alter.SkippedFirstFit)
	}

	// Verify file was not modified.
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
	result, err := alter.ProcessLicence(cfg, dir, alter.Recut, client)
	if err != nil {
		t.Fatalf("ProcessLicence() error: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if result.Category != alter.SkippedFirstFit {
		t.Errorf("category = %q, want %q", result.Category, alter.SkippedFirstFit)
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

	var result *alter.SwatchResult
	var err error
	cfg := &config.Config{License: "none"}

	output := captureStderr(t, func() {
		result, err = alter.ProcessLicence(cfg, dir, alter.DryRun, client)
	})

	if err != nil {
		t.Fatalf("ProcessLicence() error: %v", err)
	}
	if result != nil {
		t.Errorf("expected nil result, got %+v", result)
	}
	if output == "" {
		t.Error("expected warning on stderr, got nothing")
	}
	want := "No licence file found and no licence configured."
	if !bytes.Contains([]byte(output), []byte(want)) {
		t.Errorf("stderr = %q, want substring %q", output, want)
	}
}

func TestProcessLicenceWarningWhenEmptyAndNoFile(t *testing.T) {
	dir := t.TempDir()
	server := licenceServer("unused")
	t.Cleanup(server.Close)
	client := testutil.NewTestClient(t, server)

	var result *alter.SwatchResult
	var err error
	cfg := &config.Config{License: ""}

	output := captureStderr(t, func() {
		result, err = alter.ProcessLicence(cfg, dir, alter.DryRun, client)
	})

	if err != nil {
		t.Fatalf("ProcessLicence() error: %v", err)
	}
	if result != nil {
		t.Errorf("expected nil result, got %+v", result)
	}
	if output == "" {
		t.Error("expected warning on stderr for empty licence, got nothing")
	}
}

func TestProcessLicenceNoWarningWhenConfigured(t *testing.T) {
	dir := t.TempDir()
	server := licenceServer("MIT text")
	t.Cleanup(server.Close)
	client := testutil.NewTestClient(t, server)

	var err error
	cfg := &config.Config{License: "mit"}

	output := captureStderr(t, func() {
		_, err = alter.ProcessLicence(cfg, dir, alter.DryRun, client)
	})

	if err != nil {
		t.Fatalf("ProcessLicence() error: %v", err)
	}
	if output != "" {
		t.Errorf("expected no stderr output, got %q", output)
	}
}

func TestProcessLicenceNoWarningWhenFileExistsAndNone(t *testing.T) {
	dir := t.TempDir()
	writeOnDisk(t, dir, "LICENSE", []byte("existing"))

	server := licenceServer("unused")
	t.Cleanup(server.Close)
	client := testutil.NewTestClient(t, server)

	var result *alter.SwatchResult
	var err error
	cfg := &config.Config{License: "none"}

	output := captureStderr(t, func() {
		result, err = alter.ProcessLicence(cfg, dir, alter.DryRun, client)
	})

	if err != nil {
		t.Fatalf("ProcessLicence() error: %v", err)
	}
	if result != nil {
		t.Errorf("expected nil result, got %+v", result)
	}
	if output != "" {
		t.Errorf("expected no stderr when LICENSE exists, got %q", output)
	}
}

func TestProcessLicenceAPIErrorPropagated(t *testing.T) {
	dir := t.TempDir()
	server := failingLicenceServer()
	t.Cleanup(server.Close)
	client := testutil.NewTestClient(t, server)

	cfg := &config.Config{License: "mit"}
	_, err := alter.ProcessLicence(cfg, dir, alter.Apply, client)
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
			result, err := alter.ProcessLicence(cfg, dir, alter.DryRun, client)
			if err != nil {
				t.Fatalf("ProcessLicence() error: %v", err)
			}
			if result != nil {
				t.Errorf("expected nil result for licence %q, got %+v", licence, result)
			}
		})
	}
}
