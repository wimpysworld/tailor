package alter

import (
	"errors"
	"fmt"
	"os"

	"github.com/wimpysworld/tailor/internal/config"
	"github.com/wimpysworld/tailor/internal/gh"
	"github.com/wimpysworld/tailor/internal/goproject"
	"github.com/wimpysworld/tailor/internal/swatch"
)

const goBuilderPath = ".github/workflows/build-go.yml"

func prepareGoSwatches(cfg *config.Config, dir string, mode ApplyMode, branch string) (map[string][]byte, error) {
	contents := make(map[string][]byte)
	root, err := os.OpenRoot(dir)
	if err != nil {
		return nil, fmt.Errorf("opening project root %q: %w", dir, err)
	}
	defer root.Close()
	planned := make(map[string]bool)
	for _, entry := range cfg.Swatches {
		if !cfg.SwatchActive(entry.Path) || entry.Alteration == swatch.Never {
			continue
		}
		switch entry.Path {
		case ".golangci.yml", ".goreleaser.yaml", goBuilderPath, ".github/dependabot.yml", "justfile":
		default:
			continue
		}
		if err := checkParents(root, entry.Path, "swatch parent"); err != nil {
			return nil, err
		}
		exists, err := prepareSwatchDestination(root, entry.Path, false)
		if err != nil {
			return nil, fmt.Errorf("checking swatch %q: %w", entry.Path, err)
		}
		planned[entry.Path] = !exists || entry.Alteration == swatch.Always || mode == Recut
	}
	if planned[goBuilderPath] {
		for _, path := range []string{".golangci.yml", ".goreleaser.yaml"} {
			if planned[path] {
				continue
			}
			info, err := root.Lstat(path)
			if err != nil || !info.Mode().IsRegular() {
				return nil, fmt.Errorf("go builder requires %s: provide a regular file or enable its swatch", path)
			}
		}
	}
	options := swatch.Options{GoDeclared: cfg.GoDeclared(), GoEnabled: cfg.GoEnabled(), DefaultBranch: branch}
	if planned[".goreleaser.yaml"] {
		options.Builds, err = goproject.Discover(dir)
		if err != nil {
			return nil, fmt.Errorf("preparing GoReleaser: %w", err)
		}
	}
	for path, needed := range planned {
		if !needed {
			continue
		}
		content, err := swatch.Render(path, options)
		if err != nil {
			return nil, fmt.Errorf("rendering swatch %q: %w", path, err)
		}
		contents[path] = content
	}
	return contents, nil
}

func resolveGoBuilder(contents map[string][]byte, cfg *config.Config, target RepoTarget, pages *pagesRun) error {
	if _, needed := contents[goBuilderPath]; !needed || !target.HasRepo {
		return nil
	}
	var repository *gh.PagesRepositoryState
	if pages != nil {
		repository = pages.repository
		if repository == nil && len(pages.skipped) != 0 {
			return nil
		}
	} else {
		var err error
		repository, err = gh.ReadPagesRepository(target.Client, target.Owner, target.Name)
		_, scope := errors.AsType[*gh.ErrInsufficientScope](err)
		_, skipped := errors.AsType[*gh.ErrSetupSkipped](err)
		if err != nil && !scope && !skipped {
			return fmt.Errorf("reading Go builder default branch: %w", err)
		}
		if repository == nil && err != nil {
			return nil
		}
	}
	if repository == nil || repository.DefaultBranch == "" {
		return fmt.Errorf("go builder repository response is missing default_branch")
	}
	branch := repository.DefaultBranch
	content, err := swatch.Render(goBuilderPath, swatch.Options{GoDeclared: cfg.GoDeclared(), GoEnabled: cfg.GoEnabled(), DefaultBranch: branch})
	if err != nil {
		return fmt.Errorf("rendering Go builder: %w", err)
	}
	contents[goBuilderPath] = content
	return nil
}
