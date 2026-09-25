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
	managedImportsPlaceholder     = "[[TAILOR_MANAGED_IMPORTS]]"
	managedBuildPlaceholderPrefix = "[[TAILOR_MANAGED_BUILD_"
	managedLintPlaceholderPrefix  = "[[TAILOR_MANAGED_LINT_"
)

func renderManagedFiles(cfg *config.Config, selections []managedSelection) (managedRenderedFiles, error) {
	return renderManagedFilesFromRegistries(cfg, selections, fixedManagedRegistry(), fixedManagedMCPRegistry())
}

func renderManagedFilesFromRegistries(cfg *config.Config, selections []managedSelection, registry []managedRegistryEntry, mcpRegistry []managedMCPServerDefinition) (managedRenderedFiles, error) {
	if cfg == nil {
		return nil, fmt.Errorf("managed file rendering requires a config")
	}
	if err := validateManagedRegistry(registry); err != nil {
		return nil, fmt.Errorf("validating managed registry: %w", err)
	}
	if err := validateManagedMCPRegistry(mcpRegistry); err != nil {
		return nil, fmt.Errorf("validating managed MCP registry: %w", err)
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
			return nil, fmt.Errorf("managed destination %q does not match the managed registry", selection.Entry.Path)
		}
		if entry.Policy == managedPolicySharedStarter {
			content, err := renderManagedMCPDestination(cfg, entry.Path, mcpRegistry)
			if err != nil {
				return nil, fmt.Errorf("rendering %q: %w", entry.Path, err)
			}
			files[entry.Path] = content
			continue
		}
		content, err := swatch.Content(entry.Path)
		if err != nil {
			return nil, fmt.Errorf("reading managed template %q: %w", entry.Path, err)
		}
		switch entry.Path {
		case "just/loader.just", "nix/loader.nix", "nix/hooks.nix":
			content, err = renderManagedLoader(entry.Path, content, registry)
		case "just/tailor.just":
			content, err = renderManagedCGO(cfg, content)
			if err != nil {
				break
			}
			for _, operation := range managedOperations() {
				content, err = renderManagedAggregate(cfg, content, registry, operation)
				if err != nil {
					break
				}
			}
		case "nix/hooks/go.nix":
			command := ""
			if policy := cfg.EffectiveGoCGO(); policy != nil {
				command = "  export CGO_ENABLED=0"
				if *policy {
					command = "  export CGO_ENABLED=1"
				}
			}
			content = bytes.ReplaceAll(content, []byte("  # TAILOR_GO_CGO"), []byte(command))
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

func renderManagedCGO(cfg *config.Config, content []byte) ([]byte, error) {
	placeholder := []byte("[[TAILOR_MANAGED_CGO]]")
	if bytes.Count(content, placeholder) != 1 {
		return nil, fmt.Errorf("managed core template must contain one CGO placeholder")
	}
	command := ""
	if policy := cfg.EffectiveGoCGO(); policy != nil {
		value := "0"
		if *policy {
			value = "1"
		}
		command = "export CGO_ENABLED := " + strconv.Quote(value)
	}
	return bytes.Replace(content, placeholder, []byte(command), 1), nil
}

func renderManagedBuildAggregate(cfg *config.Config, content []byte, registry []managedRegistryEntry) ([]byte, error) {
	return renderManagedAggregate(cfg, content, registry, managedBuild)
}

func renderManagedLintAggregate(cfg *config.Config, content []byte, registry []managedRegistryEntry) ([]byte, error) {
	return renderManagedAggregate(cfg, content, registry, managedLint)
}

func renderManagedAggregate(cfg *config.Config, content []byte, registry []managedRegistryEntry, operation managedRecipeOperation) ([]byte, error) {
	if operation != managedSetup {
		return renderManagedQuality(cfg, content, registry, operation)
	}
	if cfg == nil {
		return nil, fmt.Errorf("managed %s aggregate requires a config", operation)
	}
	if err := validateManagedRegistry(registry); err != nil {
		return nil, fmt.Errorf("validating managed registry: %w", err)
	}

	prefix := "[[TAILOR_MANAGED_" + strings.ToUpper(string(operation)) + "_"
	dependenciesPlaceholder := []byte(prefix + "DEPENDENCIES]]")
	commandPlaceholder := []byte(prefix + "COMMAND]]")
	if bytes.Count(content, dependenciesPlaceholder) != 1 {
		return nil, fmt.Errorf("managed core template must contain one %s dependencies placeholder", operation)
	}
	if operation != managedLint && bytes.Count(content, commandPlaceholder) != 1 {
		return nil, fmt.Errorf("managed core template must contain one %s command placeholder", operation)
	}

	entries := make([]managedRegistryEntry, 0)
	for _, entry := range registry {
		if entry.SetupRecipe == "" {
			continue
		}
		declared, enabled := managedCapabilityState(cfg, entry.Capability)
		if declared && enabled {
			entries = append(entries, entry)
		}
	}
	slices.SortFunc(entries, func(left, right managedRegistryEntry) int {
		return strings.Compare(left.Capability.canonicalName(), right.Capability.canonicalName())
	})

	dependencies := make([]string, 0, len(entries)+1)
	for _, entry := range entries {
		dependencies = append(dependencies, entry.SetupRecipe)
	}
	command := "@:"
	if len(dependencies) == 0 {
		command = fmt.Sprintf(`@echo "No %s recipes are enabled."`, operation)
	}
	if len(dependencies) == 0 {
		content = bytes.Replace(content, append([]byte(" "), dependenciesPlaceholder...), nil, 1)
	} else {
		content = bytes.Replace(content, dependenciesPlaceholder, []byte(strings.Join(dependencies, " ")), 1)
	}
	if operation != managedLint {
		content = bytes.Replace(content, commandPlaceholder, []byte(command), 1)
	}
	if bytes.Contains(content, []byte(prefix)) {
		return nil, fmt.Errorf("managed core template contains an unresolved %s placeholder", operation)
	}
	return content, nil
}

func renderManagedLoader(destination string, content []byte, registry []managedRegistryEntry) ([]byte, error) {
	var imports []string
	directory := path.Dir(destination)
	for _, entry := range registry {
		if destination == "nix/hooks.nix" {
			if entry.NixKind == managedNixHook {
				name := strings.TrimPrefix(entry.Path, "nix/")
				imports = append(imports, renderManagedNixImport(name, `""`, "  "))
			}
			continue
		}
		if path.Dir(entry.Path) != directory || entry.Path == destination {
			continue
		}
		switch destination {
		case "just/loader.just":
			if entry.Policy == managedPolicyCore || entry.Policy == managedPolicyFragment {
				imports = append(imports, "import? "+strconv.Quote(path.Base(entry.Path)))
			}
		case "nix/loader.nix":
			if entry.Policy == managedPolicyFragment && entry.NixKind == managedNixList {
				name := path.Base(entry.Path)
				imports = append(imports, renderManagedNixImport(name, "[ ]", "    "))
			}
		default:
			return nil, fmt.Errorf("unsupported managed loader %q", destination)
		}
	}
	if len(imports) == 0 {
		return nil, fmt.Errorf("managed loader %q has no imports", destination)
	}

	placeholder := []byte(managedImportsPlaceholder)
	if destination == "nix/hooks.nix" {
		slices.SortStableFunc(imports, func(left, right string) int {
			return strings.Compare(strconv.FormatBool(strings.Contains(left, "./hooks/core.nix")), strconv.FormatBool(strings.Contains(right, "./hooks/core.nix")))
		})
	}
	if destination == "nix/loader.nix" {
		placeholder = []byte("    # TAILOR_MANAGED_IMPORTS")
	}
	if destination == "nix/hooks.nix" {
		placeholder = []byte("  # TAILOR_MANAGED_IMPORTS")
	}
	if bytes.Count(content, placeholder) != 1 {
		return nil, fmt.Errorf("managed loader %q must contain one imports placeholder", destination)
	}
	return bytes.Replace(content, placeholder, []byte(strings.Join(imports, "\n")), 1), nil
}

func renderManagedNixImport(name, fallback, indent string) string {
	inline := indent + "(if builtins.pathExists ./" + name + " then import ./" + name + " { inherit pkgs; } else " + fallback + ")"
	// nixfmt permits the closing parenthesis beyond its 100-column width.
	if len(inline)-1 <= 100 {
		return inline
	}
	return indent + "(\n" + indent + "  if builtins.pathExists ./" + name + " then\n" +
		indent + "    import ./" + name + " { inherit pkgs; }\n" + indent + "  else\n" +
		indent + "    " + fallback + "\n" + indent + ")"
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
