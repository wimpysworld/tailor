package alter

import (
	"fmt"
	"html"
	"net/url"
	"strings"

	"github.com/wimpysworld/tailor/internal/model"
)

const (
	pagesNavigationStart       = "<!-- tailor:navigation:start -->"
	pagesNavigationEnd         = "<!-- tailor:navigation:end -->"
	pagesFooterNavigationStart = "<!-- tailor:footer-navigation:start -->"
	pagesFooterNavigationEnd   = "<!-- tailor:footer-navigation:end -->"
	pagesLicenseStart          = "<!-- tailor:license:start -->"
	pagesLicenseEnd            = "<!-- tailor:license:end -->"
)

// replacePagesLicense builds the licence tab URL from the configured identifier.
func replacePagesLicense(data []byte, license, repoURL string) ([]byte, error) {
	source := string(data)
	if !strings.Contains(source, pagesLicenseStart) && !strings.Contains(source, pagesLicenseEnd) {
		return data, nil
	}
	region, err := readPagesRegion(source, pagesLicenseStart, pagesLicenseEnd, "pages license")
	if err != nil {
		return nil, err
	}
	if repoURL == "" {
		return data, nil
	}
	content := ""
	if license != "" && license != "none" {
		href := repoURL + "?tab=" + url.QueryEscape(license+"-1-ov-file")
		content = region.indent + fmt.Sprintf(`<li><a href="%s">License</a></li>`, html.EscapeString(href)) + region.newline
	}
	return region.replace(source, content), nil
}

// replacePagesNavigation leaves unmarked sites unchanged.
func replacePagesNavigation(data []byte, repository *model.RepositorySettings, repoURL string) ([]byte, error) {
	updated, err := replacePagesNavigationRegion(data, repository, repoURL, false)
	if err != nil {
		return nil, err
	}
	return replacePagesNavigationRegion(updated, repository, repoURL, true)
}

func replacePagesNavigationRegion(data []byte, repository *model.RepositorySettings, repoURL string, footer bool) ([]byte, error) {
	start, end := pagesNavigationStart, pagesNavigationEnd
	if footer {
		start, end = pagesFooterNavigationStart, pagesFooterNavigationEnd
	}
	source := string(data)
	if !strings.Contains(source, start) && !strings.Contains(source, end) {
		return data, nil
	}
	region, err := readPagesRegion(source, start, end, "pages navigation")
	if err != nil {
		return nil, err
	}
	// Local preflight validates markers before repository context is available.
	if repoURL == "" {
		return data, nil
	}
	wiki := repository != nil && repository.HasWiki != nil && *repository.HasWiki
	discussions := repository != nil && repository.HasDiscussions != nil && *repository.HasDiscussions
	label := "Documentation"
	if wiki {
		label = "Overview"
	}
	var items []string
	attributes := ""
	if !footer {
		attributes = ` target="_blank" rel="noopener noreferrer"`
	}
	add := func(label, suffix string) {
		items = append(items, region.indent+fmt.Sprintf(`<li><a%s href="%s">%s</a></li>`, attributes, html.EscapeString(repoURL+suffix), label))
	}
	add(label, "?tab=readme-ov-file")
	if wiki {
		add("Documentation", "/wiki")
	}
	if discussions {
		add("Discussions", "/discussions")
	}
	if !footer {
		add("Download", "/releases")
	} else if !wiki || !discussions {
		add("Support", "/blob/HEAD/SUPPORT.md")
	}
	return region.replace(source, strings.Join(items, region.newline)+region.newline), nil
}

func hasPagesRepositoryLinks(data []byte) bool {
	source := string(data)
	return strings.Contains(source, pagesNavigationStart) || strings.Contains(source, pagesFooterNavigationStart) || strings.Contains(source, pagesLicenseStart)
}

type pagesRegion struct {
	start, end      int
	indent, newline string
}

func readPagesRegion(source, startMarker, endMarker, name string) (pagesRegion, error) {
	var region pagesRegion
	if strings.Count(source, startMarker) != 1 || strings.Count(source, endMarker) != 1 {
		return region, fmt.Errorf("%s requires exactly one %s and %s in index.html", name, startMarker, endMarker)
	}
	start, end := strings.Index(source, startMarker), strings.Index(source, endMarker)
	if start >= end {
		return region, fmt.Errorf("%s markers are reversed", name)
	}
	for _, marker := range []string{startMarker, endMarker} {
		i := strings.Index(source, marker)
		lineStart := strings.LastIndex(source[:i], "\n") + 1
		lineEnd := strings.Index(source[i:], "\n")
		if lineEnd < 0 {
			lineEnd = len(source) - i
		}
		if strings.TrimSpace(source[lineStart:i+lineEnd]) != marker {
			return region, fmt.Errorf("%s markers must be on separate lines", name)
		}
	}
	region = pagesRegion{start: start + len(startMarker), end: end, newline: "\n"}
	if strings.Contains(source, "\r\n") {
		region.newline = "\r\n"
	}
	region.indent = source[strings.LastIndex(source[:end], "\n")+1 : end]
	return region, nil
}

func (r pagesRegion) replace(source, content string) []byte {
	return []byte(source[:r.start] + r.newline + content + r.indent + source[r.end:])
}
