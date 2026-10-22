package config

import (
	"reflect"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestPythonConfiguration(t *testing.T) {
	defaults, err := DefaultConfig("MIT")
	if err != nil || !defaults.PythonDeclared() || defaults.PythonEnabled() {
		t.Fatalf("Python default: %v %v", defaults, err)
	}
	for _, declaration := range []string{"", "languages: {}", "languages: {python: false}", "languages: {python: true}", "languages: {go: {enabled: true}, python: true}"} {
		t.Run(declaration, func(t *testing.T) {
			cfg, err := parseAndValidate([]byte("license: MIT\n"+declaration+"\n"), "config")
			if err != nil {
				t.Fatal(err)
			}
			if cfg.PythonDeclared() != strings.Contains(declaration, "python:") || cfg.PythonEnabled() != strings.Contains(declaration, "python: true") {
				t.Fatal("incorrect Python state")
			}
			before, err := yaml.Marshal(cfg.Languages)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := MergeDefaults(cfg); err != nil {
				t.Fatal(err)
			}
			after, err := yaml.Marshal(cfg.Languages)
			if err != nil || string(before) != string(after) {
				t.Fatal("merge changed languages")
			}
			dir := t.TempDir()
			if err := Write(dir, cfg, "2026-10-21", "Refitted"); err != nil {
				t.Fatal(err)
			}
			loaded, err := Load(dir)
			if err != nil {
				t.Fatalf("round trip load: %v", err)
			}
			if !reflect.DeepEqual(cfg.Languages, loaded.Languages) {
				t.Fatalf("round trip changed languages: got %+v, want %+v", loaded.Languages, cfg.Languages)
			}
			for _, path := range []string{"ruff.toml", "pyrightconfig.json"} {
				if cfg.SwatchActive(path) != cfg.PythonEnabled() {
					t.Fatal("incorrect activation")
				}
			}
		})
	}
	for _, value := range []string{"null", "", "'true'", "yes", "1", "[]", "{}", "{enabled: true}"} {
		if _, err := parseAndValidate([]byte("languages: {python: "+value+"}\n"), "config"); err == nil {
			t.Errorf("accepted %s", value)
		}
	}
}
