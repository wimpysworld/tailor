package swatch_test

import (
	"io/fs"
	"strings"
	"testing"

	"github.com/wimpysworld/tailor"
	"github.com/wimpysworld/tailor/internal/swatch"
)

// TestContentAvailableForAllRegisteredSwatches verifies that the embedded
// filesystem contains a file for every swatch in the registry.
func TestContentAvailableForAllRegisteredSwatches(t *testing.T) {
	all := swatch.All()
	if len(all) == 0 {
		t.Fatal("All() returned no swatches")
	}

	for _, s := range all {
		t.Run(s.Path, func(t *testing.T) {
			data, err := swatch.Content(s.Path)
			if err != nil {
				t.Fatalf("Content(%q) returned error: %v", s.Path, err)
			}
			if len(data) == 0 {
				t.Fatalf("Content(%q) returned empty bytes", s.Path)
			}
		})
	}
}

// TestAllEmbeddedFilesAreRegistered checks that every ordinary embedded source
// maps to a registered destination. Managed templates have private coverage tests.
func TestAllEmbeddedFilesAreRegistered(t *testing.T) {
	registered := make(map[string]bool)
	for _, p := range swatch.Paths() {
		registered[p] = true
	}
	private := map[string]bool{
		"just/python.just": false, "nix/python.nix": false, "setup/python.sh": false,
		"quality/build-python.sh": false, "quality/test-python.sh": false, "quality/lint-python.sh": false, "quality/python-preflight.sh": false, "quality/python-preflight.py": false,
		"nix/go-ebitengine.nix": false, "nix/hooks/go-ebitengine.nix": false, "nix/lib/go-ebitengine-runtime.nix": false, "nix/shells/go-ebitengine-android.nix": false,
		"setup/go-ffmpeg-statigo.sh": false,
		"just/rust.just":             false, "nix/rust.nix": false,
		"quality/build-rust.sh": false, "quality/test-rust.sh": false, "quality/lint-rust.sh": false, "quality/rust-preflight.sh": false,
		"quality/build-go.sh": false, "quality/test-go.sh": false, "quality/lint-go.sh": false, "quality/workflows.sh": false, "quality/project.sh": false,
		"nix/hooks.nix": false, "nix/hooks/core.nix": false, "nix/hooks/go.nix": false, "nix/hooks/go-ffmpeg-statigo.nix": false,
		"just/go-ffmpeg-statigo.just": false, "nix/go-ffmpeg-statigo.nix": false,
		"just/go.just": false, "just/loader.just": false, "just/pages.just": false, "just/tailor.just": false,
		"nix/go.nix": false, "nix/loader.nix": false, "nix/pages.nix": false, "nix/playwright.nix": false, "nix/blender.nix": false,
		".mcp.json": false, ".codex/config.toml": false, "opencode.json": false, ".pi/mcp.json": false,
	}

	err := fs.WalkDir(tailor.SwatchFS, "swatches", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		rel := strings.TrimPrefix(path, "swatches/")
		if _, ok := private[rel]; ok {
			private[rel] = true
			return nil
		}
		switch rel {
		case "pages/static.yml", "pages/hugo.yml", "pages/jekyll.yml":
			rel = swatch.PagesDestination
		case "go/dependabot-disabled.yml":
			rel = ".github/dependabot.yml"
		}
		if !registered[rel] {
			t.Errorf("embedded file %q has no registry entry", rel)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("WalkDir returned error: %v", err)
	}
	for path, found := range private {
		if !found {
			t.Errorf("private embedded file %q is missing", path)
		}
	}
}

func TestContentReturnsErrorForUnknownSource(t *testing.T) {
	_, err := swatch.Content("nonexistent.txt")
	if err == nil {
		t.Fatal("Content(\"nonexistent.txt\") expected error, got nil")
	}
}
