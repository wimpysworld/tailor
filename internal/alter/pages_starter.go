package alter

import (
	"errors"
	"fmt"
	"html"
	"io/fs"
	"os"
	"path"
	"strings"
	"time"

	"github.com/wimpysworld/tailor/internal/config"
	"github.com/wimpysworld/tailor/internal/swatch"
)

// pagesStarterEmpty requires an absent or empty directory so starter creation cannot replace an authored site.
func pagesStarterEmpty(root *os.Root, dir string) (bool, error) {
	if err := checkParents(root, path.Join(dir, "index.html"), "pages source parent"); err != nil {
		return false, err
	}
	info, err := root.Lstat(dir)
	if errors.Is(err, os.ErrNotExist) {
		return true, nil
	}
	if err != nil {
		return false, err
	}
	if !info.IsDir() {
		return false, fmt.Errorf("pages source %q is not a directory", dir)
	}
	entries, err := fs.ReadDir(root.FS(), dir)
	return len(entries) == 0, err
}

func pagesStarterContent(cfg *config.Config, p *pagesPreparation, source string) ([]byte, error) {
	data, err := swatch.Content(source)
	if err != nil || source != "pages/index.html" {
		return data, err
	}
	description := "Explore the project, read the documentation and try the latest release."
	if cfg.Repository != nil && cfg.Repository.Description != nil && *cfg.Repository.Description != "" {
		description = *cfg.Repository.Description
	}
	name := p.ProjectName
	if name == "" {
		name = "Your project"
	}
	data = []byte(strings.NewReplacer(
		"{{PROJECT_NAME}}", html.EscapeString(name),
		"{{DESCRIPTION}}", html.EscapeString(description),
		"{{REPO_URL}}", html.EscapeString(p.RepoURL),
		"{{YEAR}}", fmt.Sprint(time.Now().Year()),
	).Replace(string(data)))
	return renderStaticPages(cfg, data, p.RepoURL)
}

func processPagesStarter(cfg *config.Config, dir string, mode ApplyMode, p *pagesPreparation) (results []SwatchResult, err error) {
	root, err := os.OpenRoot(dir)
	if err != nil {
		return nil, err
	}
	defer root.Close()
	empty, err := pagesStarterEmpty(root, p.Path)
	if err != nil {
		return nil, err
	}
	if !empty {
		return nil, fmt.Errorf("pages starter destination %q is no longer empty", p.Path)
	}
	// Validate every file before the first write. A partial starter is not a site.
	contents := make([][]byte, len(swatch.PagesStarterPaths))
	for i, source := range swatch.PagesStarterPaths {
		for _, entry := range cfg.Swatches {
			if entry.Path == source && entry.Alteration == swatch.Never {
				return nil, fmt.Errorf("pages starter requires %s, but its alteration mode is never", source)
			}
		}
		contents[i], err = pagesStarterContent(cfg, p, source)
		if err != nil {
			return nil, err
		}
		if len(contents[i]) > 1024*1024 {
			return nil, fmt.Errorf("static pages output exceeds 1 MiB")
		}
	}
	var created []string
	defer func() {
		if err != nil {
			for _, name := range created {
				if removeErr := root.Remove(name); removeErr != nil {
					err = errors.Join(err, fmt.Errorf("removing partial pages starter %q: %w", name, removeErr))
				}
			}
			results = nil
		}
	}()
	for i, source := range swatch.PagesStarterPaths {
		name := path.Join(p.Path, path.Base(source))
		if mode.ShouldWrite() {
			if err := checkParents(root, name, "pages starter parent"); err != nil {
				return results, err
			}
			if err := root.MkdirAll(p.Path, 0o755); err != nil {
				return results, err
			}
			file, err := root.OpenFile(name, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
			if err != nil {
				return results, err
			}
			created = append(created, name)
			_, writeErr := file.Write(contents[i])
			closeErr := file.Close()
			if err := errors.Join(writeErr, closeErr); err != nil {
				return results, err
			}
		}
		results = append(results, SwatchResult{Path: name, Category: WouldCopy})
	}
	return results, nil
}
