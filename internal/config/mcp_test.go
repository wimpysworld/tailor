package config

import (
	"strings"
	"testing"
)

func TestMCPStrictParsing(t *testing.T) {
	for _, tt := range []struct {
		name, input, wantErr  string
		wantSection, declared bool
		enabled               bool
	}{
		{name: "absent"},
		{name: "empty", input: "mcp: {}\n", wantSection: true},
		{name: "enabled", input: "mcp: {playwright: true}\n", wantSection: true, declared: true, enabled: true},
		{name: "disabled", input: "mcp: {playwright: false}\n", wantSection: true, declared: true},
		{name: "null section", input: "mcp: null\n", wantErr: "mcp must be a mapping"},
		{name: "string section", input: "mcp: playwright\n", wantErr: "mcp must be a mapping"},
		{name: "number section", input: "mcp: 1\n", wantErr: "mcp must be a mapping"},
		{name: "sequence section", input: "mcp: []\n", wantErr: "mcp must be a mapping"},
		{name: "unknown", input: "mcp: {browser: true}\n", wantErr: `unrecognised mcp setting "browser" in config; valid settings: playwright`},
		{name: "null value", input: "mcp: {playwright: null}\n", wantErr: "mcp.playwright must be a bool"},
		{name: "implicit null value", input: "mcp:\n  playwright:\n", wantErr: "mcp.playwright must be a bool"},
		{name: "string value", input: "mcp: {playwright: 'true'}\n", wantErr: "mcp.playwright must be a bool"},
		{name: "number value", input: "mcp: {playwright: 1}\n", wantErr: "mcp.playwright must be a bool"},
		{name: "legacy boolean value", input: "mcp: {playwright: yes}\n", wantErr: "mcp.playwright must be a bool"},
		{name: "sequence value", input: "mcp: {playwright: []}\n", wantErr: "mcp.playwright must be a bool"},
		{name: "nested value", input: "mcp: {playwright: {enabled: true}}\n", wantErr: "mcp.playwright must be a bool"},
		{name: "duplicate", input: "mcp: {playwright: true, playwright: false}\n", wantErr: `mapping key "playwright" already defined`},
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
			if (cfg.MCP != nil) != tt.wantSection {
				t.Fatalf("MCP present = %t, want %t", cfg.MCP != nil, tt.wantSection)
			}
			if cfg.PlaywrightDeclared() != tt.declared || cfg.PlaywrightEnabled() != tt.enabled {
				t.Fatalf("declared = %t, enabled = %t", cfg.PlaywrightDeclared(), cfg.PlaywrightEnabled())
			}
		})
	}
}

func TestCapabilityAccessorsNilSafe(t *testing.T) {
	var cfg *Config
	if cfg.PlaywrightDeclared() || cfg.PlaywrightEnabled() || cfg.PagesDeclared() || cfg.PagesEnabled() {
		t.Fatal("nil config must report capabilities as absent and disabled")
	}
}

func TestPagesAccessors(t *testing.T) {
	for _, tt := range []struct {
		name, input       string
		declared, enabled bool
	}{
		{name: "absent"},
		{name: "empty", input: "pages: {}\n"},
		{name: "disabled", input: "pages: {enabled: false}\n", declared: true},
		{name: "enabled", input: "pages: {enabled: true}\n", declared: true, enabled: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			cfg, err := parseAndValidate([]byte(tt.input), "config")
			if err != nil {
				t.Fatal(err)
			}
			if cfg.PagesDeclared() != tt.declared || cfg.PagesEnabled() != tt.enabled {
				t.Fatalf("declared = %t, enabled = %t", cfg.PagesDeclared(), cfg.PagesEnabled())
			}
		})
	}
}

func TestPlaywrightIndependentFromPages(t *testing.T) {
	for _, tt := range []struct {
		name, pages string
		declared    bool
	}{
		{name: "pages absent"},
		{name: "pages false", pages: "pages: {enabled: false}\n", declared: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			cfg, err := parseAndValidate([]byte("mcp: {playwright: true}\n"+tt.pages), "config")
			if err != nil {
				t.Fatal(err)
			}
			if !cfg.PlaywrightDeclared() || !cfg.PlaywrightEnabled() {
				t.Fatal("Playwright must remain declared and enabled")
			}
			if cfg.PagesDeclared() != tt.declared || cfg.PagesEnabled() {
				t.Fatalf("Pages declared = %t, enabled = %t", cfg.PagesDeclared(), cfg.PagesEnabled())
			}
		})
	}
}
