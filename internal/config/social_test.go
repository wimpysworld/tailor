package config

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/wimpysworld/tailor/internal/model"
)

func TestSocialParsingAndValidation(t *testing.T) {
	longPlatforms := strings.Repeat("a", 121)
	maxPlatforms := strings.Repeat("a", 120)
	longTerminal := strings.Repeat("x", 81)
	maxTerminal := strings.Repeat("x", 80)

	tests := []struct {
		name      string
		body      string
		wantError string
	}{
		{name: "omitted"},
		{name: "empty mapping", body: "social: {}"},
		{name: "disabled", body: "social:\n  enabled: false"},
		{name: "enabled minimal", body: "social:\n  enabled: true"},
		{
			name: "full valid dark",
			body: `social:
  enabled: true
  theme: dark
  accent: "#1e66f5"
  background: "#1e1e2e"
  platforms: "` + maxPlatforms + `"
  og:
    output: assets/social/og.png
    include_preview: true
    image: docs/shot.webp
    terminal:
      - "$ tailor measure"
      - "` + maxTerminal + `"
  readme:
    header: true
    preferred_links:
      - website
      - irc
      - matrix
      - bluesky
    community: irc
    packages:
      - apt
      - rpm
      - pkg
      - apk
      - flatpak
      - brew
      - scoop
      - nix
    demo: true
    stats: true
    star_history:
      enabled: true
      min_stars: 0
      min_days: 0
    sponsors: auto
    contributors: true
    footer: true
  live:
    branch: social-assets
    schedule: "0 4 * * *"`,
		},
		{
			name: "light theme and empty optional strings",
			body: `social:
  theme: light
  accent: ""
  background: ""
  platforms: ""
  og:
    image: ""
    terminal: []
  readme:
    preferred_links: []
    packages: []`,
		},
		{name: "image extensions png", body: "social:\n  og:\n    image: assets/shot.png"},
		{name: "image extensions jpg", body: "social:\n  og:\n    image: assets/shot.jpg"},
		{name: "image extensions jpeg", body: "social:\n  og:\n    image: assets/shot.jpeg"},
		{name: "image extensions avif", body: "social:\n  og:\n    image: assets/shot.avif"},
		{name: "image extensions uppercase", body: "social:\n  og:\n    image: assets/shot.PNG"},
		{name: "sponsors bool true", body: "social:\n  readme:\n    sponsors: true"},
		{name: "sponsors bool false", body: "social:\n  readme:\n    sponsors: false"},
		{name: "sponsors bool uppercase", body: "social:\n  readme:\n    sponsors: True"},
		{name: "sponsors string true", body: "social:\n  readme:\n    sponsors: \"true\""},
		{name: "sponsors string false", body: "social:\n  readme:\n    sponsors: \"false\""},
		{name: "sponsors string auto", body: "social:\n  readme:\n    sponsors: \"auto\""},
		{name: "community discussions", body: "social:\n  readme:\n    community: discussions"},
		{name: "community none", body: "social:\n  readme:\n    community: none"},
		{name: "schedule day names", body: "social:\n  live:\n    schedule: \"*/15 0-6 * * MON-FRI\""},

		// Structural / node type errors.
		{name: "social not mapping", body: "social: []", wantError: "social must be a mapping"},
		{name: "enabled string", body: "social:\n  enabled: \"true\"", wantError: "social.enabled must be a bool"},
		{name: "theme number", body: "social:\n  theme: 123", wantError: "social.theme must be a string"},
		{name: "accent number", body: "social:\n  accent: 123456", wantError: "social.accent must be a string"},
		{name: "og not mapping", body: "social:\n  og: true", wantError: "social.og must be a mapping"},
		{name: "og include_preview string", body: "social:\n  og:\n    include_preview: \"false\"", wantError: "social.og.include_preview must be a bool"},
		{name: "og terminal not sequence", body: "social:\n  og:\n    terminal: \"$ foo\"", wantError: "social.og.terminal must be a sequence"},
		{name: "og terminal item non-string", body: "social:\n  og:\n    terminal: [123]", wantError: "social.og.terminal[0] must be a string"},
		{name: "readme not mapping", body: "social:\n  readme: []", wantError: "social.readme must be a mapping"},
		{name: "readme header string", body: "social:\n  readme:\n    header: \"true\"", wantError: "social.readme.header must be a bool"},
		{name: "readme demo string", body: "social:\n  readme:\n    demo: \"true\"", wantError: "social.readme.demo must be a bool"},
		{name: "readme stats string", body: "social:\n  readme:\n    stats: \"true\"", wantError: "social.readme.stats must be a bool"},
		{name: "readme contributors string", body: "social:\n  readme:\n    contributors: \"true\"", wantError: "social.readme.contributors must be a bool"},
		{name: "readme footer string", body: "social:\n  readme:\n    footer: \"true\"", wantError: "social.readme.footer must be a bool"},
		{name: "readme sponsors null", body: "social:\n  readme:\n    sponsors: null", wantError: "social.readme.sponsors must be a string or bool"},
		{name: "readme sponsors int", body: "social:\n  readme:\n    sponsors: 1", wantError: "social.readme.sponsors must be a string or bool"},
		{name: "star_history not mapping", body: "social:\n  readme:\n    star_history: true", wantError: "social.readme.star_history must be a mapping"},
		{name: "star_history min_stars string", body: "social:\n  readme:\n    star_history:\n      min_stars: \"25\"", wantError: "social.readme.star_history.min_stars must be a int"},
		{name: "live not mapping", body: "social:\n  live: \"social-assets\"", wantError: "social.live must be a mapping"},

		// Unknown keys at every nesting level (including forbidden title/tagline/icon).
		{name: "unknown top social key title", body: "social:\n  title: demo", wantError: `unrecognised social setting "title"`},
		{name: "unknown top social key tagline", body: "social:\n  tagline: demo", wantError: `unrecognised social setting "tagline"`},
		{name: "unknown top social key icon", body: "social:\n  icon: pages/icon.svg", wantError: `unrecognised social setting "icon"`},
		{name: "unknown og key", body: "social:\n  og:\n    width: 1280", wantError: `unrecognised social.og setting "width"`},
		{name: "unknown readme key", body: "social:\n  readme:\n    badges: true", wantError: `unrecognised social.readme setting "badges"`},
		{name: "unknown star_history key", body: "social:\n  readme:\n    star_history:\n      theme: dark", wantError: `unrecognised social.readme.star_history setting "theme"`},
		{name: "unknown live key", body: "social:\n  live:\n    remote: origin", wantError: `unrecognised social.live setting "remote"`},

		// Semantic validation errors.
		{name: "invalid theme", body: "social:\n  theme: sepia", wantError: `invalid social.theme "sepia"`},
		{name: "accent missing hash", body: "social:\n  accent: \"1e66f5\"", wantError: "social.accent"},
		{name: "accent short hex", body: "social:\n  accent: \"#fff\"", wantError: "social.accent"},
		{name: "background invalid hex", body: "social:\n  background: \"#gggggg\"", wantError: "social.background"},
		{name: "platforms too long", body: "social:\n  platforms: \"" + longPlatforms + "\"", wantError: "social.platforms exceeds 120 characters"},
		{name: "platforms control char", body: "social:\n  platforms: \"Linux\\nmacOS\"", wantError: "social.platforms contains control characters"},

		{name: "og output empty", body: "social:\n  og:\n    output: \"\"", wantError: "social.og.output must be a safe project-relative path"},
		{name: "og output absolute", body: "social:\n  og:\n    output: /tmp/og.png", wantError: "social.og.output must be a safe project-relative path"},
		{name: "og output traversal", body: "social:\n  og:\n    output: ../og.png", wantError: "social.og.output must not contain empty, dot, or traversal components"},
		{name: "og output non-png", body: "social:\n  og:\n    output: pages/og.jpg", wantError: "social.og.output must end with .png"},
		{name: "og output bare dot png", body: "social:\n  og:\n    output: pages/.png", wantError: "social.og.output must end with .png"},
		{name: "og image absolute", body: "social:\n  og:\n    image: /tmp/shot.png", wantError: "social.og.image must be a safe project-relative path"},
		{name: "og image traversal", body: "social:\n  og:\n    image: docs/../shot.png", wantError: "social.og.image must not contain empty, dot, or traversal components"},
		{name: "og image unsupported ext", body: "social:\n  og:\n    image: docs/shot.svg", wantError: "social.og.image must use one of"},
		{name: "og terminal too many lines", body: "social:\n  og:\n    terminal: [a, b, c, d, e, f, g, h, i]", wantError: "social.og.terminal: 9 lines exceed the maximum of 8"},
		{name: "og terminal line too long", body: "social:\n  og:\n    terminal:\n      - \"" + longTerminal + "\"", wantError: "social.og.terminal[0]: line exceeds 80 characters"},
		{name: "og terminal line control char", body: "social:\n  og:\n    terminal:\n      - \"hello\\tworld\"", wantError: "social.og.terminal[0]: line contains control characters"},

		{name: "preferred_links too many", body: "social:\n  readme:\n    preferred_links: [website, x, bluesky, mastodon, discord]", wantError: "social.readme.preferred_links: 5 entries exceed the maximum of 4"},
		{name: "preferred_links duplicate", body: "social:\n  readme:\n    preferred_links: [irc, irc]", wantError: `social.readme.preferred_links[1]: duplicate link "irc"`},
		{name: "preferred_links unknown", body: "social:\n  readme:\n    preferred_links: [github]", wantError: `social.readme.preferred_links[0]: unrecognised link "github"`},
		{name: "community invalid", body: "social:\n  readme:\n    community: telegram", wantError: `invalid social.readme.community "telegram"`},
		{name: "packages unknown", body: "social:\n  readme:\n    packages: [snap]", wantError: `social.readme.packages[0]: unrecognised package "snap"`},
		{name: "packages duplicate", body: "social:\n  readme:\n    packages: [apt, apt]", wantError: `social.readme.packages[1]: duplicate package "apt"`},
		{name: "star_history negative min_stars", body: "social:\n  readme:\n    star_history:\n      min_stars: -1", wantError: "social.readme.star_history.min_stars must be at least 0"},
		{name: "star_history negative min_days", body: "social:\n  readme:\n    star_history:\n      min_days: -5", wantError: "social.readme.star_history.min_days must be at least 0"},
		{name: "sponsors invalid", body: "social:\n  readme:\n    sponsors: always", wantError: `invalid social.readme.sponsors "always"`},

		{name: "live branch empty", body: "social:\n  live:\n    branch: \"\"", wantError: "social.live.branch must be a valid branch name"},
		{name: "live branch spaces", body: "social:\n  live:\n    branch: social assets", wantError: "social.live.branch must be a valid branch name"},
		{name: "live branch dotdot", body: "social:\n  live:\n    branch: social..assets", wantError: "social.live.branch must be a valid branch name"},
		{name: "live branch lock", body: "social:\n  live:\n    branch: social.lock", wantError: "social.live.branch must be a valid branch name"},
		{name: "live schedule empty", body: "social:\n  live:\n    schedule: \"\"", wantError: "social.live.schedule must contain 5 whitespace-separated fields"},
		{name: "live schedule 4 fields", body: "social:\n  live:\n    schedule: \"0 4 * *\"", wantError: "social.live.schedule must contain 5 whitespace-separated fields"},
		{name: "live schedule 6 fields", body: "social:\n  live:\n    schedule: \"0 0 4 * * *\"", wantError: "social.live.schedule must contain 5 whitespace-separated fields"},
		{name: "live schedule expression", body: "social:\n  live:\n    schedule: \"${{ vars.CRON }} 4 * * *\"", wantError: "social.live.schedule must be a valid 5-field cron expression"},
		{name: "live schedule invalid token", body: "social:\n  live:\n    schedule: \"0 4 * * ;rm\"", wantError: "social.live.schedule contains invalid cron field"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := parseAndValidate([]byte("license: MIT\n"+tt.body+"\n"), "test")
			if tt.wantError == "" {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				return
			}
			if err == nil {
				t.Fatalf("expected error containing %q, got nil", tt.wantError)
			}
			if !strings.Contains(err.Error(), tt.wantError) {
				t.Fatalf("error = %q, want substring %q", err.Error(), tt.wantError)
			}
		})
	}
}

func TestSocialSponsorsBoolNormalisation(t *testing.T) {
	tests := []struct {
		name string
		raw  string
		want string
	}{
		{name: "bool true", raw: "true", want: "true"},
		{name: "bool false", raw: "false", want: "false"},
		{name: "bool True", raw: "True", want: "true"},
		{name: "bool FALSE", raw: "FALSE", want: "false"},
		{name: "string auto", raw: "auto", want: "auto"},
		{name: "quoted true", raw: `"true"`, want: "true"},
		{name: "quoted false", raw: `"false"`, want: "false"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg, err := parseAndValidate([]byte("license: MIT\nsocial:\n  readme:\n    sponsors: "+tt.raw+"\n"), "test")
			if err != nil {
				t.Fatalf("parseAndValidate() error = %v", err)
			}
			if cfg.EffectiveSocialSponsors() != tt.want {
				t.Fatalf("EffectiveSocialSponsors() = %q, want %q", cfg.EffectiveSocialSponsors(), tt.want)
			}
		})
	}
}

func TestSocialAccessorsAndDefaults(t *testing.T) {
	var nilCfg *Config
	if nilCfg.SocialDeclared() || nilCfg.SocialEnabled() {
		t.Fatal("nil Config reported social declared or enabled")
	}
	if nilCfg.EffectiveSocialTheme() != "dark" ||
		nilCfg.EffectiveSocialAccent() != "" ||
		nilCfg.EffectiveSocialBackground() != "" ||
		nilCfg.EffectiveSocialPlatforms() != "" ||
		nilCfg.EffectiveSocialOGOutput() != "pages/og.png" ||
		nilCfg.EffectiveSocialOGIncludePreview() ||
		nilCfg.EffectiveSocialOGImage() != "" ||
		nilCfg.EffectiveSocialOGTerminal() != nil ||
		!nilCfg.EffectiveSocialReadmeHeader() ||
		nilCfg.EffectiveSocialPreferredLinks() != nil ||
		nilCfg.EffectiveSocialCommunity() != "auto" ||
		nilCfg.EffectiveSocialPackages() != nil ||
		!nilCfg.EffectiveSocialDemo() ||
		!nilCfg.EffectiveSocialReadmeStats() ||
		!nilCfg.EffectiveSocialStarHistoryEnabled() ||
		nilCfg.EffectiveSocialStarHistoryMinStars() != 25 ||
		nilCfg.EffectiveSocialStarHistoryMinDays() != 30 ||
		nilCfg.EffectiveSocialSponsors() != "auto" ||
		!nilCfg.EffectiveSocialContributors() ||
		!nilCfg.EffectiveSocialFooter() ||
		nilCfg.EffectiveSocialLiveBranch() != "social-assets" ||
		nilCfg.EffectiveSocialLiveSchedule() != "0 4 * * *" {
		t.Fatal("nil Config returned unexpected social defaults")
	}

	tests := []struct {
		name     string
		social   *model.SocialSettings
		declared bool
		enabled  bool
	}{
		{name: "absent", social: nil, declared: false, enabled: false},
		{name: "empty", social: &model.SocialSettings{}, declared: false, enabled: false},
		{name: "false", social: &model.SocialSettings{Enabled: new(false)}, declared: true, enabled: false},
		{name: "true", social: &model.SocialSettings{Enabled: new(true)}, declared: true, enabled: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &Config{Social: tt.social}
			if cfg.SocialDeclared() != tt.declared || cfg.SocialEnabled() != tt.enabled {
				t.Fatalf("declared=%t enabled=%t, want declared=%t enabled=%t",
					cfg.SocialDeclared(), cfg.SocialEnabled(), tt.declared, tt.enabled)
			}
		})
	}

	explicit := &Config{
		Social: &model.SocialSettings{
			Enabled:    new(true),
			Theme:      new("light"),
			Accent:     new("#1e66f5"),
			Background: new("#eff1f5"),
			Platforms:  new("Linux · macOS"),
			OG: &model.SocialOGSettings{
				Output:         new("docs/card.png"),
				IncludePreview: new(true),
				Image:          new("docs/preview.png"),
				Terminal:       &[]string{"$ tailor"},
			},
			Readme: &model.SocialReadmeSettings{
				Header:         new(false),
				PreferredLinks: &[]string{"website", "irc"},
				Community:      new("irc"),
				Packages:       &[]string{"apt", "nix"},
				Demo:           new(false),
				Stats:          new(false),
				StarHistory: &model.SocialStarHistorySettings{
					Enabled:  new(false),
					MinStars: new(0),
					MinDays:  new(0),
				},
				Sponsors:     new("false"),
				Contributors: new(false),
				Footer:       new(false),
			},
			Live: &model.SocialLiveSettings{
				Branch:   new("live-assets"),
				Schedule: new("30 2 * * 1"),
			},
		},
	}

	if explicit.EffectiveSocialTheme() != "light" ||
		explicit.EffectiveSocialAccent() != "#1e66f5" ||
		explicit.EffectiveSocialBackground() != "#eff1f5" ||
		explicit.EffectiveSocialPlatforms() != "Linux · macOS" ||
		explicit.EffectiveSocialOGOutput() != "docs/card.png" ||
		!explicit.EffectiveSocialOGIncludePreview() ||
		explicit.EffectiveSocialOGImage() != "docs/preview.png" ||
		!reflect.DeepEqual(explicit.EffectiveSocialOGTerminal(), []string{"$ tailor"}) ||
		explicit.EffectiveSocialReadmeHeader() ||
		!reflect.DeepEqual(explicit.EffectiveSocialPreferredLinks(), []string{"website", "irc"}) ||
		explicit.EffectiveSocialCommunity() != "irc" ||
		!reflect.DeepEqual(explicit.EffectiveSocialPackages(), []string{"apt", "nix"}) ||
		explicit.EffectiveSocialDemo() ||
		explicit.EffectiveSocialReadmeStats() ||
		explicit.EffectiveSocialStarHistoryEnabled() ||
		explicit.EffectiveSocialStarHistoryMinStars() != 0 ||
		explicit.EffectiveSocialStarHistoryMinDays() != 0 ||
		explicit.EffectiveSocialSponsors() != "false" ||
		explicit.EffectiveSocialContributors() ||
		explicit.EffectiveSocialFooter() ||
		explicit.EffectiveSocialLiveBranch() != "live-assets" ||
		explicit.EffectiveSocialLiveSchedule() != "30 2 * * 1" {
		t.Fatal("explicit social values were not returned by effective helpers")
	}

	// Verify slice helpers clone rather than aliasing internal state.
	links := explicit.EffectiveSocialPreferredLinks()
	links[0] = "mutated"
	if explicit.EffectiveSocialPreferredLinks()[0] != "website" {
		t.Fatal("EffectiveSocialPreferredLinks returned an uncloned slice")
	}
}

func TestSocialRoundTripWithoutDefaults(t *testing.T) {
	tests := []struct {
		name string
		body string
	}{
		{name: "omitted"},
		{name: "empty", body: "social: {}"},
		{name: "disabled", body: "social:\n  enabled: false"},
		{name: "empty sub-mappings", body: "social:\n  og: {}\n  readme:\n    star_history: {}\n  live: {}"},
		{
			name: "full explicit",
			body: `social:
  enabled: true
  theme: light
  accent: "#1e66f5"
  background: "#eff1f5"
  platforms: "Linux · macOS"
  og:
    output: assets/og.png
    include_preview: true
    image: assets/shot.png
    terminal:
      - "$ tailor alter"
  readme:
    header: false
    preferred_links:
      - website
      - irc
    community: irc
    packages:
      - apt
      - nix
    demo: false
    stats: false
    star_history:
      enabled: false
      min_stars: 0
      min_days: 7
    sponsors: "true"
    contributors: false
    footer: false
  live:
    branch: live-social
    schedule: "15 3 * * *"`,
		},
	}

	for _, tt := range tests {
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
			if !reflect.DeepEqual(loaded.Social, cfg.Social) {
				t.Fatalf("round trip changed Social:\ngot:  %+v\nwant: %+v", loaded.Social, cfg.Social)
			}
		})
	}
}

func TestSocialMergeThreeStateAndConvergence(t *testing.T) {
	// Absent social section must stay nil on merge.
	absent := &Config{License: "MIT"}
	if _, err := MergeDefaults(absent); err != nil {
		t.Fatal(err)
	}
	if absent.Social != nil {
		t.Fatalf("MergeDefaults inserted absent social section: %+v", absent.Social)
	}

	// Empty social mapping must keep enabled nil while filling defaults and converging.
	empty := &Config{License: "MIT", Social: &model.SocialSettings{}}
	changed, err := MergeDefaults(empty)
	if err != nil {
		t.Fatal(err)
	}
	if !changed {
		t.Fatal("MergeDefaults() changed = false, want true for empty social section")
	}
	if empty.Social.Enabled != nil {
		t.Fatalf("MergeDefaults set omitted social.enabled to %v", *empty.Social.Enabled)
	}
	if empty.EffectiveSocialTheme() != "dark" ||
		empty.EffectiveSocialOGOutput() != "pages/og.png" ||
		!empty.EffectiveSocialDemo() ||
		empty.EffectiveSocialStarHistoryMinStars() != 25 ||
		!empty.EffectiveSocialContributors() ||
		!empty.EffectiveSocialFooter() ||
		empty.EffectiveSocialLiveBranch() != "social-assets" {
		t.Fatalf("MergeDefaults did not fill social defaults: %+v", empty.Social)
	}
	dir := t.TempDir()
	if err := Write(dir, empty, "2026-09-12", "Refitted"); err != nil {
		t.Fatal(err)
	}
	reloaded, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if reloaded.Social.Enabled != nil {
		t.Fatalf("reloaded social.enabled = %v, want nil", *reloaded.Social.Enabled)
	}
	if changed, err := MergeDefaults(reloaded); err != nil || changed {
		t.Fatalf("second MergeDefaults = %t, %v; want false, nil", changed, err)
	}

	// Explicit enabled: true with partial overrides keeps overrides and fills missing defaults.
	partial, err := parseAndValidate([]byte(`license: MIT
social:
  enabled: true
  theme: light
  readme:
    star_history:
      min_stars: 0
`), "test")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := MergeDefaults(partial); err != nil {
		t.Fatal(err)
	}
	if !partial.SocialEnabled() ||
		partial.EffectiveSocialTheme() != "light" ||
		partial.EffectiveSocialStarHistoryMinStars() != 0 ||
		partial.EffectiveSocialStarHistoryMinDays() != 30 ||
		partial.EffectiveSocialOGOutput() != "pages/og.png" ||
		partial.EffectiveSocialLiveSchedule() != "0 4 * * *" {
		t.Fatalf("partial social merge unexpected state: %+v", partial.Social)
	}
}
