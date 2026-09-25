package config

import (
	"fmt"
	"os"
	"reflect"
	"slices"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestGoMappingOptions(t *testing.T) {
	values := []*bool{nil, new(false), new(true)}
	for _, enabled := range []bool{false, true} {
		for _, cgo := range values {
			for _, ffmpeg := range values {
				body := fmt.Sprintf("languages:\n  go:\n    enabled: %t\n", enabled)
				if cgo != nil {
					body += fmt.Sprintf("    cgo: %t\n", *cgo)
				}
				if ffmpeg != nil {
					body += fmt.Sprintf("    ffmpeg-statigo: %t\n", *ffmpeg)
				}
				t.Run(strings.ReplaceAll(body, "\n", "/"), func(t *testing.T) {
					cfg, err := parseAndValidate([]byte("license: MIT\n"+body), "config")
					component := ffmpeg != nil && *ffmpeg
					invalid := component && (!enabled || (cgo != nil && !*cgo))
					if invalid {
						if err == nil {
							t.Fatal("accepted invalid FFmpeg prerequisites")
						}
						return
					}
					if err != nil {
						t.Fatal(err)
					}
					if !cfg.GoDeclared() || cfg.GoEnabled() != enabled || cfg.GoCGODeclared() != (cgo != nil) || cfg.FFmpegStatigoDeclared() != (ffmpeg != nil) || cfg.FFmpegStatigoEnabled() != component {
						t.Fatal("incorrect option declarations or activation")
					}
					var effective *bool
					if enabled {
						effective = cgo
						if effective == nil && component {
							effective = new(true)
						}
					}
					if !reflect.DeepEqual(cfg.EffectiveGoCGO(), effective) || cfg.GoCGOEnabled() != (effective != nil && *effective) {
						t.Fatalf("effective CGO = %v, want %v", cfg.EffectiveGoCGO(), effective)
					}
					if actual := cfg.EffectiveGoCGO(); actual != nil {
						*actual = !*actual
						if !reflect.DeepEqual(cfg.EffectiveGoCGO(), effective) {
							t.Fatal("effective CGO pointer changed the config")
						}
					}
					dir := t.TempDir()
					for cycle := range 2 {
						changed, err := MergeDefaults(cfg)
						if err != nil || (cycle == 1 && changed) {
							t.Fatalf("merge cycle %d = %t, %v", cycle, changed, err)
						}
						for _, path := range []string{".golangci.yml", ".goreleaser.yaml", ".github/workflows/build-go.yml", "Dockerfile"} {
							present := slices.ContainsFunc(cfg.Swatches, func(entry SwatchEntry) bool { return entry.Path == path })
							if present != enabled || cfg.SwatchActive(path) != enabled {
								t.Fatalf("mapping activation mismatch for %s", path)
							}
						}
						if err := Write(dir, cfg, "2026-09-18", "Refitted"); err != nil {
							t.Fatal(err)
						}
						data, err := os.ReadFile(dir + "/.tailor.yml")
						if err != nil || !strings.Contains(string(data), body) {
							t.Fatalf("writer changed mapping representation: %s, %v", data, err)
						}
						loaded, err := Load(dir)
						if err != nil {
							t.Fatal(err)
						}
						if !reflect.DeepEqual(cfg.Languages, loaded.Languages) {
							t.Fatal("merge/write/load changed Go options")
						}
						cfg = loaded
					}
					data, err := yaml.Marshal(cfg.Languages)
					if err != nil {
						t.Fatal(err)
					}
					var restored LanguageSettings
					if err := yaml.Unmarshal(data, &restored); err != nil || !reflect.DeepEqual(cfg.Languages, &restored) {
						t.Fatalf("YAML round trip = %#v, %v", restored, err)
					}
				})
			}
		}
	}
}

func TestGoMappingStrictTypes(t *testing.T) {
	for _, field := range []string{"enabled", "cgo", "ffmpeg-statigo"} {
		for _, value := range []string{"null", "", "'true'", "1", "yes", "[]", "{}"} {
			body := "languages:\n  go:\n"
			if field != "enabled" {
				body += "    enabled: true\n"
			}
			body += fmt.Sprintf("    %s: %s\n", field, value)
			t.Run(field+"/"+value, func(t *testing.T) {
				_, err := parseAndValidate([]byte(body), "config")
				if err == nil || !strings.Contains(err.Error(), "languages.go."+field+" must be a bool") {
					t.Fatalf("error = %v", err)
				}
			})
		}
		body := fmt.Sprintf("languages: {go: {enabled: true, %s: false, %s: true}}", field, field)
		if _, err := parseAndValidate([]byte(body), "config"); err == nil || !strings.Contains(err.Error(), "already defined") {
			t.Fatalf("duplicate %s: %v", field, err)
		}
	}
	for _, body := range []string{
		"languages: {go: {cgo: true}}",
		"languages: {go: {ffmpeg-statigo: true}}",
		"languages: {go: {enabled: true, unknown: false}}",
		"languages: {go: {enabled: true, <<: {cgo: true}}}",
		"languages: {<<: {go: true}}",
	} {
		if _, err := parseAndValidate([]byte(body), "config"); err == nil {
			t.Fatalf("accepted %s", body)
		}
	}
}

func TestGoOptionsAbsentAndLegacy(t *testing.T) {
	for _, cfg := range []*Config{nil, {}, {Languages: &LanguageSettings{}}, {Languages: &LanguageSettings{Go: new(false)}}, {Languages: &LanguageSettings{Go: new(true)}}} {
		if cfg.GoCGODeclared() || cfg.GoCGOEnabled() || cfg.EffectiveGoCGO() != nil || cfg.FFmpegStatigoDeclared() || cfg.FFmpegStatigoEnabled() {
			t.Fatalf("absent options became active: %#v", cfg)
		}
	}
	for _, enabled := range []bool{false, true} {
		languages := &LanguageSettings{Go: new(enabled)}
		data, err := yaml.Marshal(languages)
		if err != nil || string(data) != fmt.Sprintf("go: %t\n", enabled) {
			t.Fatalf("legacy YAML = %q, %v", data, err)
		}
	}
}
