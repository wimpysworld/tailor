package social

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"errors"
	"fmt"
	"math"
	"strings"
	"unicode/utf8"
)

const (
	fontSans         = "'Work Sans', -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif"
	fontMono         = "'Fira Code', ui-monospace, SFMono-Regular, Menlo, monospace"
	reducedMotionCSS = "@media (prefers-reduced-motion: reduce) { * { animation: none !important; } }"
)

var glintDurations = [...]float64{
	16.3, 17.3, 18.1, 18.7, 19.3, 19.9, 20.9, 21.1, 22.3, 22.7, 23.3, 23.9,
}

// glintTiming returns a deterministic calm cycle duration (16s-24s) and
// negative animation-delay offset for key so neighbouring tiles do not flash
// in lockstep.
func glintTiming(key string) (durationSec, delaySec float64) {
	sum := sha256.Sum256([]byte(strings.ToLower(strings.TrimSpace(key))))
	durIdx := binary.BigEndian.Uint64(sum[0:8]) % uint64(len(glintDurations))
	durationSec = glintDurations[durIdx]

	frac := float64(binary.BigEndian.Uint64(sum[8:16])%1000) / 1000.0
	delay := math.Round(frac*durationSec*10) / 10
	if delay < 0.3 {
		delay = 0.3
	}
	return durationSec, -delay
}

// SVGPair holds dark and light theme variants of a rendered SVG tile.
type SVGPair struct {
	Dark  string
	Light string
}

// HeroInput holds the project metadata and optional icon bytes for RenderHero.
type HeroInput struct {
	Assets   ProjectAssets
	IconData []byte
	IconMIME string
}

// LinkPillInput configures one preferred-link pill tile for RenderLinkPill.
type LinkPillInput struct {
	Key   string
	Label string
}

// CommunityBannerInput configures the full-width community banner for
// RenderCommunityBanner.
type CommunityBannerInput struct {
	Platform        string
	Title           string
	Subtitle        string
	OnlineCount     int
	MemberCount     int
	DiscussionCount int
	CTALabel        string
}

// RenderHero renders the 840x210 themed project hero banner in dark and light
// variants. When IconData is empty the icon is omitted gracefully.
func RenderHero(in HeroInput, theme Theme) (SVGPair, error) {
	iconSVG, isRaster, err := prepareHeroIcon(in)
	if err != nil {
		return SVGPair{}, err
	}

	darkP := normalizePalette(theme.Dark, defaultDarkPalette)
	lightP := normalizePalette(theme.Light, defaultLightPalette)

	return SVGPair{
		Dark:  renderHeroVariant(in.Assets, iconSVG, isRaster, darkP, true),
		Light: renderHeroVariant(in.Assets, iconSVG, isRaster, lightP, false),
	}, nil
}

func prepareHeroIcon(in HeroInput) (markup string, isRaster bool, err error) {
	if len(in.IconData) == 0 {
		return "", false, nil
	}
	kind := strings.ToLower(strings.TrimSpace(in.Assets.IconKind))
	mime := strings.ToLower(strings.TrimSpace(in.IconMIME))
	if kind == IconKindSVG || mime == "image/svg+xml" || (kind == IconKindNone && mime == "" && looksLikeSVG(in.IconData)) {
		sanitized, err := SanitizeInlineSVG(in.IconData, 72)
		if err != nil {
			return "", false, fmt.Errorf("sanitizing hero SVG icon: %w", err)
		}
		return sanitized, false, nil
	}
	return EmbedRasterIcon(in.IconData, mime, 72), true, nil
}

func looksLikeSVG(raw []byte) bool {
	trimmed := strings.TrimSpace(string(raw))
	return strings.HasPrefix(trimmed, "<svg") || strings.HasPrefix(trimmed, "<?xml")
}

func renderHeroVariant(assets ProjectAssets, iconMarkup string, isRaster bool, p Palette, dark bool) string {
	const (
		width  = 840
		height = 210
	)

	title := strings.TrimSpace(assets.Title)
	if title == "" {
		title = "Project"
	}
	tagline := strings.TrimSpace(assets.Tagline)
	footerURL := displayFooterURL(assets.FooterURL)
	platforms := strings.TrimSpace(assets.Platforms)

	hasIcon := iconMarkup != ""
	textX := 36
	maxTagChars := 72
	if hasIcon {
		textX = 128
		maxTagChars = 60
	}
	taglineLines := WrapText(tagline, maxTagChars, 2)

	waveOpacity1 := "0.10"
	waveOpacity2 := "0.05"
	if dark {
		waveOpacity1 = "0.16"
		waveOpacity2 = "0.08"
	}

	titleY := 76
	if len(taglineLines) == 0 {
		titleY = 96
	} else if len(taglineLines) == 1 {
		titleY = 82
	}

	var b strings.Builder
	fmt.Fprintf(&b, `<svg xmlns="http://www.w3.org/2000/svg" width="%d" height="%d" viewBox="0 0 %d %d" role="img" aria-label="%s">`,
		width, height, width, height, XMLEscape(title))
	fmt.Fprintf(&b, `<title>%s</title>`, XMLEscape(title))
	b.WriteString(`<defs>`)
	fmt.Fprintf(&b, `<clipPath id="hero-clip"><rect x="0.5" y="0.5" width="839" height="209" rx="14"/></clipPath>`)
	fmt.Fprintf(&b, `<linearGradient id="hero-bg" x1="0" y1="0" x2="1" y2="1"><stop offset="0%%" stop-color="%s"/><stop offset="100%%" stop-color="%s"/></linearGradient>`,
		p.CardBackground, p.Background)
	fmt.Fprintf(&b, `<linearGradient id="hero-aurora" x1="0" y1="0" x2="0" y2="1"><stop offset="0%%" stop-color="%s" stop-opacity="%s"/><stop offset="100%%" stop-color="%s" stop-opacity="0"/></linearGradient>`,
		p.Primary, waveOpacity1, p.Primary)
	fmt.Fprintf(&b, `<linearGradient id="hero-aurora-soft" x1="0" y1="0" x2="0" y2="1"><stop offset="0%%" stop-color="%s" stop-opacity="%s"/><stop offset="100%%" stop-color="%s" stop-opacity="0"/></linearGradient>`,
		p.Primary, waveOpacity2, p.Primary)
	fmt.Fprintf(&b, `<linearGradient id="hero-top-taper" x1="0" y1="0" x2="1" y2="0"><stop offset="0%%" stop-color="%s" stop-opacity="0"/><stop offset="8%%" stop-color="%s" stop-opacity="0.55"/><stop offset="92%%" stop-color="%s" stop-opacity="0.55"/><stop offset="100%%" stop-color="%s" stop-opacity="0"/></linearGradient>`,
		p.Primary, p.Primary, p.Primary, p.Primary)
	fmt.Fprintf(&b, `<linearGradient id="hero-glint" x1="0" y1="0" x2="1" y2="0"><stop offset="0%%" stop-color="%s" stop-opacity="0"/><stop offset="50%%" stop-color="%s" stop-opacity="0.9"/><stop offset="100%%" stop-color="%s" stop-opacity="0"/></linearGradient>`,
		p.Primary, p.Primary, p.Primary)
	if isRaster {
		fmt.Fprintf(&b, `<clipPath id="hero-icon-clip"><rect x="0" y="0" width="72" height="72" rx="12"/></clipPath>`)
	}
	b.WriteString(`</defs>`)

	b.WriteString(`<style>`)
	fmt.Fprintf(&b, `.h-title{font-family:%s;font-size:30px;font-weight:700;fill:%s;letter-spacing:-0.02em}`, fontSans, p.HeadingColor)
	fmt.Fprintf(&b, `.h-tag{font-family:%s;font-size:15.5px;font-weight:400;fill:%s}`, fontSans, p.Color)
	fmt.Fprintf(&b, `.h-foot{font-family:%s;font-size:12.5px;font-weight:500;fill:%s}`, fontMono, p.MutedColor)
	heroDur, heroDelay := glintTiming("hero")
	b.WriteString(`@keyframes wave-flow-left{from{transform:translateX(0)}to{transform:translateX(-840px)}}`)
	b.WriteString(`@keyframes wave-flow-right{from{transform:translateX(-840px)}to{transform:translateX(0)}}`)
	b.WriteString(`@keyframes wave-bob-a{from{transform:translateY(-4px)}to{transform:translateY(4px)}}`)
	b.WriteString(`@keyframes wave-bob-b{from{transform:translateY(5px)}to{transform:translateY(-5px)}}`)
	b.WriteString(`@keyframes hero-glint-move{0%{transform:translateX(-260px);opacity:0}2%{opacity:1}12%{opacity:1}14%,100%{transform:translateX(900px);opacity:0}}`)
	b.WriteString(`.wave-a{animation:wave-flow-left 19s linear infinite}`)
	b.WriteString(`.wave-b{animation:wave-flow-right 29s linear infinite}`)
	b.WriteString(`.wave-bob-a{animation:wave-bob-a 7.3s ease-in-out infinite alternate}`)
	b.WriteString(`.wave-bob-b{animation:wave-bob-b 11.1s ease-in-out infinite alternate}`)
	fmt.Fprintf(&b, `.glint{opacity:0;animation:hero-glint-move %.1fs ease-in-out %.1fs infinite}`, heroDur, heroDelay)
	b.WriteString(reducedMotionCSS)
	b.WriteString(`</style>`)

	b.WriteString(`<g clip-path="url(#hero-clip)">`)
	b.WriteString(`<rect x="0" y="0" width="840" height="210" fill="url(#hero-bg)"/>`)
	// Two seamless 1680px horizontally tiling waves scrolling in opposite directions with gentle vertical bobbing.
	b.WriteString(`<g class="wave-bob-b"><path class="wave-b" d="M 0 164 C 47 164 93 132 140 132 C 187 132 233 164 280 164 C 327 164 373 132 420 132 C 467 132 513 164 560 164 C 607 164 653 132 700 132 C 747 132 793 164 840 164 C 887 164 933 132 980 132 C 1027 132 1073 164 1120 164 C 1167 164 1213 132 1260 132 C 1307 132 1353 164 1400 164 C 1447 164 1493 132 1540 132 C 1587 132 1633 164 1680 164 L 1680 220 L 0 220 Z" fill="url(#hero-aurora-soft)"/></g>`)
	b.WriteString(`<g class="wave-bob-a"><path class="wave-a" d="M 0 160 C 70 160 140 124 210 124 C 280 124 350 160 420 160 C 490 160 560 124 630 124 C 700 124 770 160 840 160 C 910 160 980 124 1050 124 C 1120 124 1190 160 1260 160 C 1330 160 1400 124 1470 124 C 1540 124 1610 160 1680 160 L 1680 220 L 0 220 Z" fill="url(#hero-aurora)"/></g>`)
	// Tapered rounded top accent highlight following the rx="14" corners and gliding glint.
	b.WriteString(`<path d="M 1 14 A 13 13 0 0 1 14 1 L 826 1 A 13 13 0 0 1 839 14" fill="none" stroke="url(#hero-top-taper)" stroke-width="2.5"/>`)
	b.WriteString(`<rect class="glint" x="0" y="0" width="220" height="3" fill="url(#hero-glint)"/>`)

	if hasIcon {
		b.WriteString(`<g transform="translate(36, 46)">`)
		if isRaster {
			fmt.Fprintf(&b, `<rect x="-2" y="-2" width="76" height="76" rx="14" fill="%s" stroke="%s" stroke-width="1"/>`, p.CodeBackground, p.MutedBorder)
			b.WriteString(`<g clip-path="url(#hero-icon-clip)">`)
			b.WriteString(iconMarkup)
			b.WriteString(`</g>`)
		} else {
			b.WriteString(iconMarkup)
		}
		b.WriteString(`</g>`)
	}

	fmt.Fprintf(&b, `<text x="%d" y="%d" class="h-title">%s</text>`, textX, titleY, XMLEscape(title))
	for i, line := range taglineLines {
		fmt.Fprintf(&b, `<text x="%d" y="%d" class="h-tag">%s</text>`, textX, titleY+28+i*22, XMLEscape(line))
	}

	// Quiet muted footer.
	if footerURL != "" {
		fmt.Fprintf(&b, `<text x="36" y="187" class="h-foot">%s</text>`, XMLEscape(footerURL))
	}
	if platforms != "" {
		fmt.Fprintf(&b, `<text x="804" y="187" text-anchor="end" class="h-foot">%s</text>`, XMLEscape(platforms))
	}

	b.WriteString(`</g>`)
	fmt.Fprintf(&b, `<rect x="0.5" y="0.5" width="839" height="209" rx="14" fill="none" stroke="%s" stroke-width="1"/>`, cardBorder(p))
	b.WriteString(`</svg>`)
	return b.String()
}

// RenderLinkPill renders a 184x44 themed preferred-link pill SVG pair for one
// supported link key.
func RenderLinkPill(in LinkPillInput, theme Theme) (SVGPair, error) {
	key := strings.ToLower(strings.TrimSpace(in.Key))
	icon, ok := LookupLinkIcon(key)
	if !ok {
		return SVGPair{}, fmt.Errorf("unknown link pill key %q", in.Key)
	}
	label := strings.TrimSpace(in.Label)
	if label == "" {
		label = icon.Label
	}

	darkP := normalizePalette(theme.Dark, defaultDarkPalette)
	lightP := normalizePalette(theme.Light, defaultLightPalette)

	return SVGPair{
		Dark:  renderLinkPillVariant(key, icon, label, darkP, true),
		Light: renderLinkPillVariant(key, icon, label, lightP, false),
	}, nil
}

func renderLinkPillVariant(key string, icon IconInfo, label string, p Palette, dark bool) string {
	const (
		width  = 184
		height = 44
	)

	accent := p.Primary
	if dark && icon.AccentDark != "" {
		accent = icon.AccentDark
	} else if !dark && icon.AccentLight != "" {
		accent = icon.AccentLight
	}
	displayLabel := label
	if wrapped := WrapText(label, 15, 1); len(wrapped) > 0 {
		displayLabel = wrapped[0]
	}

	var b strings.Builder
	fmt.Fprintf(&b, `<svg xmlns="http://www.w3.org/2000/svg" width="%d" height="%d" viewBox="0 0 %d %d" role="img" aria-label="%s">`,
		width, height, width, height, XMLEscape(label))
	fmt.Fprintf(&b, `<title>%s</title>`, XMLEscape(label))
	b.WriteString(`<defs>`)
	b.WriteString(`<clipPath id="pill-clip"><rect x="0.5" y="0.5" width="183" height="43" rx="10"/></clipPath>`)
	fmt.Fprintf(&b, `<linearGradient id="pill-glint" x1="0" y1="0" x2="1" y2="0"><stop offset="0%%" stop-color="%s" stop-opacity="0"/><stop offset="50%%" stop-color="%s" stop-opacity="0.85"/><stop offset="100%%" stop-color="%s" stop-opacity="0"/></linearGradient>`,
		accent, accent, accent)
	b.WriteString(`</defs>`)

	b.WriteString(`<style>`)
	fmt.Fprintf(&b, `.p-label{font-family:%s;font-size:13.5px;font-weight:600;fill:%s}`, fontSans, p.HeadingColor)
	pillDur, pillDelay := glintTiming(key)
	b.WriteString(`@keyframes pill-sweep{0%{transform:translateX(-90px);opacity:0}2%{opacity:1}12%{opacity:1}14%,100%{transform:translateX(210px);opacity:0}}`)
	fmt.Fprintf(&b, `.p-glint{opacity:0;animation:pill-sweep %.1fs ease-in-out %.1fs infinite}`, pillDur, pillDelay)
	b.WriteString(reducedMotionCSS)
	b.WriteString(`</style>`)

	b.WriteString(`<g clip-path="url(#pill-clip)">`)
	fmt.Fprintf(&b, `<rect x="0" y="0" width="184" height="44" fill="%s"/>`, p.CardBackground)
	b.WriteString(`<rect class="p-glint" x="0" y="0" width="76" height="2" fill="url(#pill-glint)"/>`)
	// 20x20 inline vector icon.
	fmt.Fprintf(&b, `<g transform="translate(14, 12) scale(0.833333)" fill="%s"><path d="%s"/></g>`, accent, icon.Path)
	fmt.Fprintf(&b, `<text x="44" y="27" class="p-label">%s</text>`, XMLEscape(displayLabel))
	// Quiet right chevron indicator.
	fmt.Fprintf(&b, `<path d="M167 17.5 L171.5 22 L167 26.5" fill="none" stroke="%s" stroke-width="1.6" stroke-linecap="round" stroke-linejoin="round"/>`, p.MutedColor)
	b.WriteString(`</g>`)
	fmt.Fprintf(&b, `<rect x="0.5" y="0.5" width="183" height="43" rx="10" fill="none" stroke="%s" stroke-width="1"/>`, cardBorder(p))
	b.WriteString(`</svg>`)
	return b.String()
}

// RenderCommunityBanner renders the 840x68 full-width themed community banner
// for discord, telegram, matrix, slack, irc, forum, or discussions.
func RenderCommunityBanner(in CommunityBannerInput, theme Theme) (SVGPair, error) {
	platform := strings.ToLower(strings.TrimSpace(in.Platform))
	defaults, ok := communityDefaults[platform]
	if !ok {
		return SVGPair{}, fmt.Errorf("unsupported community platform %q", in.Platform)
	}
	icon, ok := LookupLinkIcon(platform)
	if !ok {
		return SVGPair{}, fmt.Errorf("missing icon for community platform %q", platform)
	}

	title := strings.TrimSpace(in.Title)
	if title == "" {
		title = defaults.title
	}
	subtitle := strings.TrimSpace(in.Subtitle)
	if subtitle == "" {
		subtitle = defaults.subtitle
	}
	cta := strings.TrimSpace(in.CTALabel)
	if cta == "" {
		cta = "JOIN"
	}

	liveText := formatCommunityLiveText(platform, in.OnlineCount, in.MemberCount, in.DiscussionCount)

	darkP := normalizePalette(theme.Dark, defaultDarkPalette)
	lightP := normalizePalette(theme.Light, defaultLightPalette)

	return SVGPair{
		Dark:  renderCommunityVariant(platform, icon, title, subtitle, liveText, cta, darkP),
		Light: renderCommunityVariant(platform, icon, title, subtitle, liveText, cta, lightP),
	}, nil
}

type communityCopy struct {
	title    string
	subtitle string
}

var communityDefaults = map[string]communityCopy{
	"discord": {
		title:    "Join the Discord community",
		subtitle: "Chat, get help, and share what you build",
	},
	"telegram": {
		title:    "Join the Telegram community",
		subtitle: "Chat, get updates, and connect with the community",
	},
	"matrix": {
		title:    "Join the Matrix room",
		subtitle: "Open federated chat for users and contributors",
	},
	"slack": {
		title:    "Join the Slack workspace",
		subtitle: "Connect with maintainers and contributors",
	},
	"irc": {
		title:    "Join the IRC channel",
		subtitle: "Real-time chat with the community",
	},
	"forum": {
		title:    "Join the community forum",
		subtitle: "Questions, ideas, and long-form discussions",
	},
	"discussions": {
		title:    "Join GitHub Discussions",
		subtitle: "Ask questions, share ideas, and follow updates",
	},
}

func formatCommunityLiveText(platform string, online, members, discussions int) string {
	if platform == "discussions" {
		if discussions <= 0 {
			return ""
		}
		if discussions == 1 {
			return "1 discussion"
		}
		return formatCount(discussions) + " discussions"
	}
	switch {
	case online > 0 && members > 0:
		return fmt.Sprintf("%s online · %s members", formatCount(online), formatCount(members))
	case online > 0:
		return formatCount(online) + " online"
	case members > 0:
		return formatCount(members) + " members"
	default:
		return ""
	}
}

func renderCommunityVariant(platform string, icon IconInfo, title, subtitle, liveText, cta string, p Palette) string {
	const (
		width  = 840
		height = 68
	)

	ctaBg := p.PrimaryBackground
	if ctaBg == "" {
		ctaBg = p.Primary
	}
	ctaFg := contrastOnFill(ctaBg)

	maxTitleChars := 48
	maxSubChars := 60
	if liveText != "" {
		maxTitleChars = 38
		maxSubChars = 46
	}
	if wrapped := WrapText(title, maxTitleChars, 1); len(wrapped) > 0 {
		title = wrapped[0]
	}
	if wrapped := WrapText(subtitle, maxSubChars, 1); len(wrapped) > 0 {
		subtitle = wrapped[0]
	}

	var b strings.Builder
	fmt.Fprintf(&b, `<svg xmlns="http://www.w3.org/2000/svg" width="%d" height="%d" viewBox="0 0 %d %d" role="img" aria-label="%s">`,
		width, height, width, height, XMLEscape(title))
	fmt.Fprintf(&b, `<title>%s</title>`, XMLEscape(title))
	b.WriteString(`<defs>`)
	b.WriteString(`<clipPath id="comm-clip"><rect x="0.5" y="0.5" width="839" height="67" rx="12"/></clipPath>`)
	fmt.Fprintf(&b, `<linearGradient id="comm-glint" x1="0" y1="0" x2="1" y2="0"><stop offset="0%%" stop-color="%s" stop-opacity="0"/><stop offset="50%%" stop-color="%s" stop-opacity="0.85"/><stop offset="100%%" stop-color="%s" stop-opacity="0"/></linearGradient>`,
		p.Primary, p.Primary, p.Primary)
	b.WriteString(`</defs>`)

	b.WriteString(`<style>`)
	fmt.Fprintf(&b, `.c-title{font-family:%s;font-size:15.5px;font-weight:700;fill:%s}`, fontSans, p.HeadingColor)
	fmt.Fprintf(&b, `.c-sub{font-family:%s;font-size:12.5px;font-weight:400;fill:%s}`, fontSans, p.MutedColor)
	fmt.Fprintf(&b, `.c-live{font-family:%s;font-size:12px;font-weight:500;fill:%s}`, fontMono, p.Color)
	fmt.Fprintf(&b, `.c-cta{font-family:%s;font-size:12px;font-weight:700;fill:%s;letter-spacing:0.06em}`, fontSans, ctaFg)
	if liveText != "" {
		b.WriteString(`@keyframes sonar-ring{0%{transform:scale(1);opacity:0.65}100%{transform:scale(2.4);opacity:0}}`)
		b.WriteString(`.sonar{transform-box:fill-box;transform-origin:center;animation:sonar-ring 2.6s ease-out infinite}`)
	} else {
		commDur, commDelay := glintTiming(platform)
		b.WriteString(`@keyframes comm-sweep{0%{transform:translateX(-220px);opacity:0}2%{opacity:1}12%{opacity:1}14%,100%{transform:translateX(880px);opacity:0}}`)
		fmt.Fprintf(&b, `.c-glint{opacity:0;animation:comm-sweep %.1fs ease-in-out %.1fs infinite}`, commDur, commDelay)
	}
	b.WriteString(reducedMotionCSS)
	b.WriteString(`</style>`)

	b.WriteString(`<g clip-path="url(#comm-clip)">`)
	fmt.Fprintf(&b, `<rect x="0" y="0" width="840" height="68" fill="%s"/>`, p.CardBackground)
	if liveText == "" {
		b.WriteString(`<rect class="c-glint" x="0" y="0" width="180" height="2" fill="url(#comm-glint)"/>`)
	}

	// Icon badge.
	fmt.Fprintf(&b, `<rect x="20" y="16" width="36" height="36" rx="9" fill="%s" stroke="%s" stroke-width="1"/>`, p.CodeBackground, p.MutedBorder)
	fmt.Fprintf(&b, `<g transform="translate(27, 23) scale(0.916667)" fill="%s"><path d="%s"/></g>`, p.Primary, icon.Path)

	// Title and subtitle.
	fmt.Fprintf(&b, `<text x="68" y="30" class="c-title">%s</text>`, XMLEscape(title))
	fmt.Fprintf(&b, `<text x="68" y="49" class="c-sub">%s</text>`, XMLEscape(subtitle))

	// Optional live count with sonar pulse.
	if liveText != "" {
		liveW := int(math.Ceil(float64(utf8.RuneCountInString(liveText))*7.6)) + 38
		if liveW < 100 {
			liveW = 100
		}
		if liveW > 320 {
			liveW = 320
		}
		liveX := 728 - liveW
		dotX := liveX + 14
		fmt.Fprintf(&b, `<rect x="%d" y="20" width="%d" height="28" rx="14" fill="%s" stroke="%s" stroke-width="1"/>`,
			liveX, liveW, p.CodeBackground, p.MutedBorder)
		fmt.Fprintf(&b, `<circle class="sonar" cx="%d" cy="34" r="4" fill="%s"/>`, dotX, p.Primary)
		fmt.Fprintf(&b, `<circle cx="%d" cy="34" r="3.5" fill="%s"/>`, dotX, p.Primary)
		fmt.Fprintf(&b, `<text x="%d" y="38" class="c-live">%s</text>`, dotX+12, XMLEscape(liveText))
	}

	// JOIN CTA pill.
	fmt.Fprintf(&b, `<rect x="742" y="18" width="80" height="32" rx="8" fill="%s"/>`, ctaBg)
	fmt.Fprintf(&b, `<text x="782" y="38" text-anchor="middle" class="c-cta">%s</text>`, XMLEscape(cta))

	b.WriteString(`</g>`)
	fmt.Fprintf(&b, `<rect x="0.5" y="0.5" width="839" height="67" rx="12" fill="none" stroke="%s" stroke-width="1"/>`, cardBorder(p))
	b.WriteString(`</svg>`)
	return b.String()
}

// RenderPackageCard renders a 200x78 themed Haberdasher clickable package tile
// for one supported package host (apt, rpm, pkg, apk, flatpak, brew, scoop, or nix).
func RenderPackageCard(host string, theme Theme) (SVGPair, error) {
	key := strings.ToLower(strings.TrimSpace(host))
	info, ok := LookupPackageHost(key)
	if !ok {
		return SVGPair{}, fmt.Errorf("unknown Haberdasher package host %q", host)
	}

	darkP := normalizePalette(theme.Dark, defaultDarkPalette)
	lightP := normalizePalette(theme.Light, defaultLightPalette)

	return SVGPair{
		Dark:  renderPackageCardVariant(key, info, darkP, true),
		Light: renderPackageCardVariant(key, info, lightP, false),
	}, nil
}

func renderPackageCardVariant(host string, info PackageHostInfo, p Palette, dark bool) string {
	const (
		width  = 200
		height = 78
	)

	accent := info.AccentLight
	if dark {
		accent = info.AccentDark
	}
	var b strings.Builder
	fmt.Fprintf(&b, `<svg xmlns="http://www.w3.org/2000/svg" width="%d" height="%d" viewBox="0 0 %d %d" role="img" aria-label="%s">`,
		width, height, width, height, XMLEscape(info.Label))
	fmt.Fprintf(&b, `<title>%s (%s)</title>`, XMLEscape(info.Label), XMLEscape(info.Domain))
	b.WriteString(`<defs>`)
	b.WriteString(`<clipPath id="pkg-clip"><rect x="0.5" y="0.5" width="199" height="77" rx="11"/></clipPath>`)
	fmt.Fprintf(&b, `<linearGradient id="pkg-glint" x1="0" y1="0" x2="1" y2="0"><stop offset="0%%" stop-color="%s" stop-opacity="0"/><stop offset="50%%" stop-color="%s" stop-opacity="0.85"/><stop offset="100%%" stop-color="%s" stop-opacity="0"/></linearGradient>`,
		accent, accent, accent)
	b.WriteString(`</defs>`)

	b.WriteString(`<style>`)
	fmt.Fprintf(&b, `.k-label{font-family:%s;font-size:12px;font-weight:700;fill:%s}`, fontSans, p.HeadingColor)
	fmt.Fprintf(&b, `.k-domain{font-family:%s;font-size:10px;font-weight:500;fill:%s}`, fontMono, p.MutedColor)
	pkgDur, pkgDelay := glintTiming(host)
	b.WriteString(`@keyframes pkg-sweep{0%{transform:translateX(-90px);opacity:0}2%{opacity:1}12%{opacity:1}14%,100%{transform:translateX(220px);opacity:0}}`)
	fmt.Fprintf(&b, `.k-glint{opacity:0;animation:pkg-sweep %.1fs ease-in-out %.1fs infinite}`, pkgDur, pkgDelay)
	b.WriteString(reducedMotionCSS)
	b.WriteString(`</style>`)

	b.WriteString(`<g clip-path="url(#pkg-clip)">`)
	fmt.Fprintf(&b, `<rect x="0" y="0" width="200" height="78" fill="%s"/>`, p.CardBackground)
	b.WriteString(`<rect class="k-glint" x="0" y="0" width="80" height="2.5" fill="url(#pkg-glint)"/>`)

	// 36x36 inline host icon (24 * 1.5 = 36).
	fmt.Fprintf(&b, `<g transform="translate(14, 21) scale(1.5)" fill="%s"><path d="%s"/></g>`, accent, info.Path)
	fmt.Fprintf(&b, `<text x="60" y="34" class="k-label">%s</text>`, XMLEscape(info.Label))
	fmt.Fprintf(&b, `<text x="60" y="52" class="k-domain">%s</text>`, XMLEscape(info.Domain))

	b.WriteString(`</g>`)
	fmt.Fprintf(&b, `<rect x="0.5" y="0.5" width="199" height="77" rx="11" fill="none" stroke="%s" stroke-width="1"/>`, cardBorder(p))
	b.WriteString(`</svg>`)
	return b.String()
}

// RenderDemo renders an 840px-wide macOS-style framed demo SVG wrapping an
// embedded raster image (WebP or PNG) in dark and light variants.
func RenderDemo(theme Theme, projectTitle string, imgBytes []byte, mime string, imgWidth, imgHeight int) (SVGPair, error) {
	if len(imgBytes) == 0 {
		return SVGPair{}, errors.New("demo image bytes are empty")
	}
	if imgWidth <= 0 || imgHeight <= 0 {
		return SVGPair{}, fmt.Errorf("demo image dimensions must be positive, got %dx%d", imgWidth, imgHeight)
	}

	resolvedMIME := normalizeRasterMIME(mime, imgBytes)
	encoded := base64.StdEncoding.EncodeToString(imgBytes)
	title := strings.TrimSpace(projectTitle)
	if title == "" {
		title = "Demo"
	}
	if wrapped := WrapText(title, 60, 1); len(wrapped) > 0 {
		title = wrapped[0]
	}

	darkP := normalizePalette(theme.Dark, defaultDarkPalette)
	lightP := normalizePalette(theme.Light, defaultLightPalette)

	return SVGPair{
		Dark:  renderDemoVariant(title, encoded, resolvedMIME, imgWidth, imgHeight, darkP, true),
		Light: renderDemoVariant(title, encoded, resolvedMIME, imgWidth, imgHeight, lightP, false),
	}, nil
}

func renderDemoVariant(title, encodedImg, mime string, imgWidth, imgHeight int, p Palette, dark bool) string {
	const (
		width     = 840
		titlebarH = 36
	)

	bodyH := max(int(math.Round(float64(imgHeight)*float64(width)/float64(imgWidth))), 1)
	totalH := titlebarH + bodyH

	canvasBg := p.CodeBackground
	if dark {
		canvasBg = "#1a1f28"
	}

	var b strings.Builder
	fmt.Fprintf(&b, `<svg xmlns="http://www.w3.org/2000/svg" width="%d" height="%d" viewBox="0 0 %d %d" role="img" aria-label="%s demo">`,
		width, totalH, width, totalH, XMLEscape(title))
	fmt.Fprintf(&b, `<title>%s demo</title>`, XMLEscape(title))
	b.WriteString(`<defs>`)
	fmt.Fprintf(&b, `<clipPath id="demo-clip"><rect x="0.5" y="0.5" width="839" height="%d" rx="12"/></clipPath>`, totalH-1)
	fmt.Fprintf(&b, `<linearGradient id="demo-glint" x1="0" y1="0" x2="1" y2="0"><stop offset="0%%" stop-color="%s" stop-opacity="0"/><stop offset="50%%" stop-color="%s" stop-opacity="0.85"/><stop offset="100%%" stop-color="%s" stop-opacity="0"/></linearGradient>`,
		p.Primary, p.Primary, p.Primary)
	b.WriteString(`</defs>`)

	b.WriteString(`<style>`)
	fmt.Fprintf(&b, `.d-title{font-family:%s;font-size:12px;font-weight:500;fill:%s}`, fontMono, p.MutedColor)
	demoDur, demoDelay := glintTiming("demo")
	b.WriteString(`@keyframes demo-sweep{0%{transform:translateX(-220px);opacity:0}2%{opacity:1}12%{opacity:1}14%,100%{transform:translateX(880px);opacity:0}}`)
	fmt.Fprintf(&b, `.d-glint{opacity:0;animation:demo-sweep %.1fs ease-in-out %.1fs infinite}`, demoDur, demoDelay)
	b.WriteString(reducedMotionCSS)
	b.WriteString(`</style>`)

	b.WriteString(`<g clip-path="url(#demo-clip)">`)
	// Titlebar chrome and hairline separator.
	fmt.Fprintf(&b, `<rect x="0" y="0" width="840" height="%d" fill="%s"/>`, titlebarH, p.CardBackground)
	b.WriteString(`<rect class="d-glint" x="0" y="0" width="180" height="2" fill="url(#demo-glint)"/>`)
	b.WriteString(`<circle cx="20" cy="18" r="5.5" fill="#ff5f56"/>`)
	b.WriteString(`<circle cx="38" cy="18" r="5.5" fill="#ffbd2e"/>`)
	b.WriteString(`<circle cx="56" cy="18" r="5.5" fill="#27c93f"/>`)
	fmt.Fprintf(&b, `<text x="420" y="22" text-anchor="middle" class="d-title">%s</text>`, XMLEscape(title))
	fmt.Fprintf(&b, `<line x1="0" y1="%d" x2="840" y2="%d" stroke="%s" stroke-width="1"/>`, titlebarH, titlebarH, p.MutedBorder)

	// Window body with embedded raster image.
	fmt.Fprintf(&b, `<rect x="0" y="%d" width="840" height="%d" fill="%s"/>`, titlebarH, bodyH, canvasBg)
	fmt.Fprintf(&b, `<image x="0" y="%d" width="840" height="%d" href="data:%s;base64,%s" preserveAspectRatio="xMidYMid meet"/>`,
		titlebarH, bodyH, mime, encodedImg)

	b.WriteString(`</g>`)
	fmt.Fprintf(&b, `<rect x="0.5" y="0.5" width="839" height="%d" rx="12" fill="none" stroke="%s" stroke-width="1"/>`, totalH-1, cardBorder(p))
	b.WriteString(`</svg>`)
	return b.String()
}

// RenderFooter renders the slim 840x44 animated "Fitted by Tailor" footer
// banner in dark and light theme variants using the host project's palette.
func RenderFooter(theme Theme) (SVGPair, error) {
	darkP := normalizePalette(theme.Dark, defaultDarkPalette)
	lightP := normalizePalette(theme.Light, defaultLightPalette)

	return SVGPair{
		Dark:  renderFooterVariant(darkP, true),
		Light: renderFooterVariant(lightP, false),
	}, nil
}

func renderFooterVariant(p Palette, dark bool) string {
	const (
		width  = 840
		height = 44
	)

	glintColor := "#b88a44"
	if dark {
		glintColor = "#f0cb78"
	}

	var b strings.Builder
	fmt.Fprintf(&b, `<svg xmlns="http://www.w3.org/2000/svg" width="%d" height="%d" viewBox="0 0 %d %d" role="img" aria-label="Fitted by Tailor">`,
		width, height, width, height)
	b.WriteString(`<title>Fitted by Tailor</title>`)
	b.WriteString(`<defs>`)
	b.WriteString(`<clipPath id="footer-clip"><rect x="0.75" y="0.75" width="838.5" height="42.5" rx="10"/></clipPath>`)
	fmt.Fprintf(&b, `<linearGradient id="ft-glint" x1="0" y1="0" x2="1" y2="0"><stop offset="0%%" stop-color="%s" stop-opacity="0"/><stop offset="50%%" stop-color="%s" stop-opacity="0.85"/><stop offset="100%%" stop-color="%s" stop-opacity="0"/></linearGradient>`,
		glintColor, glintColor, glintColor)
	b.WriteString(`</defs>`)

	b.WriteString(`<style>`)
	fmt.Fprintf(&b, `.f-label{font-family:%s;font-size:12.5px;font-weight:500;fill:%s}`, fontSans, p.MutedColor)
	fmt.Fprintf(&b, `.f-brand{font-family:%s;font-size:12.5px;font-weight:700;fill:%s}`, fontSans, p.HeadingColor)
	fmt.Fprintf(&b, `.f-url{font-family:%s;font-size:11.5px;font-weight:500;fill:%s}`, fontMono, p.MutedColor)
	ftDur, ftDelay := glintTiming("footer")
	b.WriteString(`@keyframes stitch-march{from{stroke-dashoffset:0}to{stroke-dashoffset:-100}}`)
	b.WriteString(`@keyframes ft-sweep{0%{transform:translateX(-220px);opacity:0}2%{opacity:1}12%{opacity:1}14%,100%{transform:translateX(880px);opacity:0}}`)
	b.WriteString(`@keyframes snip-top{0%,78%,100%{transform:rotate(0deg)}83%{transform:rotate(-8deg)}87%{transform:rotate(3deg)}91%{transform:rotate(-5deg)}95%{transform:rotate(0deg)}}`)
	b.WriteString(`@keyframes snip-bot{0%,78%,100%{transform:rotate(0deg)}83%{transform:rotate(8deg)}87%{transform:rotate(-3deg)}91%{transform:rotate(5deg)}95%{transform:rotate(0deg)}}`)
	b.WriteString(`.ft-stitch{animation:stitch-march 20s linear infinite}`)
	fmt.Fprintf(&b, `.ft-glint{opacity:0;animation:ft-sweep %.1fs ease-in-out %.1fs infinite}`, ftDur, ftDelay)
	b.WriteString(`.f-blade-top{transform-origin:20.5px 21px;animation:snip-top 10s ease-in-out infinite}`)
	b.WriteString(`.f-blade-bot{transform-origin:20.5px 21px;animation:snip-bot 10s ease-in-out infinite}`)
	b.WriteString(reducedMotionCSS)
	b.WriteString(`</style>`)

	b.WriteString(`<g clip-path="url(#footer-clip)">`)
	fmt.Fprintf(&b, `<rect x="0" y="0" width="840" height="44" fill="%s"/>`, p.CardBackground)
	b.WriteString(`<rect class="ft-glint" x="0" y="0" width="180" height="2" fill="url(#ft-glint)"/>`)

	// Miniature 20x20 inline golden Tailor shears with occasional gentle snip wiggle.
	b.WriteString(`<g transform="translate(18, 12) scale(0.5)">`)
	b.WriteString(`<g class="f-blade-top">`)
	b.WriteString(`<path d="M14 27 17.5 19.5 31 3 25 19.5 20 29Z" fill="#b88a44"/>`)
	b.WriteString(`<path d="M17.5 19.5 31 3 23 18.5Z" fill="#f0cb78"/>`)
	b.WriteString(`<path fill="#f0cb78" fill-rule="evenodd" d="M22 31a8 5.6 0 1 0-16 0 8 5.6 0 1 0 16 0Zm-3 0a5 2.8 0 1 1-10 0 5 2.8 0 1 1 10 0Z" transform="rotate(-40 14 31)"/>`)
	b.WriteString(`</g>`)
	b.WriteString(`<g class="f-blade-bot">`)
	b.WriteString(`<path d="M11 18.5 19 19 37 9 25 21.5Q20 25 12 23Z" fill="#b88a44"/>`)
	b.WriteString(`<path d="M11 18.5 19 19 37 9 22 19.5Z" fill="#f0cb78"/>`)
	b.WriteString(`<path fill="#b88a44" fill-rule="evenodd" d="M14 20a6 5.5 0 1 0-12 0 6 5.5 0 1 0 12 0Zm-3 0a3 2.7 0 1 1-6 0 3 2.7 0 1 1 6 0Z" transform="rotate(-20 8 20)"/>`)
	b.WriteString(`</g>`)
	b.WriteString(`<circle cx="20.5" cy="21" r="2.1" fill="#594020"/>`)
	b.WriteString(`<circle cx="20.5" cy="21" r="0.9" fill="#f5efe6"/>`)
	b.WriteString(`</g>`)

	b.WriteString(`<text x="48" y="27" class="f-label">Fitted by <tspan class="f-brand">Tailor</tspan></text>`)
	b.WriteString(`<text x="820" y="26" text-anchor="end" class="f-url">tailor.wimpys.world</text>`)

	b.WriteString(`</g>`)
	fmt.Fprintf(&b, `<rect class="ft-stitch" x="0.75" y="0.75" width="838.5" height="42.5" rx="10" fill="none" stroke="%s" stroke-width="1.25" stroke-dasharray="5 5"/>`, cardBorder(p))
	b.WriteString(`</svg>`)
	return b.String()
}

func displayFooterURL(raw string) string {
	s := strings.TrimSpace(raw)
	s = strings.TrimPrefix(s, "https://")
	s = strings.TrimPrefix(s, "http://")
	s = strings.TrimRight(s, "/")
	return s
}

func normalizePalette(p, fallback Palette) Palette {
	pick := func(v, fb string) string {
		if strings.TrimSpace(v) == "" {
			return fb
		}
		return v
	}
	return Palette{
		Background:        pick(p.Background, fallback.Background),
		CardBackground:    pick(p.CardBackground, fallback.CardBackground),
		CodeBackground:    pick(p.CodeBackground, fallback.CodeBackground),
		Color:             pick(p.Color, fallback.Color),
		MutedColor:        pick(p.MutedColor, fallback.MutedColor),
		MutedBorder:       pick(p.MutedBorder, fallback.MutedBorder),
		Primary:           pick(p.Primary, fallback.Primary),
		PrimaryBackground: pick(p.PrimaryBackground, fallback.PrimaryBackground),
		HeadingColor:      pick(p.HeadingColor, fallback.HeadingColor),
	}
}

// cardBorder lifts dark-theme outer borders slightly so cards separate cleanly
// from dark GitHub canvases while leaving light-theme borders unchanged.
func cardBorder(p Palette) string {
	c, ok := parseHexColor(p.CardBackground)
	if ok && (0.299*c.r+0.587*c.g+0.114*c.b) < 128 {
		return mixHex("#ffffff", p.MutedBorder, 0.12)
	}
	return p.MutedBorder
}

// mixHex blends hex1 and hex2 in sRGB where weight is the fraction of hex1 (0..1).
func mixHex(hex1, hex2 string, weight float64) string {
	c1, ok1 := parseHexColor(hex1)
	c2, ok2 := parseHexColor(hex2)
	if !ok1 {
		return hex2
	}
	if !ok2 {
		return hex1
	}
	w := min(max(weight, 0), 1)
	return rgbColor{
		r: c1.r*w + c2.r*(1-w),
		g: c1.g*w + c2.g*(1-w),
		b: c1.b*w + c2.b*(1-w),
		a: 1,
	}.hex()
}

// contrastOnFill returns a high-contrast dark or light foreground hex colour
// for text drawn on top of bgHex, using WCAG relative luminance.
func contrastOnFill(bgHex string) string {
	c, ok := parseHexColor(bgHex)
	if !ok {
		return "#ffffff"
	}
	lum := 0.2126*linearizeChannel(c.r/255.0) +
		0.7152*linearizeChannel(c.g/255.0) +
		0.0722*linearizeChannel(c.b/255.0)
	if lum > 0.35 {
		return "#111418"
	}
	return "#ffffff"
}

func linearizeChannel(v float64) float64 {
	if v <= 0.03928 {
		return v / 12.92
	}
	return math.Pow((v+0.055)/1.055, 2.4)
}
