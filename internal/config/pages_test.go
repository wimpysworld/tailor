package config

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/wimpysworld/tailor/internal/model"
)

func TestPagesParsing(t *testing.T) {
	for _, tt := range []struct {
		name, body string
		wantError  bool
	}{
		{"omitted", "", false},
		{"empty", "pages: {}", false},
		{"disabled", "pages:\n  enabled: false", false},
		{"valid", "pages:\n  enabled: true\n  generator: hugo\n  path: web/site\n  branch: release/docs\n  cname: www.example.com", false},
		{"clear", "pages:\n  cname: \"\"", false},
		{"unknown", "pages:\n  workflow: true", true},
		{"generator", "pages:\n  generator: astro", true},
		{"number", "pages:\n  path: 123", true},
		{"bool string", "pages:\n  enabled: \"true\"", true},
		{"null domain", "pages:\n  cname: null", true},
		{"absolute", "pages:\n  path: /tmp/site", true},
		{"traversal", "pages:\n  path: web/../site", true},
		{"expression", "pages:\n  path: '${{ secrets.TOKEN }}'", true},
		{"branch lock", "pages:\n  branch: release.lock/docs", true},
		{"branch ref", "pages:\n  branch: 'main@{1}'", true},
		{"domain url", "pages:\n  cname: https://example.com", true},
		{"domain label", "pages:\n  cname: bad-.example.com", true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			_, err := parseAndValidate([]byte("license: MIT\n"+tt.body+"\n"), "test")
			if (err != nil) != tt.wantError {
				t.Fatalf("parse error = %v, want error %v", err, tt.wantError)
			}
		})
	}
}

func TestPagesRoundTripWithoutDefaults(t *testing.T) {
	for _, tt := range []struct {
		name, body string
	}{
		{"omitted", ""},
		{"empty", "pages: {}"},
		{"disabled", "pages:\n  enabled: false"},
		{"empty links", "pages:\n  links: {}"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			if err := os.WriteFile(filepath.Join(dir, ConfigSwatchPath), []byte("license: none\n"+tt.body+"\n"), 0o644); err != nil {
				t.Fatal(err)
			}
			cfg, err := Load(dir)
			if err != nil {
				t.Fatal(err)
			}
			if err := Write(dir, cfg, "2026-09-12", "Altered"); err != nil {
				t.Fatal(err)
			}
			loaded, err := Load(dir)
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(loaded.Pages, cfg.Pages) {
				t.Fatalf("round trip changed Pages declarations: got %+v, want %+v", loaded.Pages, cfg.Pages)
			}
		})
	}
}

func TestPagesMergeAndRoundTrip(t *testing.T) {
	for _, domain := range []string{"", "www.example.com"} {
		cfg, err := parseAndValidate([]byte("license: MIT\npages:\n  enabled: true\n  cname: \""+domain+"\"\n"), "test")
		if err != nil {
			t.Fatal(err)
		}
		if _, err := MergeDefaults(cfg); err != nil {
			t.Fatal(err)
		}
		if !*cfg.Pages.Enabled || *cfg.Pages.Generator != "static" || *cfg.Pages.Path != "pages" || cfg.Pages.Branch != nil || *cfg.Pages.CNAME != domain || cfg.Pages.Links != nil {
			t.Fatalf("merge lost Pages declarations or inserted links: %+v", cfg.Pages)
		}
		dir := t.TempDir()
		if err := Write(dir, cfg, "2026-09-07", "Fitted"); err != nil {
			t.Fatal(err)
		}
		loaded, err := Load(dir)
		if err != nil {
			t.Fatal(err)
		}
		if loaded.Pages.CNAME == nil || *loaded.Pages.CNAME != domain {
			t.Fatal("round trip lost domain")
		}
	}
	cfg := &Config{License: "MIT"}
	if _, err := MergeDefaults(cfg); err != nil {
		t.Fatal(err)
	}
	if cfg.Languages != nil || cfg.MCP != nil || cfg.Pages != nil {
		t.Fatalf("merge inserted an absent capability section: languages=%+v mcp=%+v pages=%+v", cfg.Languages, cfg.MCP, cfg.Pages)
	}

	cfg = &Config{License: "MIT", Pages: &model.PagesSettings{}}
	if _, err := MergeDefaults(cfg); err != nil {
		t.Fatal(err)
	}
	if cfg.Pages.Enabled != nil || cfg.Pages.Links != nil || cfg.Pages.Generator == nil || *cfg.Pages.Generator != "static" || cfg.Pages.Path == nil || *cfg.Pages.Path != "pages" {
		t.Fatalf("merge changed capability declarations or lost normal Pages defaults: %+v", cfg.Pages)
	}
}

func TestHomepageProvenance(t *testing.T) {
	for _, tt := range []struct {
		name, homepage string
		inferred       bool
	}{
		{"inferred", "https://github.com/owner/repo", true},
		{"explicit", "https://example.com", false},
		{"explicit empty", "", false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &Config{License: "MIT"}
			if tt.inferred {
				ApplyRepoDefaults(cfg, "repo", tt.homepage)
			} else {
				cfg.Repository = &model.RepositorySettings{Homepage: &tt.homepage}
			}
			if _, err := MergeDefaults(cfg); err != nil {
				t.Fatal(err)
			}
			dir := t.TempDir()
			if err := Write(dir, cfg, "2026-09-07", "Fitted"); err != nil {
				t.Fatal(err)
			}
			loaded, err := Load(dir)
			if err != nil {
				t.Fatal(err)
			}
			if loaded.HomepageDeclared() == tt.inferred {
				t.Fatal("round trip changed homepage provenance")
			}
			if tt.inferred {
				data, err := os.ReadFile(filepath.Join(dir, ConfigSwatchPath))
				if err != nil {
					t.Fatal(err)
				}
				data = []byte(strings.Replace(string(data), "homepage: "+tt.homepage, "homepage: https://example.com", 1))
				changed, err := parseAndValidate(data, "test")
				if err != nil {
					t.Fatal(err)
				}
				if !changed.HomepageDeclared() {
					t.Fatal("edited homepage was not explicit")
				}
			}
		})
	}
}
