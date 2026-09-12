package config

import (
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/wimpysworld/tailor/internal/swatch"
)

func TestLanguagesStrictParsing(t *testing.T) {
	for _, tt := range []struct {
		name, input, wantErr string
		declared, enabled    bool
	}{
		{name: "absent"},
		{name: "empty", input: "languages: {}\n"},
		{name: "enabled", input: "languages: {go: true}\n", declared: true, enabled: true},
		{name: "disabled", input: "languages: {go: false}\n", declared: true},
		{name: "null section", input: "languages: null\n", wantErr: "languages must be a mapping"},
		{name: "list section", input: "languages: []\n", wantErr: "languages must be a mapping"},
		{name: "scalar section", input: "languages: true\n", wantErr: "languages must be a mapping"},
		{name: "unknown", input: "languages: {rust: true}\n", wantErr: `unrecognised languages setting "rust" in config; valid settings: go`},
		{name: "null", input: "languages: {go: null}\n", wantErr: "languages.go must be a bool"},
		{name: "implicit null", input: "languages:\n  go:\n", wantErr: "languages.go must be a bool"},
		{name: "quoted", input: "languages: {go: 'true'}\n", wantErr: "languages.go must be a bool"},
		{name: "number", input: "languages: {go: 1}\n", wantErr: "languages.go must be a bool"},
		{name: "legacy boolean", input: "languages: {go: yes}\n", wantErr: "languages.go must be a bool"},
		{name: "list", input: "languages: {go: []}\n", wantErr: "languages.go must be a bool"},
		{name: "map", input: "languages: {go: {}}\n", wantErr: "languages.go must be a bool"},
		{name: "duplicate", input: "languages: {go: true, go: false}\n", wantErr: `mapping key "go" already defined`},
	} {
		t.Run(tt.name, func(t *testing.T) {
			cfg, err := parseAndValidate([]byte("license: MIT\n"+tt.input), "config")
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("error = %v, want %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if cfg.GoDeclared() != tt.declared || cfg.GoEnabled() != tt.enabled {
				t.Fatalf("declared = %t, enabled = %t", cfg.GoDeclared(), cfg.GoEnabled())
			}
			dir := t.TempDir()
			if err := Write(dir, cfg, "2026-09-12", "Refitted"); err != nil {
				t.Fatal(err)
			}
			reloaded, err := Load(dir)
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(cfg.Languages, reloaded.Languages) {
				t.Fatalf("round trip languages = %#v, want %#v", reloaded.Languages, cfg.Languages)
			}
		})
	}
}

func TestGoDefaultsAndMerge(t *testing.T) {
	paths := []string{".golangci.yml", ".goreleaser.yaml", ".github/workflows/build-go.yml", "Dockerfile"}
	defaults, err := DefaultConfig("MIT")
	if err != nil {
		t.Fatal(err)
	}
	if !defaults.GoDeclared() || defaults.GoEnabled() {
		t.Fatal("default config must declare languages.go false")
	}
	for _, entry := range defaults.Swatches {
		if slices.Contains(paths, entry.Path) {
			t.Fatalf("inactive Go default %s", entry.Path)
		}
	}
	for _, tt := range []struct {
		name      string
		languages *LanguageSettings
	}{
		{name: "absent"},
		{name: "empty", languages: &LanguageSettings{}},
		{name: "false", languages: &LanguageSettings{Go: new(false)}},
		{name: "true", languages: &LanguageSettings{Go: new(true)}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &Config{License: "MIT", Languages: tt.languages}
			if _, err := MergeDefaults(cfg); err != nil {
				t.Fatal(err)
			}
			if cfg.Languages != tt.languages {
				t.Fatal("merge changed the language declaration")
			}
			for _, path := range paths {
				found := slices.ContainsFunc(cfg.Swatches, func(entry SwatchEntry) bool { return entry.Path == path })
				if found != cfg.GoEnabled() || cfg.SwatchActive(path) != cfg.GoEnabled() {
					t.Fatalf("activation for %s does not match languages.go", path)
				}
			}
			if !cfg.SwatchActive("justfile") || !cfg.SwatchActive(".github/dependabot.yml") {
				t.Fatal("shared swatches must remain active")
			}
			if changed, err := MergeDefaults(cfg); err != nil || changed {
				t.Fatalf("second merge = %t, %v", changed, err)
			}
		})
	}
}

func TestGoMergePreservesExistingEntries(t *testing.T) {
	for _, enabled := range []bool{false, true} {
		for _, mode := range []swatch.AlterationMode{swatch.Always, swatch.FirstFit, swatch.Never} {
			for _, path := range []string{".golangci.yml", "Dockerfile"} {
				entry := SwatchEntry{Path: path, Alteration: mode}
				cfg := &Config{Languages: &LanguageSettings{Go: &enabled}, Swatches: []SwatchEntry{entry}}
				MergeDefaultSwatches(cfg)
				if cfg.Swatches[0] != entry {
					t.Fatalf("merge changed existing entry %v", entry)
				}
			}
		}
	}
}
