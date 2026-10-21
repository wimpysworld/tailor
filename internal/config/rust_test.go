package config

import (
	"reflect"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestRustConfiguration(t *testing.T) {
	defaults, err := DefaultConfig("MIT")
	if err != nil || !defaults.RustDeclared() || defaults.RustEnabled() {
		t.Fatalf("Rust default: %v, %v", defaults, err)
	}
	for _, declaration := range []string{"", "languages: {}", "languages: {rust: false}", "languages: {rust: true}", "languages: {go: true, rust: true}", "languages: {go: {enabled: true, cgo: false}, rust: true}"} {
		t.Run(declaration, func(t *testing.T) {
			cfg, err := parseAndValidate([]byte("license: MIT\n"+declaration+"\n"), "config")
			if err != nil {
				t.Fatal(err)
			}
			if cfg.RustDeclared() != strings.Contains(declaration, "rust:") || cfg.RustEnabled() != strings.Contains(declaration, "rust: true") {
				t.Fatal("Rust state differs from declaration")
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
				t.Fatal("merge changed language declarations")
			}
			root := t.TempDir()
			if err := Write(root, cfg, "2026-09-25", "Refitted"); err != nil {
				t.Fatal(err)
			}
			loaded, err := Load(root)
			if err != nil || !reflect.DeepEqual(cfg.Languages, loaded.Languages) {
				t.Fatalf("round trip: %v", err)
			}
		})
	}
	for _, value := range []string{"null", "", "'true'", "yes", "1", "[]", "{}", "{enabled: true}"} {
		if _, err := parseAndValidate([]byte("languages: {rust: "+value+"}\n"), "config"); err == nil {
			t.Errorf("accepted rust: %s", value)
		}
	}
	if _, err := parseAndValidate([]byte("languages: {rust: true, rust: false}\n"), "config"); err == nil {
		t.Fatal("accepted duplicate Rust declaration")
	}
}
