package social

import (
	"bytes"
	"encoding/xml"
	"errors"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/wimpysworld/tailor/internal/model"
)

var (
	svgXMLNSAttrRe  = regexp.MustCompile(`xmlns(?::[a-z]+)?="http://www\.w3\.org/(?:2000/svg|1999/xlink)"`)
	svgDataURIRe    = regexp.MustCompile(`(?i)data:image/(?:png|jpeg|jpg|webp|gif|avif);base64,[A-Za-z0-9+/=]+`)
	svgEventAttrRe  = regexp.MustCompile(`(?i)\bon[a-z]+\s*=`)
	svgExternalRe   = regexp.MustCompile(`(?i)(?:https?://|//|javascript:)`)
	svgWidthAttrRe  = regexp.MustCompile(`\bwidth="(\d+)"`)
	svgHeightAttrRe = regexp.MustCompile(`\bheight="(\d+)"`)
)

func iaGetTheme() Theme {
	return Theme{
		DefaultMode: "dark",
		Dark: Palette{
			Background:        "#1a1410",
			CardBackground:    "#241c16",
			CodeBackground:    "#201913",
			Color:             "#ede4d8",
			MutedColor:        "#a89c8e",
			MutedBorder:       "#382d24",
			Primary:           "#f2c96b",
			PrimaryBackground: "#d9a742",
			HeadingColor:      "#fff8ee",
		},
		Light: Palette{
			Background:        "#fbf7f0",
			CardBackground:    "#ffffff",
			CodeBackground:    "#f2ebe0",
			Color:             "#2c241d",
			MutedColor:        "#6b5d50",
			MutedBorder:       "#dfd5c7",
			Primary:           "#8c5a00",
			PrimaryBackground: "#784c00",
			HeadingColor:      "#1a1410",
		},
	}
}

func assertValidGitHubSVG(t *testing.T, label, svg string) {
	t.Helper()

	if strings.TrimSpace(svg) == "" {
		t.Fatalf("%s: empty SVG", label)
	}

	// Verify strict well-formed XML and root element.
	dec := xml.NewDecoder(strings.NewReader(svg))
	dec.Strict = true
	var rootName string
	for {
		tok, err := dec.Token()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			t.Fatalf("%s: invalid XML: %v\nsvg:\n%s", label, err, svg)
		}
		if se, ok := tok.(xml.StartElement); ok {
			if rootName == "" {
				rootName = se.Name.Local
			}
			local := strings.ToLower(se.Name.Local)
			switch local {
			case "script", "foreignobject", "a":
				t.Errorf("%s: forbidden element <%s> found in SVG", label, se.Name.Local)
			}
			for _, attr := range se.Attr {
				if strings.HasPrefix(strings.ToLower(attr.Name.Local), "on") {
					t.Errorf("%s: forbidden event handler attribute %q found", label, attr.Name.Local)
				}
			}
		}
	}
	if rootName != "svg" {
		t.Errorf("%s: root element = %q, want %q", label, rootName, "svg")
	}

	if !strings.Contains(svg, "@media (prefers-reduced-motion: reduce)") {
		t.Errorf("%s: missing prefers-reduced-motion rule", label)
	}
	lower := strings.ToLower(svg)
	for _, forbidden := range []string{"<script", "<foreignobject", ":hover", "@font-face", "@import"} {
		if strings.Contains(lower, forbidden) {
			t.Errorf("%s: forbidden pattern %q found in SVG", label, forbidden)
		}
	}
	if svgEventAttrRe.MatchString(svg) {
		t.Errorf("%s: event handler attribute found in SVG", label)
	}

	withoutNS := svgXMLNSAttrRe.ReplaceAllString(svg, "")
	withoutData := svgDataURIRe.ReplaceAllString(withoutNS, "")
	if match := svgExternalRe.FindString(withoutData); match != "" {
		t.Errorf("%s: unexpected external URL or scheme %q inside SVG markup", label, match)
	}
}

func assertSVGDimensions(t *testing.T, label, svg string, wantW, wantH int) {
	t.Helper()
	// Check root opening tag only.
	end := strings.IndexByte(svg, '>')
	if end < 0 {
		t.Fatalf("%s: malformed root tag", label)
	}
	openTag := svg[:end+1]
	wm := svgWidthAttrRe.FindStringSubmatch(openTag)
	hm := svgHeightAttrRe.FindStringSubmatch(openTag)
	if len(wm) != 2 || len(hm) != 2 {
		t.Fatalf("%s: missing width/height on root <svg>: %s", label, openTag)
	}
	gotW, _ := strconv.Atoi(wm[1])
	gotH, _ := strconv.Atoi(hm[1])
	if gotW != wantW || gotH != wantH {
		t.Errorf("%s: dimensions = %dx%d, want %dx%d", label, gotW, gotH, wantW, wantH)
	}
}

func TestIconsCoverage(t *testing.T) {
	for _, link := range model.PagesLinkTypes {
		info, ok := LookupLinkIcon(link.Key)
		if !ok {
			t.Errorf("LookupLinkIcon(%q) missing for PagesLinkTypes entry", link.Key)
			continue
		}
		if info.Path == "" {
			t.Errorf("LookupLinkIcon(%q) has empty path", link.Key)
		}
		if info.Label == "" {
			t.Errorf("LookupLinkIcon(%q) has empty label", link.Key)
		}
	}

	for _, extra := range []string{"discussions", "github"} {
		info, ok := LookupLinkIcon(extra)
		if !ok || info.Path == "" {
			t.Errorf("LookupLinkIcon(%q) missing or empty", extra)
		}
	}

	for _, host := range model.SocialPackages {
		info, ok := LookupPackageHost(host)
		if !ok {
			t.Errorf("LookupPackageHost(%q) missing for SocialPackages entry", host)
			continue
		}
		wantDomain := host + ".wimpys.world"
		if info.Domain != wantDomain {
			t.Errorf("LookupPackageHost(%q).Domain = %q, want %q", host, info.Domain, wantDomain)
		}
		if info.Path == "" || info.Label == "" || info.AccentDark == "" || info.AccentLight == "" {
			t.Errorf("LookupPackageHost(%q) incomplete metadata: %+v", host, info)
		}
	}
}

func TestSanitizeInlineSVG(t *testing.T) {
	t.Run("strips unsafe elements attributes urls and namespaces ids", func(t *testing.T) {
		raw := []byte(`<?xml version="1.0" encoding="UTF-8"?>
<svg xmlns="http://www.w3.org/2000/svg" xmlns:xlink="http://www.w3.org/1999/xlink" viewBox="0 0 64 64" onload="alert(1)">
  <style>
    @import url("https://evil.example/a.css");
    @font-face { font-family: Bad; src: url(https://evil.example/f.woff2); }
    #badge { fill: url(#grad); }
    #badge:hover { fill: red; }
    .ext { background: url(https://evil.example/bg.png); }
  </style>
  <script>alert("xss")</script>
  <foreignObject width="64" height="64"><div xmlns="http://www.w3.org/1999/xhtml">bad</div></foreignObject>
  <defs>
    <linearGradient id="grad"><stop offset="0%" stop-color="#f2c96b"/></linearGradient>
    <clipPath id="clip"><rect width="64" height="64"/></clipPath>
  </defs>
  <a href="https://evil.example/click" onclick="evil()">
    <rect id="badge" width="64" height="64" fill="url(#grad)" clip-path="url(#clip)" style="stroke: #fff; background: url(https://evil.example/x)"/>
  </a>
  <use xlink:href="#badge" onclick="bad()"/>
  <use href="https://evil.example/other.svg#x"/>
  <image href="https://evil.example/pixel.png" width="10" height="10"/>
</svg>`)

		got, err := SanitizeInlineSVG(raw, 72)
		if err != nil {
			t.Fatalf("SanitizeInlineSVG() error = %v", err)
		}

		// Must be valid XML on its own when wrapped in a host SVG.
		host := `<svg xmlns="http://www.w3.org/2000/svg" width="100" height="100"><style>@media (prefers-reduced-motion: reduce) { * { animation: none !important; } }</style>` + got + `</svg>`
		assertValidGitHubSVG(t, "sanitized-inline", host)

		if !strings.HasPrefix(got, `<svg width="72" height="72" viewBox="0 0 64 64" preserveAspectRatio="xMidYMid meet"`) {
			t.Errorf("unexpected root wrapper: %s", got)
		}
		if strings.Contains(got, `id="grad"`) || strings.Contains(got, `id="badge"`) {
			t.Errorf("un-namespaced id remained in output: %s", got)
		}
		if strings.Contains(got, `url(#grad)`) || strings.Contains(got, `href="#badge"`) {
			t.Errorf("un-namespaced reference remained in output: %s", got)
		}
		if !strings.Contains(got, `fill="url(#i`) || !strings.Contains(got, `href="#i`) {
			t.Errorf("expected SHA-256 prefixed references in output: %s", got)
		}
		if strings.Contains(got, "<image") {
			t.Errorf("external <image> should have been removed: %s", got)
		}
	})

	t.Run("synthesizes viewBox from width and height", func(t *testing.T) {
		raw := []byte(`<svg xmlns="http://www.w3.org/2000/svg" width="128px" height="64px"><circle cx="32" cy="32" r="20" fill="#fff"/></svg>`)
		got, err := SanitizeInlineSVG(raw, 72)
		if err != nil {
			t.Fatalf("SanitizeInlineSVG() error = %v", err)
		}
		if !strings.Contains(got, `viewBox="0 0 128 64"`) {
			t.Errorf("expected synthesized viewBox, got: %s", got)
		}
	})

	t.Run("sanitizes repository pages/icon.svg", func(t *testing.T) {
		raw, err := os.ReadFile(filepath.Join("..", "..", "pages", "icon.svg"))
		if err != nil {
			t.Fatalf("reading repo pages/icon.svg: %v", err)
		}
		got, err := SanitizeInlineSVG(raw, 72)
		if err != nil {
			t.Fatalf("SanitizeInlineSVG(pages/icon.svg) error = %v", err)
		}
		host := `<svg xmlns="http://www.w3.org/2000/svg" width="80" height="80"><style>@media (prefers-reduced-motion: reduce) { * { animation: none !important; } }</style>` + got + `</svg>`
		assertValidGitHubSVG(t, "repo-icon", host)
		if strings.Contains(got, `id="shears"`) || strings.Contains(got, `id="tailor-title"`) {
			t.Errorf("expected namespaced ids for pages/icon.svg, got: %s", got)
		}
	})

	t.Run("rejects invalid inputs", func(t *testing.T) {
		cases := []struct {
			name string
			raw  []byte
			size int
		}{
			{"empty", nil, 72},
			{"zero size", []byte(`<svg/>`), 0},
			{"malformed xml", []byte(`<svg><path></svg>`), 72},
			{"non-svg root", []byte(`<html><body>hi</body></html>`), 72},
		}
		for _, tc := range cases {
			if _, err := SanitizeInlineSVG(tc.raw, tc.size); err == nil {
				t.Errorf("%s: expected error, got nil", tc.name)
			}
		}
	})
}

func TestEmbedRasterIcon(t *testing.T) {
	pngBytes := []byte{0x89, 'P', 'N', 'G', 0x0d, 0x0a, 0x1a, 0x0a, 0x00, 0x01}
	got := EmbedRasterIcon(pngBytes, "image/png", 72)
	if !strings.HasPrefix(got, `<image href="data:image/png;base64,`) {
		t.Errorf("unexpected raster embed: %s", got)
	}
	if !strings.Contains(got, `width="72" height="72" preserveAspectRatio="xMidYMid meet"/>`) {
		t.Errorf("missing dimensions/preserveAspectRatio: %s", got)
	}

	// Unsafe MIME falls back to safe detected/PNG MIME.
	safe := EmbedRasterIcon(pngBytes, "image/svg+xml", 48)
	if strings.Contains(safe, "svg+xml") || !strings.Contains(safe, "data:image/png;base64,") {
		t.Errorf("expected unsafe MIME fallback to image/png, got: %s", safe)
	}

	if EmbedRasterIcon(nil, "image/png", 72) != "" {
		t.Error("expected empty string for nil bytes")
	}
	if EmbedRasterIcon(pngBytes, "image/png", 0) != "" {
		t.Error("expected empty string for zero size")
	}
}

func TestXMLEscapeAndWrapText(t *testing.T) {
	escaped := XMLEscape("A & B <tag> \"q\" 's'\x00\x07")
	want := "A &amp; B &lt;tag&gt; &quot;q&quot; &#39;s&#39;"
	if escaped != want {
		t.Errorf("XMLEscape() = %q, want %q", escaped, want)
	}

	lines := WrapText("Ready-to-wear project templates for GitHub repositories with calm defaults and zero fuss", 35, 2)
	if len(lines) != 2 {
		t.Fatalf("WrapText() returned %d lines (%v), want 2", len(lines), lines)
	}
	if !strings.HasSuffix(lines[1], "…") {
		t.Errorf("expected truncated last line to end with ellipsis, got %q", lines[1])
	}
	for i, line := range lines {
		if len([]rune(line)) > 35 {
			t.Errorf("line %d exceeds maxChars 35: %q", i, line)
		}
	}

	if got := WrapText("   ", 20, 2); got != nil {
		t.Errorf("expected nil for blank input, got %v", got)
	}
	if got := WrapText("hello", 20, 0); got != nil {
		t.Errorf("expected nil for maxLines=0, got %v", got)
	}
}

func TestRenderHero(t *testing.T) {
	iconSVG := []byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 64 64"><circle id="dot" cx="32" cy="32" r="24" fill="#f2c96b"/></svg>`)
	pngBytes := []byte{0x89, 'P', 'N', 'G', 0x0d, 0x0a, 0x1a, 0x0a, 0x00, 0x01, 0x02, 0x03}

	tests := []struct {
		name  string
		in    HeroInput
		theme Theme
	}{
		{
			name: "with inline SVG icon and ia-get theme",
			in: HeroInput{
				Assets: ProjectAssets{
					Title:     "ia-get",
					Tagline:   "Download Internet Archive items cleanly & reliably from the command line",
					IconKind:  IconKindSVG,
					FooterURL: "https://ia-get.wimpys.world",
					Platforms: "Linux · macOS · Windows",
				},
				IconData: iconSVG,
			},
			theme: iaGetTheme(),
		},
		{
			name: "with raster PNG icon and default theme",
			in: HeroInput{
				Assets: ProjectAssets{
					Title:     "tailor",
					Tagline:   "Ready-to-wear project templates for GitHub repositories",
					IconKind:  IconKindPNG,
					FooterURL: "https://tailor.wimpys.world",
				},
				IconData: pngBytes,
				IconMIME: "image/png",
			},
			theme: DefaultTheme(),
		},
		{
			name: "without icon",
			in: HeroInput{
				Assets: ProjectAssets{
					Title:     "minimal",
					Tagline:   "No icon provided",
					FooterURL: "https://github.com/wimpysworld/minimal",
				},
			},
			theme: DefaultTheme(),
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			pair, err := RenderHero(tc.in, tc.theme)
			if err != nil {
				t.Fatalf("RenderHero() error = %v", err)
			}
			assertValidGitHubSVG(t, tc.name+"-dark", pair.Dark)
			assertValidGitHubSVG(t, tc.name+"-light", pair.Light)
			assertSVGDimensions(t, tc.name+"-dark", pair.Dark, 840, 210)
			assertSVGDimensions(t, tc.name+"-light", pair.Light, 840, 210)

			if !strings.Contains(pair.Dark, tc.theme.Dark.Primary) {
				t.Errorf("dark hero missing Dark.Primary %s", tc.theme.Dark.Primary)
			}
			if !strings.Contains(pair.Dark, tc.theme.Dark.Background) {
				t.Errorf("dark hero missing Dark.Background %s", tc.theme.Dark.Background)
			}
			if !strings.Contains(pair.Light, tc.theme.Light.Primary) {
				t.Errorf("light hero missing Light.Primary %s", tc.theme.Light.Primary)
			}
			if !strings.Contains(pair.Light, tc.theme.Light.Background) {
				t.Errorf("light hero missing Light.Background %s", tc.theme.Light.Background)
			}
			for _, wantWave := range []string{
				"@keyframes wave-flow",
				"translateX(-840px)",
				"18s linear infinite",
				"28s linear infinite",
				"L 1680 220 L 0 220 Z",
			} {
				if !strings.Contains(pair.Dark, wantWave) {
					t.Errorf("dark hero missing seamless wave markup %q", wantWave)
				}
			}
			if strings.Contains(pair.Dark, "<line ") || strings.Contains(pair.Light, "<line ") {
				t.Error("hero banner must not render horizontal divider line above footer")
			}
		})
	}
}

func TestRenderLinkPill(t *testing.T) {
	theme := iaGetTheme()

	keys := make([]string, 0, len(model.PagesLinkTypes)+2)
	for _, link := range model.PagesLinkTypes {
		keys = append(keys, link.Key)
	}
	keys = append(keys, "discussions", "github")

	for _, key := range keys {
		t.Run(key, func(t *testing.T) {
			pair, err := RenderLinkPill(LinkPillInput{Key: key}, theme)
			if err != nil {
				t.Fatalf("RenderLinkPill(%q) error = %v", key, err)
			}
			assertValidGitHubSVG(t, "pill-"+key+"-dark", pair.Dark)
			assertValidGitHubSVG(t, "pill-"+key+"-light", pair.Light)
			assertSVGDimensions(t, "pill-"+key+"-dark", pair.Dark, 184, 44)
			assertSVGDimensions(t, "pill-"+key+"-light", pair.Light, 184, 44)
			if !strings.Contains(pair.Dark, theme.Dark.CardBackground) {
				t.Errorf("dark pill missing CardBackground %s", theme.Dark.CardBackground)
			}
			if !strings.Contains(pair.Light, theme.Light.CardBackground) {
				t.Errorf("light pill missing CardBackground %s", theme.Light.CardBackground)
			}
		})
	}

	if _, err := RenderLinkPill(LinkPillInput{Key: "nonexistent"}, theme); err == nil {
		t.Error("expected error for unknown link pill key")
	}
}

func TestRenderCommunityBanner(t *testing.T) {
	theme := iaGetTheme()

	tests := []struct {
		name     string
		in       CommunityBannerInput
		wantText string
	}{
		{
			name:     "discord with live counts",
			in:       CommunityBannerInput{Platform: "discord", OnlineCount: 42, MemberCount: 1280},
			wantText: "42 online · 1,280 members",
		},
		{
			name:     "matrix static",
			in:       CommunityBannerInput{Platform: "matrix"},
			wantText: "Join the Matrix room",
		},
		{
			name:     "slack with member count",
			in:       CommunityBannerInput{Platform: "slack", MemberCount: 310},
			wantText: "310 members",
		},
		{
			name:     "irc static",
			in:       CommunityBannerInput{Platform: "irc", Subtitle: "#tailor on Libera.Chat"},
			wantText: "#tailor on Libera.Chat",
		},
		{
			name:     "forum static",
			in:       CommunityBannerInput{Platform: "forum"},
			wantText: "Join the community forum",
		},
		{
			name:     "discussions with count",
			in:       CommunityBannerInput{Platform: "discussions", DiscussionCount: 18},
			wantText: "18 discussions",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			pair, err := RenderCommunityBanner(tc.in, theme)
			if err != nil {
				t.Fatalf("RenderCommunityBanner() error = %v", err)
			}
			assertValidGitHubSVG(t, tc.name+"-dark", pair.Dark)
			assertValidGitHubSVG(t, tc.name+"-light", pair.Light)
			assertSVGDimensions(t, tc.name+"-dark", pair.Dark, 840, 68)
			assertSVGDimensions(t, tc.name+"-light", pair.Light, 840, 68)
			if !strings.Contains(pair.Dark, tc.wantText) {
				t.Errorf("dark community banner missing %q", tc.wantText)
			}
			if !strings.Contains(pair.Dark, "JOIN") {
				t.Errorf("dark community banner missing JOIN pill")
			}
			if !strings.Contains(pair.Dark, theme.Dark.Primary) || !strings.Contains(pair.Light, theme.Light.Primary) {
				t.Errorf("community banner did not reflect dark/light Primary colours")
			}
			if strings.Contains(pair.Dark, `width="4" height="68"`) || strings.Contains(pair.Light, `width="4" height="68"`) {
				t.Error("community banner must not render leftmost vertical accent bar")
			}
		})
	}

	if _, err := RenderCommunityBanner(CommunityBannerInput{Platform: "carrier-pigeon"}, theme); err == nil {
		t.Error("expected error for unsupported community platform")
	}
}

func TestRenderPackageCard(t *testing.T) {
	theme := iaGetTheme()

	for _, host := range model.SocialPackages {
		t.Run(host, func(t *testing.T) {
			pair, err := RenderPackageCard(host, theme)
			if err != nil {
				t.Fatalf("RenderPackageCard(%q) error = %v", host, err)
			}
			assertValidGitHubSVG(t, "pkg-"+host+"-dark", pair.Dark)
			assertValidGitHubSVG(t, "pkg-"+host+"-light", pair.Light)
			assertSVGDimensions(t, "pkg-"+host+"-dark", pair.Dark, 198, 78)
			assertSVGDimensions(t, "pkg-"+host+"-light", pair.Light, 198, 78)

			info, _ := LookupPackageHost(host)
			if !strings.Contains(pair.Dark, info.Domain) || !strings.Contains(pair.Dark, info.Label) {
				t.Errorf("dark package card missing domain %q or label %q", info.Domain, info.Label)
			}
			if !strings.Contains(pair.Dark, info.AccentDark) {
				t.Errorf("dark package card missing AccentDark %s", info.AccentDark)
			}
			if !strings.Contains(pair.Light, info.AccentLight) {
				t.Errorf("light package card missing AccentLight %s", info.AccentLight)
			}
		})
	}

	if _, err := RenderPackageCard("snap", theme); err == nil {
		t.Error("expected error for unknown package host")
	}
}

func TestRenderStats(t *testing.T) {
	theme := iaGetTheme()
	pair, err := RenderStats(StatsInput{
		Commits:      750,
		OpenIssues:   2,
		ClosedIssues: 41,
		OpenPRs:      0,
		MergedPRs:    180,
		CreatedAt:    time.Date(2026, 3, 10, 0, 0, 0, 0, time.UTC),
	}, theme)
	if err != nil {
		t.Fatalf("RenderStats() error = %v", err)
	}

	assertValidGitHubSVG(t, "stats-dark", pair.Dark)
	assertValidGitHubSVG(t, "stats-light", pair.Light)
	assertSVGDimensions(t, "stats-dark", pair.Dark, 840, 98)
	assertSVGDimensions(t, "stats-light", pair.Light, 840, 98)

	for _, want := range []string{
		"COMMITS", "ISSUES", "PULL REQUESTS", "ESTABLISHED",
		"750", "2 open · 41 closed", "180 merged · 0 open", "Mar 2026",
	} {
		if !strings.Contains(pair.Dark, want) {
			t.Errorf("dark stats SVG missing %q", want)
		}
	}
	if strings.Count(pair.Dark, `<line x1="`) != 3 {
		t.Errorf("expected 3 column dividers for 4-column health strip, got %d", strings.Count(pair.Dark, `<line x1="`))
	}
	if !strings.Contains(pair.Dark, theme.Dark.Primary) || !strings.Contains(pair.Light, theme.Light.Primary) {
		t.Error("stats SVG did not apply dark/light theme Primary colours")
	}

	zeroPair, err := RenderStats(StatsInput{}, theme)
	if err != nil {
		t.Fatalf("RenderStats(zero) error = %v", err)
	}
	assertValidGitHubSVG(t, "stats-zero-dark", zeroPair.Dark)
	if !strings.Contains(zeroPair.Dark, "—") {
		t.Error("zero CreatedAt should render em dash fallback")
	}
}

func TestRenderStarHistory(t *testing.T) {
	theme := iaGetTheme()
	base := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)

	t.Run("gating thresholds fall back to 840x108 Star CTA banner", func(t *testing.T) {
		cases := []struct {
			name      string
			in        StarHistoryInput
			wantChart bool
			wantTitle string
		}{
			{
				name:      "empty points renders Star CTA",
				in:        StarHistoryInput{MinStars: 25, MinDays: 30, ProjectTitle: "Tailor"},
				wantChart: false,
				wantTitle: "Star Tailor on GitHub",
			},
			{
				name: "below min_stars renders Star CTA",
				in: StarHistoryInput{
					Points: []StarPoint{
						{Date: base, Stars: 1},
						{Date: base.AddDate(0, 3, 0), Stars: 24},
					},
					MinStars:     25,
					MinDays:      30,
					ProjectTitle: "Tailor",
				},
				wantChart: false,
				wantTitle: "Star Tailor on GitHub",
			},
			{
				name: "below min_days renders Star CTA with generic title when ProjectTitle empty",
				in: StarHistoryInput{
					Points: []StarPoint{
						{Date: base, Stars: 10},
						{Date: base.AddDate(0, 0, 29), Stars: 100},
					},
					MinStars: 25,
					MinDays:  30,
				},
				wantChart: false,
				wantTitle: "Star this repository on GitHub",
			},
			{
				name: "meets exact min_stars and min_days renders chart",
				in: StarHistoryInput{
					Points: []StarPoint{
						{Date: base, Stars: 1},
						{Date: base.AddDate(0, 0, 30), Stars: 25},
					},
					MinStars: 25,
					MinDays:  30,
				},
				wantChart: true,
			},
			{
				name: "single point allowed when min_days is 0 renders chart",
				in: StarHistoryInput{
					Points:   []StarPoint{{Date: base, Stars: 30}},
					MinStars: 25,
					MinDays:  0,
				},
				wantChart: true,
			},
		}

		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				pair, ok, err := RenderStarHistory(tc.in, theme)
				if err != nil {
					t.Fatalf("RenderStarHistory() error = %v", err)
				}
				if !ok {
					t.Fatalf("RenderStarHistory() ok = false, want true")
				}
				assertValidGitHubSVG(t, tc.name+"-dark", pair.Dark)
				assertValidGitHubSVG(t, tc.name+"-light", pair.Light)
				if tc.wantChart {
					assertSVGDimensions(t, tc.name+"-dark", pair.Dark, 840, 280)
					assertSVGDimensions(t, tc.name+"-light", pair.Light, 840, 280)
					return
				}
				assertSVGDimensions(t, tc.name+"-dark", pair.Dark, 840, 108)
				assertSVGDimensions(t, tc.name+"-light", pair.Light, 840, 108)
				for _, want := range []string{tc.wantTitle, "STAR", "star-drift", theme.Dark.Primary} {
					if !strings.Contains(pair.Dark, want) {
						t.Errorf("dark Star CTA missing %q", want)
					}
				}
				if !strings.Contains(pair.Light, theme.Light.Primary) {
					t.Errorf("light Star CTA missing Light.Primary %s", theme.Light.Primary)
				}
			})
		}
	})

	t.Run("multi-point curve with stroke-dashoffset and theme colours", func(t *testing.T) {
		// Pass intentionally unsorted points to verify defensive sorting.
		pts := []StarPoint{
			{Date: base.AddDate(0, 8, 0), Stars: 410},
			{Date: base, Stars: 5},
			{Date: base.AddDate(0, 4, 0), Stars: 165},
			{Date: base.AddDate(0, 2, 0), Stars: 60},
			{Date: base.AddDate(0, 6, 0), Stars: 290},
		}
		pair, ok, err := RenderStarHistory(StarHistoryInput{
			Points:   pts,
			MinStars: 25,
			MinDays:  30,
		}, theme)
		if err != nil || !ok {
			t.Fatalf("RenderStarHistory() = ok:%v err:%v", ok, err)
		}
		assertValidGitHubSVG(t, "stars-dark", pair.Dark)
		assertValidGitHubSVG(t, "stars-light", pair.Light)
		assertSVGDimensions(t, "stars-dark", pair.Dark, 840, 280)
		assertSVGDimensions(t, "stars-light", pair.Light, 840, 280)

		if !strings.Contains(pair.Dark, "stroke-dashoffset") {
			t.Error("star history SVG missing stroke-dashoffset animation")
		}
		if !strings.Contains(pair.Dark, "410 stars") {
			t.Error("star history SVG missing latest star count badge")
		}
		if !strings.Contains(pair.Dark, theme.Dark.Primary) || !strings.Contains(pair.Light, theme.Light.Primary) {
			t.Error("star history SVG did not reflect dark/light Primary colours")
		}
		for _, want := range []string{
			"12s linear infinite",
			"sh-apex-loop",
			"scale(1.4)",
			"sh-halo-loop",
			`class="sh-halo"`,
			`class="sh-apex"`,
			`class="sh-sparkle sh-sp0"`,
			`class="sh-sparkle sh-sp7"`,
			"#f9e2af",
			"#fab387",
			"#f5c2e7",
			".sh-halo, .sh-sparkle { opacity: 0 !important; }",
		} {
			if !strings.Contains(pair.Dark, want) {
				t.Errorf("dark star history SVG missing %q", want)
			}
		}
		if strings.Count(pair.Dark, `class="sh-sparkle `) != 8 {
			t.Errorf("expected 8 radial burst sparkles, got %d", strings.Count(pair.Dark, `class="sh-sparkle `))
		}
	})
}

func TestRenderSponsors(t *testing.T) {
	theme := iaGetTheme()

	t.Run("renders fixed 840x108 CTA card with multi-coloured drifting hearts", func(t *testing.T) {
		pair, err := RenderSponsors(SponsorsInput{}, theme)
		if err != nil {
			t.Fatalf("RenderSponsors() error = %v", err)
		}
		assertValidGitHubSVG(t, "sponsors-dark", pair.Dark)
		assertValidGitHubSVG(t, "sponsors-light", pair.Light)
		assertSVGDimensions(t, "sponsors-dark", pair.Dark, 840, 108)
		assertSVGDimensions(t, "sponsors-light", pair.Light, 840, 108)

		for _, want := range []string{
			"Sponsors",
			"Support ongoing maintenance and open-source development",
			"SPONSOR",
			"@keyframes sp-rise-",
			"prefers-reduced-motion: reduce",
			theme.Dark.Primary,
		} {
			if !strings.Contains(pair.Dark, want) {
				t.Errorf("dark sponsors SVG missing %q", want)
			}
		}
		if !strings.Contains(pair.Light, theme.Light.Primary) {
			t.Error("light sponsors SVG missing Light.Primary")
		}
		for _, c := range sponsorDarkHeartColors {
			if !strings.Contains(pair.Dark, c) {
				t.Errorf("dark sponsors SVG missing multi-colour heart %s", c)
			}
		}
		for _, c := range sponsorLightHeartColors {
			if !strings.Contains(pair.Light, c) {
				t.Errorf("light sponsors SVG missing multi-colour heart %s", c)
			}
		}
		if strings.Contains(pair.Dark, "Be the first to sponsor") || strings.Contains(pair.Light, "Be the first to sponsor") {
			t.Error("sponsors CTA banner must not render legacy empty footer row")
		}
	})

	t.Run("supports custom title, subtitle, and CTA label at fixed 840x108", func(t *testing.T) {
		pair, err := RenderSponsors(SponsorsInput{
			Title:    "Back Tailor",
			Subtitle: "Help fund ongoing development",
			CTALabel: "BACK US",
		}, theme)
		if err != nil {
			t.Fatalf("RenderSponsors(custom) error = %v", err)
		}
		assertValidGitHubSVG(t, "sponsors-custom-dark", pair.Dark)
		assertValidGitHubSVG(t, "sponsors-custom-light", pair.Light)
		assertSVGDimensions(t, "sponsors-custom-dark", pair.Dark, 840, 108)
		assertSVGDimensions(t, "sponsors-custom-light", pair.Light, 840, 108)
		for _, want := range []string{"Back Tailor", "Help fund ongoing development", "BACK US"} {
			if !strings.Contains(pair.Dark, want) {
				t.Errorf("custom sponsors SVG missing %q", want)
			}
		}
	})
}

func TestRenderContributors(t *testing.T) {
	theme := iaGetTheme()
	avatarURI := "data:image/png;base64,iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mNk+M9QDwADhgGAWjR9awAAAABJRU5ErkJggg=="

	t.Run("renders floating balloon avatars, initials fallback, and +more pill", func(t *testing.T) {
		contributors := []Contributor{
			{Login: "flexiondotorg", AvatarDataURI: avatarURI},
			{Login: "alice-dev"},
			{Login: "bob", AvatarDataURI: "https://evil.example/a.png"}, // unsafe URL falls back to initials
		}
		pair, err := RenderContributors(theme, "Tailor", contributors, 25)
		if err != nil {
			t.Fatalf("RenderContributors() error = %v", err)
		}
		assertValidGitHubSVG(t, "contributors-dark", pair.Dark)
		assertValidGitHubSVG(t, "contributors-light", pair.Light)
		assertSVGDimensions(t, "contributors-dark", pair.Dark, 840, 150)
		assertSVGDimensions(t, "contributors-light", pair.Light, 840, 150)

		for _, want := range []string{
			"Thank you to our contributors",
			"Every commit, issue, and review helps shape Tailor",
			"25 contributors",
			"+22 more",
			`width="160" height="26"`,
			`width="96" height="26"`,
			`width="38" height="38"`,
			`stroke-width="1.5"`,
			"AD",
			"BO",
			"@keyframes cb-rise-",
			"prefers-reduced-motion: reduce",
			"opacity: 1 !important",
		} {
			if !strings.Contains(pair.Dark, want) {
				t.Errorf("dark contributors SVG missing %q", want)
			}
		}
		if !strings.Contains(pair.Dark, theme.Dark.Primary) || !strings.Contains(pair.Light, theme.Light.Primary) {
			t.Error("contributors SVG did not apply dark/light Primary colours")
		}
	})

	t.Run("singular contributor label without +more pill", func(t *testing.T) {
		pair, err := RenderContributors(theme, "Tailor", []Contributor{{Login: "flexiondotorg"}}, 1)
		if err != nil {
			t.Fatalf("RenderContributors(singular) error = %v", err)
		}
		assertValidGitHubSVG(t, "contributors-singular-dark", pair.Dark)
		if !strings.Contains(pair.Dark, "1 contributor") || strings.Contains(pair.Dark, "1 contributors") {
			t.Errorf("expected singular '1 contributor' label")
		}
		if strings.Contains(pair.Dark, " more") {
			t.Error("unexpected +more pill when all contributors are shown")
		}
	})
}

func TestGlintTimingCalmAndStaggered(t *testing.T) {
	theme := iaGetTheme()

	checkGroup := func(t *testing.T, groupName string, keys []string) {
		t.Helper()
		type pairKey struct {
			dur   float64
			delay float64
		}
		seen := make(map[pairKey]string, len(keys))
		for _, k := range keys {
			dur, delay := glintTiming(k)
			if dur < 16.0 || dur > 24.0 {
				t.Errorf("%s[%q]: duration = %.1fs, want within [16, 24]", groupName, k, dur)
			}
			if delay >= 0 || -delay > dur {
				t.Errorf("%s[%q]: delay = %.1fs, want negative within cycle %.1fs", groupName, k, delay, dur)
			}
			// Deterministic check.
			dur2, delay2 := glintTiming(k)
			if dur != dur2 || delay != delay2 {
				t.Errorf("%s[%q]: glintTiming is non-deterministic", groupName, k)
			}
			pk := pairKey{dur: dur, delay: delay}
			if prev, dup := seen[pk]; dup {
				t.Errorf("%s: keys %q and %q share identical glint timing (%.1fs, %.1fs)", groupName, prev, k, dur, delay)
			}
			seen[pk] = k
		}
	}

	checkGroup(t, "links", []string{"bluesky", "mastodon", "linkedin"})
	checkGroup(t, "packages", []string{"apt", "rpm", "pkg", "apk", "brew", "scoop", "nix"})

	// Verify calm resting keyframes (14%,100% invisible rest) appear across hero, pill, community, and package cards.
	hero, err := RenderHero(HeroInput{Assets: ProjectAssets{Title: "Tailor"}}, theme)
	if err != nil {
		t.Fatalf("RenderHero() error = %v", err)
	}
	pill, err := RenderLinkPill(LinkPillInput{Key: "bluesky"}, theme)
	if err != nil {
		t.Fatalf("RenderLinkPill() error = %v", err)
	}
	comm, err := RenderCommunityBanner(CommunityBannerInput{Platform: "discord"}, theme)
	if err != nil {
		t.Fatalf("RenderCommunityBanner() error = %v", err)
	}
	pkg, err := RenderPackageCard("apt", theme)
	if err != nil {
		t.Fatalf("RenderPackageCard() error = %v", err)
	}

	for name, svg := range map[string]string{
		"hero":      hero.Dark,
		"pill":      pill.Dark,
		"community": comm.Dark,
		"package":   pkg.Dark,
	} {
		if !strings.Contains(svg, "14%,100%") {
			t.Errorf("%s SVG missing calm 14%%,100%% rest keyframe", name)
		}
	}
}

func TestRenderHeroRejectsMalformedInlineSVG(t *testing.T) {
	_, err := RenderHero(HeroInput{
		Assets: ProjectAssets{
			Title:    "broken",
			IconKind: IconKindSVG,
		},
		IconData: bytes.Repeat([]byte("<svg><unclosed>"), 1),
	}, DefaultTheme())
	if err == nil {
		t.Fatal("expected error for malformed inline SVG icon")
	}
}
