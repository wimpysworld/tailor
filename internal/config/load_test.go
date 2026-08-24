package config

import (
	"bytes"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/wimpysworld/tailor/internal/testutil"
)

func TestLoadValidConfig(t *testing.T) {
	dir := t.TempDir()
	testutil.WriteConfig(t, dir, specYAML)

	cfg, err := Load(dir)
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}

	if cfg.License != "BlueOak-1.0.0" {
		t.Errorf("License = %q, want %q", cfg.License, "BlueOak-1.0.0")
	}
	if cfg.Repository == nil {
		t.Fatal("Repository is nil")
	}
	if len(cfg.Swatches) != 16 {
		t.Errorf("Swatches count = %d, want 16", len(cfg.Swatches))
	}
}

func TestLoadMissingFile(t *testing.T) {
	dir := t.TempDir()
	_, err := Load(dir)
	if err == nil {
		t.Fatal("Load() expected error for missing file, got nil")
	}
	if !strings.Contains(err.Error(), "reading config") {
		t.Errorf("error = %q, want it to mention reading config", err.Error())
	}
}

func TestLoadMalformedYAML(t *testing.T) {
	dir := t.TempDir()
	testutil.WriteConfig(t, dir, "{{invalid yaml content")

	_, err := Load(dir)
	if err == nil {
		t.Fatal("Load() expected error for malformed YAML, got nil")
	}
	if !strings.Contains(err.Error(), "parsing config") {
		t.Errorf("error = %q, want it to mention parsing config", err.Error())
	}
}

func TestLoadRejectsExternalConfigSymlinkWithoutBlocking(t *testing.T) {
	dir := t.TempDir()
	outside := filepath.Join(t.TempDir(), "outside.yml")
	if err := os.WriteFile(outside, []byte("license: none\nswatches: []\n"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	if err := os.Symlink(outside, filepath.Join(dir, configPath)); err != nil {
		t.Skipf("Symlink unavailable: %v", err)
	}

	result := make(chan error, 1)
	go func() {
		_, err := Load(dir)
		result <- err
	}()

	select {
	case err := <-result:
		if err == nil {
			t.Fatal("Load() error = nil, want external symlink error")
		}
		if !strings.HasPrefix(err.Error(), "reading config: ") {
			t.Errorf("error = %q, want reading config prefix", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Load() blocked while opening an external config symlink")
	}
}

func TestLoadRejectsConfigDirectory(t *testing.T) {
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, configPath), 0o755); err != nil {
		t.Fatalf("Mkdir: %v", err)
	}

	_, err := Load(dir)
	want := "reading config: .tailor.yml is not a regular file"
	if err == nil || err.Error() != want {
		t.Fatalf("Load() error = %v, want %q", err, want)
	}
}

func TestLoadRejectsConfigSpecialFile(t *testing.T) {
	dir := t.TempDir()
	listener, err := (&net.ListenConfig{}).Listen(t.Context(), "unix", filepath.Join(dir, configPath))
	if err != nil {
		t.Skipf("Unix sockets unavailable: %v", err)
	}
	t.Cleanup(func() { _ = listener.Close() })

	_, err = Load(dir)
	if err == nil {
		t.Fatal("Load() error = nil, want special-file error")
	}
	if !strings.HasPrefix(err.Error(), "reading config: ") {
		t.Errorf("error = %q, want reading config prefix", err)
	}
}

func TestLoadRejectsConfigFIFONonBlocking(t *testing.T) {
	if _, err := exec.LookPath("mkfifo"); err != nil {
		t.Skipf("mkfifo unavailable: %v", err)
	}

	dir := t.TempDir()
	path := filepath.Join(dir, configPath)
	if err := exec.CommandContext(t.Context(), "mkfifo", path).Run(); err != nil {
		t.Skipf("FIFO unavailable: %v", err)
	}
	info, err := os.Lstat(path)
	if err != nil {
		t.Fatalf("Lstat: %v", err)
	}
	if info.Mode()&os.ModeNamedPipe == 0 {
		t.Skipf("mkfifo created mode %v, want a named pipe", info.Mode())
	}

	result := make(chan error, 1)
	go func() {
		_, err := Load(dir)
		result <- err
	}()

	select {
	case err := <-result:
		want := "reading config: .tailor.yml is not a regular file"
		if err == nil || err.Error() != want {
			t.Fatalf("Load() error = %v, want %q", err, want)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Load() blocked while opening a config FIFO")
	}
}

func TestLoadAcceptsRegularConfigFile(t *testing.T) {
	dir := t.TempDir()
	testutil.WriteConfig(t, dir, "license: none\nswatches: []\n")

	cfg, err := Load(dir)
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}
	if cfg.License != "none" {
		t.Errorf("License = %q, want %q", cfg.License, "none")
	}
}

func TestLoadConfigSizeLimit(t *testing.T) {
	tests := []struct {
		name          string
		size          int
		wantSizeError bool
	}{
		{name: "exactly 1 MiB", size: maxConfigSize},
		{name: "1 MiB plus one byte", size: maxConfigSize + 1, wantSizeError: true},
	}
	wantSizeError := "reading config: .tailor.yml exceeds maximum size of 1 MiB"

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			data := bytes.Repeat([]byte{'['}, tt.size)
			if !tt.wantSizeError {
				prefix := []byte("license: none\nswatches: []\n#")
				prefix = append(prefix, bytes.Repeat([]byte{'x'}, tt.size-len(prefix))...)
				data = prefix
			}
			if err := os.WriteFile(filepath.Join(dir, configPath), data, 0o644); err != nil {
				t.Fatalf("WriteFile: %v", err)
			}

			_, err := Load(dir)
			if tt.wantSizeError {
				if err == nil || err.Error() != wantSizeError {
					t.Fatalf("Load() error = %v, want %q", err, wantSizeError)
				}
				return
			}
			if err != nil {
				t.Fatalf("Load() error at exact 1 MiB boundary: %v", err)
			}
		})
	}
}

func TestLoadRejectsTriggeredForActivePath(t *testing.T) {
	dir := t.TempDir()
	testutil.WriteConfig(t, dir, `license: BlueOak-1.0.0
swatches:
  - path: justfile
    alteration: triggered
`)

	_, err := Load(dir)
	if err == nil {
		t.Fatal("Load() expected error for retired alteration mode, got nil")
	}
	if !strings.Contains(err.Error(), `"triggered"`) {
		t.Errorf("error = %q, want it to mention the retired value", err.Error())
	}
}

func TestLoadAcceptsLegacyTriggeredOnlyForRetiredWorkflowPaths(t *testing.T) {
	for _, path := range RetiredWorkflowPaths() {
		t.Run(path, func(t *testing.T) {
			dir := t.TempDir()
			testutil.WriteConfig(t, dir, fmt.Sprintf(`license: none
swatches:
  - path: %s
    alteration: triggered
`, path))

			cfg, err := Load(dir)
			if err != nil {
				t.Fatalf("Load() error: %v", err)
			}
			if len(cfg.Swatches) != 1 || cfg.Swatches[0].Path != path || cfg.Swatches[0].Alteration != "triggered" {
				t.Errorf("Swatches = %v, want one triggered entry for %q", cfg.Swatches, path)
			}
		})
	}
}

func TestLoadNeverAlterationMode(t *testing.T) {
	dir := t.TempDir()
	testutil.WriteConfig(t, dir, `license: BlueOak-1.0.0
swatches:
  - path: justfile
    alteration: never
`)

	cfg, err := Load(dir)
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}
	if cfg.Swatches[0].Alteration != "never" {
		t.Errorf("Alteration = %q, want %q", cfg.Swatches[0].Alteration, "never")
	}
}

func TestLoadInvalidAlterationMode(t *testing.T) {
	dir := t.TempDir()
	testutil.WriteConfig(t, dir, `license: BlueOak-1.0.0
swatches:
  - path: justfile
    alteration: sometimes
`)

	_, err := Load(dir)
	if err == nil {
		t.Fatal("Load() expected error for invalid alteration, got nil")
	}
	if !strings.Contains(err.Error(), `"sometimes"`) {
		t.Errorf("error = %q, want it to mention the invalid value", err.Error())
	}
}

func TestLoadEmptyPath(t *testing.T) {
	dir := t.TempDir()
	testutil.WriteConfig(t, dir, `license: BlueOak-1.0.0
swatches:
  - path: ""
    alteration: always
`)

	_, err := Load(dir)
	if err == nil {
		t.Fatal("Load() expected error for empty path, got nil")
	}
	if !strings.Contains(err.Error(), "path must not be empty") {
		t.Errorf("error = %q, want path must not be empty", err.Error())
	}
}

func TestLoadLabelsAbsent(t *testing.T) {
	dir := t.TempDir()
	testutil.WriteConfig(t, dir, `license: BlueOak-1.0.0
swatches: []
`)

	cfg, err := Load(dir)
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}
	if cfg.Labels != nil {
		t.Errorf("Labels = %v, want nil when absent", cfg.Labels)
	}
}

func TestLoadLabelsEmpty(t *testing.T) {
	dir := t.TempDir()
	testutil.WriteConfig(t, dir, `license: BlueOak-1.0.0
labels: []
swatches: []
`)

	cfg, err := Load(dir)
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}
	if cfg.Labels == nil {
		t.Fatal("Labels is nil, want non-nil empty slice")
	}
	if len(cfg.Labels) != 0 {
		t.Errorf("Labels length = %d, want 0", len(cfg.Labels))
	}
}

func TestLoadLabelsPopulated(t *testing.T) {
	dir := t.TempDir()
	testutil.WriteConfig(t, dir, `license: BlueOak-1.0.0
labels:
  - name: bug
    color: d73a4a
    description: Something is not working
swatches: []
`)

	cfg, err := Load(dir)
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}
	if len(cfg.Labels) != 1 {
		t.Fatalf("Labels count = %d, want 1", len(cfg.Labels))
	}
	if cfg.Labels[0].Name != "bug" {
		t.Errorf("Labels[0].Name = %q, want %q", cfg.Labels[0].Name, "bug")
	}
}

func TestLoadRejectsInvalidLabel(t *testing.T) {
	dir := t.TempDir()
	testutil.WriteConfig(t, dir, `license: BlueOak-1.0.0
labels:
  - name: bug
    color: zzzzzz
    description: Something is not working
swatches: []
`)

	_, err := Load(dir)
	if err == nil {
		t.Fatal("Load() expected error for invalid label color, got nil")
	}
	if !strings.Contains(err.Error(), "not a valid 6-character hex") {
		t.Errorf("error = %q, want hex validation error", err)
	}
}

func TestExistsTrue(t *testing.T) {
	dir := t.TempDir()
	testutil.WriteConfig(t, dir, "license: BlueOak-1.0.0\nswatches: []\n")

	if !Exists(dir) {
		t.Error("Exists() = false, want true")
	}
}

func TestExistsFalse(t *testing.T) {
	dir := t.TempDir()

	if Exists(dir) {
		t.Error("Exists() = true, want false")
	}
}

func TestExistsFalseForDirectory(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, ".tailor.yml"), 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}

	if Exists(dir) {
		t.Error("Exists() = true for a directory, want false")
	}
}

func TestLoadAbsentLicense(t *testing.T) {
	dir := t.TempDir()
	testutil.WriteConfig(t, dir, `swatches:
  - path: justfile
    alteration: always
`)

	cfg, err := Load(dir)
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}
	if cfg.License != "" {
		t.Errorf("License = %q, want empty string for absent key", cfg.License)
	}
}

func TestLoadEmptyLicense(t *testing.T) {
	dir := t.TempDir()
	testutil.WriteConfig(t, dir, `license: ""
swatches:
  - path: justfile
    alteration: always
`)

	cfg, err := Load(dir)
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}
	if cfg.License != "" {
		t.Errorf("License = %q, want empty string", cfg.License)
	}
}

func TestLoadEmptySwatchesList(t *testing.T) {
	dir := t.TempDir()
	testutil.WriteConfig(t, dir, `license: BlueOak-1.0.0
swatches: []
`)

	cfg, err := Load(dir)
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}

	if len(cfg.Swatches) != 0 {
		t.Errorf("Swatches count = %d, want 0", len(cfg.Swatches))
	}
}

func TestLoadWithoutRepositorySection(t *testing.T) {
	dir := t.TempDir()
	testutil.WriteConfig(t, dir, `license: Apache-2.0
swatches:
  - path: justfile
    alteration: first-fit
`)

	cfg, err := Load(dir)
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}

	if cfg.Repository != nil {
		t.Errorf("Repository = %+v, want nil when section is absent", cfg.Repository)
	}
	if cfg.License != "Apache-2.0" {
		t.Errorf("License = %q, want %q", cfg.License, "Apache-2.0")
	}
}
