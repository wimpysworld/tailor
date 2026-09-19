package alter

import (
	"bytes"
	"fmt"
	"path"
	"slices"
	"strconv"
	"strings"

	"github.com/wimpysworld/tailor/internal/config"
	"github.com/wimpysworld/tailor/internal/swatch"
)

const (
	managedImportsPlaceholder          = "[[TAILOR_MANAGED_IMPORTS]]"
	managedLintDependenciesPlaceholder = "[[TAILOR_MANAGED_LINT_DEPENDENCIES]]"
	managedLintPlaceholderPrefix       = "[[TAILOR_MANAGED_LINT_"
)

func renderManagedFiles(cfg *config.Config, selections []managedSelection) (managedRenderedFiles, error) {
	if cfg == nil {
		return nil, fmt.Errorf("managed file rendering requires a config")
	}
	registry := fixedManagedRegistry()
	if err := validateManagedRegistry(registry); err != nil {
		return nil, fmt.Errorf("validating managed registry: %w", err)
	}
	byPath := make(map[string]managedRegistryEntry, len(registry))
	for _, entry := range registry {
		byPath[entry.Path] = entry
	}

	ordered, err := orderManagedSelections(selections)
	if err != nil {
		return nil, fmt.Errorf("ordering managed selections: %w", err)
	}
	files := make(managedRenderedFiles)
	for _, selection := range ordered {
		if !selection.Enabled {
			continue
		}
		entry, registered := byPath[selection.Entry.Path]
		if !registered || entry != selection.Entry {
			return nil, fmt.Errorf("managed destination %q does not match the fixed registry", selection.Entry.Path)
		}
		content, err := swatch.Content(entry.Path)
		if err != nil {
			return nil, fmt.Errorf("reading managed template %q: %w", entry.Path, err)
		}
		switch entry.Path {
		case "just/loader.just", "nix/loader.nix":
			content, err = renderManagedLoader(entry.Path, content, registry)
		case "just/tailor.just":
			content, err = renderManagedLintAggregate(cfg, content, registry)
		case "just/pages.just":
			content, err = renderManagedPages(cfg, content)
		}
		if err != nil {
			return nil, fmt.Errorf("rendering %q: %w", entry.Path, err)
		}
		files[entry.Path] = content
	}
	return files, nil
}

func renderManagedLintAggregate(cfg *config.Config, content []byte, registry []managedRegistryEntry) ([]byte, error) {
	if cfg == nil {
		return nil, fmt.Errorf("managed lint aggregate requires a config")
	}
	if err := validateManagedRegistry(registry); err != nil {
		return nil, fmt.Errorf("validating managed registry: %w", err)
	}

	placeholder := []byte(managedLintDependenciesPlaceholder)
	if bytes.Count(content, placeholder) != 1 {
		return nil, fmt.Errorf("managed core template must contain one lint dependencies placeholder")
	}

	lintEntries := make([]managedRegistryEntry, 0)
	for _, entry := range registry {
		if entry.LintRecipe == "" {
			continue
		}
		declared, enabled := managedCapabilityState(cfg, entry.Capability)
		if declared && enabled {
			lintEntries = append(lintEntries, entry)
		}
	}
	slices.SortFunc(lintEntries, func(left, right managedRegistryEntry) int {
		return strings.Compare(left.Capability.canonicalName(), right.Capability.canonicalName())
	})

	dependencies := make([]string, 0, len(lintEntries)+1)
	dependencies = append(dependencies, "lint-actions")
	for _, entry := range lintEntries {
		dependencies = append(dependencies, entry.LintRecipe)
	}
	content = bytes.Replace(content, placeholder, []byte(strings.Join(dependencies, " ")), 1)
	if bytes.Contains(content, []byte(managedLintPlaceholderPrefix)) {
		return nil, fmt.Errorf("managed core template contains an unresolved lint placeholder")
	}
	return content, nil
}

func renderManagedLoader(destination string, content []byte, registry []managedRegistryEntry) ([]byte, error) {
	var imports []string
	directory := path.Dir(destination)
	for _, entry := range registry {
		if path.Dir(entry.Path) != directory || entry.Path == destination {
			continue
		}
		switch destination {
		case "just/loader.just":
			if entry.Policy == managedPolicyCore || entry.Policy == managedPolicyFragment {
				imports = append(imports, "import? "+strconv.Quote(path.Base(entry.Path)))
			}
		case "nix/loader.nix":
			if entry.Policy == managedPolicyFragment {
				name := path.Base(entry.Path)
				imports = append(imports, "    (if builtins.pathExists ./"+name+" then import ./"+name+" { inherit pkgs; } else [ ])")
			}
		default:
			return nil, fmt.Errorf("unsupported managed loader %q", destination)
		}
	}
	if len(imports) == 0 {
		return nil, fmt.Errorf("managed loader %q has no imports", destination)
	}

	placeholder := []byte(managedImportsPlaceholder)
	if destination == "nix/loader.nix" {
		placeholder = []byte("    # TAILOR_MANAGED_IMPORTS")
	}
	if bytes.Count(content, placeholder) != 1 {
		return nil, fmt.Errorf("managed loader %q must contain one imports placeholder", destination)
	}
	return bytes.Replace(content, placeholder, []byte(strings.Join(imports, "\n")), 1), nil
}

func renderManagedPages(cfg *config.Config, content []byte) ([]byte, error) {
	if err := config.ValidatePages(cfg); err != nil {
		return nil, fmt.Errorf("validating Pages preview: %w", err)
	}
	generator := "static"
	source := "pages"
	if cfg.Pages != nil {
		if cfg.Pages.Generator != nil {
			generator = *cfg.Pages.Generator
		}
		if cfg.Pages.Path != nil {
			source = *cfg.Pages.Path
		}
	}

	preview := source
	message := "Error: preview requires " + strconv.Quote(path.Join(preview, "index.html")) + ". Add that file first."
	switch generator {
	case "hugo":
		preview = path.Join(source, "public")
		message = "Error: preview requires built Pages output " + strconv.Quote(path.Join(preview, "index.html")) + ". Run hugo --source " + strconv.Quote(source) + " first."
	case "jekyll":
		preview = path.Join(source, "_site")
		message = "Error: preview requires built Pages output " + strconv.Quote(path.Join(preview, "index.html")) + ". Run bundle exec jekyll build --source " + strconv.Quote(source) + " --destination " + strconv.Quote(preview) + " first."
	}

	replacements := map[string]string{
		"[[TAILOR_PAGES_PREVIEW_PATH]]":    strconv.Quote(preview),
		"[[TAILOR_PAGES_MISSING_MESSAGE]]": strconv.Quote(message),
	}
	for placeholder, replacement := range replacements {
		if bytes.Count(content, []byte(placeholder)) != 1 {
			return nil, fmt.Errorf("managed Pages template must contain one %s placeholder", placeholder)
		}
		content = bytes.Replace(content, []byte(placeholder), []byte(replacement), 1)
	}
	if slices.ContainsFunc([]string{managedImportsPlaceholder, "[[TAILOR_PAGES_PREVIEW_PATH]]", "[[TAILOR_PAGES_MISSING_MESSAGE]]"}, func(placeholder string) bool {
		return bytes.Contains(content, []byte(placeholder))
	}) {
		return nil, fmt.Errorf("managed Pages template contains an unresolved placeholder")
	}
	return content, nil
}
