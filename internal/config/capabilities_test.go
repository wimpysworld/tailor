package config

import (
	"fmt"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/wimpysworld/tailor/internal/swatch"
	"github.com/wimpysworld/tailor/internal/testutil"
)

type capabilityState struct {
	name  string
	value *bool
}

func TestCapabilityDeclarationsSurviveRepeatedPersistence(t *testing.T) {
	states := []capabilityState{
		{name: "absent"},
		{name: "false", value: new(false)},
		{name: "true", value: new(true)},
	}
	wantModes := map[string]swatch.AlterationMode{
		".gitignore":                       swatch.Never,
		"SECURITY.md":                      swatch.FirstFit,
		".github/pull_request_template.md": swatch.Always,
	}

	for _, goState := range states {
		for _, pagesState := range states {
			for _, playwrightState := range states {
				name := fmt.Sprintf("go=%s/pages=%s/playwright=%s", goState.name, pagesState.name, playwrightState.name)
				t.Run(name, func(t *testing.T) {
					dir := t.TempDir()
					var input strings.Builder
					input.WriteString("license: none\n")
					writeCapabilityState(&input, "languages", "go", goState.value)
					writeCapabilityState(&input, "pages", "enabled", pagesState.value)
					writeCapabilityState(&input, "mcp", "playwright", playwrightState.value)
					input.WriteString(`swatches:
  - path: .gitignore
    alteration: never
  - path: SECURITY.md
    alteration: first-fit
  - path: .github/pull_request_template.md
    alteration: always
`)
					testutil.WriteConfig(t, dir, input.String())

					cfg, err := Load(dir)
					if err != nil {
						t.Fatal(err)
					}
					assertCapabilityStates(t, cfg, goState.value, pagesState.value, playwrightState.value)
					assertSwatchModes(t, cfg, wantModes)

					for cycle := 1; cycle <= 2; cycle++ {
						changed, err := MergeDefaults(cfg)
						if err != nil {
							t.Fatalf("cycle %d merge: %v", cycle, err)
						}
						if cycle == 2 && changed {
							t.Fatal("second merge changed the persisted config")
						}
						assertCapabilityStates(t, cfg, goState.value, pagesState.value, playwrightState.value)
						assertSwatchModes(t, cfg, wantModes)

						if err := Write(dir, cfg, "2026-09-18", "Refitted"); err != nil {
							t.Fatalf("cycle %d write: %v", cycle, err)
						}
						cfg, err = Load(dir)
						if err != nil {
							t.Fatalf("cycle %d reload: %v", cycle, err)
						}
						assertCapabilityStates(t, cfg, goState.value, pagesState.value, playwrightState.value)
						assertSwatchModes(t, cfg, wantModes)
					}

					if changed, err := MergeDefaults(cfg); err != nil || changed {
						t.Fatalf("merge after two cycles = %t, %v", changed, err)
					}
				})
			}
		}
	}
}

func TestCapabilitySectionsDirectWriteAndReload(t *testing.T) {
	for _, tt := range []struct {
		name string
		body string
	}{
		{name: "absent"},
		{name: "empty mappings", body: "languages: {}\nmcp: {}\npages: {}\n"},
		{name: "partial pages", body: "languages: {}\nmcp: {}\npages:\n  cname: docs.example.com\n"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			testutil.WriteConfig(t, dir, "license: none\n"+tt.body+`swatches:
  - path: .gitignore
    alteration: never
  - path: SECURITY.md
    alteration: first-fit
  - path: .github/pull_request_template.md
    alteration: always
`)
			before, err := Load(dir)
			if err != nil {
				t.Fatal(err)
			}
			if err := Write(dir, before, "2026-09-18", "Refitted"); err != nil {
				t.Fatal(err)
			}
			after, err := Load(dir)
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(after.Languages, before.Languages) || !reflect.DeepEqual(after.MCP, before.MCP) || !reflect.DeepEqual(after.Pages, before.Pages) {
				t.Fatalf("direct write changed capability sections: before=%+v/%+v/%+v after=%+v/%+v/%+v", before.Languages, before.MCP, before.Pages, after.Languages, after.MCP, after.Pages)
			}
			if !reflect.DeepEqual(after.Swatches, before.Swatches) {
				t.Fatalf("direct write changed swatches: got %v, want %v", after.Swatches, before.Swatches)
			}
		})
	}
}

func TestCapabilitySectionsMergeWriteConverges(t *testing.T) {
	for _, tt := range []struct {
		name      string
		body      string
		wantCNAME *string
	}{
		{name: "empty mappings", body: "languages: {}\nmcp: {}\npages: {}\n"},
		{name: "partial pages", body: "languages: {}\nmcp: {}\npages:\n  cname: docs.example.com\n", wantCNAME: new("docs.example.com")},
	} {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			testutil.WriteConfig(t, dir, "license: none\n"+tt.body+`swatches:
  - path: .gitignore
    alteration: never
  - path: SECURITY.md
    alteration: first-fit
  - path: .github/pull_request_template.md
    alteration: always
`)
			cfg, err := Load(dir)
			if err != nil {
				t.Fatal(err)
			}

			for cycle := 1; cycle <= 2; cycle++ {
				changed, err := MergeDefaults(cfg)
				if err != nil {
					t.Fatalf("cycle %d merge: %v", cycle, err)
				}
				if cycle == 2 && changed {
					t.Fatal("second merge changed the persisted config")
				}
				assertAbsentCapabilityDeclarations(t, cfg, tt.wantCNAME)

				if err := Write(dir, cfg, "2026-09-18", "Refitted"); err != nil {
					t.Fatalf("cycle %d write: %v", cycle, err)
				}
				cfg, err = Load(dir)
				if err != nil {
					t.Fatalf("cycle %d reload: %v", cycle, err)
				}
				assertAbsentCapabilityDeclarations(t, cfg, tt.wantCNAME)
			}
		})
	}
}

func assertAbsentCapabilityDeclarations(t *testing.T, cfg *Config, wantCNAME *string) {
	t.Helper()
	if cfg.Languages == nil || cfg.Languages.Go != nil {
		t.Errorf("languages.go = %+v, want nil in existing section", cfg.Languages)
	}
	if cfg.MCP == nil || cfg.MCP.Playwright != nil {
		t.Errorf("mcp.playwright = %+v, want nil in existing section", cfg.MCP)
	}
	if cfg.Pages == nil {
		t.Fatal("pages section is absent")
	}
	if cfg.Pages.Enabled != nil || cfg.Pages.Links != nil {
		t.Errorf("Pages declarations = enabled %v, links %v, want nil", cfg.Pages.Enabled, cfg.Pages.Links)
	}
	testutil.AssertPtrEqual(t, cfg.Pages.CNAME, wantCNAME, "pages.cname")
}

func writeCapabilityState(output *strings.Builder, section, key string, value *bool) {
	if value != nil {
		fmt.Fprintf(output, "%s:\n  %s: %t\n", section, key, *value)
	}
}

func assertCapabilityStates(t *testing.T, cfg *Config, goValue, pagesValue, playwrightValue *bool) {
	t.Helper()
	switch {
	case goValue == nil:
		if cfg.Languages != nil {
			t.Errorf("languages section = %+v, want absent", cfg.Languages)
		}
	case cfg.Languages == nil:
		t.Error("languages section is absent")
	default:
		testutil.AssertPtrEqual(t, cfg.Languages.Go, goValue, "languages.go")
	}
	switch {
	case pagesValue == nil:
		if cfg.Pages != nil {
			t.Errorf("pages section = %+v, want absent", cfg.Pages)
		}
	case cfg.Pages == nil:
		t.Error("pages section is absent")
	default:
		testutil.AssertPtrEqual(t, cfg.Pages.Enabled, pagesValue, "pages.enabled")
	}
	switch {
	case playwrightValue == nil:
		if cfg.MCP != nil {
			t.Errorf("mcp section = %+v, want absent", cfg.MCP)
		}
	case cfg.MCP == nil:
		t.Error("mcp section is absent")
	default:
		testutil.AssertPtrEqual(t, cfg.MCP.Playwright, playwrightValue, "mcp.playwright")
	}

	assertCapabilityAccessors(t, cfg, goValue, pagesValue, playwrightValue)
}

func assertCapabilityAccessors(t *testing.T, cfg *Config, goValue, pagesValue, playwrightValue *bool) {
	t.Helper()
	for _, capability := range []struct {
		name              string
		want              *bool
		declared, enabled bool
	}{
		{name: "languages.go", want: goValue, declared: cfg.GoDeclared(), enabled: cfg.GoEnabled()},
		{name: "pages.enabled", want: pagesValue, declared: cfg.PagesDeclared(), enabled: cfg.PagesEnabled()},
		{name: "mcp.playwright", want: playwrightValue, declared: cfg.PlaywrightDeclared(), enabled: cfg.PlaywrightEnabled()},
	} {
		wantDeclared := capability.want != nil
		wantEnabled := wantDeclared && *capability.want
		if capability.declared != wantDeclared || capability.enabled != wantEnabled {
			t.Errorf("%s accessors = declared %t, enabled %t, want declared %t, enabled %t", capability.name, capability.declared, capability.enabled, wantDeclared, wantEnabled)
		}
	}
}

func assertSwatchModes(t *testing.T, cfg *Config, want map[string]swatch.AlterationMode) {
	t.Helper()
	for path, mode := range want {
		if !slices.ContainsFunc(cfg.Swatches, func(entry SwatchEntry) bool {
			return entry.Path == path && entry.Alteration == mode
		}) {
			t.Errorf("swatches lost %s mode for %s: %v", mode, path, cfg.Swatches)
		}
	}
}
