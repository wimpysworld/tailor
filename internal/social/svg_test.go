package social

import (
	"bytes"
	"encoding/binary"
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
				"@keyframes wave-flow-left",
				"@keyframes wave-flow-right",
				"@keyframes wave-bob-a",
				"@keyframes wave-bob-b",
				"translateX(-840px)",
				"translateY(-4px)",
				"translateY(4px)",
				"translateY(5px)",
				"translateY(-5px)",
				"19s linear infinite",
				"29s linear infinite",
				"7.3s ease-in-out infinite alternate",
				"11.1s ease-in-out infinite alternate",
				`<g class="wave-bob-b"><path class="wave-b"`,
				`<g class="wave-bob-a"><path class="wave-a"`,
				"L 1680 220 L 0 220 Z",
			} {
				if !strings.Contains(pair.Dark, wantWave) {
					t.Errorf("dark hero missing seamless opposite bobbing wave markup %q", wantWave)
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
			if !strings.Contains(pair.Dark, `stroke="`+cardBorder(theme.Dark)+`"`) {
				t.Errorf("dark pill missing lifted border %s", cardBorder(theme.Dark))
			}
			if !strings.Contains(pair.Light, `stroke="`+theme.Light.MutedBorder+`"`) {
				t.Errorf("light pill missing neutral MutedBorder %s", theme.Light.MutedBorder)
			}
			if strings.Contains(pair.Dark, `opacity="0.22"`) || strings.Contains(pair.Light, `opacity="0.22"`) {
				t.Error("link pill must not render permanent static top accent bar")
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
		wantRect string
	}{
		{
			name:     "discord with live counts",
			in:       CommunityBannerInput{Platform: "discord", OnlineCount: 42, MemberCount: 1280},
			wantText: "42 online · 1,280 members",
			wantRect: `x="500" y="20" width="228" height="28" rx="14"`,
		},
		{
			name:     "discord with typical live counts fits without overflow",
			in:       CommunityBannerInput{Platform: "discord", OnlineCount: 186, MemberCount: 2055},
			wantText: "186 online · 2,055 members",
			wantRect: `x="492" y="20" width="236" height="28" rx="14"`,
		},
		{
			name:     "discord with large live counts expands past old 240px cap",
			in:       CommunityBannerInput{Platform: "discord", OnlineCount: 1234, MemberCount: 99999},
			wantText: "1,234 online · 99,999 members",
			wantRect: `x="469" y="20" width="259" height="28" rx="14"`,
		},
		{
			name:     "telegram static",
			in:       CommunityBannerInput{Platform: "telegram"},
			wantText: "Join the Telegram community",
		},
		{
			name:     "telegram with member count",
			in:       CommunityBannerInput{Platform: "telegram", MemberCount: 840},
			wantText: "840 members",
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
			if tc.wantRect != "" && !strings.Contains(pair.Dark, tc.wantRect) {
				t.Errorf("dark community banner missing live pill rect %q", tc.wantRect)
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
			if strings.Contains(pair.Dark, `opacity="0.22"`) || strings.Contains(pair.Light, `opacity="0.22"`) {
				t.Error("community banner must not render permanent static top accent bar")
			}
			if !strings.Contains(pair.Dark, `stroke="`+theme.Dark.MutedBorder+`"`) || !strings.Contains(pair.Light, `stroke="`+theme.Light.MutedBorder+`"`) {
				t.Error("community banner missing neutral MutedBorder")
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
			assertSVGDimensions(t, "pkg-"+host+"-dark", pair.Dark, 200, 78)
			assertSVGDimensions(t, "pkg-"+host+"-light", pair.Light, 200, 78)

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
			if !strings.Contains(pair.Dark, `stroke="`+cardBorder(theme.Dark)+`"`) {
				t.Errorf("dark package card missing lifted border %s", cardBorder(theme.Dark))
			}
			if !strings.Contains(pair.Light, `stroke="`+theme.Light.MutedBorder+`"`) {
				t.Errorf("light package card missing neutral MutedBorder %s", theme.Light.MutedBorder)
			}
			if strings.Contains(pair.Dark, `opacity="0.25"`) || strings.Contains(pair.Light, `opacity="0.25"`) {
				t.Error("package card must not render permanent static top accent bar")
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
		"font-size:14px;font-weight:700",
	} {
		if !strings.Contains(pair.Dark, want) {
			t.Errorf("dark stats SVG missing %q", want)
		}
	}
	if strings.Count(pair.Dark, "font-size:14px;font-weight:700") != 2 {
		t.Errorf("expected both .s-val-sans and .s-val-mono to declare uniform 14px bold font size")
	}
	if strings.Contains(pair.Dark, `font-size="`) {
		t.Error("stats SVG value text must not override uniform font-size via per-element attribute")
	}
	if strings.Count(pair.Dark, `y="67" text-anchor="middle"`) != 4 {
		t.Errorf("expected all 4 metric values to share the identical y=67 baseline")
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
				for _, want := range []string{
					tc.wantTitle,
					"Follow new releases and help others discover the project.",
					"Star on GitHub",
					`x="612" y="32" width="196" height="44" rx="22"`,
					`fill="#facc15" fill-opacity="0.16" stroke="#facc15" stroke-width="1.5"`,
					`stroke="` + cardBorder(theme.Dark) + `"`,
					`id="st-glint"`,
					`class="st-glint"`,
					"@keyframes st-sweep",
					"14%,100%",
					"@keyframes star-twinkle",
					"opacity: 0.85 !important; transform: none !important;",
				} {
					if !strings.Contains(pair.Dark, want) {
						t.Errorf("dark Star CTA missing %q", want)
					}
				}
				for _, want := range []string{
					`fill="#ca8a04" fill-opacity="0.16" stroke="#ca8a04" stroke-width="1.5"`,
					`stroke="` + theme.Light.MutedBorder + `"`,
					`id="st-glint"`,
					`class="st-glint"`,
				} {
					if !strings.Contains(pair.Light, want) {
						t.Errorf("light Star CTA missing %q", want)
					}
				}
				if strings.Count(pair.Dark, `class="st-twinkle `) != 14 {
					t.Errorf("dark Star CTA twinkle count = %d, want 14", strings.Count(pair.Dark, `class="st-twinkle `))
				}
				for _, c := range starCTADarkColors {
					if !strings.Contains(pair.Dark, c) {
						t.Errorf("dark Star CTA missing gold star colour %s", c)
					}
				}
				for _, c := range starCTALightColors {
					if !strings.Contains(pair.Light, c) {
						t.Errorf("light Star CTA missing gold star colour %s", c)
					}
				}
				for _, forbidden := range []string{"keep the project active", "stroke-dasharray", "star-drift"} {
					if strings.Contains(pair.Dark, forbidden) || strings.Contains(pair.Light, forbidden) {
						t.Errorf("Star CTA must not contain legacy markup %q", forbidden)
					}
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
		if !strings.Contains(pair.Dark, `width="111" height="28" rx="14"`) {
			t.Error("star history SVG missing dynamic wider pill width for 410 stars")
		}
		if !strings.Contains(pair.Dark, theme.Dark.Primary) || !strings.Contains(pair.Light, theme.Light.Primary) {
			t.Error("star history SVG did not reflect dark/light Primary colours")
		}
		for _, want := range []string{
			"14s linear infinite",
			"@keyframes sh-line-loop",
			"14.00%,98%{stroke-dashoffset:0;opacity:1}",
			"@keyframes star-race",
			"14.00%,84.00%{transform:translate(",
			"97.50%{transform:translate(-75.0px,-36.0px);opacity:1}",
			"99%,100%{transform:translate(-75.0px,-36.0px);opacity:0}",
			"@keyframes sh-star-spin-loop",
			"cubic-bezier(0.32,0,0.72,0.25)",
			"cubic-bezier(0.42,0,0.85,0.28)",
			"@keyframes sh-collapse-loop",
			"sh-collapse",
			"transform-origin:0px 236.0px",
			"scaleY(0)",
			"cubic-bezier(0.55,0.05,0.9,0.35)",
			"sh-apex-loop",
			"scale(1.65)",
			"scale(0.88)",
			"sh-halo-loop",
			"19%,100%{transform:scale(3.8);opacity:0}",
			`class="sh-halo"`,
			`class="sh-star-pos"`,
			`class="sh-star-spin"`,
			`class="sh-apex"`,
			`class="sh-comet-tail"`,
			`class="sh-comet-sparkles"`,
			"@keyframes sh-comet-tail-loop",
			"0%,88.5%{transform:scaleX(0.05);opacity:0}",
			"97.5%{transform:scaleX(1.05);opacity:0.85}",
			`id="star-comet-glow"`,
			`id="star-comet-core"`,
			`stop-opacity="0.34"`,
			`stop-opacity="0.38"`,
			`d="M-2 0L8 -3.2L128 0L8 3.2Z"`,
			`d="M-1 0L6 -1.3L78 0L6 1.3Z"`,
			`fill="#facc15" stroke="#ca8a04"`,
			`class="sh-sparkle sh-sp0"`,
			`class="sh-sparkle sh-sp13"`,
			`class="sh-trail sh-tr0"`,
			`class="sh-trail sh-tr13"`,
			"#facc15",
			"#fffbeb",
			"#fb923c",
			"#f472b6",
			".sh-collapse { transform: scaleY(1) !important; opacity: 1 !important; }",
			".sh-star-pos {",
			".sh-halo, .sh-sparkle, .sh-comet-tail, .sh-trail { opacity: 0 !important; }",
		} {
			if !strings.Contains(pair.Dark, want) {
				t.Errorf("dark star history SVG missing %q", want)
			}
		}
		if !strings.Contains(pair.Light, `fill="#facc15" stroke="#ca8a04"`) {
			t.Error("light star history SVG must keep fixed yellow star colours regardless of theme")
		}
		if strings.Count(pair.Dark, `class="sh-sparkle `) != 14 {
			t.Errorf("expected 14 arcade fountain sparkles, got %d", strings.Count(pair.Dark, `class="sh-sparkle `))
		}
		if strings.Count(pair.Dark, `class="sh-trail `) != 14 {
			t.Errorf("expected 14 comet trail sparks, got %d", strings.Count(pair.Dark, `class="sh-trail `))
		}
		// Ensure comet wake sparkles paint AFTER the comet tail so they shine in front of the trail.
		tailPos := strings.Index(pair.Dark, `class="sh-comet-tail"`)
		sparklesPos := strings.Index(pair.Dark, `class="sh-comet-sparkles"`)
		if tailPos < 0 || sparklesPos < 0 || sparklesPos <= tailPos {
			t.Errorf("expected sh-comet-sparkles (%d) after sh-comet-tail (%d) in SVG paint order", sparklesPos, tailPos)
		}
		// Ensure star-race and sh-line-loop use purely linear timing between stops (no per-stop cubic-bezier).
		for _, kf := range []string{"@keyframes sh-line-loop{", "@keyframes star-race{"} {
			start := strings.Index(pair.Dark, kf)
			if start < 0 {
				t.Fatalf("missing %s", kf)
			}
			end := strings.Index(pair.Dark[start:], "}}")
			if end < 0 {
				t.Fatalf("unterminated %s", kf)
			}
			body := pair.Dark[start : start+end]
			if strings.Contains(body, "animation-timing-function") {
				t.Errorf("%s must not override linear timing on intermediate stops", kf)
			}
		}
		// Verify Bezier swoop generates 14 stops from 84.0% to 97.5% with monotonic acceleration.
		swoop := buildStarSwoopStops(chartXY{x: 804, y: 80}, 158, -75, -36)
		if len(swoop) != 14 {
			t.Fatalf("buildStarSwoopStops() len = %d, want 14", len(swoop))
		}
		if swoop[0].pct != 84.0 || swoop[len(swoop)-1].pct != 97.5 {
			t.Errorf("swoop range = [%.2f, %.2f], want [84.00, 97.50]", swoop[0].pct, swoop[len(swoop)-1].pct)
		}
		if swoop[len(swoop)-1].x != -75 || swoop[len(swoop)-1].y != -36 {
			t.Errorf("swoop exit = (%.1f, %.1f), want (-75.0, -36.0)", swoop[len(swoop)-1].x, swoop[len(swoop)-1].y)
		}
		// Verify starClimbProgress accelerates strongly then brakes rapidly into the apex.
		if pEarly, pMid, pSplit, pNear := starClimbProgress(0.25), starClimbProgress(0.5), starClimbProgress(0.78), starClimbProgress(0.95); !(0 < pEarly && pEarly < pMid && pMid < pSplit && pSplit < pNear && pNear < 1) {
			t.Errorf("starClimbProgress must be strictly increasing: %.4f, %.4f, %.4f, %.4f", pEarly, pMid, pSplit, pNear)
		}
		if (starClimbProgress(1.0) - starClimbProgress(0.95)) >= (starClimbProgress(0.78) - starClimbProgress(0.73)) {
			t.Error("starClimbProgress should brake rapidly near the apex compared to slope speed")
		}
	})

	t.Run("dynamic star-count pill enforces minimum width and scales for large counts", func(t *testing.T) {
		smallPair, ok, err := RenderStarHistory(StarHistoryInput{
			Points: []StarPoint{
				{Date: base, Stars: 1},
				{Date: base.AddDate(0, 2, 0), Stars: 37},
			},
			MinStars: 25,
			MinDays:  30,
		}, theme)
		if err != nil || !ok {
			t.Fatalf("RenderStarHistory(37) = ok:%v err:%v", ok, err)
		}
		assertValidGitHubSVG(t, "stars-37-dark", smallPair.Dark)
		if !strings.Contains(smallPair.Dark, `width="106" height="28" rx="14"`) {
			t.Error("expected minimum pill width 106 for 37 stars")
		}

		largePair, ok, err := RenderStarHistory(StarHistoryInput{
			Points: []StarPoint{
				{Date: base, Stars: 10},
				{Date: base.AddDate(0, 6, 0), Stars: 1240},
			},
			MinStars: 25,
			MinDays:  30,
		}, theme)
		if err != nil || !ok {
			t.Fatalf("RenderStarHistory(1240) = ok:%v err:%v", ok, err)
		}
		assertValidGitHubSVG(t, "stars-1240-dark", largePair.Dark)
		if !strings.Contains(largePair.Dark, `width="128" height="28" rx="14"`) {
			t.Error("expected dynamic pill width 128 for 1,240 stars")
		}
	})
}

func TestRenderSponsors(t *testing.T) {
	theme := iaGetTheme()

	t.Run("renders fixed 840x108 CTA card with Vitrine capsule, rose glint, and drifting hearts", func(t *testing.T) {
		pair, err := RenderSponsors(SponsorsInput{}, theme)
		if err != nil {
			t.Fatalf("RenderSponsors() error = %v", err)
		}
		assertValidGitHubSVG(t, "sponsors-dark", pair.Dark)
		assertValidGitHubSVG(t, "sponsors-light", pair.Light)
		assertSVGDimensions(t, "sponsors-dark", pair.Dark, 840, 108)
		assertSVGDimensions(t, "sponsors-light", pair.Light, 840, 108)

		for _, want := range []string{
			"Enjoy my work?",
			"Treat me to a coffee, help cover hosting, infrastructure, and signing costs.",
			"Sponsor on GitHub",
			`x="612" y="32" width="196" height="44" rx="22"`,
			`fill="#f38ba8" fill-opacity="0.16" stroke="#f38ba8" stroke-width="1.5"`,
			`stroke="` + cardBorder(theme.Dark) + `"`,
			`id="sp-glint"`,
			`class="sp-glint"`,
			"@keyframes sp-sweep",
			"14%,100%",
			"@keyframes sp-rise-",
			"translateY(",
			"@keyframes sp-sway-",
			"translateX(",
			"ease-in-out",
			"infinite alternate",
			`class="sp-heart sp-h0"><g class="sp-sway sp-s0"`,
			".sp-heart, .sp-sway { opacity: 1 !important; transform: none !important; }",
			"prefers-reduced-motion: reduce",
		} {
			if !strings.Contains(pair.Dark, want) {
				t.Errorf("dark sponsors SVG missing %q", want)
			}
		}
		for _, want := range []string{
			`fill="#e64553" fill-opacity="0.16" stroke="#e64553" stroke-width="1.5"`,
			`stroke="` + theme.Light.MutedBorder + `"`,
			`id="sp-glint"`,
			`class="sp-glint"`,
		} {
			if !strings.Contains(pair.Light, want) {
				t.Errorf("light sponsors SVG missing %q", want)
			}
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
		if strings.Contains(pair.Dark, `opacity="0.28"`) || strings.Contains(pair.Light, `opacity="0.28"`) {
			t.Error("sponsors CTA banner must not render permanent static top accent bar")
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
			"translateY(",
			"@keyframes cb-sway-",
			"translateX(",
			"ease-in-out",
			"infinite alternate",
			`class="cb-float cb-f0"><g class="cb-sway cb-s0"`,
			".cb-float, .cb-sway { opacity: 1 !important; transform: none !important; }",
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
		for i, c := range contributors {
			_, _, swayPx, _, _, _ := contributorBalloonTiming(c.Login, i, len(contributors))
			if swayPx < 12 || swayPx > 16 {
				t.Errorf("contributor %q swayPx = %d, want in [12, 16]", c.Login, swayPx)
			}
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
	checkGroup(t, "banners", []string{"hero", "discord", "sponsors", "stars-cta", "demo", "footer"})

	// Verify calm resting keyframes (14%,100% invisible rest) appear across hero, pill, community, package, sponsors, star CTA, and footer cards,
	// while hero uses the tapered rounded top highlight path instead of a flat top bar.
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
	sponsors, err := RenderSponsors(SponsorsInput{}, theme)
	if err != nil {
		t.Fatalf("RenderSponsors() error = %v", err)
	}
	starCTA, _, err := RenderStarHistory(StarHistoryInput{MinStars: 25, MinDays: 30, ProjectTitle: "Tailor"}, theme)
	if err != nil {
		t.Fatalf("RenderStarHistory(CTA) error = %v", err)
	}
	footer, err := RenderFooter(theme)
	if err != nil {
		t.Fatalf("RenderFooter() error = %v", err)
	}

	const wantHeroTaperPath = `<path d="M 1 14 A 13 13 0 0 1 14 1 L 826 1 A 13 13 0 0 1 839 14" fill="none" stroke="url(#hero-top-taper)" stroke-width="2.5"/>`
	for label, svg := range map[string]string{"dark": hero.Dark, "light": hero.Light} {
		if !strings.Contains(svg, `id="hero-top-taper"`) || !strings.Contains(svg, wantHeroTaperPath) {
			t.Errorf("%s hero missing tapered rounded top accent highlight", label)
		}
		if strings.Contains(svg, `width="840" height="3"`) {
			t.Errorf("%s hero must not render flat full-width top bar", label)
		}
	}

	for name, pair := range map[string]SVGPair{
		"hero":      hero,
		"pill":      pill,
		"community": comm,
		"package":   pkg,
		"sponsors":  sponsors,
		"stars-cta": starCTA,
		"footer":    footer,
	} {
		if !strings.Contains(pair.Dark, "14%,100%") {
			t.Errorf("%s dark SVG missing calm 14%%,100%% rest keyframe", name)
		}
		if !strings.Contains(pair.Light, "14%,100%") {
			t.Errorf("%s light SVG missing calm 14%%,100%% rest keyframe", name)
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

func TestRenderDemo(t *testing.T) {
	theme := iaGetTheme()
	sampleWebP := makeTestWebPVP8X(960, 540)

	pair, err := RenderDemo(theme, "Tailor", sampleWebP, "image/webp", 960, 540)
	if err != nil {
		t.Fatalf("RenderDemo() error = %v", err)
	}
	assertValidGitHubSVG(t, "demo-dark", pair.Dark)
	assertValidGitHubSVG(t, "demo-light", pair.Light)
	// 840px wide * (540/960) = 472.5 -> 473px body + 36px titlebar = 509px total.
	assertSVGDimensions(t, "demo-dark", pair.Dark, 840, 509)
	assertSVGDimensions(t, "demo-light", pair.Light, 840, 509)

	for _, want := range []string{
		`fill="#ff5f56"`,
		`fill="#ffbd2e"`,
		`fill="#27c93f"`,
		`fill="#1a1f28"`,
		`>Tailor</text>`,
		`href="data:image/webp;base64,`,
		theme.Dark.MutedBorder,
	} {
		if !strings.Contains(pair.Dark, want) {
			t.Errorf("dark demo SVG missing %q", want)
		}
	}
	if !strings.Contains(pair.Light, theme.Light.CodeBackground) {
		t.Errorf("light demo SVG missing Light.CodeBackground %s", theme.Light.CodeBackground)
	}

	if _, err := RenderDemo(theme, "Tailor", nil, "image/webp", 800, 600); err == nil {
		t.Error("expected error for empty image bytes")
	}
	if _, err := RenderDemo(theme, "Tailor", sampleWebP, "image/webp", 0, 600); err == nil {
		t.Error("expected error for non-positive width")
	}
	if _, err := RenderDemo(theme, "Tailor", sampleWebP, "image/webp", 800, -1); err == nil {
		t.Error("expected error for non-positive height")
	}
}

func TestDecodeDemoImageDimensions(t *testing.T) {
	t.Run("PNG IHDR", func(t *testing.T) {
		data := makeTestPNGHeader(1200, 630)
		w, h, err := decodeDemoImageDimensions(data, "image/png")
		if err != nil || w != 1200 || h != 630 {
			t.Fatalf("decodeDemoImageDimensions(PNG) = %dx%d, %v; want 1200x630, nil", w, h, err)
		}
	})

	t.Run("WebP VP8X", func(t *testing.T) {
		data := makeTestWebPVP8X(1024, 576)
		w, h, err := decodeDemoImageDimensions(data, "image/webp")
		if err != nil || w != 1024 || h != 576 {
			t.Fatalf("decodeDemoImageDimensions(VP8X) = %dx%d, %v; want 1024x576, nil", w, h, err)
		}
	})

	t.Run("WebP VP8 lossy", func(t *testing.T) {
		buf := make([]byte, 30)
		copy(buf[0:4], "RIFF")
		copy(buf[8:12], "WEBP")
		copy(buf[12:16], "VP8 ")
		buf[20] = 0x10 // keyframe (bit 0 == 0)
		buf[23], buf[24], buf[25] = 0x9d, 0x01, 0x2a
		binary.LittleEndian.PutUint16(buf[26:28], 800)
		binary.LittleEndian.PutUint16(buf[28:30], 450)
		w, h, err := decodeDemoImageDimensions(buf, "image/webp")
		if err != nil || w != 800 || h != 450 {
			t.Fatalf("decodeDemoImageDimensions(VP8 ) = %dx%d, %v; want 800x450, nil", w, h, err)
		}
	})

	t.Run("WebP VP8L lossless", func(t *testing.T) {
		buf := make([]byte, 25)
		copy(buf[0:4], "RIFF")
		copy(buf[8:12], "WEBP")
		copy(buf[12:16], "VP8L")
		buf[20] = 0x2f
		bits := uint32(640-1) | (uint32(360-1) << 14)
		binary.LittleEndian.PutUint32(buf[21:25], bits)
		w, h, err := decodeDemoImageDimensions(buf, "image/webp")
		if err != nil || w != 640 || h != 360 {
			t.Fatalf("decodeDemoImageDimensions(VP8L) = %dx%d, %v; want 640x360, nil", w, h, err)
		}
	})

	t.Run("repository demo.webp and demo-still.png", func(t *testing.T) {
		for _, tc := range []struct {
			file string
			mime string
		}{
			{"demo.webp", "image/webp"},
			{"demo-still.png", "image/png"},
		} {
			raw, err := os.ReadFile(filepath.Join("..", "..", "pages", tc.file))
			if err != nil {
				t.Fatalf("reading pages/%s: %v", tc.file, err)
			}
			w, h, err := decodeDemoImageDimensions(raw, tc.mime)
			if err != nil || w <= 0 || h <= 0 {
				t.Fatalf("decodeDemoImageDimensions(%s) = %dx%d, %v", tc.file, w, h, err)
			}
		}
	})

	t.Run("rejects malformed headers", func(t *testing.T) {
		for _, tc := range []struct {
			name string
			data []byte
			mime string
		}{
			{"short png", []byte{0x89, 'P', 'N', 'G'}, "image/png"},
			{"short webp", []byte("RIFF\x00\x00\x00\x00WEBP"), "image/webp"},
			{"unknown chunk", []byte("RIFF\x00\x00\x00\x00WEBPVXXX00000000000000"), "image/webp"},
			{"unsupported mime", makeTestPNGHeader(10, 10), "image/gif"},
		} {
			if _, _, err := decodeDemoImageDimensions(tc.data, tc.mime); err == nil {
				t.Errorf("%s: expected error, got nil", tc.name)
			}
		}
	})
}

func TestRenderFooter(t *testing.T) {
	theme := iaGetTheme()
	pair, err := RenderFooter(theme)
	if err != nil {
		t.Fatalf("RenderFooter() error = %v", err)
	}

	assertValidGitHubSVG(t, "footer-dark", pair.Dark)
	assertValidGitHubSVG(t, "footer-light", pair.Light)
	assertSVGDimensions(t, "footer-dark", pair.Dark, 840, 44)
	assertSVGDimensions(t, "footer-light", pair.Light, 840, 44)

	for _, want := range []string{
		"Fitted by ",
		">Tailor</tspan>",
		"tailor.wimpys.world",
		`<rect class="ft-stitch" x="0.75" y="0.75" width="838.5" height="42.5" rx="10" fill="none" stroke="` + cardBorder(theme.Dark) + `" stroke-width="1.25" stroke-dasharray="5 5"/>`,
		"@keyframes stitch-march",
		"20s linear infinite",
		`id="ft-glint"`,
		`class="ft-glint"`,
		"@keyframes ft-sweep",
		"14%,100%",
		"@keyframes snip-top",
		"@keyframes snip-bot",
		"10s ease-in-out infinite",
		"#f0cb78",
		"#b88a44",
		theme.Dark.CardBackground,
		theme.Dark.MutedColor,
		theme.Dark.HeadingColor,
	} {
		if !strings.Contains(pair.Dark, want) {
			t.Errorf("dark footer SVG missing %q", want)
		}
	}
	for _, want := range []string{
		`<rect class="ft-stitch" x="0.75" y="0.75" width="838.5" height="42.5" rx="10" fill="none" stroke="` + theme.Light.MutedBorder + `" stroke-width="1.25" stroke-dasharray="5 5"/>`,
		`stop-color="#b88a44"`,
		theme.Light.CardBackground,
		theme.Light.MutedColor,
		theme.Light.HeadingColor,
	} {
		if !strings.Contains(pair.Light, want) {
			t.Errorf("light footer SVG missing %q", want)
		}
	}
	if strings.Contains(pair.Dark, "<line ") || strings.Contains(pair.Light, "<line ") {
		t.Error("footer SVG must replace separate top/bottom stitch lines with one continuous perimeter stitch")
	}
}

func makeTestPNGHeader(width, height uint32) []byte {
	buf := make([]byte, 24)
	copy(buf[0:8], pngSignature)
	binary.BigEndian.PutUint32(buf[8:12], 13)
	copy(buf[12:16], "IHDR")
	binary.BigEndian.PutUint32(buf[16:20], width)
	binary.BigEndian.PutUint32(buf[20:24], height)
	return buf
}

func makeTestWebPVP8X(width, height uint32) []byte {
	buf := make([]byte, 30)
	copy(buf[0:4], "RIFF")
	binary.LittleEndian.PutUint32(buf[4:8], 22)
	copy(buf[8:12], "WEBP")
	copy(buf[12:16], "VP8X")
	binary.LittleEndian.PutUint32(buf[16:20], 10)
	w := width - 1
	h := height - 1
	buf[24] = byte(w & 0xff)
	buf[25] = byte((w >> 8) & 0xff)
	buf[26] = byte((w >> 16) & 0xff)
	buf[27] = byte(h & 0xff)
	buf[28] = byte((h >> 8) & 0xff)
	buf[29] = byte((h >> 16) & 0xff)
	return buf
}
