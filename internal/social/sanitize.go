package social

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"
)

var (
	urlFuncRegexp      = regexp.MustCompile(`(?i)url\(\s*(?:'([^']*)'|"([^"]*)"|([^)]*))\s*\)`)
	externalSchemeRe   = regexp.MustCompile(`(?i)(?:https?:|javascript:|vbscript:|//)`)
	cssHoverRe         = regexp.MustCompile(`(?i):hover\b`)
	cssExpressionRe    = regexp.MustCompile(`(?i)expression\s*\(`)
	safeRasterDataRe   = regexp.MustCompile(`(?i)^data:image/(?:png|jpeg|jpg|webp|gif|avif);base64,[A-Za-z0-9+/=\s]+$`)
	cssIDSelectorToken = regexp.MustCompile(`#([A-Za-z_][A-Za-z0-9_-]*)`)
)

type svgNode struct {
	name     string
	attrs    []xml.Attr
	children []*svgNode
	text     string
}

// SanitizeInlineSVG parses raw inline SVG bytes, strips unsafe elements and
// attributes (<script>, <foreignObject>, <a>, event handlers, external URLs,
// and :hover rules), namespaces all internal IDs and references with a short
// SHA-256 prefix, and wraps the content in a nested <svg> element of the
// requested pixel size preserving aspect ratio.
func SanitizeInlineSVG(raw []byte, size int) (string, error) {
	if size <= 0 {
		return "", fmt.Errorf("inline SVG size must be positive, got %d", size)
	}
	if len(bytes.TrimSpace(raw)) == 0 {
		return "", errors.New("inline SVG input is empty")
	}

	root, idSet, err := parseInlineSVGTree(raw)
	if err != nil {
		return "", err
	}

	sum := sha256.Sum256(raw)
	prefix := "i" + hex.EncodeToString(sum[:4]) + "-"

	viewBox := resolveSVGViewBox(root.attrs)

	var b strings.Builder
	fmt.Fprintf(&b, `<svg width="%d" height="%d" viewBox="%s" preserveAspectRatio="xMidYMid meet"`,
		size, size, XMLEscape(viewBox))

	for _, attr := range sanitizeRootAttrs(root.attrs, prefix) {
		fmt.Fprintf(&b, ` %s="%s"`, attr.Name.Local, XMLEscape(attr.Value))
	}
	b.WriteByte('>')

	for _, child := range root.children {
		writeSanitizedNode(&b, child, prefix, idSet)
	}
	b.WriteString("</svg>")

	return b.String(), nil
}

func parseInlineSVGTree(raw []byte) (*svgNode, map[string]struct{}, error) {
	dec := xml.NewDecoder(bytes.NewReader(raw))
	dec.Strict = true

	var (
		root      *svgNode
		stack     []*svgNode
		skipDepth int
		unwrapA   int
		idSet     = make(map[string]struct{})
	)

	for {
		tok, err := dec.Token()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, nil, fmt.Errorf("parsing inline SVG: %w", err)
		}

		switch t := tok.(type) {
		case xml.StartElement:
			local := strings.ToLower(t.Name.Local)
			if skipDepth > 0 {
				skipDepth++
				continue
			}
			if isBlockedSVGElement(local) {
				skipDepth = 1
				continue
			}
			if local == "a" {
				unwrapA++
				continue
			}
			node := &svgNode{
				name:  t.Name.Local,
				attrs: append([]xml.Attr(nil), t.Attr...),
			}
			for _, attr := range t.Attr {
				if strings.EqualFold(attr.Name.Local, "id") {
					if id := strings.TrimSpace(attr.Value); id != "" {
						idSet[id] = struct{}{}
					}
				}
			}
			if root == nil {
				if local != "svg" {
					return nil, nil, fmt.Errorf("inline SVG root element must be <svg>, got <%s>", t.Name.Local)
				}
				root = node
				stack = append(stack, node)
				continue
			}
			parent := stack[len(stack)-1]
			parent.children = append(parent.children, node)
			stack = append(stack, node)

		case xml.EndElement:
			local := strings.ToLower(t.Name.Local)
			if skipDepth > 0 {
				skipDepth--
				continue
			}
			if local == "a" && unwrapA > 0 {
				unwrapA--
				continue
			}
			if len(stack) > 0 {
				stack = stack[:len(stack)-1]
			}

		case xml.CharData:
			if skipDepth > 0 || len(stack) == 0 {
				continue
			}
			text := string(t)
			if strings.TrimSpace(text) == "" {
				continue
			}
			parent := stack[len(stack)-1]
			parent.children = append(parent.children, &svgNode{text: text})
		}
	}

	if root == nil {
		return nil, nil, errors.New("inline SVG contains no <svg> root element")
	}
	return root, idSet, nil
}

func isBlockedSVGElement(lowerName string) bool {
	switch lowerName {
	case "script", "foreignobject", "iframe", "object", "embed", "audio", "video":
		return true
	default:
		return false
	}
}

func resolveSVGViewBox(attrs []xml.Attr) string {
	var widthRaw, heightRaw string
	for _, attr := range attrs {
		switch strings.ToLower(attr.Name.Local) {
		case "viewbox":
			vb := strings.TrimSpace(attr.Value)
			if isValidViewBox(vb) {
				return vb
			}
		case "width":
			widthRaw = attr.Value
		case "height":
			heightRaw = attr.Value
		}
	}
	w, okW := parseSVGLength(widthRaw)
	h, okH := parseSVGLength(heightRaw)
	if okW && okH && w > 0 && h > 0 {
		return fmt.Sprintf("0 0 %s %s", formatSVGNum(w), formatSVGNum(h))
	}
	return "0 0 24 24"
}

func isValidViewBox(vb string) bool {
	if vb == "" {
		return false
	}
	fields := strings.FieldsFunc(vb, func(r rune) bool {
		return unicode.IsSpace(r) || r == ','
	})
	if len(fields) != 4 {
		return false
	}
	for i, f := range fields {
		v, err := strconv.ParseFloat(f, 64)
		if err != nil {
			return false
		}
		if (i == 2 || i == 3) && v <= 0 {
			return false
		}
	}
	return true
}

func parseSVGLength(raw string) (float64, bool) {
	s := strings.TrimSpace(raw)
	s = strings.TrimSuffix(s, "px")
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, false
	}
	v, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0, false
	}
	return v, true
}

func formatSVGNum(v float64) string {
	return strconv.FormatFloat(v, 'f', -1, 64)
}

func sanitizeRootAttrs(attrs []xml.Attr, prefix string) []xml.Attr {
	allowed := map[string]bool{
		"fill":            true,
		"fill-rule":       true,
		"clip-rule":       true,
		"stroke":          true,
		"stroke-width":    true,
		"stroke-linecap":  true,
		"stroke-linejoin": true,
		"color":           true,
		"role":            true,
		"aria-labelledby": true,
	}
	var out []xml.Attr
	for _, attr := range attrs {
		if attr.Name.Space != "" {
			continue
		}
		local := strings.ToLower(attr.Name.Local)
		if !allowed[local] {
			continue
		}
		sanitized, ok := sanitizeAttrValue("svg", local, attr.Value, prefix)
		if !ok {
			continue
		}
		out = append(out, xml.Attr{Name: xml.Name{Local: attr.Name.Local}, Value: sanitized})
	}
	return out
}

func writeSanitizedNode(b *strings.Builder, node *svgNode, prefix string, idSet map[string]struct{}) {
	if node.name == "" {
		b.WriteString(XMLEscape(node.text))
		return
	}

	lower := strings.ToLower(node.name)
	if lower == "style" {
		var rawCSS strings.Builder
		for _, child := range node.children {
			if child.name == "" {
				rawCSS.WriteString(child.text)
			}
		}
		cleaned := sanitizeInlineSVGStyle(rawCSS.String(), prefix, idSet)
		if cleaned == "" {
			return
		}
		b.WriteString("<style>")
		b.WriteString(XMLEscape(cleaned))
		b.WriteString("</style>")
		return
	}

	attrs := sanitizeElementAttrs(lower, node.attrs, prefix)
	// Drop <image> elements that have no safe raster href left.
	if lower == "image" && !hasAttr(attrs, "href") {
		return
	}

	b.WriteByte('<')
	b.WriteString(node.name)
	for _, attr := range attrs {
		fmt.Fprintf(b, ` %s="%s"`, attr.Name.Local, XMLEscape(attr.Value))
	}
	if len(node.children) == 0 {
		b.WriteString("/>")
		return
	}
	b.WriteByte('>')
	for _, child := range node.children {
		writeSanitizedNode(b, child, prefix, idSet)
	}
	b.WriteString("</")
	b.WriteString(node.name)
	b.WriteByte('>')
}

func hasAttr(attrs []xml.Attr, local string) bool {
	for _, attr := range attrs {
		if strings.EqualFold(attr.Name.Local, local) {
			return true
		}
	}
	return false
}

func sanitizeElementAttrs(elemLower string, attrs []xml.Attr, prefix string) []xml.Attr {
	var (
		out      []xml.Attr
		seenHref bool
	)
	for _, attr := range attrs {
		localLower := strings.ToLower(attr.Name.Local)
		if localLower == "xmlns" || strings.EqualFold(attr.Name.Space, "xmlns") {
			continue
		}
		// Strip inline event handlers (onclick, onload, onmouseover, etc.).
		if strings.HasPrefix(localLower, "on") {
			continue
		}
		// Convert xlink:href to SVG2 href and drop other namespaced attributes.
		targetName := attr.Name.Local
		if attr.Name.Space != "" {
			if localLower == "href" && (strings.Contains(strings.ToLower(attr.Name.Space), "xlink") || strings.EqualFold(attr.Name.Space, "xlink")) {
				targetName = "href"
			} else {
				continue
			}
		}
		if strings.EqualFold(targetName, "href") {
			if seenHref {
				continue
			}
			val, ok := sanitizeHrefAttr(elemLower, attr.Value, prefix)
			if !ok {
				continue
			}
			seenHref = true
			out = append(out, xml.Attr{Name: xml.Name{Local: "href"}, Value: val})
			continue
		}

		val, ok := sanitizeAttrValue(elemLower, localLower, attr.Value, prefix)
		if !ok {
			continue
		}
		out = append(out, xml.Attr{Name: xml.Name{Local: targetName}, Value: val})
	}
	return out
}

func sanitizeHrefAttr(elemLower, raw, prefix string) (string, bool) {
	val := strings.TrimSpace(raw)
	if val == "" {
		return "", false
	}
	if strings.HasPrefix(val, "#") {
		ref := strings.TrimSpace(val[1:])
		if ref == "" {
			return "", false
		}
		return "#" + prefix + ref, true
	}
	if elemLower == "image" && safeRasterDataRe.MatchString(val) {
		return val, true
	}
	return "", false
}

func sanitizeAttrValue(_ string, attrLower, raw, prefix string) (string, bool) {
	val := strings.TrimSpace(raw)
	if val == "" {
		return "", false
	}
	switch attrLower {
	case "id":
		return prefix + val, true
	case "aria-labelledby", "aria-describedby":
		fields := strings.Fields(val)
		if len(fields) == 0 {
			return "", false
		}
		for i, f := range fields {
			fields[i] = prefix + f
		}
		return strings.Join(fields, " "), true
	case "style":
		cleaned := sanitizeInlineStyleAttr(val, prefix)
		if cleaned == "" {
			return "", false
		}
		return cleaned, true
	}

	if externalSchemeRe.MatchString(val) || cssHoverRe.MatchString(val) || cssExpressionRe.MatchString(val) {
		return "", false
	}

	rewritten, ok := rewriteURLFragments(val, prefix)
	if !ok {
		return "", false
	}
	return rewritten, true
}

func rewriteURLFragments(val, prefix string) (string, bool) {
	unsafe := false
	rewritten := urlFuncRegexp.ReplaceAllStringFunc(val, func(match string) string {
		sub := urlFuncRegexp.FindStringSubmatch(match)
		target := ""
		for _, g := range sub[1:] {
			if g != "" {
				target = strings.TrimSpace(g)
				break
			}
		}
		if !strings.HasPrefix(target, "#") || len(target) == 1 {
			unsafe = true
			return ""
		}
		return "url(#" + prefix + target[1:] + ")"
	})
	if unsafe {
		return "", false
	}
	return rewritten, true
}

func sanitizeInlineStyleAttr(raw, prefix string) string {
	decls := splitTopLevel(stripCSSComments(raw), ';')
	var kept []string
	for _, decl := range decls {
		decl = strings.TrimSpace(decl)
		if decl == "" {
			continue
		}
		if externalSchemeRe.MatchString(decl) || cssHoverRe.MatchString(decl) || cssExpressionRe.MatchString(decl) {
			continue
		}
		rewritten, ok := rewriteURLFragments(decl, prefix)
		if !ok {
			continue
		}
		kept = append(kept, rewritten)
	}
	return strings.Join(kept, "; ")
}

func sanitizeInlineSVGStyle(raw, prefix string, idSet map[string]struct{}) string {
	cleaned := stripCSSComments(raw)
	var out strings.Builder
	i := 0
	for i < len(cleaned) {
		i = skipWhitespace(cleaned, i)
		if i >= len(cleaned) {
			break
		}
		if cleaned[i] == '}' {
			i++
			continue
		}
		delimIdx, delim := scanToBraceOrSemicolon(cleaned, i)
		if delim == 0 {
			break
		}
		prelude := strings.TrimSpace(cleaned[i:delimIdx])
		if delim == ';' {
			// Skip @import and stray at-rules.
			i = delimIdx + 1
			continue
		}
		blockStart := delimIdx + 1
		blockEnd := findMatchingBrace(cleaned, blockStart)
		body := cleaned[blockStart:blockEnd]
		if blockEnd < len(cleaned) {
			i = blockEnd + 1
		} else {
			i = blockEnd
		}

		lowerPrelude := strings.ToLower(prelude)
		if strings.HasPrefix(lowerPrelude, "@font-face") || strings.HasPrefix(lowerPrelude, "@import") {
			continue
		}
		if strings.HasPrefix(lowerPrelude, "@") {
			// Recursively sanitize nested rules (such as @media).
			nested := sanitizeInlineSVGStyle(body, prefix, idSet)
			if nested != "" && !externalSchemeRe.MatchString(prelude) && !cssHoverRe.MatchString(prelude) {
				out.WriteString(prelude)
				out.WriteString(" { ")
				out.WriteString(nested)
				out.WriteString(" } ")
			}
			continue
		}
		if cssHoverRe.MatchString(prelude) || externalSchemeRe.MatchString(prelude) {
			continue
		}

		sanitizedBody := sanitizeInlineStyleAttr(body, prefix)
		if sanitizedBody == "" {
			continue
		}
		sanitizedSel := namespaceCSSSelectorIDs(prelude, prefix, idSet)
		out.WriteString(sanitizedSel)
		out.WriteString(" { ")
		out.WriteString(sanitizedBody)
		out.WriteString(" } ")
	}
	return strings.TrimSpace(out.String())
}

func namespaceCSSSelectorIDs(selector, prefix string, idSet map[string]struct{}) string {
	if len(idSet) == 0 {
		return selector
	}
	return cssIDSelectorToken.ReplaceAllStringFunc(selector, func(match string) string {
		name := match[1:]
		if _, ok := idSet[name]; ok {
			return "#" + prefix + name
		}
		return match
	})
}

// EmbedRasterIcon encodes raw raster image bytes into a self-contained SVG
// <image> element using a base64 data URI. It returns an empty string when
// raw is empty or size is non-positive.
func EmbedRasterIcon(raw []byte, mime string, size int) string {
	if len(raw) == 0 || size <= 0 {
		return ""
	}
	resolvedMIME := normalizeRasterMIME(mime, raw)
	encoded := base64.StdEncoding.EncodeToString(raw)
	return fmt.Sprintf(`<image href="data:%s;base64,%s" width="%d" height="%d" preserveAspectRatio="xMidYMid meet"/>`,
		resolvedMIME, encoded, size, size)
}

func normalizeRasterMIME(mime string, raw []byte) string {
	candidate := strings.ToLower(strings.TrimSpace(mime))
	if isAllowedRasterMIME(candidate) {
		if candidate == "image/jpg" {
			return "image/jpeg"
		}
		return candidate
	}
	detected := strings.ToLower(strings.TrimSpace(http.DetectContentType(raw)))
	if isAllowedRasterMIME(detected) {
		return detected
	}
	return "image/png"
}

func isAllowedRasterMIME(mime string) bool {
	switch mime {
	case "image/png", "image/jpeg", "image/jpg", "image/webp", "image/gif", "image/avif":
		return true
	default:
		return false
	}
}

// XMLEscape escapes s for safe use in XML element text and quoted attribute
// values, stripping illegal XML 1.0 control characters.
func XMLEscape(s string) string {
	if s == "" {
		return ""
	}
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		if !isValidXMLRune(r) {
			continue
		}
		switch r {
		case '&':
			b.WriteString("&amp;")
		case '<':
			b.WriteString("&lt;")
		case '>':
			b.WriteString("&gt;")
		case '"':
			b.WriteString("&quot;")
		case '\'':
			b.WriteString("&#39;")
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}

func isValidXMLRune(r rune) bool {
	return r == 0x09 || r == 0x0A || r == 0x0D ||
		(r >= 0x20 && r <= 0xD7FF) ||
		(r >= 0xE000 && r <= 0xFFFD) ||
		(r >= 0x10000 && r <= 0x10FFFF)
}

// WrapText wraps s conservatively onto at most maxLines lines, keeping each
// line within maxChars runes where possible and ending with an ellipsis when
// text is truncated.
func WrapText(s string, maxChars, maxLines int) []string {
	if maxLines <= 0 {
		return nil
	}
	words := strings.Fields(s)
	if len(words) == 0 {
		return nil
	}
	if maxChars <= 0 {
		return []string{strings.Join(words, " ")}
	}

	var (
		lines     []string
		current   string
		truncated bool
	)

	for i, word := range words {
		if current == "" {
			if utf8.RuneCountInString(word) <= maxChars {
				current = word
				continue
			}
			// Single word longer than maxChars.
			if len(lines) == maxLines-1 {
				current = truncateRunes(word, maxChars)
				truncated = true
				break
			}
			current = truncateRunes(word, maxChars)
			lines = append(lines, current)
			current = ""
			if i < len(words)-1 && len(lines) == maxLines {
				truncated = true
				break
			}
			continue
		}

		candidate := current + " " + word
		if utf8.RuneCountInString(candidate) <= maxChars {
			current = candidate
			continue
		}

		if len(lines) == maxLines-1 {
			truncated = true
			break
		}
		lines = append(lines, current)
		if utf8.RuneCountInString(word) <= maxChars {
			current = word
		} else {
			current = truncateRunes(word, maxChars)
		}
	}

	if current != "" && len(lines) < maxLines {
		lines = append(lines, current)
	}
	if truncated && len(lines) > 0 {
		lines[len(lines)-1] = appendEllipsis(lines[len(lines)-1], maxChars)
	}
	return lines
}

func truncateRunes(s string, maxChars int) string {
	if maxChars <= 0 {
		return ""
	}
	runes := []rune(s)
	if len(runes) <= maxChars {
		return s
	}
	if maxChars == 1 {
		return "…"
	}
	return string(runes[:maxChars-1]) + "…"
}

func appendEllipsis(line string, maxChars int) string {
	trimmed := strings.TrimRightFunc(line, func(r rune) bool {
		return unicode.IsSpace(r) || r == ',' || r == ';' || r == ':'
	})
	if strings.HasSuffix(trimmed, "…") {
		return trimmed
	}
	runes := []rune(trimmed)
	if maxChars <= 1 {
		return "…"
	}
	if len(runes)+1 <= maxChars {
		return trimmed + "…"
	}
	return strings.TrimSpace(string(runes[:maxChars-1])) + "…"
}
