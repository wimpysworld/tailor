package config

import (
	"reflect"
	"testing"
)

func TestPagesLinksValidation(t *testing.T) {
	for _, tt := range []struct {
		name, body string
		bad        bool
	}{
		{"all", "links: {website: https://example.com, x: https://x.com/user, bluesky: https://bsky.app/profile/example.com, mastodon: https://example.com/@user, discord: https://discord.gg/example, matrix: 'https://matrix.to/#/#room:example.com', slack: https://example.slack.com, linkedin: https://linkedin.com/in/example, youtube: https://youtube.com/@example, instagram: https://instagram.com/example, tiktok: https://tiktok.com/@example, peertube: https://video.example.com, forum: https://forum.example.com, email: user@example.com, feed: https://example.com/feed.xml}", false},
		{"empty", "links: {}", false},
		{"streaming and sponsors", "links: {twitch: https://twitch.tv/example, github_sponsors: https://github.com/sponsors/example, podcast: https://example.com/podcast}", false},
		{"new services", "links: {pixelfed: https://photos.example.com/user, steam: https://steamcommunity.com/id/example, itchio: https://example.itch.io, patreon: https://patreon.com/example, kofi: https://ko-fi.com/example}", false},
		{"empty values", "links: {website: ''}", false},
		{"unknown", "links: {github: https://github.com/user}", true},
		{"null", "links: null", true},
		{"sequence", "links: []", true},
		{"number", "links: {website: 123}", true},
		{"null value", "links: {website: null}", true},
		{"duplicate", "links: {website: '', website: ''}", true},
		{"http", "links: {website: http://example.com}", true},
		{"relative", "links: {website: /about}", true},
		{"credentials", "links: {website: https://user:password@example.com}", true},
		{"script", "links: {website: 'javascript:alert(1)'}", true},
		{"space", "links: {website: 'https://example.com/a b'}", true},
		{"email scheme", "links: {email: 'mailto:user@example.com'}", true},
		{"email name", "links: {email: 'User <user@example.com>'}", true},
		{"email header", "links: {email: 'user@example.com?subject=test'}", true},
		{"email newline", `links: {email: "user@example.com\nBcc:evil@example.com"}`, true},
		{"hugo", "generator: hugo\n  links: {website: https://example.com}", true},
		{"jekyll", "generator: jekyll\n  links: {email: user@example.com}", true},
		{"hugo empty", "generator: hugo\n  links: {website: ''}", false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			_, err := parseAndValidate([]byte("pages:\n  "+tt.body+"\n"), "test")
			if (err != nil) != tt.bad {
				t.Fatalf("error = %v, want error %v", err, tt.bad)
			}
		})
	}
}

func TestPagesLinksPersonalDefaults(t *testing.T) {
	cfg, err := DefaultConfig("MIT")
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Pages.Links == nil || len(*cfg.Pages.Links) != 23 {
		t.Fatal("new configs must list all 23 supported links")
	}
	want := map[string]string{
		"podcast":         "https://linuxmatters.sh",
		"website":         "https://wimpys.world/",
		"bluesky":         "https://bsky.app/profile/wimpys.world",
		"mastodon":        "https://wimpysworld.social/@martin",
		"discord":         "https://discord.com/invite/vUsydfP",
		"matrix":          "https://matrix.to/#/@wimpress:matrix.org",
		"linkedin":        "https://linkedin.com/in/martinwimpress",
		"youtube":         "https://youtube.com/WimpysWorld",
		"twitch":          "https://twitch.tv/WimpysWorld",
		"github_sponsors": "https://github.com/sponsors/flexiondotorg",
		"steam":           "https://steamcommunity.com/id/wimpress/",
		"itchio":          "https://wimpress.itch.io/",
	}
	for key, value := range *cfg.Pages.Links {
		if value != want[key] {
			t.Errorf("default %s = %q, want %q", key, value, want[key])
		}
	}
	for _, generator := range []string{"static", "hugo", "jekyll"} {
		for _, declaration := range []string{"", "  links: {}\n"} {
			existing, err := parseAndValidate([]byte("pages:\n  generator: "+generator+"\n"+declaration), "test")
			if err != nil {
				t.Fatal(err)
			}
			want := existing.Pages.Links
			if _, err := MergeDefaults(existing); err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(existing.Pages.Links, want) {
				t.Fatalf("merge added personal links to existing %s config", generator)
			}
			if err := ValidatePages(existing); err != nil {
				t.Fatal(err)
			}
		}
	}
}

func TestPagesLinksOrderRoundTrip(t *testing.T) {
	cfg, err := parseAndValidate([]byte("pages:\n  links:\n    email: user@example.com\n    x: ''\n    website: https://example.com\n"), "test")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := MergeDefaults(cfg); err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	if err := Write(dir, cfg, "2026-09-09", "Fitted"); err != nil {
		t.Fatal(err)
	}
	loaded, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	var keys []string
	for _, link := range loaded.Pages.OrderedLinks() {
		keys = append(keys, link.Key)
	}
	if !reflect.DeepEqual(keys, []string{"email", "x", "website"}) {
		t.Fatalf("link order changed: %v", keys)
	}
}

func TestPagesLinksMergeRoundTrip(t *testing.T) {
	for _, declaration := range []string{"", "  links: {}\n", "  links: {website: '', email: user@example.com}\n"} {
		cfg, err := parseAndValidate([]byte("pages:\n  enabled: false\n"+declaration), "test")
		if err != nil {
			t.Fatal(err)
		}
		want := cfg.Pages.Links
		if _, err := MergeDefaults(cfg); err != nil {
			t.Fatal(err)
		}
		dir := t.TempDir()
		if err := Write(dir, cfg, "2026-09-09", "Fitted"); err != nil {
			t.Fatal(err)
		}
		loaded, err := Load(dir)
		if err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(loaded.Pages.Links, want) {
			t.Fatalf("links changed: got %v, want %v", loaded.Pages.Links, want)
		}
	}
}
