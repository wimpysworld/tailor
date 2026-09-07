package config

import (
	"testing"

	"github.com/wimpysworld/tailor/internal/model"
)

func TestImmutableReleasesDefaultAndOmission(t *testing.T) {
	defaults, err := DefaultConfig("MIT")
	if err != nil || defaults.ImmutableReleases == nil || defaults.ImmutableReleases.Enabled == nil || *defaults.ImmutableReleases.Enabled {
		t.Fatalf("default = %v, error = %v", defaults, err)
	}
	for _, enabled := range []*bool{nil, new(false), new(true)} {
		cfg := &Config{License: "MIT", ImmutableReleases: &model.ImmutableReleasesSettings{Enabled: enabled}}
		if _, err := MergeDefaults(cfg); err != nil {
			t.Fatal(err)
		}
		if cfg.ImmutableReleases.Enabled != enabled {
			t.Fatal("merge changed declared toggle")
		}
		text := writeConfig(t, cfg, "2026-09-07", "Refitted")
		roundtrip, err := parseAndValidate([]byte(text), "test")
		if err != nil {
			t.Fatal(err)
		}
		if enabled == nil {
			if roundtrip.ImmutableReleases != nil && roundtrip.ImmutableReleases.Enabled != nil {
				t.Fatal("roundtrip managed an omitted toggle")
			}
		} else if roundtrip.ImmutableReleases == nil || roundtrip.ImmutableReleases.Enabled == nil || *roundtrip.ImmutableReleases.Enabled != *enabled {
			t.Fatal("roundtrip changed toggle")
		}
	}
	cfg := &Config{License: "MIT"}
	if _, err := MergeDefaults(cfg); err != nil || cfg.ImmutableReleases != nil {
		t.Fatalf("merge managed an omitted section: %v", err)
	}
}

func TestImmutableReleasesValidation(t *testing.T) {
	for _, text := range []string{"immutable_releases:\n  enabled: banana\n", "immutable_releases:\n  enforced_by_owner: true\n", "repository:\n  immutable_releases_enabled: false\n"} {
		if _, err := parseAndValidate([]byte(text), "test"); err == nil {
			t.Errorf("accepted %q", text)
		}
	}
}
