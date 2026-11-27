// Package social extracts project themes and resolves shared assets for
// generated Open Graph cards and README SVGs.
package social

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"math"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/wimpysworld/tailor/internal/model"
)

// maxStyleCSSSize bounds stylesheet reads from the project root.
const maxStyleCSSSize = 512 << 10

// Palette holds resolved #rrggbb hex colours for one theme mode.
type Palette struct {
	Background        string
	CardBackground    string
	CodeBackground    string
	Color             string
	MutedColor        string
	MutedBorder       string
	Primary           string
	PrimaryBackground string
	HeadingColor      string
}

// Theme holds the resolved dark and light palettes for a project along with
// the default theme mode ("dark" or "light").
type Theme struct {
	Dark        Palette
	Light       Palette
	DefaultMode string
}

// Built-in @digicreon/mucss@1.4.9 default palettes.
var (
	defaultDarkPalette = Palette{
		Background:        "#13171f",
		CardBackground:    "#181c25",
		CodeBackground:    "#1a1f28",
		Color:             "#c2c7d0",
		MutedColor:        "#7b8495",
		MutedBorder:       "#202632",
		Primary:           "#01aaff",
		PrimaryBackground: "#0172ad",
		HeadingColor:      "#f0f1f3",
	}

	defaultLightPalette = Palette{
		Background:        "#ffffff",
		CardBackground:    "#ffffff",
		CodeBackground:    "#f3f5f7",
		Color:             "#373c44",
		MutedColor:        "#646b79",
		MutedBorder:       "#e7eaf0",
		Primary:           "#0172ad",
		PrimaryBackground: "#0172ad",
		HeadingColor:      "#2d3138",
	}
)

// DefaultTheme returns the built-in @digicreon/mucss@1.4.9 theme with dark as
// the default mode.
func DefaultTheme() Theme {
	return Theme{
		Dark:        defaultDarkPalette,
		Light:       defaultLightPalette,
		DefaultMode: model.DefaultSocialTheme,
	}
}

// ExtractTheme reads <pagesPath>/style.css (when present inside root), resolves
// dark and light µCSS palettes with @digicreon/mucss@1.4.9 defaults, and
// applies optional accent and background overrides from cfg.
func ExtractTheme(root *os.Root, pagesPath string, cfg *model.SocialSettings) (Theme, error) {
	theme := DefaultTheme()
	if cfg != nil && cfg.Theme != nil && (*cfg.Theme == "dark" || *cfg.Theme == "light") {
		theme.DefaultMode = *cfg.Theme
	}

	cleanedPages, err := cleanRelativeDir(pagesPath)
	if err != nil {
		return Theme{}, err
	}

	if root != nil {
		cssPath := path.Join(cleanedPages, "style.css")
		data, exists, err := readRootRegularFile(root, cssPath, maxStyleCSSSize)
		if err != nil {
			return Theme{}, err
		}
		if exists {
			darkTokens, lightTokens := parseStylesheetTokens(string(data))
			theme.Dark = buildPalette(darkTokens, defaultDarkPalette, modeDark)
			theme.Light = buildPalette(lightTokens, defaultLightPalette, modeLight)
		}
	}

	if cfg != nil && cfg.Accent != nil && strings.TrimSpace(*cfg.Accent) != "" {
		accent, ok := parseHexColor(strings.TrimSpace(*cfg.Accent))
		if !ok {
			return Theme{}, fmt.Errorf("invalid social.accent colour %q", *cfg.Accent)
		}
		hex := accent.hex()
		theme.Dark.Primary = hex
		theme.Dark.PrimaryBackground = hex
		theme.Light.Primary = hex
		theme.Light.PrimaryBackground = hex
	}

	if cfg != nil && cfg.Background != nil && strings.TrimSpace(*cfg.Background) != "" {
		bg, ok := parseHexColor(strings.TrimSpace(*cfg.Background))
		if !ok {
			return Theme{}, fmt.Errorf("invalid social.background colour %q", *cfg.Background)
		}
		hex := bg.hex()
		if theme.DefaultMode == "light" {
			theme.Light.Background = hex
		} else {
			theme.Dark.Background = hex
		}
	}

	return theme, nil
}

type themeMode int

const (
	modeNone themeMode = iota
	modeLight
	modeDark
	modeBoth
)

type tokenEntry struct {
	value    string
	priority int
	order    int
}

type tokenStore map[string]tokenEntry

func (s tokenStore) set(name, value string, priority, order int) {
	existing, ok := s[name]
	if !ok || priority > existing.priority || (priority == existing.priority && order >= existing.order) {
		s[name] = tokenEntry{value: value, priority: priority, order: order}
	}
}

func defaultTokenStore(mode themeMode) tokenStore {
	defaults := defaultDarkPalette
	if mode == modeLight {
		defaults = defaultLightPalette
	}
	store := tokenStore{
		"--mu-background-color":      {value: defaults.Background},
		"--mu-card-background-color": {value: defaults.CardBackground},
		"--mu-code-background-color": {value: defaults.CodeBackground},
		"--mu-color":                 {value: defaults.Color},
		"--mu-muted-color":           {value: defaults.MutedColor},
		"--mu-muted-border-color":    {value: defaults.MutedBorder},
		"--mu-primary":               {value: defaults.Primary},
		"--mu-primary-background":    {value: defaults.PrimaryBackground},
		"--mu-h1-color":              {value: defaults.HeadingColor},
	}
	if mode == modeLight {
		// In @digicreon/mucss@1.4.9 light mode, card background tracks background.
		store["--mu-card-background-color"] = tokenEntry{value: "var(--mu-background-color)"}
	}
	return store
}

func parseStylesheetTokens(css string) (darkTokens, lightTokens tokenStore) {
	darkTokens = defaultTokenStore(modeDark)
	lightTokens = defaultTokenStore(modeLight)
	cleaned := stripCSSComments(css)
	order := 0
	parseCSSBlock(cleaned, modeNone, darkTokens, lightTokens, &order)
	return darkTokens, lightTokens
}

func buildPalette(tokens tokenStore, defaults Palette, mode themeMode) Palette {
	resolve := func(prop, fallback string) (string, bool) {
		entry, ok := tokens[prop]
		if !ok {
			return fallback, false
		}
		if entry.priority == 0 && !strings.Contains(strings.ToLower(entry.value), "var(") {
			return fallback, false
		}
		c, resolved := resolveColorExpr(entry.value, tokens, mode, make(map[string]bool), 0)
		if !resolved {
			return fallback, false
		}
		return c.hex(), entry.priority > 0
	}

	p := Palette{}
	p.Background, _ = resolve("--mu-background-color", defaults.Background)
	p.CardBackground, _ = resolve("--mu-card-background-color", defaults.CardBackground)
	p.CodeBackground, _ = resolve("--mu-code-background-color", defaults.CodeBackground)

	var colorOverridden bool
	p.Color, colorOverridden = resolve("--mu-color", defaults.Color)
	p.MutedColor, _ = resolve("--mu-muted-color", defaults.MutedColor)
	p.MutedBorder, _ = resolve("--mu-muted-border-color", defaults.MutedBorder)

	var primaryOverridden bool
	p.Primary, primaryOverridden = resolve("--mu-primary", defaults.Primary)

	primaryBgFallback := defaults.PrimaryBackground
	if primaryOverridden {
		primaryBgFallback = p.Primary
	}
	p.PrimaryBackground, _ = resolve("--mu-primary-background", primaryBgFallback)

	headingFallback := defaults.HeadingColor
	if colorOverridden {
		headingFallback = p.Color
	}
	p.HeadingColor, _ = resolve("--mu-h1-color", headingFallback)

	return p
}

// stripCSSComments removes /* ... */ comments while keeping quoted strings intact.
func stripCSSComments(src string) string {
	var b strings.Builder
	b.Grow(len(src))

	var quote byte
	for i := 0; i < len(src); {
		ch := src[i]
		if quote != 0 {
			b.WriteByte(ch)
			if ch == '\\' && i+1 < len(src) {
				i++
				b.WriteByte(src[i])
			} else if ch == quote {
				quote = 0
			}
			i++
			continue
		}
		if ch == '"' || ch == '\'' {
			quote = ch
			b.WriteByte(ch)
			i++
			continue
		}
		if ch == '/' && i+1 < len(src) && src[i+1] == '*' {
			end := strings.Index(src[i+2:], "*/")
			if end < 0 {
				break
			}
			b.WriteByte(' ')
			i += end + 4
			continue
		}
		b.WriteByte(ch)
		i++
	}
	return b.String()
}

// parseCSSBlock walks CSS rules and nested @media/@layer/@supports blocks,
// recording custom property declarations for matching root theme selectors.
func parseCSSBlock(src string, mediaMode themeMode, darkTokens, lightTokens tokenStore, order *int) {
	i := 0
	for i < len(src) {
		i = skipWhitespace(src, i)
		if i >= len(src) {
			return
		}
		if src[i] == '}' {
			i++
			continue
		}

		preludeStart := i
		delimIdx, delim := scanToBraceOrSemicolon(src, i)
		if delim == 0 {
			return
		}
		prelude := strings.TrimSpace(src[preludeStart:delimIdx])
		if delim == ';' {
			i = delimIdx + 1
			continue
		}

		// delim == '{'
		blockStart := delimIdx + 1
		blockEnd := findMatchingBrace(src, blockStart)
		body := src[blockStart:blockEnd]
		if blockEnd < len(src) {
			i = blockEnd + 1
		} else {
			i = blockEnd
		}

		if strings.HasPrefix(prelude, "@") {
			handleAtRule(prelude, body, mediaMode, darkTokens, lightTokens, order)
			continue
		}

		target, priority, ok := classifySelectorList(prelude, mediaMode)
		if !ok {
			continue
		}
		recordDeclarations(body, target, priority, darkTokens, lightTokens, order)
	}
}

func handleAtRule(prelude, body string, mediaMode themeMode, darkTokens, lightTokens tokenStore, order *int) {
	lower := strings.ToLower(prelude)
	switch {
	case strings.HasPrefix(lower, "@media"):
		nextMode, ok := parseMediaColorScheme(lower, mediaMode)
		if ok {
			parseCSSBlock(body, nextMode, darkTokens, lightTokens, order)
		}
	case strings.HasPrefix(lower, "@layer"), strings.HasPrefix(lower, "@supports"):
		parseCSSBlock(body, mediaMode, darkTokens, lightTokens, order)
	}
}

func parseMediaColorScheme(lowerPrelude string, inherited themeMode) (themeMode, bool) {
	idx := strings.Index(lowerPrelude, "prefers-color-scheme")
	if idx < 0 {
		return modeNone, false
	}
	rest := lowerPrelude[idx+len("prefers-color-scheme"):]
	colon := strings.IndexByte(rest, ':')
	if colon < 0 {
		return modeNone, false
	}
	rest = strings.TrimSpace(rest[colon+1:])
	var mode themeMode
	switch {
	case strings.HasPrefix(rest, "dark"):
		mode = modeDark
	case strings.HasPrefix(rest, "light"):
		mode = modeLight
	default:
		return modeNone, false
	}
	if inherited != modeNone && inherited != mode {
		return modeNone, false
	}
	return mode, true
}

func classifySelectorList(prelude string, mediaMode themeMode) (themeMode, int, bool) {
	parts := splitTopLevel(prelude, ',')
	var (
		hasLight, hasDark, hasBoth bool
		lightPri, darkPri, bothPri int
	)

	for _, part := range parts {
		target, pri, ok := classifySelector(part, mediaMode)
		if !ok {
			continue
		}
		switch target {
		case modeLight:
			hasLight = true
			lightPri = max(lightPri, pri)
		case modeDark:
			hasDark = true
			darkPri = max(darkPri, pri)
		case modeBoth:
			hasBoth = true
			bothPri = max(bothPri, pri)
		}
	}

	// Selectors like `:root, :root[data-theme="light"]` scope light overrides
	// without overwriting dark-mode defaults.
	switch {
	case hasLight && hasDark:
		return modeBoth, max(lightPri, darkPri), true
	case hasLight:
		return modeLight, lightPri, true
	case hasDark:
		return modeDark, darkPri, true
	case hasBoth:
		return modeBoth, bothPri, true
	default:
		return modeNone, 0, false
	}
}

// classifySelector checks whether a single selector targets the document root
// without combinators or non-root classes/elements.
func classifySelector(sel string, mediaMode themeMode) (themeMode, int, bool) {
	s := strings.TrimSpace(sel)
	if s == "" {
		return modeNone, 0, false
	}

	var (
		rootCount int
		explicit  themeMode
		negated   themeMode
		notAny    bool
	)

	i := 0
	if hasPrefixFold(s, "html") && (len(s) == 4 || s[4] == ':' || s[4] == '[') {
		rootCount++
		i = 4
	}

	for i < len(s) {
		switch {
		case hasPrefixFold(s[i:], ":root"):
			rootCount++
			i += len(":root")
		case hasPrefixFold(s[i:], ":host("):
			end := findMatchingParen(s, i+len(":host("))
			if end >= len(s) {
				return modeNone, 0, false
			}
			inner := strings.TrimSpace(s[i+len(":host(") : end])
			t, _, ok := classifySelector(":root"+inner, modeNone)
			if !ok {
				return modeNone, 0, false
			}
			rootCount++
			switch t {
			case modeDark:
				explicit = modeDark
			case modeLight:
				explicit = modeLight
			}
			i = end + 1
		case hasPrefixFold(s[i:], ":host"):
			rootCount++
			i += len(":host")
		case hasPrefixFold(s[i:], ":not("):
			end := findMatchingParen(s, i+len(":not("))
			if end >= len(s) {
				return modeNone, 0, false
			}
			inner := strings.TrimSpace(s[i+len(":not(") : end])
			attrMode, isAny, ok := parseDataThemeAttr(inner)
			if !ok {
				return modeNone, 0, false
			}
			if isAny {
				notAny = true
			} else {
				negated = attrMode
			}
			i = end + 1
		case s[i] == '[':
			end := strings.IndexByte(s[i:], ']')
			if end < 0 {
				return modeNone, 0, false
			}
			attr := s[i : i+end+1]
			attrMode, isAny, ok := parseDataThemeAttr(attr)
			if !ok || isAny {
				return modeNone, 0, false
			}
			explicit = attrMode
			i += end + 1
		default:
			// Reject whitespace combinators, classes, IDs, or other selectors.
			return modeNone, 0, false
		}
	}

	if rootCount == 0 && explicit == modeNone {
		return modeNone, 0, false
	}

	if explicit != modeNone {
		if mediaMode != modeNone && mediaMode != explicit {
			return modeNone, 0, false
		}
		if rootCount > 0 {
			return explicit, 3, true
		}
		return explicit, 2, true
	}

	if negated != modeNone {
		inferred := modeLight
		if negated == modeLight {
			inferred = modeDark
		}
		if mediaMode != modeNone && mediaMode != inferred {
			return modeNone, 0, false
		}
		return inferred, 2, true
	}

	if mediaMode != modeNone {
		return mediaMode, 2, true
	}
	if notAny {
		return modeBoth, 1, true
	}
	if rootCount >= 2 {
		return modeBoth, 2, true
	}
	return modeBoth, 1, true
}

func parseDataThemeAttr(raw string) (mode themeMode, anyAttr bool, ok bool) {
	s := strings.TrimSpace(raw)
	if len(s) < 2 || s[0] != '[' || s[len(s)-1] != ']' {
		return modeNone, false, false
	}
	inner := strings.TrimSpace(s[1 : len(s)-1])
	if strings.EqualFold(inner, "data-theme") {
		return modeNone, true, true
	}
	name, val, found := strings.Cut(inner, "=")
	if !found || !strings.EqualFold(strings.TrimSpace(name), "data-theme") {
		return modeNone, false, false
	}
	val = strings.TrimSpace(val)
	if len(val) >= 2 && ((val[0] == '"' && val[len(val)-1] == '"') || (val[0] == '\'' && val[len(val)-1] == '\'')) {
		val = val[1 : len(val)-1]
	}
	switch strings.ToLower(strings.TrimSpace(val)) {
	case "dark":
		return modeDark, false, true
	case "light":
		return modeLight, false, true
	default:
		return modeNone, false, false
	}
}

func recordDeclarations(body string, target themeMode, priority int, darkTokens, lightTokens tokenStore, order *int) {
	decls := splitTopLevel(body, ';')
	for _, decl := range decls {
		colon := strings.IndexByte(decl, ':')
		if colon < 0 {
			continue
		}
		name := strings.TrimSpace(decl[:colon])
		if !strings.HasPrefix(name, "--") {
			continue
		}
		value := strings.TrimSpace(decl[colon+1:])
		if value == "" {
			continue
		}

		declPriority := priority
		if stripped, important := stripImportant(value); important {
			value = stripped
			declPriority += 10
		}

		*order++
		switch target {
		case modeDark:
			darkTokens.set(name, value, declPriority, *order)
		case modeLight:
			lightTokens.set(name, value, declPriority, *order)
		case modeBoth:
			darkTokens.set(name, value, declPriority, *order)
			lightTokens.set(name, value, declPriority, *order)
		}
	}
}

func stripImportant(val string) (string, bool) {
	lower := strings.ToLower(val)
	idx := strings.LastIndex(lower, "!important")
	if idx < 0 || strings.TrimSpace(lower[idx+len("!important"):]) != "" {
		return val, false
	}
	return strings.TrimSpace(val[:idx]), true
}

type rgbColor struct {
	r, g, b, a float64
}

func (c rgbColor) hex() string {
	return fmt.Sprintf("#%02x%02x%02x", clampByte(c.r), clampByte(c.g), clampByte(c.b))
}

func clampByte(v float64) uint8 {
	rounded := math.Round(v)
	if rounded < 0 {
		return 0
	}
	if rounded > 255 {
		return 255
	}
	return uint8(rounded)
}

const maxColorResolveDepth = 32

func resolveColorExpr(expr string, tokens tokenStore, mode themeMode, visiting map[string]bool, depth int) (rgbColor, bool) {
	if depth > maxColorResolveDepth {
		return rgbColor{}, false
	}
	s := strings.TrimSpace(expr)
	if s == "" {
		return rgbColor{}, false
	}

	if strings.HasPrefix(s, "#") {
		return parseHexColor(s)
	}

	switch strings.ToLower(s) {
	case "white":
		return rgbColor{r: 255, g: 255, b: 255, a: 1}, true
	case "black":
		return rgbColor{r: 0, g: 0, b: 0, a: 1}, true
	}

	fnName, args, ok := parseFuncCall(s)
	if !ok {
		return rgbColor{}, false
	}

	switch strings.ToLower(fnName) {
	case "rgb", "rgba":
		return parseRGBFunc(args)
	case "var":
		return resolveVarFunc(args, tokens, mode, visiting, depth+1)
	case "light-dark":
		return resolveLightDarkFunc(args, tokens, mode, visiting, depth+1)
	case "color-mix":
		return resolveColorMixFunc(args, tokens, mode, visiting, depth+1)
	default:
		return rgbColor{}, false
	}
}

func parseHexColor(s string) (rgbColor, bool) {
	if !strings.HasPrefix(s, "#") {
		return rgbColor{}, false
	}
	h := s[1:]
	switch len(h) {
	case 3, 4:
		r, ok1 := parseHexNibble(h[0])
		g, ok2 := parseHexNibble(h[1])
		b, ok3 := parseHexNibble(h[2])
		if !ok1 || !ok2 || !ok3 {
			return rgbColor{}, false
		}
		a := 1.0
		if len(h) == 4 {
			an, ok4 := parseHexNibble(h[3])
			if !ok4 {
				return rgbColor{}, false
			}
			a = float64(an*17) / 255.0
		}
		return rgbColor{r: float64(r * 17), g: float64(g * 17), b: float64(b * 17), a: a}, true
	case 6, 8:
		r, err1 := strconv.ParseUint(h[0:2], 16, 8)
		g, err2 := strconv.ParseUint(h[2:4], 16, 8)
		b, err3 := strconv.ParseUint(h[4:6], 16, 8)
		if err1 != nil || err2 != nil || err3 != nil {
			return rgbColor{}, false
		}
		a := 1.0
		if len(h) == 8 {
			ab, err4 := strconv.ParseUint(h[6:8], 16, 8)
			if err4 != nil {
				return rgbColor{}, false
			}
			a = float64(ab) / 255.0
		}
		return rgbColor{r: float64(r), g: float64(g), b: float64(b), a: a}, true
	default:
		return rgbColor{}, false
	}
}

func parseHexNibble(b byte) (uint8, bool) {
	switch {
	case b >= '0' && b <= '9':
		return b - '0', true
	case b >= 'a' && b <= 'f':
		return b - 'a' + 10, true
	case b >= 'A' && b <= 'F':
		return b - 'A' + 10, true
	default:
		return 0, false
	}
}

func parseRGBFunc(args string) (rgbColor, bool) {
	var channels []string
	alphaStr := ""

	if strings.Contains(args, ",") {
		parts := splitTopLevel(args, ',')
		if len(parts) != 3 && len(parts) != 4 {
			return rgbColor{}, false
		}
		channels = parts[:3]
		if len(parts) == 4 {
			alphaStr = parts[3]
		}
	} else {
		colorPart, aPart, hasSlash := strings.Cut(args, "/")
		channels = strings.Fields(colorPart)
		if len(channels) != 3 {
			return rgbColor{}, false
		}
		if hasSlash {
			alphaStr = strings.TrimSpace(aPart)
			if alphaStr == "" {
				return rgbColor{}, false
			}
		}
	}

	r, ok1 := parseRGBChannel(channels[0])
	g, ok2 := parseRGBChannel(channels[1])
	b, ok3 := parseRGBChannel(channels[2])
	if !ok1 || !ok2 || !ok3 {
		return rgbColor{}, false
	}

	a := 1.0
	if alphaStr != "" {
		parsedA, okA := parseAlpha(alphaStr)
		if !okA {
			return rgbColor{}, false
		}
		a = parsedA
	}
	return rgbColor{r: r, g: g, b: b, a: a}, true
}

func parseRGBChannel(raw string) (float64, bool) {
	s := strings.TrimSpace(raw)
	if before, ok := strings.CutSuffix(s, "%"); ok {
		pct, err := strconv.ParseFloat(strings.TrimSpace(before), 64)
		if err != nil || math.IsNaN(pct) || math.IsInf(pct, 0) {
			return 0, false
		}
		return min(max((pct*255.0)/100.0, 0), 255), true
	}
	v, err := strconv.ParseFloat(s, 64)
	if err != nil || math.IsNaN(v) || math.IsInf(v, 0) {
		return 0, false
	}
	return min(max(v, 0), 255), true
}

func parseAlpha(raw string) (float64, bool) {
	s := strings.TrimSpace(raw)
	if before, ok := strings.CutSuffix(s, "%"); ok {
		pct, err := strconv.ParseFloat(strings.TrimSpace(before), 64)
		if err != nil || math.IsNaN(pct) || math.IsInf(pct, 0) {
			return 0, false
		}
		return min(max(pct/100.0, 0), 1), true
	}
	v, err := strconv.ParseFloat(s, 64)
	if err != nil || math.IsNaN(v) || math.IsInf(v, 0) {
		return 0, false
	}
	return min(max(v, 0), 1), true
}

func resolveVarFunc(args string, tokens tokenStore, mode themeMode, visiting map[string]bool, depth int) (rgbColor, bool) {
	name, fallback, hasFallback := splitFirstTopLevelComma(args)
	name = strings.TrimSpace(name)
	if !strings.HasPrefix(name, "--") {
		return rgbColor{}, false
	}

	if !visiting[name] {
		if entry, ok := tokens[name]; ok {
			visiting[name] = true
			c, resolved := resolveColorExpr(entry.value, tokens, mode, visiting, depth)
			delete(visiting, name)
			if resolved {
				return c, true
			}
		}
	}

	if hasFallback {
		return resolveColorExpr(fallback, tokens, mode, visiting, depth)
	}
	return rgbColor{}, false
}

func resolveLightDarkFunc(args string, tokens tokenStore, mode themeMode, visiting map[string]bool, depth int) (rgbColor, bool) {
	parts := splitTopLevel(args, ',')
	if len(parts) != 2 {
		return rgbColor{}, false
	}
	if mode == modeLight {
		return resolveColorExpr(parts[0], tokens, mode, visiting, depth)
	}
	return resolveColorExpr(parts[1], tokens, mode, visiting, depth)
}

func resolveColorMixFunc(args string, tokens tokenStore, mode themeMode, visiting map[string]bool, depth int) (rgbColor, bool) {
	parts := splitTopLevel(args, ',')
	if len(parts) != 3 {
		return rgbColor{}, false
	}
	space := strings.Join(strings.Fields(strings.ToLower(parts[0])), " ")
	if space != "in srgb" {
		return rgbColor{}, false
	}

	c1Expr, p1, hasP1, ok1 := parseColorMixComponent(parts[1])
	c2Expr, p2, hasP2, ok2 := parseColorMixComponent(parts[2])
	if !ok1 || !ok2 {
		return rgbColor{}, false
	}

	switch {
	case !hasP1 && !hasP2:
		p1, p2 = 50, 50
	case hasP1 && !hasP2:
		p2 = 100 - p1
	case !hasP1 && hasP2:
		p1 = 100 - p2
	}

	sum := p1 + p2
	if sum <= 0 {
		return rgbColor{}, false
	}

	c1, ok1 := resolveColorExpr(c1Expr, tokens, mode, visiting, depth)
	c2, ok2 := resolveColorExpr(c2Expr, tokens, mode, visiting, depth)
	if !ok1 || !ok2 {
		return rgbColor{}, false
	}

	w1 := p1 / sum
	w2 := p2 / sum

	a1 := c1.a * w1
	a2 := c2.a * w2
	alpha := a1 + a2
	if alpha <= 0 {
		return rgbColor{r: 0, g: 0, b: 0, a: 0}, true
	}

	alphaMult := 1.0
	if sum < 100 {
		alphaMult = sum / 100.0
	}
	return rgbColor{
		r: min(max((c1.r*a1+c2.r*a2)/alpha, 0), 255),
		g: min(max((c1.g*a1+c2.g*a2)/alpha, 0), 255),
		b: min(max((c1.b*a1+c2.b*a2)/alpha, 0), 255),
		a: min(max(alpha*alphaMult, 0), 1),
	}, true
}

func parseColorMixComponent(raw string) (colorExpr string, pct float64, hasPct, ok bool) {
	tokens := splitTopLevelWhitespace(raw)
	switch len(tokens) {
	case 1:
		return tokens[0], 0, false, true
	case 2:
		if p, valid := parsePercentage(tokens[1]); valid {
			return tokens[0], p, true, true
		}
		if p, valid := parsePercentage(tokens[0]); valid {
			return tokens[1], p, true, true
		}
		return "", 0, false, false
	default:
		return "", 0, false, false
	}
}

func parsePercentage(raw string) (float64, bool) {
	before, ok := strings.CutSuffix(strings.TrimSpace(raw), "%")
	if !ok {
		return 0, false
	}
	v, err := strconv.ParseFloat(strings.TrimSpace(before), 64)
	if err != nil || math.IsNaN(v) || math.IsInf(v, 0) || v < 0 || v > 100 {
		return 0, false
	}
	return v, true
}

func parseFuncCall(s string) (name, args string, ok bool) {
	openIdx := strings.IndexByte(s, '(')
	if openIdx <= 0 || s[len(s)-1] != ')' {
		return "", "", false
	}
	if findMatchingParen(s, openIdx+1) != len(s)-1 {
		return "", "", false
	}
	return strings.TrimSpace(s[:openIdx]), s[openIdx+1 : len(s)-1], true
}

func skipWhitespace(s string, i int) int {
	for i < len(s) && (s[i] == ' ' || s[i] == '\t' || s[i] == '\n' || s[i] == '\r' || s[i] == '\f') {
		i++
	}
	return i
}

func scanToBraceOrSemicolon(s string, start int) (int, byte) {
	var quote byte
	parenDepth := 0
	bracketDepth := 0
	for i := start; i < len(s); i++ {
		ch := s[i]
		if quote != 0 {
			if ch == '\\' && i+1 < len(s) {
				i++
			} else if ch == quote {
				quote = 0
			}
			continue
		}
		switch ch {
		case '"', '\'':
			quote = ch
		case '(':
			parenDepth++
		case ')':
			if parenDepth > 0 {
				parenDepth--
			}
		case '[':
			bracketDepth++
		case ']':
			if bracketDepth > 0 {
				bracketDepth--
			}
		case '{', ';':
			if parenDepth == 0 && bracketDepth == 0 {
				return i, ch
			}
		}
	}
	return len(s), 0
}

func findMatchingBrace(s string, start int) int {
	var quote byte
	depth := 1
	for i := start; i < len(s); i++ {
		ch := s[i]
		if quote != 0 {
			if ch == '\\' && i+1 < len(s) {
				i++
			} else if ch == quote {
				quote = 0
			}
			continue
		}
		switch ch {
		case '"', '\'':
			quote = ch
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				return i
			}
		}
	}
	return len(s)
}

func findMatchingParen(s string, start int) int {
	var quote byte
	depth := 1
	for i := start; i < len(s); i++ {
		ch := s[i]
		if quote != 0 {
			if ch == '\\' && i+1 < len(s) {
				i++
			} else if ch == quote {
				quote = 0
			}
			continue
		}
		switch ch {
		case '"', '\'':
			quote = ch
		case '(':
			depth++
		case ')':
			depth--
			if depth == 0 {
				return i
			}
		}
	}
	return len(s)
}

func splitTopLevel(s string, sep byte) []string {
	var parts []string
	var quote byte
	parenDepth := 0
	bracketDepth := 0
	start := 0

	for i := 0; i < len(s); i++ {
		ch := s[i]
		if quote != 0 {
			if ch == '\\' && i+1 < len(s) {
				i++
			} else if ch == quote {
				quote = 0
			}
			continue
		}
		switch ch {
		case '"', '\'':
			quote = ch
		case '(':
			parenDepth++
		case ')':
			if parenDepth > 0 {
				parenDepth--
			}
		case '[':
			bracketDepth++
		case ']':
			if bracketDepth > 0 {
				bracketDepth--
			}
		default:
			if ch == sep && parenDepth == 0 && bracketDepth == 0 {
				parts = append(parts, strings.TrimSpace(s[start:i]))
				start = i + 1
			}
		}
	}
	parts = append(parts, strings.TrimSpace(s[start:]))
	return parts
}

func splitFirstTopLevelComma(s string) (left, right string, found bool) {
	var quote byte
	parenDepth := 0
	bracketDepth := 0
	for i := 0; i < len(s); i++ {
		ch := s[i]
		if quote != 0 {
			if ch == '\\' && i+1 < len(s) {
				i++
			} else if ch == quote {
				quote = 0
			}
			continue
		}
		switch ch {
		case '"', '\'':
			quote = ch
		case '(':
			parenDepth++
		case ')':
			if parenDepth > 0 {
				parenDepth--
			}
		case '[':
			bracketDepth++
		case ']':
			if bracketDepth > 0 {
				bracketDepth--
			}
		case ',':
			if parenDepth == 0 && bracketDepth == 0 {
				return s[:i], s[i+1:], true
			}
		}
	}
	return s, "", false
}

func splitTopLevelWhitespace(s string) []string {
	var tokens []string
	var quote byte
	parenDepth := 0
	start := -1

	for i := 0; i < len(s); i++ {
		ch := s[i]
		if quote != 0 {
			if ch == '\\' && i+1 < len(s) {
				i++
			} else if ch == quote {
				quote = 0
			}
			continue
		}
		isSpace := ch == ' ' || ch == '\t' || ch == '\n' || ch == '\r' || ch == '\f'
		if isSpace && parenDepth == 0 {
			if start >= 0 {
				tokens = append(tokens, s[start:i])
				start = -1
			}
			continue
		}
		if start < 0 {
			start = i
		}
		switch ch {
		case '"', '\'':
			quote = ch
		case '(':
			parenDepth++
		case ')':
			if parenDepth > 0 {
				parenDepth--
			}
		}
	}
	if start >= 0 {
		tokens = append(tokens, s[start:])
	}
	return tokens
}

func hasPrefixFold(s, prefix string) bool {
	return len(s) >= len(prefix) && strings.EqualFold(s[:len(prefix)], prefix)
}

func cleanRelativeDir(dir string) (string, error) {
	trimmed := strings.TrimSpace(dir)
	if trimmed == "" {
		return "pages", nil
	}
	if strings.ContainsRune(trimmed, '\\') || strings.HasPrefix(trimmed, "/") {
		return "", fmt.Errorf("social pages path %q must be a relative path", dir)
	}
	cleaned := path.Clean(trimmed)
	if cleaned == ".." || strings.HasPrefix(cleaned, "../") {
		return "", fmt.Errorf("social pages path %q escapes project root", dir)
	}
	return cleaned, nil
}

func checkRootParents(root *os.Root, relPath string) error {
	parent := path.Dir(relPath)
	if parent == "." || parent == "" {
		return nil
	}
	current := ""
	for component := range strings.SplitSeq(parent, "/") {
		if current == "" {
			current = component
		} else {
			current = current + "/" + component
		}
		info, err := root.Lstat(filepath.FromSlash(current))
		if err != nil {
			if errors.Is(err, fs.ErrNotExist) {
				return nil
			}
			return fmt.Errorf("checking parent %q: %w", current, err)
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("parent %q is a symlink", current)
		}
		if !info.IsDir() {
			return fmt.Errorf("parent %q is not a directory", current)
		}
	}
	return nil
}

func checkRootRegularFile(root *os.Root, relPath string, maxSize int64) (bool, error) {
	if err := checkRootParents(root, relPath); err != nil {
		return false, err
	}
	osPath := filepath.FromSlash(relPath)
	info, err := root.Lstat(osPath)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return false, nil
		}
		return false, fmt.Errorf("inspecting %q: %w", relPath, err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return false, fmt.Errorf("%q is a symlink", relPath)
	}
	if !info.Mode().IsRegular() {
		return false, fmt.Errorf("%q is not a regular file", relPath)
	}
	if info.Size() > maxSize {
		return false, fmt.Errorf("%q exceeds maximum size of %d bytes", relPath, maxSize)
	}
	return true, nil
}

func readRootRegularFile(root *os.Root, relPath string, maxSize int64) ([]byte, bool, error) {
	exists, err := checkRootRegularFile(root, relPath, maxSize)
	if err != nil || !exists {
		return nil, false, err
	}

	file, err := root.Open(filepath.FromSlash(relPath))
	if err != nil {
		return nil, false, fmt.Errorf("opening %q: %w", relPath, err)
	}
	defer file.Close()

	info, err := file.Stat()
	if err != nil {
		return nil, false, fmt.Errorf("reading metadata for %q: %w", relPath, err)
	}
	if !info.Mode().IsRegular() {
		return nil, false, fmt.Errorf("%q is not a regular file", relPath)
	}
	if info.Size() > maxSize {
		return nil, false, fmt.Errorf("%q exceeds maximum size of %d bytes", relPath, maxSize)
	}

	data, err := io.ReadAll(io.LimitReader(file, maxSize+1))
	if err != nil {
		return nil, false, fmt.Errorf("reading %q: %w", relPath, err)
	}
	if int64(len(data)) > maxSize {
		return nil, false, fmt.Errorf("%q exceeds maximum size of %d bytes", relPath, maxSize)
	}
	return data, true, nil
}
