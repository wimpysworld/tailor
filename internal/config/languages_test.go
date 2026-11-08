package config

import (
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/wimpysworld/tailor/internal/swatch"
	"gopkg.in/yaml.v3"
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
		{name: "unknown", input: "languages: {ruby: true}\n", wantErr: `unrecognised languages setting "ruby" in config; valid settings: go, rust, python, shell, javascript, typescript`},
		{name: "null", input: "languages: {go: null}\n", wantErr: "languages.go must be a bool"},
		{name: "implicit null", input: "languages:\n  go:\n", wantErr: "languages.go must be a bool"},
		{name: "quoted", input: "languages: {go: 'true'}\n", wantErr: "languages.go must be a bool"},
		{name: "number", input: "languages: {go: 1}\n", wantErr: "languages.go must be a bool"},
		{name: "legacy boolean", input: "languages: {go: yes}\n", wantErr: "languages.go must be a bool"},
		{name: "list", input: "languages: {go: []}\n", wantErr: "languages.go must be a bool"},
		{name: "map", input: "languages: {go: {}}\n", wantErr: "languages.go.enabled is required"},
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

func TestShellAndJavaScriptConfiguration(t *testing.T) {
	for _, tt := range []struct {
		name, input                           string
		shellDeclared, shellEnabled           bool
		javascriptDeclared, javascriptEnabled bool
	}{
		{name: "absent"},
		{name: "empty", input: "languages: {}"},
		{name: "existing Go", input: "languages: {go: true}"},
		{name: "shell false", input: "languages: {shell: false}", shellDeclared: true},
		{name: "shell true", input: "languages: {shell: true}", shellDeclared: true, shellEnabled: true},
		{name: "javascript false", input: "languages: {javascript: false}", javascriptDeclared: true},
		{name: "javascript true", input: "languages: {javascript: true}", javascriptDeclared: true, javascriptEnabled: true},
		{name: "typescript false", input: "languages: {typescript: false}", javascriptDeclared: true},
		{name: "typescript true", input: "languages: {typescript: true}", javascriptDeclared: true, javascriptEnabled: true},
		{name: "independent shell", input: "languages: {shell: true, javascript: false}", shellDeclared: true, shellEnabled: true, javascriptDeclared: true},
		{name: "independent alias", input: "languages: {shell: false, typescript: true}", shellDeclared: true, javascriptDeclared: true, javascriptEnabled: true},
		{name: "Go mapping", input: "languages: {go: {enabled: true, cgo: false}, shell: true, javascript: true}", shellDeclared: true, shellEnabled: true, javascriptDeclared: true, javascriptEnabled: true},
		{name: "Go mapping alias", input: "languages: {go: {enabled: false}, python: false, rust: true, shell: false, typescript: false}", shellDeclared: true, javascriptDeclared: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			cfg, err := parseAndValidate([]byte("license: MIT\n"+tt.input+"\n"), "config")
			if err != nil {
				t.Fatal(err)
			}
			if cfg.ShellDeclared() != tt.shellDeclared || cfg.ShellEnabled() != tt.shellEnabled {
				t.Fatalf("Shell state = %t, %t", cfg.ShellDeclared(), cfg.ShellEnabled())
			}
			if cfg.JavaScriptDeclared() != tt.javascriptDeclared || cfg.JavaScriptEnabled() != tt.javascriptEnabled {
				t.Fatalf("JavaScript state = %t, %t", cfg.JavaScriptDeclared(), cfg.JavaScriptEnabled())
			}
			for _, name := range []string{".oxlintrc.json", ".prettierrc.json", ".prettierignore"} {
				if cfg.SwatchActive(name) != tt.javascriptEnabled {
					t.Fatalf("incorrect activation for %s", name)
				}
			}
			for _, name := range []string{".shellcheckrc", ".editorconfig"} {
				if cfg.SwatchActive(name) != tt.shellEnabled {
					t.Fatalf("incorrect activation for %s", name)
				}
			}
			if cfg.Languages != nil && (cfg.Languages.javaScriptYAMLKey() == "typescript") != strings.Contains(tt.input, "typescript:") {
				t.Fatal("incorrect JavaScript spelling")
			}
			for range 2 {
				before, err := yaml.Marshal(cfg.Languages)
				if err != nil {
					t.Fatal(err)
				}
				if _, err := MergeDefaults(cfg); err != nil {
					t.Fatal(err)
				}
				for _, name := range []string{".oxlintrc.json", ".prettierrc.json", ".prettierignore"} {
					found := false
					for _, entry := range cfg.Swatches {
						if entry.Path == name {
							found = true
						}
					}
					if found != tt.javascriptEnabled {
						t.Fatalf("incorrect merged entry for %s", name)
					}
				}
				for _, name := range []string{".shellcheckrc", ".editorconfig"} {
					found := false
					for _, entry := range cfg.Swatches {
						if entry.Path == name {
							found = true
							if entry.Alteration != swatch.FirstFit {
								t.Fatalf("incorrect default mode for %s", name)
							}
						}
					}
					if found != tt.shellEnabled {
						t.Fatalf("incorrect merged entry for %s", name)
					}
				}
				after, err := yaml.Marshal(cfg.Languages)
				if err != nil || string(before) != string(after) {
					t.Fatalf("merge changed languages: %s -> %s (%v)", before, after, err)
				}
				if cfg.Languages != nil {
					var decoded LanguageSettings
					if err := yaml.Unmarshal(after, &decoded); err != nil {
						t.Fatal(err)
					}
					if !reflect.DeepEqual(cfg.Languages, &decoded) {
						t.Fatalf("MarshalYAML changed languages: got %+v, want %+v", decoded, cfg.Languages)
					}
				}
				dir := t.TempDir()
				if err := Write(dir, cfg, "2026-10-21", "Refitted"); err != nil {
					t.Fatal(err)
				}
				loaded, err := Load(dir)
				if err != nil {
					t.Fatal(err)
				}
				if !reflect.DeepEqual(cfg.Languages, loaded.Languages) {
					t.Fatalf("Write changed languages: got %+v, want %+v", loaded.Languages, cfg.Languages)
				}
				cfg = loaded
			}
		})
	}
}

func TestIsolatedShellAndJavaScriptWrite(t *testing.T) {
	for _, key := range []string{"shell", "javascript", "typescript"} {
		for _, value := range []string{"true", "false"} {
			t.Run(key+"/"+value, func(t *testing.T) {
				cfg, err := parseAndValidate([]byte("license: MIT\nlanguages: {"+key+": "+value+"}\n"), "config")
				if err != nil {
					t.Fatal(err)
				}
				dir := t.TempDir()
				if err := Write(dir, cfg, "2026-10-21", "Refitted"); err != nil {
					t.Fatal(err)
				}
				loaded, err := Load(dir)
				if err != nil || !reflect.DeepEqual(cfg.Languages, loaded.Languages) {
					t.Fatalf("isolated language changed after Write and Load: %v", err)
				}
			})
		}
	}
}

func TestShellAndJavaScriptStrictParsing(t *testing.T) {
	for _, key := range []string{"shell", "javascript", "typescript"} {
		for _, value := range []string{"null", "", "'true'", "yes", "1", "[]", "{}", "{enabled: true}", "{unknown: true}"} {
			t.Run(key+"/"+value, func(t *testing.T) {
				_, err := parseAndValidate([]byte("languages: {"+key+": "+value+"}\n"), "config")
				if err == nil || !strings.Contains(err.Error(), "languages."+key+" must be a bool") {
					t.Fatalf("error = %v, want strict Boolean error", err)
				}
			})
		}
		t.Run(key+"/duplicate", func(t *testing.T) {
			_, err := parseAndValidate([]byte("languages: {"+key+": true, "+key+": false}\n"), "config")
			if err == nil || !strings.Contains(err.Error(), "already defined") {
				t.Fatalf("error = %v, want duplicate key error", err)
			}
		})
	}
	for _, first := range []string{"javascript", "typescript"} {
		second := "javascript"
		if first == second {
			second = "typescript"
		}
		for _, left := range []string{"true", "false", "null", "'true'"} {
			for _, right := range []string{"true", "false", "null", "{}"} {
				t.Run(first+"/"+left+"/"+right, func(t *testing.T) {
					_, err := parseAndValidate([]byte("languages: {"+first+": "+left+", "+second+": "+right+"}\n"), "config")
					if err == nil || !strings.Contains(err.Error(), "languages.javascript and languages.typescript cannot both be declared") {
						t.Fatalf("error = %v, want alias conflict", err)
					}
				})
			}
		}
	}
}

func TestShellAndJavaScriptNilConfig(t *testing.T) {
	var cfg *Config
	if cfg.ShellDeclared() || cfg.ShellEnabled() || cfg.JavaScriptDeclared() || cfg.JavaScriptEnabled() {
		t.Fatal("nil config declares a language")
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

func TestMCPDeclarationDoesNotRegisterSwatches(t *testing.T) {
	paths := func(cfg *Config) []string {
		active := cfg.ActiveDefaultSwatches()
		result := make([]string, len(active))
		for i, entry := range active {
			result[i] = entry.Path
		}
		return result
	}
	want := paths(&Config{})

	for _, tt := range []struct {
		name string
		mcp  *MCPSettings
	}{
		{name: "absent"},
		{name: "empty", mcp: &MCPSettings{}},
		{name: "false", mcp: &MCPSettings{Playwright: new(false)}},
		{name: "true", mcp: &MCPSettings{Playwright: new(true)}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &Config{MCP: tt.mcp}
			if got := paths(cfg); !reflect.DeepEqual(got, want) {
				t.Fatalf("active swatches changed with MCP declaration: got %v, want %v", got, want)
			}
			if added := MergeDefaultSwatches(cfg); len(added) != len(want)-1 {
				t.Fatalf("added swatches = %d, want %d", len(added), len(want)-1)
			}
		})
	}

	for _, path := range swatch.Paths() {
		lower := strings.ToLower(path)
		if strings.Contains(lower, "mcp") || strings.Contains(lower, "playwright") {
			t.Errorf("MCP tooling path is registered for production: %s", path)
		}
	}
}
