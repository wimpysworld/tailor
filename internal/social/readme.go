package social

import (
	"fmt"
	"html"
	"path"
	"slices"
	"strings"
)

// Supported README social marker section names.
const (
	MarkerHeader       = "header"
	MarkerLinks        = "links"
	MarkerCommunity    = "community"
	MarkerPackages     = "packages"
	MarkerStats        = "stats"
	MarkerStars        = "stars"
	MarkerSponsors     = "sponsors"
	MarkerContributors = "contributors"
)

// DefaultStaticSocialDir is the default project-relative POSIX directory for
// static README SVGs.
const DefaultStaticSocialDir = "pages/social"

// ReadmeMarkerNames lists all supported README social marker names in
// canonical order.
var ReadmeMarkerNames = []string{
	MarkerHeader,
	MarkerLinks,
	MarkerCommunity,
	MarkerPackages,
	MarkerStats,
	MarkerSponsors,
	MarkerStars,
	MarkerContributors,
}

// MarkerStart returns the opening HTML comment marker for a social section.
func MarkerStart(name string) string {
	return "<!-- tailor:social-" + name + ":start -->"
}

// MarkerEnd returns the closing HTML comment marker for a social section.
func MarkerEnd(name string) string {
	return "<!-- tailor:social-" + name + ":end -->"
}

// UpdateReadmeMarkers replaces the content inside existing standalone
// <!-- tailor:social-<name>:start --> / <!-- tailor:social-<name>:end -->
// marker pairs in content using blocks. Markers not present in content are
// ignored and surrounding prose is preserved untouched.
func UpdateReadmeMarkers(content string, blocks map[string]string) (updated string, changed bool, err error) {
	for name := range blocks {
		if !slices.Contains(ReadmeMarkerNames, name) {
			return "", false, fmt.Errorf("unknown README social marker %q", name)
		}
	}

	newline := "\n"
	if strings.Contains(content, "\r\n") {
		newline = "\r\n"
	}

	current := content
	for _, name := range ReadmeMarkerNames {
		body, ok := blocks[name]
		if !ok {
			continue
		}
		next, err := replaceMarkerBlock(current, name, body, newline)
		if err != nil {
			return "", false, err
		}
		current = next
	}

	return current, current != content, nil
}

func replaceMarkerBlock(source, name, body, newline string) (string, error) {
	startMarker := MarkerStart(name)
	endMarker := MarkerEnd(name)

	startCount := strings.Count(source, startMarker)
	endCount := strings.Count(source, endMarker)
	if startCount == 0 && endCount == 0 {
		return source, nil
	}
	if startCount != 1 || endCount != 1 {
		return "", fmt.Errorf("social marker %q requires exactly one %s and one %s (found %d start, %d end)",
			name, startMarker, endMarker, startCount, endCount)
	}

	startIdx := strings.Index(source, startMarker)
	endIdx := strings.Index(source, endMarker)
	if startIdx >= endIdx {
		return "", fmt.Errorf("social markers for %q are reversed", name)
	}

	startLineEnd, err := checkStandaloneMarkerLine(source, startIdx, startMarker, name)
	if err != nil {
		return "", err
	}
	endLineStart, err := checkStandaloneMarkerLineStart(source, endIdx, endMarker, name)
	if err != nil {
		return "", err
	}

	// Reject overlapping markers from other sections inside this block.
	inner := source[startLineEnd:endLineStart]
	for _, other := range ReadmeMarkerNames {
		if other == name {
			continue
		}
		if strings.Contains(inner, MarkerStart(other)) || strings.Contains(inner, MarkerEnd(other)) {
			return "", fmt.Errorf("social marker %q overlaps with marker %q", name, other)
		}
	}

	normalized := normalizeNewlines(body, newline)
	normalized = strings.TrimRight(normalized, "\r\n")

	var replacement string
	if normalized == "" {
		replacement = newline
	} else {
		replacement = newline + normalized + newline
	}

	return source[:startLineEnd] + replacement + source[endLineStart:], nil
}

// checkStandaloneMarkerLine verifies marker sits alone on its line and returns
// the byte offset immediately after the marker (before its trailing newline).
func checkStandaloneMarkerLine(source string, idx int, marker, name string) (int, error) {
	lineStart := strings.LastIndex(source[:idx], "\n") + 1
	lineEndRel := strings.IndexByte(source[idx:], '\n')
	lineEnd := len(source)
	if lineEndRel >= 0 {
		lineEnd = idx + lineEndRel
	}
	if lineEnd > lineStart && source[lineEnd-1] == '\r' {
		lineEnd--
	}
	line := source[lineStart:lineEnd]
	if strings.TrimSpace(line) != marker {
		return 0, fmt.Errorf("social marker %q must be on a standalone line", name)
	}
	return lineEnd, nil
}

// checkStandaloneMarkerLineStart verifies marker sits alone on its line and
// returns the byte offset of the start of that line (preserving indentation).
func checkStandaloneMarkerLineStart(source string, idx int, marker, name string) (int, error) {
	lineStart := strings.LastIndex(source[:idx], "\n") + 1
	lineEndRel := strings.IndexByte(source[idx:], '\n')
	lineEnd := len(source)
	if lineEndRel >= 0 {
		lineEnd = idx + lineEndRel
	}
	line := strings.TrimSuffix(source[lineStart:lineEnd], "\r")
	if strings.TrimSpace(line) != marker {
		return 0, fmt.Errorf("social marker %q must be on a standalone line", name)
	}
	return lineStart, nil
}

func normalizeNewlines(s, newline string) string {
	s = strings.ReplaceAll(s, "\r\n", "\n")
	if newline == "\r\n" {
		s = strings.ReplaceAll(s, "\n", "\r\n")
	}
	return s
}

// StaticAssetPair returns the relative POSIX dark and light SVG paths under
// staticDir (defaulting to pages/social).
func StaticAssetPair(staticDir, name string) (darkPath, lightPath string) {
	dir := strings.TrimSpace(staticDir)
	if dir == "" {
		dir = DefaultStaticSocialDir
	}
	dir = strings.TrimRight(strings.ReplaceAll(dir, "\\", "/"), "/")
	return path.Join(dir, name+"-dark.svg"), path.Join(dir, name+"-light.svg")
}

// LiveAssetPair returns commit-pinned raw.githubusercontent.com dark and light
// SVG URLs for an asset on the live social-assets branch.
func LiveAssetPair(owner, repo, sha, name string) (darkURL, lightURL string) {
	base := fmt.Sprintf("https://raw.githubusercontent.com/%s/%s/%s",
		strings.TrimSpace(owner),
		strings.TrimSpace(repo),
		strings.TrimSpace(sha),
	)
	return base + "/" + name + "-dark.svg", base + "/" + name + "-light.svg"
}

// FormatPicture formats a theme-aware HTML <picture> tag pairing dark and
// light SVG sources.
func FormatPicture(darkSrc, lightSrc, alt string) string {
	return fmt.Sprintf(`<picture><source media="(prefers-color-scheme: dark)" srcset="%s"><img src="%s" alt="%s"></picture>`,
		html.EscapeString(darkSrc),
		html.EscapeString(lightSrc),
		html.EscapeString(alt),
	)
}

// FormatLinkedPicture wraps FormatPicture in an outer <a href="..."> element
// when href is non-empty, or returns FormatPicture directly when href is empty.
func FormatLinkedPicture(href, darkSrc, lightSrc, alt string) string {
	pic := FormatPicture(darkSrc, lightSrc, alt)
	trimmedHref := strings.TrimSpace(href)
	if trimmedHref == "" {
		return pic
	}
	return fmt.Sprintf(`<a href="%s">%s</a>`, html.EscapeString(trimmedHref), pic)
}

// ReadmeLinkEntry describes one preferred-link pill for FormatReadmeBlocks.
type ReadmeLinkEntry struct {
	Key   string
	Label string
	URL   string
}

// ReadmeCommunityEntry describes the community banner for FormatReadmeBlocks.
type ReadmeCommunityEntry struct {
	Platform string
	URL      string
	Alt      string
}

// ReadmePackageEntry describes one Haberdasher package tile for
// FormatReadmeBlocks.
type ReadmePackageEntry struct {
	Host    string
	Package string
	URL     string
}

// ReadmeBlocksInput controls which README marker blocks FormatReadmeBlocks
// generates and how static and commit-pinned live URLs are resolved.
type ReadmeBlocksInput struct {
	StaticDir string

	Owner   string
	Repo    string
	LiveSHA string

	ProjectTitle string
	Header       bool
	HeaderURL    string

	Links []ReadmeLinkEntry

	Community     *ReadmeCommunityEntry
	LiveCommunity bool

	Packages []ReadmePackageEntry

	Stats        bool
	StarHistory  bool
	Sponsors     bool
	SponsorsURL  string
	Contributors bool
}

// FormatReadmeBlocks generates formatted HTML blocks keyed by README marker
// name (header, links, community, packages, stats, sponsors, stars, contributors).
func FormatReadmeBlocks(in ReadmeBlocksInput) map[string]string {
	blocks := make(map[string]string)

	staticDir := strings.TrimSpace(in.StaticDir)
	if staticDir == "" {
		staticDir = DefaultStaticSocialDir
	}
	title := strings.TrimSpace(in.ProjectTitle)
	if title == "" {
		title = strings.TrimSpace(in.Repo)
	}
	if title == "" {
		title = "Project"
	}

	hasLiveCoords := strings.TrimSpace(in.Owner) != "" &&
		strings.TrimSpace(in.Repo) != "" &&
		strings.TrimSpace(in.LiveSHA) != ""

	resolveLivePair := func(name string) (string, string) {
		if hasLiveCoords {
			return LiveAssetPair(in.Owner, in.Repo, in.LiveSHA, name)
		}
		return StaticAssetPair(staticDir, name)
	}

	if in.Header {
		darkSrc, lightSrc := StaticAssetPair(staticDir, "hero")
		blocks[MarkerHeader] = wrapCentered(FormatLinkedPicture(in.HeaderURL, darkSrc, lightSrc, title))
	}

	if len(in.Links) > 0 {
		var lines []string
		for _, link := range in.Links {
			key := strings.ToLower(strings.TrimSpace(link.Key))
			if key == "" {
				continue
			}
			label := strings.TrimSpace(link.Label)
			if label == "" {
				if info, ok := LookupLinkIcon(key); ok {
					label = info.Label
				} else {
					label = key
				}
			}
			href := normalizeLinkHref(key, link.URL)
			darkSrc, lightSrc := StaticAssetPair(staticDir, "link-"+key)
			lines = append(lines, "  "+FormatLinkedPicture(href, darkSrc, lightSrc, label))
		}
		if len(lines) > 0 {
			blocks[MarkerLinks] = "<p align=\"center\">\n" + strings.Join(lines, "\n") + "\n</p>"
		}
	}

	if in.Community != nil {
		platform := strings.ToLower(strings.TrimSpace(in.Community.Platform))
		alt := strings.TrimSpace(in.Community.Alt)
		if alt == "" {
			if info, ok := LookupLinkIcon(platform); ok {
				alt = info.Label + " community"
			} else {
				alt = "Community"
			}
		}
		var darkSrc, lightSrc string
		if in.LiveCommunity && hasLiveCoords {
			darkSrc, lightSrc = LiveAssetPair(in.Owner, in.Repo, in.LiveSHA, "community")
		} else {
			darkSrc, lightSrc = StaticAssetPair(staticDir, "community")
		}
		blocks[MarkerCommunity] = wrapCentered(FormatLinkedPicture(in.Community.URL, darkSrc, lightSrc, alt))
	}

	if len(in.Packages) > 0 {
		var lines []string
		for _, pkg := range in.Packages {
			host := strings.ToLower(strings.TrimSpace(pkg.Host))
			info, ok := LookupPackageHost(host)
			if !ok {
				continue
			}
			href := strings.TrimSpace(pkg.URL)
			if href == "" {
				href = "https://" + info.Domain
				if pkgName := strings.ToLower(strings.TrimSpace(pkg.Package)); pkgName != "" {
					href += "/#" + pkgName
				}
			}
			darkSrc, lightSrc := StaticAssetPair(staticDir, "pkg-"+host)
			lines = append(lines, "  "+FormatLinkedPicture(href, darkSrc, lightSrc, info.Label))
		}
		if len(lines) > 0 {
			blocks[MarkerPackages] = "<p align=\"center\">\n" + strings.Join(lines, "\n") + "\n</p>"
		}
	}

	if in.Stats {
		darkSrc, lightSrc := resolveLivePair("stats")
		href := ""
		if strings.TrimSpace(in.Owner) != "" && strings.TrimSpace(in.Repo) != "" {
			href = fmt.Sprintf("https://github.com/%s/%s/pulse", strings.TrimSpace(in.Owner), strings.TrimSpace(in.Repo))
		}
		blocks[MarkerStats] = wrapCentered(FormatLinkedPicture(href, darkSrc, lightSrc, title+" statistics"))
	}

	if in.StarHistory {
		darkSrc, lightSrc := resolveLivePair("stars")
		href := ""
		if strings.TrimSpace(in.Owner) != "" && strings.TrimSpace(in.Repo) != "" {
			href = fmt.Sprintf("https://github.com/%s/%s/stargazers", strings.TrimSpace(in.Owner), strings.TrimSpace(in.Repo))
		}
		blocks[MarkerStars] = wrapCentered(FormatLinkedPicture(href, darkSrc, lightSrc, title+" star history"))
	}

	if in.Sponsors {
		darkSrc, lightSrc := resolveLivePair("sponsors")
		href := strings.TrimSpace(in.SponsorsURL)
		if href == "" && strings.TrimSpace(in.Owner) != "" {
			href = "https://github.com/sponsors/" + strings.TrimSpace(in.Owner)
		}
		blocks[MarkerSponsors] = wrapCentered(FormatLinkedPicture(href, darkSrc, lightSrc, title+" sponsors"))
	}

	if in.Contributors {
		darkSrc, lightSrc := resolveLivePair("contributors")
		href := ""
		if strings.TrimSpace(in.Owner) != "" && strings.TrimSpace(in.Repo) != "" {
			href = fmt.Sprintf("https://github.com/%s/%s/graphs/contributors", strings.TrimSpace(in.Owner), strings.TrimSpace(in.Repo))
		}
		blocks[MarkerContributors] = wrapCentered(FormatLinkedPicture(href, darkSrc, lightSrc, title+" contributors"))
	}

	return blocks
}

func wrapCentered(inner string) string {
	return "<p align=\"center\">\n  " + inner + "\n</p>"
}

func normalizeLinkHref(key, raw string) string {
	val := strings.TrimSpace(raw)
	if val == "" {
		return ""
	}
	if key == "email" && !strings.Contains(val, "://") && !strings.HasPrefix(strings.ToLower(val), "mailto:") {
		return "mailto:" + val
	}
	return val
}
