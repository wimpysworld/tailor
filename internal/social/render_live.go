package social

import (
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"math"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

const (
	starGlyphPath  = "M12 2.4l2.92 5.92 6.53.95-4.72 4.6 1.11 6.5L12 17.3l-5.84 3.07 1.11-6.5-4.72-4.6 6.53-.95L12 2.4Z"
	heartGlyphPath = "M12 21.35l-1.45-1.32C5.4 15.36 2 12.28 2 8.5 2 5.42 4.42 3 7.5 3c1.74 0 3.41.81 4.5 2.09C13.09 3.81 14.76 3 16.5 3 19.58 3 22 5.42 22 8.5c0 3.78-3.4 6.86-8.55 11.54L12 21.35Z"
	apexStarPath   = "M0 -9.5L2.47 -3.4L9.04 -2.94L3.99 1.3L5.58 7.69L0 4.2L-5.58 7.69L-3.99 1.3L-9.04 -2.94L-2.47 -3.4Z"
	miniStarPath   = "M0 -4.6L1.2 -1.65L4.38 -1.42L1.93 0.63L2.7 3.72L0 2L-2.7 3.72L-1.93 0.63L-4.38 -1.42L-1.2 -1.65Z"
	sparklePath    = "M0 -4.8L1.3 -1.3L4.8 0L1.3 1.3L0 4.8L-1.3 1.3L-4.8 0L-1.3 -1.3Z"
)

var (
	sponsorDarkHeartColors  = [5]string{"#f38ba8", "#fab387", "#cba6f7", "#b4befe", "#f5c2e7"}
	sponsorLightHeartColors = [5]string{"#e64553", "#fe640b", "#8839ef", "#1e66f5", "#ea76cb"}
)

// StatsInput holds the four live project health metrics displayed in RenderStats.
type StatsInput struct {
	Commits      int
	OpenIssues   int
	ClosedIssues int
	OpenPRs      int
	MergedPRs    int
	CreatedAt    time.Time
}

// StarPoint represents a cumulative star count on a specific date.
type StarPoint struct {
	Date  time.Time
	Stars int
}

// StarHistoryInput configures the animated star-history chart rendered by
// RenderStarHistory.
type StarHistoryInput struct {
	Points       []StarPoint
	MinStars     int
	MinDays      int
	Title        string
	ProjectTitle string
}

// SponsorsInput configures the fixed-height sponsors CTA banner rendered by
// RenderSponsors.
type SponsorsInput struct {
	Title    string
	Subtitle string
	CTALabel string
}

// RenderStats renders the 840x98 four-column project health strip in dark and
// light theme variants.
func RenderStats(in StatsInput, theme Theme) (SVGPair, error) {
	darkP := normalizePalette(theme.Dark, defaultDarkPalette)
	lightP := normalizePalette(theme.Light, defaultLightPalette)

	return SVGPair{
		Dark:  renderStatsVariant(in, darkP),
		Light: renderStatsVariant(in, lightP),
	}, nil
}

type statColumn struct {
	label string
	value string
	mono  bool
}

func renderStatsVariant(in StatsInput, p Palette) string {
	const (
		width  = 840
		height = 98
		colW   = 210
	)

	established := "—"
	if !in.CreatedAt.IsZero() {
		established = in.CreatedAt.UTC().Format("Jan 2006")
	}

	cols := []statColumn{
		{
			label: "COMMITS",
			value: formatCount(max(in.Commits, 0)),
			mono:  true,
		},
		{
			label: "ISSUES",
			value: fmt.Sprintf("%s open · %s closed", formatCount(max(in.OpenIssues, 0)), formatCount(max(in.ClosedIssues, 0))),
			mono:  false,
		},
		{
			label: "PULL REQUESTS",
			value: fmt.Sprintf("%s merged · %s open", formatCount(max(in.MergedPRs, 0)), formatCount(max(in.OpenPRs, 0))),
			mono:  false,
		},
		{
			label: "ESTABLISHED",
			value: established,
			mono:  false,
		},
	}

	var b strings.Builder
	fmt.Fprintf(&b, `<svg xmlns="http://www.w3.org/2000/svg" width="%d" height="%d" viewBox="0 0 %d %d" role="img" aria-label="Project statistics">`,
		width, height, width, height)
	b.WriteString(`<title>Project statistics</title>`)
	b.WriteString(`<defs>`)
	b.WriteString(`<clipPath id="stats-clip"><rect x="0.5" y="0.5" width="839" height="97" rx="12"/></clipPath>`)
	b.WriteString(`</defs>`)

	b.WriteString(`<style>`)
	fmt.Fprintf(&b, `.s-label{font-family:%s;font-size:10.5px;font-weight:600;fill:%s;letter-spacing:0.08em}`, fontSans, p.MutedColor)
	fmt.Fprintf(&b, `.s-val-sans{font-family:%s;font-size:14px;font-weight:700;fill:%s}`, fontSans, p.HeadingColor)
	fmt.Fprintf(&b, `.s-val-mono{font-family:%s;font-size:14px;font-weight:700;fill:%s}`, fontMono, p.HeadingColor)
	b.WriteString(`@keyframes stat-pulse{0%,100%{opacity:0.45;transform:scale(0.92)}50%{opacity:1;transform:scale(1.15)}}`)
	b.WriteString(`.s-dot{transform-box:fill-box;transform-origin:center;animation:stat-pulse 3.4s ease-in-out infinite}`)
	b.WriteString(`.d0{animation-delay:0s}.d1{animation-delay:0.7s}.d2{animation-delay:1.4s}.d3{animation-delay:2.1s}`)
	b.WriteString(reducedMotionCSS)
	b.WriteString(`</style>`)

	b.WriteString(`<g clip-path="url(#stats-clip)">`)
	fmt.Fprintf(&b, `<rect x="0" y="0" width="840" height="98" fill="%s"/>`, p.CardBackground)

	for i, col := range cols {
		if i > 0 {
			divX := i * colW
			fmt.Fprintf(&b, `<line x1="%d" y1="20" x2="%d" y2="78" stroke="%s" stroke-width="1"/>`, divX, divX, p.MutedBorder)
		}

		centerX := i*colW + colW/2
		labelWidth := len(col.label)*7 + 12
		dotX := centerX - labelWidth/2 + 3
		labelX := dotX + 10

		fmt.Fprintf(&b, `<circle class="s-dot d%d" cx="%d" cy="30" r="3" fill="%s"/>`, i, dotX, p.Primary)
		fmt.Fprintf(&b, `<text x="%d" y="34" class="s-label">%s</text>`, labelX, XMLEscape(col.label))

		val := col.value
		if wrapped := WrapText(val, 28, 1); len(wrapped) > 0 {
			val = wrapped[0]
		}
		cls := "s-val-sans"
		if col.mono {
			cls = "s-val-mono"
		}
		fmt.Fprintf(&b, `<text x="%d" y="67" text-anchor="middle" class="%s">%s</text>`,
			centerX, cls, XMLEscape(val))
	}

	b.WriteString(`</g>`)
	fmt.Fprintf(&b, `<rect x="0.5" y="0.5" width="839" height="97" rx="12" fill="none" stroke="%s" stroke-width="1"/>`, cardBorder(p))
	b.WriteString(`</svg>`)
	return b.String()
}

// RenderStarHistory renders the 840x280 animated star-history chart in dark and
// light variants. When the data is empty or falls below MinStars or MinDays, it
// renders an 840x108 themed "Star on GitHub" call-to-action banner instead and
// still returns ok=true.
func RenderStarHistory(in StarHistoryInput, theme Theme) (SVGPair, bool, error) {
	darkP := normalizePalette(theme.Dark, defaultDarkPalette)
	lightP := normalizePalette(theme.Light, defaultLightPalette)

	if len(in.Points) == 0 {
		return renderStarCTAPair(in.ProjectTitle, darkP, lightP), true, nil
	}

	pts := slices.Clone(in.Points)
	slices.SortFunc(pts, func(a, b StarPoint) int {
		return a.Date.Compare(b.Date)
	})

	latestStars := pts[len(pts)-1].Stars
	maxStars := latestStars
	for _, pt := range pts {
		if pt.Stars > maxStars {
			maxStars = pt.Stars
		}
	}
	spanDays := int(pts[len(pts)-1].Date.Sub(pts[0].Date).Hours() / 24)

	if (in.MinStars > 0 && latestStars < in.MinStars) || (in.MinDays > 0 && spanDays < in.MinDays) {
		return renderStarCTAPair(in.ProjectTitle, darkP, lightP), true, nil
	}

	title := strings.TrimSpace(in.Title)
	if title == "" {
		title = "Star History"
	}

	return SVGPair{
		Dark:  renderStarHistoryVariant(pts, maxStars, latestStars, title, darkP, true),
		Light: renderStarHistoryVariant(pts, maxStars, latestStars, title, lightP, false),
	}, true, nil
}

func renderStarCTAPair(projectTitle string, darkP, lightP Palette) SVGPair {
	proj := strings.TrimSpace(projectTitle)
	title := "Star this repository on GitHub"
	if proj != "" {
		title = "Star " + proj + " on GitHub"
	}
	if wrapped := WrapText(title, 52, 1); len(wrapped) > 0 {
		title = wrapped[0]
	}
	const subtitle = "Follow new releases and help others discover the project"

	return SVGPair{
		Dark:  renderStarCTAVariant(title, subtitle, darkP, true),
		Light: renderStarCTAVariant(title, subtitle, lightP, false),
	}
}

func renderStarCTAVariant(title, subtitle string, p Palette, dark bool) string {
	const (
		width  = 840
		height = 108
	)

	ctaBg := p.PrimaryBackground
	if ctaBg == "" {
		ctaBg = p.Primary
	}
	ctaFg := contrastOnFill(ctaBg)

	starOp := "0.10"
	if dark {
		starOp = "0.15"
	}

	var b strings.Builder
	fmt.Fprintf(&b, `<svg xmlns="http://www.w3.org/2000/svg" width="%d" height="%d" viewBox="0 0 %d %d" role="img" aria-label="%s">`,
		width, height, width, height, XMLEscape(title))
	fmt.Fprintf(&b, `<title>%s</title>`, XMLEscape(title))
	b.WriteString(`<defs>`)
	fmt.Fprintf(&b, `<clipPath id="st-clip"><rect x="0.5" y="0.5" width="839" height="%d" rx="14"/></clipPath>`, height-1)
	b.WriteString(`</defs>`)

	b.WriteString(`<style>`)
	fmt.Fprintf(&b, `.st-title{font-family:%s;font-size:15.5px;font-weight:700;fill:%s}`, fontSans, p.HeadingColor)
	fmt.Fprintf(&b, `.st-sub{font-family:%s;font-size:12.5px;font-weight:400;fill:%s}`, fontSans, p.MutedColor)
	fmt.Fprintf(&b, `.st-empty{font-family:%s;font-size:12.5px;font-weight:500;fill:%s}`, fontSans, p.MutedColor)
	fmt.Fprintf(&b, `.st-cta{font-family:%s;font-size:11.5px;font-weight:700;fill:%s;letter-spacing:0.06em}`, fontSans, ctaFg)
	b.WriteString(`@keyframes star-drift{0%{transform:translateY(8px) scale(0.86);opacity:0}25%{opacity:1}55%{transform:translateY(-4px) scale(1.08);opacity:0.75}80%{opacity:1}100%{transform:translateY(-14px) scale(0.92);opacity:0}}`)
	b.WriteString(`.st-star{animation:star-drift 7.5s ease-in-out infinite}`)
	b.WriteString(`.sd1{animation-delay:0s}.sd2{animation-delay:2.5s}.sd3{animation-delay:5s}`)
	b.WriteString(reducedMotionCSS)
	b.WriteString(`</style>`)

	b.WriteString(`<g clip-path="url(#st-clip)">`)
	fmt.Fprintf(&b, `<rect x="0" y="0" width="840" height="%d" fill="%s"/>`, height, p.CardBackground)

	// Subtle drifting and twinkling 5-point stars in Primary.
	fmt.Fprintf(&b, `<g opacity="%s" fill="%s">`, starOp, p.Primary)
	fmt.Fprintf(&b, `<g class="st-star sd1"><g transform="translate(560, %d) scale(0.65)"><path d="%s"/></g></g>`, height-36, starGlyphPath)
	fmt.Fprintf(&b, `<g class="st-star sd2"><g transform="translate(625, %d) scale(0.50)"><path d="%s"/></g></g>`, height-46, starGlyphPath)
	fmt.Fprintf(&b, `<g class="st-star sd3"><g transform="translate(675, %d) scale(0.58)"><path d="%s"/></g></g>`, height-32, starGlyphPath)
	b.WriteString(`</g>`)

	// Header star icon badge.
	fmt.Fprintf(&b, `<rect x="24" y="18" width="34" height="34" rx="9" fill="%s" stroke="%s" stroke-width="1"/>`, p.CodeBackground, p.MutedBorder)
	fmt.Fprintf(&b, `<g transform="translate(32, 26) scale(0.75)" fill="%s"><path d="%s"/></g>`, p.Primary, starGlyphPath)

	fmt.Fprintf(&b, `<text x="68" y="32" class="st-title">%s</text>`, XMLEscape(title))
	fmt.Fprintf(&b, `<text x="68" y="50" class="st-sub">%s</text>`, XMLEscape(subtitle))

	// STAR CTA pill.
	fmt.Fprintf(&b, `<rect x="720" y="19" width="96" height="32" rx="8" fill="%s"/>`, ctaBg)
	fmt.Fprintf(&b, `<g transform="translate(734, 28) scale(0.583333)" fill="%s"><path d="%s"/></g>`, ctaFg, starGlyphPath)
	fmt.Fprintf(&b, `<text x="778" y="39" text-anchor="middle" class="st-cta">STAR</text>`)

	fmt.Fprintf(&b, `<rect x="24" y="68" width="792" height="26" rx="8" fill="%s" stroke="%s" stroke-width="1" stroke-dasharray="4 3"/>`,
		p.CodeBackground, p.MutedBorder)
	b.WriteString(`<text x="420" y="85" text-anchor="middle" class="st-empty">Starring helps keep the project active and easy to find on GitHub</text>`)

	b.WriteString(`</g>`)
	fmt.Fprintf(&b, `<rect x="0.5" y="0.5" width="839" height="%d" rx="14" fill="none" stroke="%s" stroke-width="1"/>`, height-1, cardBorder(p))
	b.WriteString(`</svg>`)
	return b.String()
}

type chartXY struct {
	x float64
	y float64
}

type fountainParticleSpec struct {
	dx        float64
	dyUp      float64
	launchPct float64
	peakPct   float64
	endPct    float64
	peakScale float64
	kind      string // "star", "sparkle", or "dot"
	colorIdx  int    // 0:#facc15, 1:#fffbeb, 2:#fb923c, 3:#f472b6, 4:p.Primary
}

var arcadeFountainSpecs = [14]fountainParticleSpec{
	{dx: -18, dyUp: -52, launchPct: 14.0, peakPct: 19.2, endPct: 25.0, peakScale: 1.10, kind: "star", colorIdx: 0},
	{dx: 12, dyUp: -48, launchPct: 14.2, peakPct: 19.0, endPct: 24.6, peakScale: 1.00, kind: "sparkle", colorIdx: 1},
	{dx: -38, dyUp: -44, launchPct: 14.3, peakPct: 19.8, endPct: 25.6, peakScale: 0.95, kind: "sparkle", colorIdx: 2},
	{dx: 24, dyUp: -36, launchPct: 14.1, peakPct: 18.8, endPct: 24.2, peakScale: 0.85, kind: "dot", colorIdx: 3},
	{dx: -6, dyUp: -56, launchPct: 14.5, peakPct: 20.0, endPct: 25.8, peakScale: 1.05, kind: "sparkle", colorIdx: 4},
	{dx: -52, dyUp: -30, launchPct: 14.6, peakPct: 20.2, endPct: 25.5, peakScale: 0.90, kind: "star", colorIdx: 0},
	{dx: 19, dyUp: -44, launchPct: 14.8, peakPct: 19.6, endPct: 25.0, peakScale: 0.80, kind: "star", colorIdx: 2},
	{dx: -26, dyUp: -36, launchPct: 14.4, peakPct: 18.9, endPct: 24.0, peakScale: 0.85, kind: "dot", colorIdx: 1},
	{dx: 6, dyUp: -38, launchPct: 15.0, peakPct: 19.8, endPct: 24.8, peakScale: 0.95, kind: "dot", colorIdx: 4},
	{dx: -44, dyUp: -50, launchPct: 15.1, peakPct: 20.5, endPct: 26.0, peakScale: 1.00, kind: "sparkle", colorIdx: 3},
	{dx: 26, dyUp: -24, launchPct: 14.9, peakPct: 19.2, endPct: 23.8, peakScale: 0.90, kind: "sparkle", colorIdx: 0},
	{dx: -12, dyUp: -30, launchPct: 15.3, peakPct: 19.9, endPct: 24.5, peakScale: 0.80, kind: "dot", colorIdx: 2},
	{dx: -32, dyUp: -20, launchPct: 15.2, peakPct: 19.5, endPct: 24.2, peakScale: 0.85, kind: "star", colorIdx: 1},
	{dx: 16, dyUp: -54, launchPct: 15.5, peakPct: 20.8, endPct: 26.0, peakScale: 1.05, kind: "star", colorIdx: 4},
}

type cometTrailSpec struct {
	prog       float64
	driftBack  float64
	sideSpread float64
	gravityY   float64
	kind       string
	colorIdx   int
}

var cometTrailSpecs = [6]cometTrailSpec{
	{prog: 0.05, driftBack: 26, sideSpread: -9, gravityY: 10, kind: "sparkle", colorIdx: 1},
	{prog: 0.18, driftBack: 32, sideSpread: 10, gravityY: 12, kind: "dot", colorIdx: 0},
	{prog: 0.34, driftBack: 36, sideSpread: -12, gravityY: 11, kind: "star", colorIdx: 2},
	{prog: 0.52, driftBack: 30, sideSpread: 11, gravityY: 14, kind: "sparkle", colorIdx: 3},
	{prog: 0.70, driftBack: 38, sideSpread: -8, gravityY: 10, kind: "dot", colorIdx: 4},
	{prog: 0.86, driftBack: 28, sideSpread: 9, gravityY: 9, kind: "star", colorIdx: 0},
}

type cometTrailParticle struct {
	x        float64
	y        float64
	dx       float64
	dy       float64
	startPct float64
	peakPct  float64
	endPct   float64
	kind     string
	color    string
}

func renderStarHistoryVariant(pts []StarPoint, maxStars, latestStars int, title string, p Palette, dark bool) string {
	const (
		width  = 840
		height = 280
		plotL  = 64.0
		plotR  = 804.0
		plotT  = 66.0
		plotB  = 236.0
		exitX  = 48.0
		exitY  = 22.0
	)
	plotW := plotR - plotL
	plotH := plotB - plotT

	yMax := niceAxisMax(maxStars)
	coords := mapStarCoords(pts, plotL, plotW, plotT, plotH, float64(yMax))
	linePath, areaPath, sampledCoords, pathLen := buildSmoothChartPaths(coords, plotB)

	gradTopOp := "0.24"
	if dark {
		gradTopOp = "0.30"
	}

	dateRange := formatStarDateRange(pts[0].Date, pts[len(pts)-1].Date)
	badgeLabel := formatCount(latestStars) + " stars"
	badgeW := max(int(math.Ceil(float64(len(badgeLabel))*8.5))+34, 106)
	badgeX := int(plotR) - badgeW
	badgeBorder := mixHex(p.Primary, p.MutedBorder, 0.45)

	last := coords[len(coords)-1]
	midY := (last.y + plotB) / 2.0
	tailAngleDeg := math.Atan2(midY-exitY, last.x-exitX) * 180.0 / math.Pi

	sparkPalette := [5]string{"#facc15", "#fffbeb", "#fb923c", "#f472b6", p.Primary}
	trailParticles := buildCometTrailParticles(last.x, midY, exitX, exitY, sparkPalette)

	var b strings.Builder
	fmt.Fprintf(&b, `<svg xmlns="http://www.w3.org/2000/svg" width="%d" height="%d" viewBox="0 0 %d %d" role="img" aria-label="%s">`,
		width, height, width, height, XMLEscape(title))
	fmt.Fprintf(&b, `<title>%s: %s</title>`, XMLEscape(title), XMLEscape(badgeLabel))
	b.WriteString(`<defs>`)
	b.WriteString(`<clipPath id="star-clip"><rect x="0.5" y="0.5" width="839" height="279" rx="14"/></clipPath>`)
	fmt.Fprintf(&b, `<linearGradient id="star-area" x1="0" y1="0" x2="0" y2="1"><stop offset="0%%" stop-color="%s" stop-opacity="%s"/><stop offset="100%%" stop-color="%s" stop-opacity="0"/></linearGradient>`,
		p.Primary, gradTopOp, p.Primary)
	b.WriteString(`<linearGradient id="star-comet-glow" x1="0" y1="0" x2="1" y2="0"><stop offset="0%" stop-color="#facc15" stop-opacity="0.82"/><stop offset="45%" stop-color="#fb923c" stop-opacity="0.42"/><stop offset="100%" stop-color="#f472b6" stop-opacity="0"/></linearGradient>`)
	b.WriteString(`<linearGradient id="star-comet-core" x1="0" y1="0" x2="1" y2="0"><stop offset="0%" stop-color="#fffbeb" stop-opacity="0.96"/><stop offset="55%" stop-color="#facc15" stop-opacity="0.55"/><stop offset="100%" stop-color="#facc15" stop-opacity="0"/></linearGradient>`)
	b.WriteString(`</defs>`)

	b.WriteString(`<style>`)
	fmt.Fprintf(&b, `.sh-title{font-family:%s;font-size:15px;font-weight:700;fill:%s}`, fontSans, p.HeadingColor)
	fmt.Fprintf(&b, `.sh-range{font-family:%s;font-size:11.5px;font-weight:400;fill:%s}`, fontMono, p.MutedColor)
	fmt.Fprintf(&b, `.sh-axis{font-family:%s;font-size:11px;font-weight:500;fill:%s}`, fontMono, p.MutedColor)
	fmt.Fprintf(&b, `.sh-badge{font-family:%s;font-size:12px;font-weight:600;fill:%s}`, fontMono, p.HeadingColor)
	writeStarLineKeyframes(&b, sampledCoords, pathLen)
	b.WriteString(`@keyframes sh-area-loop{0%,3%{opacity:0}14%,98%{opacity:1}100%{opacity:0}}`)
	b.WriteString(`@keyframes sh-collapse-loop{0%,84%{transform:scaleY(1);opacity:1;animation-timing-function:cubic-bezier(0.55,0.05,0.9,0.35)}93%{transform:scaleY(0);opacity:1}98%,100%{transform:scaleY(0);opacity:0}}`)
	writeStarRaceKeyframes(&b, sampledCoords, last, midY, exitX, exitY)
	b.WriteString(`@keyframes sh-star-spin-loop{0%{transform:rotate(0deg);animation-timing-function:cubic-bezier(0.32,0,0.72,0.25)}14%,84%{transform:rotate(360deg)}87%{transform:rotate(390deg);animation-timing-function:cubic-bezier(0.42,0,0.85,0.28)}94%,100%{transform:rotate(1110deg)}}`)
	b.WriteString(`@keyframes sh-apex-loop{0%,13.5%{transform:scale(0.95)}14%{transform:scale(1)}16.5%{transform:scale(1.65)}20%{transform:scale(0.88)}24%{transform:scale(1)}36%{transform:scale(1.06)}48%{transform:scale(0.98)}60%{transform:scale(1.06)}72%{transform:scale(0.98)}84%{transform:scale(1)}87%{transform:scale(1.22)}91%{transform:scale(1.05)}95%,100%{transform:scale(0.6)}}`)
	b.WriteString(`@keyframes sh-halo-loop{0%,13.8%{transform:scale(0.3);opacity:0}15.2%{transform:scale(1.1);opacity:0.9}19%,100%{transform:scale(3.8);opacity:0}}`)
	b.WriteString(`@keyframes sh-comet-tail-loop{0%,86.8%{transform:scaleX(0.05);opacity:0}88.2%{transform:scaleX(1);opacity:0.95}92.5%{transform:scaleX(1.18);opacity:0.9}94%,100%{transform:scaleX(0.25);opacity:0}}`)
	for i, sp := range arcadeFountainSpecs {
		dx2 := sp.dx * 1.25
		dy2 := sp.dyUp + 18.0
		fmt.Fprintf(&b, `@keyframes sh-sp%d{0%%,%.1f%%{transform:translate(0px,0px) scale(0);opacity:0;animation-timing-function:cubic-bezier(0.18,0.82,0.32,1)}%.1f%%{transform:translate(%.1fpx,%.1fpx) scale(%.2f);opacity:1;animation-timing-function:cubic-bezier(0.45,0.05,0.85,0.45)}%.1f%%,100%%{transform:translate(%.1fpx,%.1fpx) scale(0);opacity:0}}`,
			i, sp.launchPct, sp.peakPct, sp.dx, sp.dyUp, sp.peakScale, sp.endPct, dx2, dy2)
	}
	for i, tp := range trailParticles {
		fmt.Fprintf(&b, `@keyframes sh-tr%d{0%%,%.2f%%{transform:translate(0px,0px) scale(0.2);opacity:0}%.2f%%{transform:translate(%.1fpx,%.1fpx) scale(1);opacity:1}%.2f%%,100%%{transform:translate(%.1fpx,%.1fpx) scale(0);opacity:0}}`,
			i, tp.startPct, tp.peakPct, tp.dx*0.25, tp.dy*0.25, tp.endPct, tp.dx, tp.dy)
	}
	fmt.Fprintf(&b, `.sh-collapse{transform-origin:0px %.1fpx;animation:sh-collapse-loop 14s linear infinite}`, plotB)
	fmt.Fprintf(&b, `.sh-line{stroke-dasharray:%.1f;stroke-dashoffset:%.1f;animation:sh-line-loop 14s linear infinite}`, pathLen, pathLen)
	b.WriteString(`.sh-area{opacity:0;animation:sh-area-loop 14s linear infinite}`)
	fmt.Fprintf(&b, `.sh-star-pos{transform:translate(%.1fpx,%.1fpx);animation:star-race 14s linear infinite}`, last.x, last.y)
	b.WriteString(`.sh-star-spin{animation:sh-star-spin-loop 14s linear infinite}`)
	b.WriteString(`.sh-apex{animation:sh-apex-loop 14s ease-in-out infinite}`)
	b.WriteString(`.sh-halo{transform:scale(0.3);opacity:0;animation:sh-halo-loop 14s ease-out infinite}`)
	b.WriteString(`.sh-comet-tail{transform-origin:0px 0px;opacity:0;animation:sh-comet-tail-loop 14s linear infinite}`)
	for i := range arcadeFountainSpecs {
		fmt.Fprintf(&b, `.sh-sp%d{opacity:0;animation:sh-sp%d 14s linear infinite}`, i, i)
	}
	for i := range trailParticles {
		fmt.Fprintf(&b, `.sh-tr%d{opacity:0;animation:sh-tr%d 14s ease-out infinite}`, i, i)
	}
	fmt.Fprintf(&b, `@media (prefers-reduced-motion: reduce) { * { animation: none !important; } .sh-collapse { transform: scaleY(1) !important; opacity: 1 !important; } .sh-line { stroke-dashoffset: 0 !important; opacity: 1 !important; } .sh-area, .sh-star-spin, .sh-apex { opacity: 1 !important; transform: none !important; } .sh-star-pos { transform: translate(%.1fpx,%.1fpx) !important; opacity: 1 !important; } .sh-halo, .sh-sparkle, .sh-comet-tail, .sh-trail { opacity: 0 !important; } }`, last.x, last.y)
	b.WriteString(`</style>`)

	b.WriteString(`<g clip-path="url(#star-clip)">`)
	fmt.Fprintf(&b, `<rect x="0" y="0" width="840" height="280" fill="%s"/>`, p.CardBackground)

	// Header and current star count badge.
	fmt.Fprintf(&b, `<text x="28" y="32" class="sh-title">%s</text>`, XMLEscape(title))
	fmt.Fprintf(&b, `<text x="28" y="49" class="sh-range">%s</text>`, XMLEscape(dateRange))

	fmt.Fprintf(&b, `<rect x="%d" y="18" width="%d" height="28" rx="14" fill="%s" stroke="%s" stroke-width="1"/>`,
		badgeX, badgeW, p.CodeBackground, badgeBorder)
	fmt.Fprintf(&b, `<g transform="translate(%d, 25) scale(0.583333)" fill="#facc15"><path d="%s"/></g>`,
		badgeX+11, starGlyphPath)
	fmt.Fprintf(&b, `<text x="%d" y="36" class="sh-badge">%s</text>`, badgeX+29, XMLEscape(badgeLabel))

	// Horizontal Y-axis gridlines and ticks.
	const tickCount = 4
	for i := range tickCount + 1 {
		frac := float64(i) / float64(tickCount)
		y := plotB - frac*plotH
		val := int(math.Round(frac * float64(yMax)))
		dash := ` stroke-dasharray="3 4"`
		if i == 0 {
			dash = ""
		}
		fmt.Fprintf(&b, `<line x1="%.1f" y1="%.1f" x2="%.1f" y2="%.1f" stroke="%s" stroke-width="1"%s/>`,
			plotL, y, plotR, y, p.MutedBorder, dash)
		fmt.Fprintf(&b, `<text x="54" y="%.1f" text-anchor="end" class="sh-axis">%s</text>`,
			y+4, XMLEscape(formatCount(val)))
	}

	// X-axis date labels.
	xTicks := buildDateTicks(pts[0].Date, pts[len(pts)-1].Date, plotL, plotR)
	for i, tick := range xTicks {
		anchor := "middle"
		if i == 0 {
			anchor = "start"
		} else if i == len(xTicks)-1 {
			anchor = "end"
		}
		fmt.Fprintf(&b, `<text x="%.1f" y="258" text-anchor="%s" class="sh-axis">%s</text>`,
			tick.x, anchor, XMLEscape(tick.label))
	}

	// Area gradient fill + animated polyline curve inside gravity rope-collapse group.
	b.WriteString(`<g class="sh-collapse">`)
	fmt.Fprintf(&b, `<path class="sh-area" d="%s" fill="url(#star-area)"/>`, areaPath)
	fmt.Fprintf(&b, `<path class="sh-line" d="%s" fill="none" stroke="%s" stroke-width="2.75" stroke-linecap="round" stroke-linejoin="round"/>`,
		linePath, p.Primary)
	b.WriteString(`</g>`)

	// Apex burst effects anchored at the final point: crisp shockwave halo ring and 14 asymmetric fountain particles.
	fmt.Fprintf(&b, `<g transform="translate(%.1f, %.1f)">`, last.x, last.y)
	b.WriteString(`<circle class="sh-halo" cx="0" cy="0" r="7" fill="none" stroke="#facc15" stroke-width="2"/>`)
	for i, sp := range arcadeFountainSpecs {
		writeSparkParticle(&b, fmt.Sprintf("sh-sparkle sh-sp%d", i), sparkPalette[sp.colorIdx], sp.kind)
	}
	b.WriteString(`</g>`)

	// Streaming comet wake spark particles peeling off along the shooting-star exit trajectory.
	for i, tp := range trailParticles {
		fmt.Fprintf(&b, `<g transform="translate(%.1f, %.1f)">`, tp.x, tp.y)
		writeSparkParticle(&b, fmt.Sprintf("sh-trail sh-tr%d", i), tp.color, tp.kind)
		b.WriteString(`</g>`)
	}

	// Fixed yellow/gold 5-point star racing up the accelerating tip, popping at apex,
	// dropping halfway on collapse, then igniting into a diagonal shooting-star comet.
	fmt.Fprintf(&b, `<g class="sh-star-pos" transform="translate(%.1f, %.1f)">`, last.x, last.y)
	fmt.Fprintf(&b, `<g transform="rotate(%.1f)"><g class="sh-comet-tail">`, tailAngleDeg)
	b.WriteString(`<path d="M-2 0L8 -6.5L138 0L8 6.5Z" fill="url(#star-comet-glow)"/>`)
	b.WriteString(`<path d="M-1 0L6 -2.8L88 0L6 2.8Z" fill="url(#star-comet-core)"/>`)
	b.WriteString(`</g></g>`)
	fmt.Fprintf(&b, `<g class="sh-star-spin"><g class="sh-apex"><path d="%s" fill="#facc15" stroke="#ca8a04" stroke-width="1.5" stroke-linejoin="round"/></g></g>`, apexStarPath)
	b.WriteString(`</g>`)

	b.WriteString(`</g>`)
	fmt.Fprintf(&b, `<rect x="0.5" y="0.5" width="839" height="279" rx="14" fill="none" stroke="%s" stroke-width="1"/>`, cardBorder(p))
	b.WriteString(`</svg>`)
	return b.String()
}

func writeSparkParticle(b *strings.Builder, className, color, kind string) {
	switch kind {
	case "dot":
		fmt.Fprintf(b, `<g class="%s" fill="%s"><circle cx="0" cy="0" r="2.2"/></g>`, className, color)
	case "star":
		fmt.Fprintf(b, `<g class="%s" fill="%s"><path d="%s"/></g>`, className, color, miniStarPath)
	default:
		fmt.Fprintf(b, `<g class="%s" fill="%s"><path d="%s"/></g>`, className, color, sparklePath)
	}
}

func writeStarLineKeyframes(b *strings.Builder, sampled []chartXY, pathLen float64) {
	b.WriteString(`@keyframes sh-line-loop{`)
	n := len(sampled)
	if n >= 2 {
		fmt.Fprintf(b, `0%%{stroke-dashoffset:%.1f;opacity:0}`, pathLen)
		for i := 1; i < n-1; i++ {
			frac := float64(i) / float64(n-1)
			pct := 14.0 * math.Pow(frac, 0.72)
			offset := pathLen * (1.0 - frac)
			fmt.Fprintf(b, `%.2f%%{stroke-dashoffset:%.1f;opacity:1}`, pct, offset)
		}
		b.WriteString(`14.00%,98%{stroke-dashoffset:0;opacity:1}`)
	} else {
		fmt.Fprintf(b, `0%%{stroke-dashoffset:%.1f;opacity:0;animation-timing-function:cubic-bezier(0.32,0,0.72,0.25)}1.5%%{opacity:1}14%%,98%%{stroke-dashoffset:0;opacity:1}`, pathLen)
	}
	b.WriteString(`100%{stroke-dashoffset:0;opacity:0}}`)
}

func writeStarRaceKeyframes(b *strings.Builder, sampled []chartXY, last chartXY, midY, exitX, exitY float64) {
	b.WriteString(`@keyframes star-race{`)
	n := len(sampled)
	if n >= 2 {
		for i := range n - 1 {
			pt := sampled[i]
			frac := float64(i) / float64(n-1)
			pct := 14.0 * math.Pow(frac, 0.72)
			op := "1"
			if i == 0 {
				op = "0"
			}
			fmt.Fprintf(b, `%.2f%%{transform:translate(%.1fpx,%.1fpx);opacity:%s}`, pct, pt.x, pt.y, op)
		}
	} else {
		fmt.Fprintf(b, `0%%{transform:translate(%.1fpx,%.1fpx);opacity:0}`, last.x, last.y)
	}
	fmt.Fprintf(b, `14.00%%,84%%{transform:translate(%.1fpx,%.1fpx);opacity:1;animation-timing-function:cubic-bezier(0.55,0.05,0.9,0.35)}`, last.x, last.y)
	fmt.Fprintf(b, `87%%{transform:translate(%.1fpx,%.1fpx);opacity:1;animation-timing-function:cubic-bezier(0.42,0,0.85,0.28)}`, last.x, midY)
	fmt.Fprintf(b, `94%%{transform:translate(%.1fpx,%.1fpx);opacity:1}`, exitX, exitY)
	fmt.Fprintf(b, `95%%,100%%{transform:translate(%.1fpx,%.1fpx);opacity:0}`, exitX, exitY)
	b.WriteString(`}`)
}

func buildCometTrailParticles(startX, startY, exitX, exitY float64, palette [5]string) []cometTrailParticle {
	wakeDX := startX - exitX
	wakeDY := startY - exitY
	wakeLen := math.Hypot(wakeDX, wakeDY)
	if wakeLen == 0 {
		wakeLen = 1
	}
	wx := wakeDX / wakeLen
	wy := wakeDY / wakeLen
	px := -wy
	py := wx

	out := make([]cometTrailParticle, len(cometTrailSpecs))
	for i, sp := range cometTrailSpecs {
		u := invertCometEase(sp.prog)
		startPct := 87.0 + u*7.0
		peakPct := min(startPct+0.55, 94.8)
		endPct := min(startPct+2.2, 95.8)
		out[i] = cometTrailParticle{
			x:        startX + sp.prog*(exitX-startX),
			y:        startY + sp.prog*(exitY-startY),
			dx:       sp.driftBack*wx + sp.sideSpread*px,
			dy:       sp.driftBack*wy + sp.sideSpread*py + sp.gravityY,
			startPct: startPct,
			peakPct:  peakPct,
			endPct:   endPct,
			kind:     sp.kind,
			color:    palette[sp.colorIdx],
		}
	}
	return out
}

// invertCometEase returns the time fraction u in [0,1] at which
// cubic-bezier(0.42, 0, 0.85, 0.28) reaches spatial progress prog in [0,1].
func invertCometEase(prog float64) float64 {
	if prog <= 0 {
		return 0
	}
	if prog >= 1 {
		return 1
	}
	lo, hi := 0.0, 1.0
	for range 20 {
		mid := (lo + hi) * 0.5
		if cubicBezierSample(mid, 0.0, 0.28) < prog {
			lo = mid
		} else {
			hi = mid
		}
	}
	return cubicBezierSample((lo+hi)*0.5, 0.42, 0.85)
}

func cubicBezierSample(t, p1, p2 float64) float64 {
	u := 1.0 - t
	return 3*u*u*t*p1 + 3*u*t*t*p2 + t*t*t
}

func mapStarCoords(pts []StarPoint, plotL, plotW, plotT, plotH, yMax float64) []chartXY {
	if len(pts) == 1 {
		y := plotT + plotH*(1-float64(max(pts[0].Stars, 0))/yMax)
		return []chartXY{
			{x: plotL, y: y},
			{x: plotL + plotW, y: y},
		}
	}

	start := pts[0].Date
	totalSpan := pts[len(pts)-1].Date.Sub(start).Seconds()
	out := make([]chartXY, len(pts))
	for i, pt := range pts {
		var xFrac float64
		if totalSpan > 0 {
			xFrac = pt.Date.Sub(start).Seconds() / totalSpan
		} else {
			xFrac = float64(i) / float64(len(pts)-1)
		}
		yFrac := float64(max(pt.Stars, 0)) / yMax
		if yFrac > 1 {
			yFrac = 1
		}
		out[i] = chartXY{
			x: plotL + xFrac*plotW,
			y: plotT + plotH*(1-yFrac),
		}
	}
	return out
}

// buildSmoothChartPaths produces a variation-diminishing smooth quadratic path
// (never overshooting the data hull), its closed area path, evenly arc-length
// sampled coordinates along the rendered curve for the racing star, and curve
// length for stroke-dashoffset draw-in animation.
func buildSmoothChartPaths(coords []chartXY, baseY float64) (linePath, areaPath string, sampled []chartXY, length float64) {
	var line strings.Builder
	fmt.Fprintf(&line, "M %.1f %.1f", coords[0].x, coords[0].y)

	dense := []chartXY{coords[0]}
	if len(coords) == 2 {
		fmt.Fprintf(&line, " L %.1f %.1f", coords[1].x, coords[1].y)
		dense = append(dense, coords[1])
	} else {
		const quadSteps = 16
		prev := coords[0]
		for i := 1; i < len(coords)-1; i++ {
			ctrl := coords[i]
			mid := chartXY{
				x: (coords[i].x + coords[i+1].x) / 2,
				y: (coords[i].y + coords[i+1].y) / 2,
			}
			fmt.Fprintf(&line, " Q %.1f %.1f %.1f %.1f", ctrl.x, ctrl.y, mid.x, mid.y)
			for s := 1; s <= quadSteps; s++ {
				t := float64(s) / float64(quadSteps)
				u := 1 - t
				dense = append(dense, chartXY{
					x: u*u*prev.x + 2*u*t*ctrl.x + t*t*mid.x,
					y: u*u*prev.y + 2*u*t*ctrl.y + t*t*mid.y,
				})
			}
			prev = mid
		}
		last := coords[len(coords)-1]
		fmt.Fprintf(&line, " L %.1f %.1f", last.x, last.y)
		dense = append(dense, last)
	}

	sampled, length = resampleByArcLength(dense, 24)
	if length < 1 {
		length = 1
	}

	linePath = line.String()
	last := coords[len(coords)-1]
	areaPath = fmt.Sprintf("%s L %.1f %.1f L %.1f %.1f Z", linePath, last.x, baseY, coords[0].x, baseY)
	return linePath, areaPath, sampled, math.Ceil(length)
}

func resampleByArcLength(dense []chartXY, segments int) ([]chartXY, float64) {
	if len(dense) == 0 {
		return nil, 0
	}
	if len(dense) == 1 || segments <= 0 {
		return []chartXY{dense[0]}, 0
	}

	cum := make([]float64, len(dense))
	for i := 1; i < len(dense); i++ {
		cum[i] = cum[i-1] + math.Hypot(dense[i].x-dense[i-1].x, dense[i].y-dense[i-1].y)
	}
	total := cum[len(cum)-1]
	if total == 0 {
		out := make([]chartXY, segments+1)
		for i := range out {
			out[i] = dense[0]
		}
		return out, 0
	}

	out := make([]chartXY, segments+1)
	out[0] = dense[0]
	out[segments] = dense[len(dense)-1]

	idx := 1
	for k := 1; k < segments; k++ {
		target := total * float64(k) / float64(segments)
		for idx < len(cum)-1 && cum[idx] < target {
			idx++
		}
		segLen := cum[idx] - cum[idx-1]
		var frac float64
		if segLen > 0 {
			frac = (target - cum[idx-1]) / segLen
		}
		out[k] = chartXY{
			x: dense[idx-1].x + frac*(dense[idx].x-dense[idx-1].x),
			y: dense[idx-1].y + frac*(dense[idx].y-dense[idx-1].y),
		}
	}
	return out, total
}

func niceAxisMax(v int) int {
	if v <= 4 {
		return 4
	}
	raw := float64(v)
	mag := math.Pow(10, math.Floor(math.Log10(raw)))
	norm := raw / mag
	var nice float64
	switch {
	case norm <= 1.2:
		nice = 1.2
	case norm <= 1.6:
		nice = 1.6
	case norm <= 2.0:
		nice = 2.0
	case norm <= 2.4:
		nice = 2.4
	case norm <= 3.0:
		nice = 3.0
	case norm <= 4.0:
		nice = 4.0
	case norm <= 5.0:
		nice = 5.0
	case norm <= 6.0:
		nice = 6.0
	case norm <= 8.0:
		nice = 8.0
	default:
		nice = 10.0
	}
	res := int(math.Ceil(nice * mag))
	if res < 4 {
		return 4
	}
	// Round up to a multiple of 4 so 4 grid intervals produce integer labels.
	if rem := res % 4; rem != 0 {
		res += 4 - rem
	}
	return res
}

type dateTick struct {
	x     float64
	label string
}

func buildDateTicks(start, end time.Time, plotL, plotR float64) []dateTick {
	span := end.Sub(start)
	layout := "Jan 2006"
	if span < 90*24*time.Hour {
		layout = "02 Jan"
	}
	if span <= 0 {
		return []dateTick{
			{x: plotL, label: start.Format(layout)},
			{x: plotR, label: end.Format(layout)},
		}
	}
	const count = 5
	ticks := make([]dateTick, count)
	for i := range count {
		frac := float64(i) / float64(count-1)
		t := start.Add(time.Duration(frac * float64(span)))
		ticks[i] = dateTick{
			x:     plotL + frac*(plotR-plotL),
			label: t.Format(layout),
		}
	}
	return ticks
}

func formatStarDateRange(start, end time.Time) string {
	s := start.Format("Jan 2006")
	e := end.Format("Jan 2006")
	if s == e {
		return s
	}
	return s + " - " + e
}

// RenderSponsors renders the fixed-height 840x108 sponsors CTA banner in dark
// and light variants with multi-coloured drifting balloon hearts.
func RenderSponsors(in SponsorsInput, theme Theme) (SVGPair, error) {
	title := strings.TrimSpace(in.Title)
	if title == "" {
		title = "Enjoy my work?"
	}
	if wrapped := WrapText(title, 38, 1); len(wrapped) > 0 {
		title = wrapped[0]
	}

	subtitle := strings.TrimSpace(in.Subtitle)
	if subtitle == "" {
		subtitle = "Treat me to a coffee, help cover hosting, infrastructure, and signing costs."
	}
	if wrapped := WrapText(subtitle, 82, 1); len(wrapped) > 0 {
		subtitle = wrapped[0]
	}

	cta := strings.TrimSpace(in.CTALabel)
	if cta == "" {
		cta = "Sponsor on GitHub"
	}

	darkP := normalizePalette(theme.Dark, defaultDarkPalette)
	lightP := normalizePalette(theme.Light, defaultLightPalette)

	return SVGPair{
		Dark:  renderSponsorsVariant(title, subtitle, cta, darkP, true),
		Light: renderSponsorsVariant(title, subtitle, cta, lightP, false),
	}, nil
}

type sponsorHeart struct {
	x        int
	y        int
	scale    float64
	opacity  float64
	colorIdx int
	duration float64
	delay    float64
	swayDir  string
}

var sponsorDriftingHearts = [...]sponsorHeart{
	{x: 558, y: 66, scale: 0.62, opacity: 0.35, colorIdx: 2, duration: 12.6, delay: -2.1, swayDir: "l"},
	{x: 588, y: 22, scale: 0.56, opacity: 0.68, colorIdx: 1, duration: 11.4, delay: -7.2, swayDir: "r"},
	{x: 600, y: 58, scale: 0.78, opacity: 0.52, colorIdx: 2, duration: 10.4, delay: -4.5, swayDir: "l"},
	{x: 646, y: 20, scale: 0.64, opacity: 0.48, colorIdx: 4, duration: 13.0, delay: -9.4, swayDir: "r"},
	{x: 682, y: 80, scale: 0.68, opacity: 0.72, colorIdx: 3, duration: 10.0, delay: -1.6, swayDir: "l"},
	{x: 726, y: 74, scale: 0.90, opacity: 0.50, colorIdx: 2, duration: 11.8, delay: -5.8, swayDir: "r"},
	{x: 758, y: 26, scale: 0.62, opacity: 0.78, colorIdx: 0, duration: 9.8, delay: -8.3, swayDir: "l"},
	{x: 794, y: 16, scale: 0.86, opacity: 0.76, colorIdx: 1, duration: 10.8, delay: -3.4, swayDir: "r"},
	{x: 814, y: 64, scale: 0.70, opacity: 0.62, colorIdx: 4, duration: 12.2, delay: -6.7, swayDir: "l"},
}

func renderSponsorsVariant(title, subtitle, cta string, p Palette, dark bool) string {
	const (
		width      = 840
		height     = 108
		pillX      = 612
		pillY      = 32
		pillW      = 196
		pillH      = 44
		heartVisW  = 15.0
		heartGap   = 8.0
		pillCenter = float64(pillX) + float64(pillW)/2.0
	)

	rose := "#e64553"
	colors := sponsorLightHeartColors
	if dark {
		rose = "#f38ba8"
		colors = sponsorDarkHeartColors
	}

	estTextW := float64(utf8.RuneCountInString(cta)) * 7.6
	contentW := heartVisW + heartGap + estTextW
	startX := max(pillCenter-contentW/2.0, float64(pillX)+14.0)
	heartX := int(math.Round(startX - 1.5))
	textX := int(math.Round(startX + heartVisW + heartGap))

	var b strings.Builder
	fmt.Fprintf(&b, `<svg xmlns="http://www.w3.org/2000/svg" width="%d" height="%d" viewBox="0 0 %d %d" role="img" aria-label="%s">`,
		width, height, width, height, XMLEscape(title))
	fmt.Fprintf(&b, `<title>%s</title>`, XMLEscape(title))
	b.WriteString(`<defs>`)
	b.WriteString(`<clipPath id="sp-clip"><rect x="0.5" y="0.5" width="839" height="107" rx="14"/></clipPath>`)
	b.WriteString(`<clipPath id="sp-hearts"><rect x="540" y="2" width="298" height="104"/></clipPath>`)
	fmt.Fprintf(&b, `<linearGradient id="sp-glint" x1="0" y1="0" x2="1" y2="0"><stop offset="0%%" stop-color="%s" stop-opacity="0"/><stop offset="50%%" stop-color="%s" stop-opacity="0.85"/><stop offset="100%%" stop-color="%s" stop-opacity="0"/></linearGradient>`,
		rose, rose, rose)
	b.WriteString(`</defs>`)

	b.WriteString(`<style>`)
	fmt.Fprintf(&b, `.sp-title{font-family:%s;font-size:20px;font-weight:700;fill:%s;letter-spacing:-0.01em}`, fontSans, p.HeadingColor)
	fmt.Fprintf(&b, `.sp-sub{font-family:%s;font-size:13.5px;font-weight:400;fill:%s}`, fontSans, p.Color)
	fmt.Fprintf(&b, `.sp-cta{font-family:%s;font-size:14px;font-weight:700;fill:%s}`, fontSans, p.HeadingColor)
	spDur, spDelay := glintTiming("sponsors")
	b.WriteString(`@keyframes sp-sweep{0%{transform:translateX(-220px);opacity:0}2%{opacity:1}12%{opacity:1}14%,100%{transform:translateX(880px);opacity:0}}`)
	fmt.Fprintf(&b, `.sp-glint{opacity:0;animation:sp-sweep %.1fs ease-in-out %.1fs infinite}`, spDur, spDelay)
	for i, h := range sponsorDriftingHearts {
		startY := 114 - h.y
		endY := -24 - h.y
		y15 := int(math.Round(float64(startY) + 0.15*float64(endY-startY)))
		y42 := int(math.Round(float64(startY) + 0.42*float64(endY-startY)))
		y70 := int(math.Round(float64(startY) + 0.70*float64(endY-startY)))
		y88 := int(math.Round(float64(startY) + 0.88*float64(endY-startY)))
		sway1, sway2 := -5, 6
		if h.swayDir == "r" {
			sway1, sway2 = 5, -6
		}
		fmt.Fprintf(&b, `@keyframes sp-rise-%d{0%%{transform:translate(0px,%dpx);opacity:0}15%%{transform:translate(%dpx,%dpx);opacity:1}42%%{transform:translate(%dpx,%dpx);opacity:1}70%%{transform:translate(%dpx,%dpx);opacity:1}88%%{transform:translate(%dpx,%dpx);opacity:1}100%%{transform:translate(0px,%dpx);opacity:0}}`,
			i, startY, sway1, y15, sway2, y42, sway1, y70, sway2/2, y88, endY)
		fmt.Fprintf(&b, `.sp-h%d{animation:sp-rise-%d %.1fs linear %.1fs infinite}`, i, i, h.duration, h.delay)
	}
	b.WriteString(`@media (prefers-reduced-motion: reduce) { * { animation: none !important; } .sp-heart { opacity: 1 !important; transform: none !important; } }`)
	b.WriteString(`</style>`)

	b.WriteString(`<g clip-path="url(#sp-clip)">`)
	fmt.Fprintf(&b, `<rect x="0" y="0" width="840" height="%d" fill="%s"/>`, height, p.CardBackground)
	b.WriteString(`<rect class="sp-glint" x="0" y="0" width="180" height="2" fill="url(#sp-glint)"/>`)

	// Multi-coloured drifting balloon hearts floating gently behind and around the right side.
	b.WriteString(`<g clip-path="url(#sp-hearts)">`)
	for i, h := range sponsorDriftingHearts {
		op := h.opacity
		if !dark {
			op *= 0.85
		}
		fmt.Fprintf(&b, `<g class="sp-heart sp-h%d"><g transform="translate(%d, %d) scale(%.2f)" fill="%s" opacity="%.2f"><path d="%s"/></g></g>`,
			i, h.x, h.y, h.scale, colors[h.colorIdx], op, heartGlyphPath)
	}
	b.WriteString(`</g>`)

	// Left heading and subtitle.
	fmt.Fprintf(&b, `<text x="32" y="44" class="sp-title">%s</text>`, XMLEscape(title))
	fmt.Fprintf(&b, `<text x="32" y="72" class="sp-sub">%s</text>`, XMLEscape(subtitle))

	// Right-side full-capsule rose pill (opaque card base masks hearts passing behind it).
	fmt.Fprintf(&b, `<rect x="%d" y="%d" width="%d" height="%d" rx="22" fill="%s"/>`,
		pillX, pillY, pillW, pillH, p.CardBackground)
	fmt.Fprintf(&b, `<rect x="%d" y="%d" width="%d" height="%d" rx="22" fill="%s" fill-opacity="0.16" stroke="%s" stroke-width="1.5"/>`,
		pillX, pillY, pillW, pillH, rose, rose)
	fmt.Fprintf(&b, `<g transform="translate(%d, 45) scale(0.75)" fill="%s"><path d="%s"/></g>`, heartX, rose, heartGlyphPath)
	fmt.Fprintf(&b, `<text x="%d" y="59" class="sp-cta">%s</text>`, textX, XMLEscape(cta))

	b.WriteString(`</g>`)
	fmt.Fprintf(&b, `<rect x="0.5" y="0.5" width="839" height="107" rx="14" fill="none" stroke="%s" stroke-width="1"/>`, cardBorder(p))
	b.WriteString(`</svg>`)
	return b.String()
}

// RenderContributors renders the 840x150 floating-balloon contributor avatars
// banner in dark and light theme variants.
func RenderContributors(theme Theme, projectTitle string, contributors []Contributor, totalCount int) (SVGPair, error) {
	capped := contributors
	if len(capped) > maxContributorsDisplayed {
		capped = capped[:maxContributorsDisplayed]
	}

	darkP := normalizePalette(theme.Dark, defaultDarkPalette)
	lightP := normalizePalette(theme.Light, defaultLightPalette)

	return SVGPair{
		Dark:  renderContributorsVariant(projectTitle, capped, totalCount, darkP),
		Light: renderContributorsVariant(projectTitle, capped, totalCount, lightP),
	}, nil
}

type balloonPos struct {
	cx       int
	cy       int
	row      int
	duration float64
	delay    float64
	swayDir  string
}

func layoutContributorBalloons(contributors []Contributor) []balloonPos {
	n := len(contributors)
	if n == 0 {
		return nil
	}

	var rows [][]int
	switch {
	case n <= 5:
		rows = [][]int{makeIndexSlice(0, n)}
	case n <= 12:
		first := (n + 1) / 2
		rows = [][]int{makeIndexSlice(0, first), makeIndexSlice(first, n)}
	default:
		r0 := (n + 2) / 3
		rem := n - r0
		r1 := (rem + 1) / 2
		rows = [][]int{
			makeIndexSlice(0, r0),
			makeIndexSlice(r0, r0+r1),
			makeIndexSlice(r0+r1, n),
		}
	}

	rowYs := [][]int{
		{75},
		{48, 102},
		{34, 75, 116},
	}[len(rows)-1]

	const (
		fieldCenterX = 626
		fieldMinX    = 452
		fieldMaxX    = 802
	)
	spacing := 56
	if len(rows) == 3 {
		spacing = 50
	}

	out := make([]balloonPos, n)
	for rowIdx, idxs := range rows {
		count := len(idxs)
		span := (count - 1) * spacing
		startX := fieldCenterX - span/2
		if rowIdx%2 == 1 && count < 7 {
			startX += 10
		}
		for posInRow, idx := range idxs {
			cx := startX + posInRow*spacing
			cx = min(max(cx, fieldMinX), fieldMaxX)
			dur, delay, dir := contributorBalloonTiming(contributors[idx].Login, idx, n)
			out[idx] = balloonPos{
				cx:       cx,
				cy:       rowYs[rowIdx],
				row:      rowIdx,
				duration: dur,
				delay:    delay,
				swayDir:  dir,
			}
		}
	}
	return out
}

func makeIndexSlice(start, end int) []int {
	out := make([]int, 0, max(end-start, 0))
	for i := start; i < end; i++ {
		out = append(out, i)
	}
	return out
}

func contributorBalloonTiming(login string, idx, total int) (durationSec, delaySec float64, swayDir string) {
	sum := sha256.Sum256([]byte(fmt.Sprintf("%s#%d", strings.ToLower(strings.TrimSpace(login)), idx)))
	// Deterministic duration in [12.0, 20.0] seconds.
	durFrac := float64(binary.BigEndian.Uint64(sum[0:8])%81) / 10.0
	durationSec = 12.0 + durFrac

	// Even phase spread across total contributors plus small deterministic jitter so frame 0 is balanced.
	baseFrac := float64(idx) / float64(max(total, 1))
	jitter := float64(binary.BigEndian.Uint64(sum[8:16])%100) / 1000.0
	phase := math.Mod(baseFrac+jitter, 1.0)
	delay := math.Round(phase*durationSec*10) / 10
	if delay < 0.2 {
		delay = 0.2
	}

	swayDir = "l"
	if sum[16]%2 == 1 {
		swayDir = "r"
	}
	return durationSec, -delay, swayDir
}

func contributorInitials(login string) string {
	trimmed := strings.TrimSpace(login)
	if trimmed == "" {
		return "?"
	}
	parts := strings.FieldsFunc(trimmed, func(r rune) bool {
		return r == '-' || r == '_' || r == '.' || unicode.IsSpace(r)
	})
	if len(parts) >= 2 {
		r1, _ := utf8.DecodeRuneInString(parts[0])
		r2, _ := utf8.DecodeRuneInString(parts[1])
		return strings.ToUpper(string(r1) + string(r2))
	}
	runes := []rune(trimmed)
	if len(runes) >= 2 {
		return strings.ToUpper(string(runes[:2]))
	}
	return strings.ToUpper(string(runes[0]))
}

func renderContributorsVariant(projectTitle string, contributors []Contributor, totalCount int, p Palette) string {
	const (
		width  = 840
		height = 150
	)

	const heading = "Thank you to our contributors"
	proj := strings.TrimSpace(projectTitle)
	subtitle := "Every commit, issue, and review helps shape this project"
	if proj != "" {
		subtitle = "Every commit, issue, and review helps shape " + proj
	}
	subLines := WrapText(subtitle, 54, 2)

	displayTotal := max(totalCount, len(contributors))
	countLabel := fmt.Sprintf("%s contributors", formatCount(displayTotal))
	if displayTotal == 1 {
		countLabel = "1 contributor"
	}
	moreLabel := ""
	if displayTotal > len(contributors) && len(contributors) > 0 {
		moreLabel = fmt.Sprintf("+%s more", formatCount(displayTotal-len(contributors)))
	}

	balloons := layoutContributorBalloons(contributors)
	ringColor := mixHex(p.Primary, p.MutedBorder, 0.65)
	pillBorder := mixHex(p.Primary, p.MutedBorder, 0.45)
	initialsFg := contrastOnFill(p.Primary)

	// Collect unique row Y coordinates for balloon rise keyframes.
	seenRows := make(map[int]struct{}, 3)
	var rowYs []int
	for _, bPos := range balloons {
		if _, ok := seenRows[bPos.cy]; !ok {
			seenRows[bPos.cy] = struct{}{}
			rowYs = append(rowYs, bPos.cy)
		}
	}

	var b strings.Builder
	fmt.Fprintf(&b, `<svg xmlns="http://www.w3.org/2000/svg" width="%d" height="%d" viewBox="0 0 %d %d" role="img" aria-label="%s">`,
		width, height, width, height, XMLEscape(heading))
	fmt.Fprintf(&b, `<title>%s (%s)</title>`, XMLEscape(heading), XMLEscape(countLabel))
	b.WriteString(`<defs>`)
	b.WriteString(`<clipPath id="contrib-clip"><rect x="0.5" y="0.5" width="839" height="149" rx="14"/></clipPath>`)
	b.WriteString(`<clipPath id="contrib-field"><rect x="424" y="2" width="415" height="146"/></clipPath>`)
	for i, bPos := range balloons {
		fmt.Fprintf(&b, `<clipPath id="cb-c%d"><circle cx="%d" cy="%d" r="19"/></clipPath>`, i, bPos.cx, bPos.cy)
	}
	b.WriteString(`</defs>`)

	b.WriteString(`<style>`)
	fmt.Fprintf(&b, `.cb-title{font-family:%s;font-size:18px;font-weight:700;fill:%s;letter-spacing:-0.01em}`, fontSans, p.HeadingColor)
	fmt.Fprintf(&b, `.cb-sub{font-family:%s;font-size:13px;font-weight:400;fill:%s}`, fontSans, p.MutedColor)
	fmt.Fprintf(&b, `.cb-pill{font-family:%s;font-size:12px;font-weight:600;fill:%s}`, fontMono, p.HeadingColor)
	fmt.Fprintf(&b, `.cb-more{font-family:%s;font-size:11.5px;font-weight:500;fill:%s}`, fontMono, p.MutedColor)
	fmt.Fprintf(&b, `.cb-init{font-family:%s;font-size:13px;font-weight:700;fill:%s}`, fontSans, initialsFg)
	for _, cy := range rowYs {
		startY := 172 - cy
		endY := -24 - cy
		y14 := int(math.Round(float64(startY) + 0.14*float64(endY-startY)))
		y38 := int(math.Round(float64(startY) + 0.38*float64(endY-startY)))
		y62 := int(math.Round(float64(startY) + 0.62*float64(endY-startY)))
		y86 := int(math.Round(float64(startY) + 0.86*float64(endY-startY)))
		fmt.Fprintf(&b, `@keyframes cb-rise-%d-l{0%%{transform:translate(0px,%dpx);opacity:0}14%%{transform:translate(-6px,%dpx);opacity:1}38%%{transform:translate(7px,%dpx);opacity:1}62%%{transform:translate(-7px,%dpx);opacity:1}86%%{transform:translate(5px,%dpx);opacity:1}100%%{transform:translate(0px,%dpx);opacity:0}}`,
			cy, startY, y14, y38, y62, y86, endY)
		fmt.Fprintf(&b, `@keyframes cb-rise-%d-r{0%%{transform:translate(0px,%dpx);opacity:0}14%%{transform:translate(6px,%dpx);opacity:1}38%%{transform:translate(-7px,%dpx);opacity:1}62%%{transform:translate(7px,%dpx);opacity:1}86%%{transform:translate(-5px,%dpx);opacity:1}100%%{transform:translate(0px,%dpx);opacity:0}}`,
			cy, startY, y14, y38, y62, y86, endY)
	}
	for i, bPos := range balloons {
		fmt.Fprintf(&b, `.cb-f%d{animation:cb-rise-%d-%s %.1fs linear %.1fs infinite}`,
			i, bPos.cy, bPos.swayDir, bPos.duration, bPos.delay)
	}
	b.WriteString(`@media (prefers-reduced-motion: reduce) { * { animation: none !important; } .cb-float { opacity: 1 !important; transform: none !important; } }`)
	b.WriteString(`</style>`)

	b.WriteString(`<g clip-path="url(#contrib-clip)">`)
	fmt.Fprintf(&b, `<rect x="0" y="0" width="840" height="150" fill="%s"/>`, p.CardBackground)

	// Left copy block.
	titleY := 48
	if len(subLines) > 1 {
		titleY = 42
	}
	fmt.Fprintf(&b, `<text x="28" y="%d" class="cb-title">%s</text>`, titleY, XMLEscape(heading))
	for i, line := range subLines {
		fmt.Fprintf(&b, `<text x="28" y="%d" class="cb-sub">%s</text>`, titleY+24+i*18, XMLEscape(line))
	}

	pillY := 98
	if len(subLines) > 1 {
		pillY = 104
	}
	pillW := int(math.Ceil(float64(len(countLabel))*8.5)) + 32
	if pillW < 120 {
		pillW = 120
	}
	fmt.Fprintf(&b, `<rect x="28" y="%d" width="%d" height="26" rx="13" fill="%s" stroke="%s" stroke-width="1"/>`,
		pillY, pillW, p.CodeBackground, pillBorder)
	fmt.Fprintf(&b, `<circle cx="42" cy="%d" r="3.5" fill="%s"/>`, pillY+13, p.Primary)
	fmt.Fprintf(&b, `<text x="52" y="%d" class="cb-pill">%s</text>`, pillY+17, XMLEscape(countLabel))

	if moreLabel != "" {
		moreX := 28 + pillW + 8
		moreW := int(math.Ceil(float64(len(moreLabel))*8.5)) + 28
		if moreW < 64 {
			moreW = 64
		}
		fmt.Fprintf(&b, `<rect x="%d" y="%d" width="%d" height="26" rx="13" fill="%s" stroke="%s" stroke-width="1"/>`,
			moreX, pillY, moreW, p.CodeBackground, p.MutedBorder)
		fmt.Fprintf(&b, `<text x="%d" y="%d" text-anchor="middle" class="cb-more">%s</text>`,
			moreX+moreW/2, pillY+17, XMLEscape(moreLabel))
	}

	// Floating contributor avatar balloons in the right half.
	b.WriteString(`<g clip-path="url(#contrib-field)">`)
	for i, c := range contributors {
		bPos := balloons[i]
		fmt.Fprintf(&b, `<g class="cb-float cb-f%d">`, i)
		if strings.TrimSpace(c.Login) != "" {
			fmt.Fprintf(&b, `<title>@%s</title>`, XMLEscape(strings.TrimSpace(c.Login)))
		}
		uri := strings.TrimSpace(c.AvatarDataURI)
		if uri != "" && safeRasterDataRe.MatchString(uri) {
			fmt.Fprintf(&b, `<image x="%d" y="%d" width="38" height="38" href="%s" clip-path="url(#cb-c%d)" preserveAspectRatio="xMidYMid slice"/>`,
				bPos.cx-19, bPos.cy-19, XMLEscape(uri), i)
			fmt.Fprintf(&b, `<circle cx="%d" cy="%d" r="19" fill="none" stroke="%s" stroke-width="1.5"/>`,
				bPos.cx, bPos.cy, ringColor)
		} else {
			fmt.Fprintf(&b, `<circle cx="%d" cy="%d" r="19" fill="%s" stroke="%s" stroke-width="1.5"/>`,
				bPos.cx, bPos.cy, p.Primary, ringColor)
			fmt.Fprintf(&b, `<text x="%d" y="%d" text-anchor="middle" class="cb-init">%s</text>`,
				bPos.cx, bPos.cy+5, XMLEscape(contributorInitials(c.Login)))
		}
		b.WriteString(`</g>`)
	}
	b.WriteString(`</g>`)

	b.WriteString(`</g>`)
	fmt.Fprintf(&b, `<rect x="0.5" y="0.5" width="839" height="149" rx="14" fill="none" stroke="%s" stroke-width="1"/>`, cardBorder(p))
	b.WriteString(`</svg>`)
	return b.String()
}

// formatCount formats a non-negative integer for compact UI labels, using
// comma separators below 100,000 and abbreviated suffixes above.
func formatCount(n int) string {
	if n < 0 {
		n = 0
	}
	if n >= 1_000_000 {
		return fmt.Sprintf("%.1fM", float64(n)/1_000_000)
	}
	if n >= 100_000 {
		return fmt.Sprintf("%.1fk", float64(n)/1_000)
	}
	s := strconv.Itoa(n)
	if len(s) <= 3 {
		return s
	}
	var b strings.Builder
	rem := len(s) % 3
	if rem > 0 {
		b.WriteString(s[:rem])
		b.WriteByte(',')
	}
	for i := rem; i < len(s); i += 3 {
		if i > rem {
			b.WriteByte(',')
		}
		b.WriteString(s[i : i+3])
	}
	return b.String()
}
