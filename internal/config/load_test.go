package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/wimpysworld/tailor/internal/testutil"
)

func TestLoadValidConfig(t *testing.T) {
	dir := t.TempDir()
	testutil.WriteConfig(t, dir, specYAML)

	cfg, err := Load(dir)
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}

	if cfg.License != "MIT" {
		t.Errorf("License = %q, want %q", cfg.License, "MIT")
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

func TestLoadInvalidAlterationMode(t *testing.T) {
	dir := t.TempDir()
	testutil.WriteConfig(t, dir, `license: MIT
swatches:
  - source: justfile
    destination: justfile
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

func TestLoadEmptySource(t *testing.T) {
	dir := t.TempDir()
	testutil.WriteConfig(t, dir, `license: MIT
swatches:
  - source: ""
    destination: justfile
    alteration: always
`)

	_, err := Load(dir)
	if err == nil {
		t.Fatal("Load() expected error for empty source, got nil")
	}
	if !strings.Contains(err.Error(), "source must not be empty") {
		t.Errorf("error = %q, want source must not be empty", err.Error())
	}
}

func TestLoadEmptyDestination(t *testing.T) {
	dir := t.TempDir()
	testutil.WriteConfig(t, dir, `license: MIT
swatches:
  - source: justfile
    destination: ""
    alteration: always
`)

	_, err := Load(dir)
	if err == nil {
		t.Fatal("Load() expected error for empty destination, got nil")
	}
	if !strings.Contains(err.Error(), "destination must not be empty") {
		t.Errorf("error = %q, want destination must not be empty", err.Error())
	}
}

func TestExistsTrue(t *testing.T) {
	dir := t.TempDir()
	testutil.WriteConfig(t, dir, "license: MIT\nswatches: []\n")

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
	// Create .tailor/config.yml as a directory, not a file.
	if err := os.MkdirAll(filepath.Join(dir, ".tailor", "config.yml"), 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}

	if Exists(dir) {
		t.Error("Exists() = true for a directory, want false")
	}
}

func TestLoadEmptySwatchesList(t *testing.T) {
	dir := t.TempDir()
	testutil.WriteConfig(t, dir, `license: MIT
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
  - source: justfile
    destination: justfile
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
