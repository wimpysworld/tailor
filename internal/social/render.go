package social

import (
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"math"
	"strings"
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
	fmt.Fprintf(&b, `<linearGradient id="hero-aurora" x1="0" y1="0" x2="1" y2="1"><stop offset="0%%" stop-color="%s" stop-opacity="%s"/><stop offset="100%%" stop-color="%s" stop-opacity="0"/></linearGradient>`,
		p.Primary, waveOpacity1, p.Primary)
	fmt.Fprintf(&b, `<linearGradient id="hero-aurora-soft" x1="1" y1="0" x2="0" y2="1"><stop offset="0%%" stop-color="%s" stop-opacity="%s"/><stop offset="100%%" stop-color="%s" stop-opacity="0"/></linearGradient>`,
		p.Primary, waveOpacity2, p.Primary)
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
	b.WriteString(`@keyframes hero-wave{0%{transform:translateX(-18px)}100%{transform:translateX(18px)}}`)
	b.WriteString(`@keyframes hero-glint-move{0%{transform:translateX(-260px);opacity:0}2%{opacity:1}12%{opacity:1}14%,100%{transform:translateX(900px);opacity:0}}`)
	b.WriteString(`.wave-a{animation:hero-wave 14s ease-in-out infinite alternate}`)
	fmt.Fprintf(&b, `.glint{animation:hero-glint-move %.1fs ease-in-out %.1fs infinite}`, heroDur, heroDelay)
	b.WriteString(reducedMotionCSS)
	b.WriteString(`</style>`)

	b.WriteString(`<g clip-path="url(#hero-clip)">`)
	b.WriteString(`<rect x="0" y="0" width="840" height="210" fill="url(#hero-bg)"/>`)
	// Calm harmonic aurora waves in Primary.
	b.WriteString(`<path class="wave-a" d="M-40 162 C180 112 360 198 580 128 C700 90 790 118 890 88 L890 220 L-40 220 Z" fill="url(#hero-aurora)"/>`)
	b.WriteString(`<path d="M-20 176 C220 144 430 196 660 146 C750 126 810 140 870 122 L870 220 L-20 220 Z" fill="url(#hero-aurora-soft)"/>`)
	// Subtle top accent bar and gliding glint.
	fmt.Fprintf(&b, `<rect x="0" y="0" width="840" height="3" fill="%s" opacity="0.28"/>`, p.Primary)
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

	// Quiet muted footer separated by a subtle hairline.
	fmt.Fprintf(&b, `<line x1="36" y1="162" x2="804" y2="162" stroke="%s" stroke-width="1"/>`, p.MutedBorder)
	if footerURL != "" {
		fmt.Fprintf(&b, `<text x="36" y="187" class="h-foot">%s</text>`, XMLEscape(footerURL))
	}
	if platforms != "" {
		fmt.Fprintf(&b, `<text x="804" y="187" text-anchor="end" class="h-foot">%s</text>`, XMLEscape(platforms))
	}

	b.WriteString(`</g>`)
	fmt.Fprintf(&b, `<rect x="0.5" y="0.5" width="839" height="209" rx="14" fill="none" stroke="%s" stroke-width="1"/>`, p.MutedBorder)
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
	border := mixHex(accent, p.MutedBorder, 0.35)
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
	fmt.Fprintf(&b, `.p-glint{animation:pill-sweep %.1fs ease-in-out %.1fs infinite}`, pillDur, pillDelay)
	b.WriteString(reducedMotionCSS)
	b.WriteString(`</style>`)

	b.WriteString(`<g clip-path="url(#pill-clip)">`)
	fmt.Fprintf(&b, `<rect x="0" y="0" width="184" height="44" fill="%s"/>`, p.CardBackground)
	fmt.Fprintf(&b, `<rect x="0" y="0" width="184" height="2" fill="%s" opacity="0.22"/>`, accent)
	b.WriteString(`<rect class="p-glint" x="0" y="0" width="76" height="2" fill="url(#pill-glint)"/>`)
	// 20x20 inline vector icon.
	fmt.Fprintf(&b, `<g transform="translate(14, 12) scale(0.833333)" fill="%s"><path d="%s"/></g>`, accent, icon.Path)
	fmt.Fprintf(&b, `<text x="44" y="27" class="p-label">%s</text>`, XMLEscape(displayLabel))
	// Quiet right chevron indicator.
	fmt.Fprintf(&b, `<path d="M167 17.5 L171.5 22 L167 26.5" fill="none" stroke="%s" stroke-width="1.6" stroke-linecap="round" stroke-linejoin="round"/>`, p.MutedColor)
	b.WriteString(`</g>`)
	fmt.Fprintf(&b, `<rect x="0.5" y="0.5" width="183" height="43" rx="10" fill="none" stroke="%s" stroke-width="1"/>`, border)
	b.WriteString(`</svg>`)
	return b.String()
}

// RenderCommunityBanner renders the 840x68 full-width themed community banner
// for discord, matrix, slack, irc, forum, or discussions.
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
		fmt.Fprintf(&b, `.c-glint{animation:comm-sweep %.1fs ease-in-out %.1fs infinite}`, commDur, commDelay)
	}
	b.WriteString(reducedMotionCSS)
	b.WriteString(`</style>`)

	b.WriteString(`<g clip-path="url(#comm-clip)">`)
	fmt.Fprintf(&b, `<rect x="0" y="0" width="840" height="68" fill="%s"/>`, p.CardBackground)
	fmt.Fprintf(&b, `<rect x="0" y="0" width="4" height="68" fill="%s"/>`, p.Primary)
	if liveText == "" {
		fmt.Fprintf(&b, `<rect x="0" y="0" width="840" height="2" fill="%s" opacity="0.22"/>`, p.Primary)
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
		liveW := len([]rune(liveText))*7 + 28
		if liveW < 100 {
			liveW = 100
		}
		if liveW > 240 {
			liveW = 240
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
	fmt.Fprintf(&b, `<rect x="0.5" y="0.5" width="839" height="67" rx="12" fill="none" stroke="%s" stroke-width="1"/>`, p.MutedBorder)
	b.WriteString(`</svg>`)
	return b.String()
}

// RenderPackageCard renders a 198x78 themed Haberdasher clickable package tile
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
		width  = 198
		height = 78
	)

	accent := info.AccentLight
	if dark {
		accent = info.AccentDark
	}
	border := mixHex(accent, p.MutedBorder, 0.35)

	var b strings.Builder
	fmt.Fprintf(&b, `<svg xmlns="http://www.w3.org/2000/svg" width="%d" height="%d" viewBox="0 0 %d %d" role="img" aria-label="%s">`,
		width, height, width, height, XMLEscape(info.Label))
	fmt.Fprintf(&b, `<title>%s (%s)</title>`, XMLEscape(info.Label), XMLEscape(info.Domain))
	b.WriteString(`<defs>`)
	b.WriteString(`<clipPath id="pkg-clip"><rect x="0.5" y="0.5" width="197" height="77" rx="11"/></clipPath>`)
	fmt.Fprintf(&b, `<linearGradient id="pkg-glint" x1="0" y1="0" x2="1" y2="0"><stop offset="0%%" stop-color="%s" stop-opacity="0"/><stop offset="50%%" stop-color="%s" stop-opacity="0.85"/><stop offset="100%%" stop-color="%s" stop-opacity="0"/></linearGradient>`,
		accent, accent, accent)
	b.WriteString(`</defs>`)

	b.WriteString(`<style>`)
	fmt.Fprintf(&b, `.k-label{font-family:%s;font-size:12px;font-weight:700;fill:%s}`, fontSans, p.HeadingColor)
	fmt.Fprintf(&b, `.k-domain{font-family:%s;font-size:10px;font-weight:500;fill:%s}`, fontMono, p.MutedColor)
	pkgDur, pkgDelay := glintTiming(host)
	b.WriteString(`@keyframes pkg-sweep{0%{transform:translateX(-90px);opacity:0}2%{opacity:1}12%{opacity:1}14%,100%{transform:translateX(220px);opacity:0}}`)
	fmt.Fprintf(&b, `.k-glint{animation:pkg-sweep %.1fs ease-in-out %.1fs infinite}`, pkgDur, pkgDelay)
	b.WriteString(reducedMotionCSS)
	b.WriteString(`</style>`)

	b.WriteString(`<g clip-path="url(#pkg-clip)">`)
	fmt.Fprintf(&b, `<rect x="0" y="0" width="198" height="78" fill="%s"/>`, p.CardBackground)
	fmt.Fprintf(&b, `<rect x="0" y="0" width="198" height="2.5" fill="%s" opacity="0.25"/>`, accent)
	b.WriteString(`<rect class="k-glint" x="0" y="0" width="80" height="2.5" fill="url(#pkg-glint)"/>`)

	// 36x36 inline host icon (24 * 1.5 = 36).
	fmt.Fprintf(&b, `<g transform="translate(14, 21) scale(1.5)" fill="%s"><path d="%s"/></g>`, accent, info.Path)
	fmt.Fprintf(&b, `<text x="60" y="34" class="k-label">%s</text>`, XMLEscape(info.Label))
	fmt.Fprintf(&b, `<text x="60" y="52" class="k-domain">%s</text>`, XMLEscape(info.Domain))

	b.WriteString(`</g>`)
	fmt.Fprintf(&b, `<rect x="0.5" y="0.5" width="197" height="77" rx="11" fill="none" stroke="%s" stroke-width="1"/>`, border)
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
