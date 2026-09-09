package alter

import (
	"bytes"
	"fmt"
	"html"
	"os"
	"path"
	"strings"

	"github.com/wimpysworld/tailor/internal/config"
	"github.com/wimpysworld/tailor/internal/model"
)

const (
	pagesLinksStart = "<!-- tailor:links:start -->"
	pagesLinksEnd   = "<!-- tailor:links:end -->"
)

// replacePagesLinks changes only the content between two standalone markers.
func replacePagesLinks(data []byte, settings *model.PagesSettings) ([]byte, error) {
	source := string(data)
	if strings.Count(source, pagesLinksStart) != 1 || strings.Count(source, pagesLinksEnd) != 1 {
		return nil, fmt.Errorf("pages.links requires exactly one %s and %s in index.html", pagesLinksStart, pagesLinksEnd)
	}
	start, end := strings.Index(source, pagesLinksStart), strings.Index(source, pagesLinksEnd)
	if start >= end {
		return nil, fmt.Errorf("pages.links markers are reversed")
	}
	for _, marker := range []string{pagesLinksStart, pagesLinksEnd} {
		i := strings.Index(source, marker)
		lineStart := strings.LastIndex(source[:i], "\n") + 1
		lineEnd := strings.Index(source[i:], "\n")
		if lineEnd < 0 {
			lineEnd = len(source) - i
		}
		if strings.TrimSpace(source[lineStart:i+lineEnd]) != marker {
			return nil, fmt.Errorf("pages.links markers must be on separate lines")
		}
	}
	newline := "\n"
	if strings.Contains(source, "\r\n") {
		newline = "\r\n"
	}
	indent := source[strings.LastIndex(source[:end], "\n")+1 : end]
	var items []string
	for _, link := range settings.OrderedLinks() {
		value := (*settings.Links)[link.Key]
		if value == "" {
			continue
		}
		if link.Key == "email" {
			value = "mailto:" + value
		}
		icon := "https://cdn.jsdelivr.net/npm/simple-icons@16.30.0/icons/" + link.Icon + ".svg"
		if strings.HasSuffix(link.Icon, "-24") {
			icon = "https://cdn.jsdelivr.net/npm/@primer/octicons@19.36.0/build/svg/" + link.Icon + ".svg"
		}
		// Inline presentation also supports authored sites without Tailor's CSS.
		items = append(items, fmt.Sprintf(`%s  <a class="icon-button" href="%s" aria-label="%s" title="%s" style="display:inline-flex;padding:10px;color:inherit"><span aria-hidden="true" style="display:block;width:24px;height:24px;background-color:currentColor;-webkit-mask:url('%s') center/contain no-repeat;mask:url('%s') center/contain no-repeat"></span></a>`, indent, html.EscapeString(value), link.Label, link.Label, icon, icon))
	}
	replacement := newline
	if len(items) != 0 {
		replacement += indent + `<nav class="connection-links" aria-label="Connect" style="display:flex;flex-wrap:wrap;gap:0.4rem">` + newline
		replacement += strings.Join(items, newline) + newline + indent + "</nav>" + newline
	}
	return []byte(source[:start+len(pagesLinksStart)] + replacement + indent + source[end:]), nil
}

func processPagesLinks(cfg *config.Config, dir string, mode ApplyMode, prepared *pagesPreparation) (*SwatchResult, error) {
	if prepared == nil || prepared.Generator != "static" || cfg.Pages.Links == nil {
		return nil, nil
	}
	root, err := os.OpenRoot(dir)
	if err != nil {
		return nil, err
	}
	defer root.Close()
	name := path.Join(prepared.Path, "index.html")
	// Read again at apply time so edits outside the markers remain intact.
	data, err := pagesReadFile(root, name)
	if err != nil {
		return nil, err
	}
	updated, err := replacePagesLinks(data, cfg.Pages)
	if err != nil {
		return nil, err
	}
	result := &SwatchResult{Path: name, Category: NoChange}
	if bytes.Equal(data, updated) {
		return result, nil
	}
	if len(updated) > 1024*1024 {
		return nil, fmt.Errorf("pages.links output exceeds 1 MiB")
	}
	result.Category = WouldOverwrite
	if mode.ShouldWrite() {
		if err := writeFile(root, name, updated); err != nil {
			return nil, err
		}
	}
	return result, nil
}

func processPagesFiles(cfg *config.Config, dir string, mode ApplyMode, prepared *pagesPreparation) ([]SwatchResult, error) {
	var results []SwatchResult
	for _, process := range []func(*config.Config, string, ApplyMode, *pagesPreparation) (*SwatchResult, error){processPagesLinks, processPagesIgnore} {
		result, err := process(cfg, dir, mode, prepared)
		if err != nil {
			return results, err
		}
		if result != nil {
			results = append(results, *result)
		}
	}
	return results, nil
}
