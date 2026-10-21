package config

import (
	"reflect"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestEbitengineDefaults(t *testing.T) {
	for _, tc := range []struct {
		declaration      string
		enabled, android bool
	}{
		{"true", true, false},
		{"false", false, false},
		{"{enabled: true}", true, false},
		{"{enabled: true, android: {enabled: false}}", true, false},
		{"{enabled: true, android: {enabled: true}}", true, true},
	} {
		t.Run(tc.declaration, func(t *testing.T) {
			cfg, err := parseAndValidate([]byte("languages: {go: {enabled: true, ebitengine: "+tc.declaration+"}}\n"), "config")
			if err != nil {
				t.Fatal(err)
			}
			got := []bool{cfg.EbitengineEnabled(), cfg.EbitengineAndroidEnabled()}
			want := []bool{tc.enabled, tc.android}
			if !reflect.DeepEqual(got, want) {
				t.Fatalf("profiles %v != %v", got, want)
			}
			if cfg.EffectiveGoCGO() != nil {
				t.Fatal("Ebitengine changed absent CGO policy")
			}
		})
	}
	for _, declaration := range []string{"", "languages: {go: true}\n", "languages: {go: {enabled: true}}\n"} {
		cfg, err := parseAndValidate([]byte("license: MIT\n"+declaration), "config")
		if err != nil {
			t.Fatal(err)
		}
		if _, err := MergeDefaults(cfg); err != nil {
			t.Fatal(err)
		}
		if cfg.EbitengineDeclared() {
			t.Fatal("merge added Ebitengine declaration")
		}
	}
}

func TestEbitengineDeclarations(t *testing.T) {
	for _, value := range []string{"true", "false", "{enabled: true}", "{enabled: false}", "{enabled: true, android: {enabled: true}}", "{enabled: false, android: {enabled: false}}", "{enabled: true, android: {enabled: false}}"} {
		t.Run(value, func(t *testing.T) {
			cfg, err := parseAndValidate([]byte("license: MIT\nlanguages: {go: {enabled: true, cgo: false, ebitengine: "+value+"}}\n"), "config")
			if err != nil {
				t.Fatal(err)
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
				t.Fatalf("merge changed declaration: %s %v", after, err)
			}
			dir := t.TempDir()
			if err := Write(dir, cfg, "2026-10-21", "Refitted"); err != nil {
				t.Fatal(err)
			}
			got, err := Load(dir)
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(cfg.Languages, got.Languages) {
				t.Fatal("round trip changed declaration")
			}
			if cgo := got.EffectiveGoCGO(); cgo == nil || *cgo {
				t.Fatal("Ebitengine changed explicit CGO false")
			}
		})
	}
}

func TestEbitengineRejectsInvalidDeclarations(t *testing.T) {
	for _, value := range []string{"null", "[]", "'true'", "{}", "{enabled: null}", "{enabled: true, unknown: true}", "{enabled: true, desktop: true}", "{enabled: true, desktop: false}", "{enabled: true, web: true}", "{enabled: true, assets: true}", "{enabled: true, delivery: true}", "{enabled: true, accept-license: true}", "{enabled: true, android: {enabled: true, platform: '35'}}", "{enabled: true, android: {enabled: true, build-tools: '35.0.0'}}", "{enabled: true, android: {enabled: true, ndk: '28.2.13676358'}}", "{enabled: true, android: {enabled: true, accept-license: true}}", "{enabled: true, android: {enabled: null}}", "{enabled: true, android: {enabled: true, enabled: false}}", "{enabled: true, enabled: false}", "{enabled: false, web: true}", "{enabled: false, android: {enabled: true}}", "{enabled: true, android: true}", "{enabled: true, android: null}", "{enabled: true, android: {}}", "{enabled: true, android: {enabled: false, unknown: true}}", "{enabled: true, android: {enabled: false, accept-license: null}}", "{enabled: true, android: {enabled: false, ndk: ''}}", "{enabled: true, assets: null}", "{enabled: true, delivery: 'false'}", "{enabled: true, android: false}", "{enabled: true, android: {enabled: true, accept-license: false}}", "{enabled: true, android: {enabled: false, platform: 35}}", "{enabled: true, android: {enabled: false, ndk: 'latest'}}", "{enabled: true, android: {enabled: false, build-tools: '${evil}'}}"} {
		t.Run(value, func(t *testing.T) {
			_, err := parseAndValidate([]byte("languages: {go: {enabled: true, ebitengine: "+value+"}}\n"), "config")
			if err == nil {
				t.Fatal("accepted invalid declaration")
			}
		})
	}
	_, err := parseAndValidate([]byte("languages: {go: {enabled: false, ebitengine: true}}\n"), "config")
	if err == nil {
		t.Fatal("accepted enabled child under disabled Go")
	}
}
