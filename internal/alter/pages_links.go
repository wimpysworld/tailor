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
	region, err := readPagesRegion(source, pagesLinksStart, pagesLinksEnd, "pages.links")
	if err != nil {
		return nil, err
	}
	indent, newline := region.indent, region.newline
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
	replacement := ""
	if len(items) != 0 {
		replacement += indent + `<nav class="connection-links" aria-label="Connect" style="display:flex;flex-wrap:wrap;gap:0.4rem">` + newline
		replacement += strings.Join(items, newline) + newline + indent + "</nav>" + newline
	}
	return region.replace(source, replacement), nil
}

func processStaticPages(cfg *config.Config, dir string, mode ApplyMode, prepared *pagesPreparation) (*SwatchResult, error) {
	if prepared == nil || prepared.Generator != "static" {
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
	updated, err := renderStaticPages(cfg, data, prepared.RepoURL)
	if err != nil {
		return nil, err
	}
	if cfg.Pages.Links == nil && !hasPagesRepositoryLinks(data) {
		return nil, nil
	}
	result := &SwatchResult{Path: name, Category: NoChange}
	if bytes.Equal(data, updated) {
		return result, nil
	}
	if len(updated) > 1024*1024 {
		return nil, fmt.Errorf("static pages output exceeds 1 MiB")
	}
	result.Category = WouldOverwrite
	if mode.ShouldWrite() {
		if err := writeFile(root, name, updated); err != nil {
			return nil, err
		}
	}
	return result, nil
}

func renderStaticPages(cfg *config.Config, data []byte, repoURL string) ([]byte, error) {
	updated, err := replacePagesNavigation(data, cfg.Repository, repoURL)
	if err != nil {
		return nil, err
	}
	updated, err = replacePagesLicense(updated, cfg.License, repoURL)
	if err != nil {
		return nil, err
	}
	if cfg.Pages.Links != nil {
		updated, err = replacePagesLinks(updated, cfg.Pages)
		if err != nil {
			return nil, err
		}
	}
	return updated, nil
}

func processPagesFiles(cfg *config.Config, dir string, mode ApplyMode, prepared *pagesPreparation) ([]SwatchResult, error) {
	var results []SwatchResult
	if prepared != nil && prepared.Starter {
		var err error
		results, err = processPagesStarter(cfg, dir, mode, prepared)
		if err != nil {
			return results, err
		}
		ignore, err := processPagesIgnore(cfg, dir, mode, prepared)
		if ignore != nil {
			results = append(results, *ignore)
		}
		return results, err
	}
	for _, process := range []func(*config.Config, string, ApplyMode, *pagesPreparation) (*SwatchResult, error){processStaticPages, processPagesIgnore} {
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
