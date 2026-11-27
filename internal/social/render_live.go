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

// SponsorsInput configures the dynamic-height sponsors banner rendered by
// RenderSponsors.
type SponsorsInput struct {
	Handles  []string
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
	fmt.Fprintf(&b, `.s-val-sans{font-family:%s;font-weight:700;fill:%s}`, fontSans, p.HeadingColor)
	fmt.Fprintf(&b, `.s-val-mono{font-family:%s;font-weight:700;fill:%s}`, fontMono, p.HeadingColor)
	b.WriteString(`@keyframes stat-pulse{0%,100%{opacity:0.45;transform:scale(0.92)}50%{opacity:1;transform:scale(1.15)}}`)
	b.WriteString(`.s-dot{transform-box:fill-box;transform-origin:center;animation:stat-pulse 3.4s ease-in-out infinite}`)
	b.WriteString(`.d0{animation-delay:0s}.d1{animation-delay:0.7s}.d2{animation-delay:1.4s}.d3{animation-delay:2.1s}`)
	b.WriteString(reducedMotionCSS)
	b.WriteString(`</style>`)

	b.WriteString(`<g clip-path="url(#stats-clip)">`)
	fmt.Fprintf(&b, `<rect x="0" y="0" width="840" height="98" fill="%s"/>`, p.CardBackground)
	fmt.Fprintf(&b, `<rect x="0" y="0" width="840" height="2.5" fill="%s" opacity="0.24"/>`, p.Primary)

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
		fontSize := statValueFontSize(val, col.mono)
		cls := "s-val-sans"
		if col.mono {
			cls = "s-val-mono"
		}
		fmt.Fprintf(&b, `<text x="%d" y="67" text-anchor="middle" font-size="%d" class="%s">%s</text>`,
			centerX, fontSize, cls, XMLEscape(val))
	}

	b.WriteString(`</g>`)
	fmt.Fprintf(&b, `<rect x="0.5" y="0.5" width="839" height="97" rx="12" fill="none" stroke="%s" stroke-width="1"/>`, p.MutedBorder)
	b.WriteString(`</svg>`)
	return b.String()
}

func statValueFontSize(val string, mono bool) int {
	n := utf8.RuneCountInString(val)
	if mono {
		switch {
		case n > 10:
			return 16
		case n > 7:
			return 19
		default:
			return 22
		}
	}
	switch {
	case n > 23:
		return 12
	case n > 19:
		return 13
	case n > 14:
		return 14
	case n > 10:
		return 17
	case n > 8:
		return 19
	default:
		return 21
	}
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
	fmt.Fprintf(&b, `<rect x="0" y="0" width="840" height="2.5" fill="%s" opacity="0.28"/>`, p.Primary)

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
	fmt.Fprintf(&b, `<rect x="0.5" y="0.5" width="839" height="%d" rx="14" fill="none" stroke="%s" stroke-width="1"/>`, height-1, p.MutedBorder)
	b.WriteString(`</svg>`)
	return b.String()
}

type chartXY struct {
	x float64
	y float64
}

func renderStarHistoryVariant(pts []StarPoint, maxStars, latestStars int, title string, p Palette, dark bool) string {
	const (
		width  = 840
		height = 280
		plotL  = 64.0
		plotR  = 804.0
		plotT  = 66.0
		plotB  = 236.0
	)
	plotW := plotR - plotL
	plotH := plotB - plotT

	yMax := niceAxisMax(maxStars)
	coords := mapStarCoords(pts, plotL, plotW, plotT, plotH, float64(yMax))
	linePath, areaPath, pathLen := buildSmoothChartPaths(coords, plotB)

	gradTopOp := "0.24"
	if dark {
		gradTopOp = "0.30"
	}

	dateRange := formatStarDateRange(pts[0].Date, pts[len(pts)-1].Date)
	badgeText := formatCount(latestStars) + " stars"
	badgeW := len([]rune(badgeText))*7 + 34
	if badgeW < 92 {
		badgeW = 92
	}
	badgeX := int(plotR) - badgeW
	badgeBorder := mixHex(p.Primary, p.MutedBorder, 0.45)

	last := coords[len(coords)-1]

	var b strings.Builder
	fmt.Fprintf(&b, `<svg xmlns="http://www.w3.org/2000/svg" width="%d" height="%d" viewBox="0 0 %d %d" role="img" aria-label="%s">`,
		width, height, width, height, XMLEscape(title))
	fmt.Fprintf(&b, `<title>%s: %s</title>`, XMLEscape(title), XMLEscape(badgeText))
	b.WriteString(`<defs>`)
	b.WriteString(`<clipPath id="star-clip"><rect x="0.5" y="0.5" width="839" height="279" rx="14"/></clipPath>`)
	fmt.Fprintf(&b, `<linearGradient id="star-area" x1="0" y1="0" x2="0" y2="1"><stop offset="0%%" stop-color="%s" stop-opacity="%s"/><stop offset="100%%" stop-color="%s" stop-opacity="0"/></linearGradient>`,
		p.Primary, gradTopOp, p.Primary)
	b.WriteString(`</defs>`)

	b.WriteString(`<style>`)
	fmt.Fprintf(&b, `.sh-title{font-family:%s;font-size:15px;font-weight:700;fill:%s}`, fontSans, p.HeadingColor)
	fmt.Fprintf(&b, `.sh-range{font-family:%s;font-size:11.5px;font-weight:400;fill:%s}`, fontMono, p.MutedColor)
	fmt.Fprintf(&b, `.sh-axis{font-family:%s;font-size:11px;font-weight:500;fill:%s}`, fontMono, p.MutedColor)
	fmt.Fprintf(&b, `.sh-badge{font-family:%s;font-size:12px;font-weight:600;fill:%s}`, fontMono, p.HeadingColor)
	fmt.Fprintf(&b, `@keyframes star-draw{from{stroke-dashoffset:%.1f}to{stroke-dashoffset:0}}`, pathLen)
	b.WriteString(`@keyframes star-fade{from{opacity:0}to{opacity:1}}`)
	fmt.Fprintf(&b, `.sh-line{stroke-dasharray:%.1f;stroke-dashoffset:0;animation:star-draw 1.8s ease-out both}`, pathLen)
	b.WriteString(`.sh-area{animation:star-fade 1.2s ease-out both}`)
	b.WriteString(reducedMotionCSS)
	b.WriteString(`</style>`)

	b.WriteString(`<g clip-path="url(#star-clip)">`)
	fmt.Fprintf(&b, `<rect x="0" y="0" width="840" height="280" fill="%s"/>`, p.CardBackground)
	fmt.Fprintf(&b, `<rect x="0" y="0" width="840" height="2.5" fill="%s" opacity="0.25"/>`, p.Primary)

	// Header and current star count badge.
	fmt.Fprintf(&b, `<text x="28" y="32" class="sh-title">%s</text>`, XMLEscape(title))
	fmt.Fprintf(&b, `<text x="28" y="49" class="sh-range">%s</text>`, XMLEscape(dateRange))

	fmt.Fprintf(&b, `<rect x="%d" y="18" width="%d" height="28" rx="14" fill="%s" stroke="%s" stroke-width="1"/>`,
		badgeX, badgeW, p.CodeBackground, badgeBorder)
	fmt.Fprintf(&b, `<g transform="translate(%d, 25) scale(0.583333)" fill="%s"><path d="%s"/></g>`,
		badgeX+11, p.Primary, starGlyphPath)
	fmt.Fprintf(&b, `<text x="%d" y="36" class="sh-badge">%s</text>`, badgeX+29, XMLEscape(badgeText))

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

	// Area gradient fill + animated polyline curve.
	fmt.Fprintf(&b, `<path class="sh-area" d="%s" fill="url(#star-area)"/>`, areaPath)
	fmt.Fprintf(&b, `<path class="sh-line" d="%s" fill="none" stroke="%s" stroke-width="2.75" stroke-linecap="round" stroke-linejoin="round"/>`,
		linePath, p.Primary)

	// Highlight dot on latest point.
	fmt.Fprintf(&b, `<circle cx="%.1f" cy="%.1f" r="8.5" fill="%s" opacity="0.22"/>`, last.x, last.y, p.Primary)
	fmt.Fprintf(&b, `<circle cx="%.1f" cy="%.1f" r="4.5" fill="%s" stroke="%s" stroke-width="2"/>`, last.x, last.y, p.Primary, p.CardBackground)

	b.WriteString(`</g>`)
	fmt.Fprintf(&b, `<rect x="0.5" y="0.5" width="839" height="279" rx="14" fill="none" stroke="%s" stroke-width="1"/>`, p.MutedBorder)
	b.WriteString(`</svg>`)
	return b.String()
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
// (never overshooting the data hull), its closed area path, and approximate
// curve length for stroke-dashoffset draw-in animation.
func buildSmoothChartPaths(coords []chartXY, baseY float64) (linePath, areaPath string, length float64) {
	var line strings.Builder
	fmt.Fprintf(&line, "M %.1f %.1f", coords[0].x, coords[0].y)

	if len(coords) == 2 {
		fmt.Fprintf(&line, " L %.1f %.1f", coords[1].x, coords[1].y)
		length = math.Hypot(coords[1].x-coords[0].x, coords[1].y-coords[0].y)
	} else {
		for i := 1; i < len(coords)-1; i++ {
			midX := (coords[i].x + coords[i+1].x) / 2
			midY := (coords[i].y + coords[i+1].y) / 2
			fmt.Fprintf(&line, " Q %.1f %.1f %.1f %.1f", coords[i].x, coords[i].y, midX, midY)
		}
		last := coords[len(coords)-1]
		fmt.Fprintf(&line, " L %.1f %.1f", last.x, last.y)

		for i := 1; i < len(coords); i++ {
			length += math.Hypot(coords[i].x-coords[i-1].x, coords[i].y-coords[i-1].y)
		}
	}
	if length < 1 {
		length = 1
	}

	linePath = line.String()
	last := coords[len(coords)-1]
	areaPath = fmt.Sprintf("%s L %.1f %.1f L %.1f %.1f Z", linePath, last.x, baseY, coords[0].x, baseY)
	return linePath, areaPath, math.Ceil(length)
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

// RenderSponsors renders the dynamic-height (>=108px) sponsors banner in dark
// and light variants, displaying active @handle pills or an inviting callout
// when there are no sponsors yet.
func RenderSponsors(in SponsorsInput, theme Theme) (SVGPair, error) {
	handles := normalizeSponsorHandles(in.Handles)

	title := strings.TrimSpace(in.Title)
	if title == "" {
		switch len(handles) {
		case 0:
			title = "Sponsors"
		case 1:
			title = "Backed by 1 sponsor"
		default:
			title = fmt.Sprintf("Backed by %s sponsors", formatCount(len(handles)))
		}
	}
	subtitle := strings.TrimSpace(in.Subtitle)
	if subtitle == "" {
		if len(handles) == 0 {
			subtitle = "Support ongoing maintenance and open-source development"
		} else {
			subtitle = "Thank you to everyone sponsoring this project"
		}
	}
	cta := strings.TrimSpace(in.CTALabel)
	if cta == "" {
		cta = "SPONSOR"
	}

	darkP := normalizePalette(theme.Dark, defaultDarkPalette)
	lightP := normalizePalette(theme.Light, defaultLightPalette)

	return SVGPair{
		Dark:  renderSponsorsVariant(handles, title, subtitle, cta, darkP, true),
		Light: renderSponsorsVariant(handles, title, subtitle, cta, lightP, false),
	}, nil
}

func normalizeSponsorHandles(raw []string) []string {
	var (
		out  []string
		seen = make(map[string]struct{}, len(raw))
	)
	for _, h := range raw {
		trimmed := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(h), "@"))
		if trimmed == "" {
			continue
		}
		key := strings.ToLower(trimmed)
		if _, dup := seen[key]; dup {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, "@"+trimmed)
	}
	return out
}

type sponsorPillPos struct {
	handle string
	x      int
	y      int
	w      int
}

func layoutSponsorPills(handles []string) ([]sponsorPillPos, int) {
	const (
		startX    = 24
		maxRight  = 816
		startY    = 70
		pillH     = 28
		colGap    = 8
		rowGap    = 8
		minHeight = 108
	)
	if len(handles) == 0 {
		return nil, minHeight
	}

	positions := make([]sponsorPillPos, 0, len(handles))
	curX := startX
	curY := startY
	for _, handle := range handles {
		display := handle
		if wrapped := WrapText(handle, 24, 1); len(wrapped) > 0 {
			display = wrapped[0]
		}
		w := utf8.RuneCountInString(display)*8 + 24
		if w < 78 {
			w = 78
		}
		if w > 220 {
			w = 220
		}
		if curX > startX && curX+w > maxRight {
			curX = startX
			curY += pillH + rowGap
		}
		positions = append(positions, sponsorPillPos{
			handle: display,
			x:      curX,
			y:      curY,
			w:      w,
		})
		curX += w + colGap
	}

	height := curY + pillH + 16
	if height < minHeight {
		height = minHeight
	}
	return positions, height
}

func renderSponsorsVariant(handles []string, title, subtitle, cta string, p Palette, dark bool) string {
	const width = 840
	pills, height := layoutSponsorPills(handles)

	ctaBg := p.PrimaryBackground
	if ctaBg == "" {
		ctaBg = p.Primary
	}
	ctaFg := contrastOnFill(ctaBg)
	pillBorder := mixHex(p.Primary, p.MutedBorder, 0.30)

	heartOp := "0.10"
	if dark {
		heartOp = "0.15"
	}

	var b strings.Builder
	fmt.Fprintf(&b, `<svg xmlns="http://www.w3.org/2000/svg" width="%d" height="%d" viewBox="0 0 %d %d" role="img" aria-label="%s">`,
		width, height, width, height, XMLEscape(title))
	fmt.Fprintf(&b, `<title>%s</title>`, XMLEscape(title))
	b.WriteString(`<defs>`)
	fmt.Fprintf(&b, `<clipPath id="sp-clip"><rect x="0.5" y="0.5" width="839" height="%d" rx="14"/></clipPath>`, height-1)
	b.WriteString(`</defs>`)

	b.WriteString(`<style>`)
	fmt.Fprintf(&b, `.sp-title{font-family:%s;font-size:15.5px;font-weight:700;fill:%s}`, fontSans, p.HeadingColor)
	fmt.Fprintf(&b, `.sp-sub{font-family:%s;font-size:12.5px;font-weight:400;fill:%s}`, fontSans, p.MutedColor)
	fmt.Fprintf(&b, `.sp-pill{font-family:%s;font-size:12px;font-weight:500;fill:%s}`, fontMono, p.Color)
	fmt.Fprintf(&b, `.sp-empty{font-family:%s;font-size:12.5px;font-weight:500;fill:%s}`, fontSans, p.MutedColor)
	fmt.Fprintf(&b, `.sp-cta{font-family:%s;font-size:11.5px;font-weight:700;fill:%s;letter-spacing:0.06em}`, fontSans, ctaFg)
	b.WriteString(`@keyframes heart-drift{0%{transform:translateY(8px) scale(0.92);opacity:0}25%{opacity:1}80%{opacity:1}100%{transform:translateY(-14px) scale(1.04);opacity:0}}`)
	b.WriteString(`.sp-heart{animation:heart-drift 7.5s ease-in-out infinite}`)
	b.WriteString(`.hd1{animation-delay:0s}.hd2{animation-delay:2.5s}.hd3{animation-delay:5s}`)
	b.WriteString(reducedMotionCSS)
	b.WriteString(`</style>`)

	b.WriteString(`<g clip-path="url(#sp-clip)">`)
	fmt.Fprintf(&b, `<rect x="0" y="0" width="840" height="%d" fill="%s"/>`, height, p.CardBackground)
	fmt.Fprintf(&b, `<rect x="0" y="0" width="840" height="2.5" fill="%s" opacity="0.28"/>`, p.Primary)

	// Subtle drifting hearts in Primary.
	fmt.Fprintf(&b, `<g opacity="%s" fill="%s">`, heartOp, p.Primary)
	fmt.Fprintf(&b, `<g class="sp-heart hd1"><g transform="translate(560, %d) scale(0.65)"><path d="%s"/></g></g>`, height-36, heartGlyphPath)
	fmt.Fprintf(&b, `<g class="sp-heart hd2"><g transform="translate(625, %d) scale(0.50)"><path d="%s"/></g></g>`, height-46, heartGlyphPath)
	fmt.Fprintf(&b, `<g class="sp-heart hd3"><g transform="translate(675, %d) scale(0.58)"><path d="%s"/></g></g>`, height-32, heartGlyphPath)
	b.WriteString(`</g>`)

	// Header heart icon badge.
	fmt.Fprintf(&b, `<rect x="24" y="18" width="34" height="34" rx="9" fill="%s" stroke="%s" stroke-width="1"/>`, p.CodeBackground, p.MutedBorder)
	fmt.Fprintf(&b, `<g transform="translate(32, 26) scale(0.75)" fill="%s"><path d="%s"/></g>`, p.Primary, heartGlyphPath)

	fmt.Fprintf(&b, `<text x="68" y="32" class="sp-title">%s</text>`, XMLEscape(title))
	fmt.Fprintf(&b, `<text x="68" y="50" class="sp-sub">%s</text>`, XMLEscape(subtitle))

	// SPONSOR CTA pill.
	fmt.Fprintf(&b, `<rect x="708" y="19" width="108" height="32" rx="8" fill="%s"/>`, ctaBg)
	fmt.Fprintf(&b, `<g transform="translate(722, 28) scale(0.583333)" fill="%s"><path d="%s"/></g>`, ctaFg, heartGlyphPath)
	fmt.Fprintf(&b, `<text x="772" y="39" text-anchor="middle" class="sp-cta">%s</text>`, XMLEscape(cta))

	if len(pills) == 0 {
		fmt.Fprintf(&b, `<rect x="24" y="68" width="792" height="26" rx="8" fill="%s" stroke="%s" stroke-width="1" stroke-dasharray="4 3"/>`,
			p.CodeBackground, p.MutedBorder)
		b.WriteString(`<text x="420" y="85" text-anchor="middle" class="sp-empty">Be the first to sponsor this project on GitHub Sponsors</text>`)
	} else {
		for _, pill := range pills {
			fmt.Fprintf(&b, `<rect x="%d" y="%d" width="%d" height="28" rx="14" fill="%s" stroke="%s" stroke-width="1"/>`,
				pill.x, pill.y, pill.w, p.CodeBackground, pillBorder)
			fmt.Fprintf(&b, `<text x="%d" y="%d" text-anchor="middle" class="sp-pill">%s</text>`,
				pill.x+pill.w/2, pill.y+18, XMLEscape(pill.handle))
		}
	}

	b.WriteString(`</g>`)
	fmt.Fprintf(&b, `<rect x="0.5" y="0.5" width="839" height="%d" rx="14" fill="none" stroke="%s" stroke-width="1"/>`, height-1, p.MutedBorder)
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
	fmt.Fprintf(&b, `<rect x="0" y="0" width="840" height="2.5" fill="%s" opacity="0.28"/>`, p.Primary)

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
	pillW := utf8.RuneCountInString(countLabel)*7 + 28
	if pillW < 108 {
		pillW = 108
	}
	fmt.Fprintf(&b, `<rect x="28" y="%d" width="%d" height="26" rx="13" fill="%s" stroke="%s" stroke-width="1"/>`,
		pillY, pillW, p.CodeBackground, pillBorder)
	fmt.Fprintf(&b, `<circle cx="42" cy="%d" r="3.5" fill="%s"/>`, pillY+13, p.Primary)
	fmt.Fprintf(&b, `<text x="52" y="%d" class="cb-pill">%s</text>`, pillY+17, XMLEscape(countLabel))

	if moreLabel != "" {
		moreX := 28 + pillW + 8
		moreW := utf8.RuneCountInString(moreLabel)*7 + 18
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
	fmt.Fprintf(&b, `<rect x="0.5" y="0.5" width="839" height="149" rx="14" fill="none" stroke="%s" stroke-width="1"/>`, p.MutedBorder)
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
