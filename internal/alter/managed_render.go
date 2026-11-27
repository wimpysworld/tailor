package alter

import (
	"bytes"
	"fmt"
	"os"
	"path"
	"slices"
	"strconv"
	"strings"

	"github.com/wimpysworld/tailor/internal/config"
	"github.com/wimpysworld/tailor/internal/social"
	"github.com/wimpysworld/tailor/internal/swatch"
)

const (
	managedImportsPlaceholder     = "[[TAILOR_MANAGED_IMPORTS]]"
	managedBuildPlaceholderPrefix = "[[TAILOR_MANAGED_BUILD_"
	managedLintPlaceholderPrefix  = "[[TAILOR_MANAGED_LINT_"
	managedOGArgsPlaceholder      = "[[TAILOR_OG_ARGS]]"
)

type managedRenderContext struct {
	Dir   string
	Owner string
	Repo  string
}

func renderManagedFiles(cfg *config.Config, selections []managedSelection) (managedRenderedFiles, error) {
	return renderManagedFilesWithContext(cfg, selections, managedRenderContext{})
}

func renderManagedFilesWithContext(cfg *config.Config, selections []managedSelection, ctx managedRenderContext) (managedRenderedFiles, error) {
	return renderManagedFilesFromRegistriesWithContext(cfg, selections, fixedManagedRegistry(), fixedManagedMCPRegistry(), ctx)
}

func renderManagedFilesFromRegistries(cfg *config.Config, selections []managedSelection, registry []managedRegistryEntry, mcpRegistry []managedMCPServerDefinition) (managedRenderedFiles, error) {
	return renderManagedFilesFromRegistriesWithContext(cfg, selections, registry, mcpRegistry, managedRenderContext{})
}

func renderManagedFilesFromRegistriesWithContext(cfg *config.Config, selections []managedSelection, registry []managedRegistryEntry, mcpRegistry []managedMCPServerDefinition, ctx managedRenderContext) (managedRenderedFiles, error) {
	if cfg == nil {
		return nil, fmt.Errorf("managed file rendering requires a config")
	}
	if err := config.ValidateEbitengine(cfg); err != nil {
		return nil, err
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
			content, err = renderManagedGoHook(cfg, content)
		case "just/pages.just":
			content, err = renderManagedPages(cfg, content)
		case "just/og.just":
			content, err = renderManagedOG(cfg, content, ctx)
		}
		if err != nil {
			return nil, fmt.Errorf("rendering %q: %w", entry.Path, err)
		}
		files[entry.Path] = content
	}
	return files, nil
}

func renderManagedGoHook(cfg *config.Config, content []byte) ([]byte, error) {
	placeholder := []byte("  # TAILOR_GO_CGO")
	if bytes.Count(content, placeholder) != 1 {
		return nil, fmt.Errorf("managed Go hook template must contain one CGO placeholder")
	}
	command := ""
	if policy := cfg.EffectiveGoCGO(); policy != nil {
		command = "  export CGO_ENABLED=0"
		if *policy {
			command = "  export CGO_ENABLED=1"
		}
	}
	return bytes.Replace(content, placeholder, []byte(command), 1), nil
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
	} else {
		placeholder = append(placeholder, '\n')
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
	return renderManagedQuality(cfg, content, registry, operation)
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
			if (entry.Policy == managedPolicyCore || entry.Policy == managedPolicyFragment) && entry.NixKind == managedNixList {
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

func renderManagedOG(cfg *config.Config, content []byte, ctx managedRenderContext) ([]byte, error) {
	if err := config.ValidateSocial(cfg); err != nil {
		return nil, fmt.Errorf("validating social Open Graph recipe: %w", err)
	}
	placeholder := []byte(managedOGArgsPlaceholder)
	if bytes.Count(content, placeholder) != 1 {
		return nil, fmt.Errorf("managed Open Graph template must contain one %s placeholder", managedOGArgsPlaceholder)
	}

	styleCSS, err := resolveOGStyleCSS(cfg, ctx.Dir)
	if err != nil {
		return nil, err
	}

	lines, err := formatOGArgs(buildOGArgs(cfg, ctx, styleCSS))
	if err != nil {
		return nil, err
	}

	rendered := bytes.Replace(content, placeholder, []byte(strings.Join(lines, "\n        ")), 1)
	if bytes.Contains(rendered, placeholder) {
		return nil, fmt.Errorf("managed Open Graph template contains an unresolved placeholder")
	}
	return rendered, nil
}

type ogArg struct {
	flag  string
	value string
	bare  bool
}

func resolveOGStyleCSS(cfg *config.Config, dir string) (string, error) {
	pagesPath := "pages"
	if cfg.Pages != nil && cfg.Pages.Path != nil && strings.TrimSpace(*cfg.Pages.Path) != "" {
		if err := config.ValidatePages(cfg); err != nil {
			return "", fmt.Errorf("validating Pages path for Open Graph recipe: %w", err)
		}
		pagesPath = path.Clean(*cfg.Pages.Path)
	}
	styleCSS := path.Join(pagesPath, "style.css")
	if cfg.PagesEnabled() {
		return styleCSS, nil
	}
	if dir == "" {
		return "", nil
	}
	root, err := os.OpenRoot(dir)
	if err != nil {
		return "", nil
	}
	defer root.Close()
	if info, statErr := root.Lstat(styleCSS); statErr == nil && info.Mode().IsRegular() {
		return styleCSS, nil
	}
	return "", nil
}

func buildOGArgs(cfg *config.Config, ctx managedRenderContext, styleCSS string) []ogArg {
	args := []ogArg{
		{flag: "--output", value: cfg.EffectiveSocialOGOutput()},
		{flag: "--theme", value: cfg.EffectiveSocialTheme()},
	}
	add := func(flag, value string) {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			args = append(args, ogArg{flag: flag, value: trimmed})
		}
	}
	add("--style-css", styleCSS)
	add("--accent", cfg.EffectiveSocialAccent())
	add("--background", cfg.EffectiveSocialBackground())
	add("--title", social.ProjectTitle(ctx.Repo))
	if cfg.Repository != nil && cfg.Repository.Description != nil {
		add("--tagline", social.StripTrailingEmoji(*cfg.Repository.Description))
	}
	add("--footer", resolveOGFooter(cfg, strings.TrimSpace(ctx.Owner), strings.TrimSpace(ctx.Repo)))
	add("--platforms", cfg.EffectiveSocialPlatforms())
	if cfg.EffectiveSocialOGIncludePreview() {
		args = append(args, ogArg{flag: "--include-preview", bare: true})
	}
	add("--image", cfg.EffectiveSocialOGImage())
	for _, terminalLine := range cfg.EffectiveSocialOGTerminal() {
		args = append(args, ogArg{flag: "--terminal-line", value: terminalLine})
	}
	return args
}

func formatOGArgs(args []ogArg) ([]string, error) {
	lines := make([]string, 0, len(args))
	for _, arg := range args {
		if arg.bare {
			lines = append(lines, arg.flag)
			continue
		}
		if strings.ContainsAny(arg.value, "\x00\r\n") {
			return nil, fmt.Errorf("managed Open Graph argument %s contains unsupported control characters", arg.flag)
		}
		lines = append(lines, arg.flag+" "+shellQuoteJustArg(arg.value))
	}
	return lines, nil
}

func resolveOGFooter(cfg *config.Config, owner, repo string) string {
	if cfg != nil && cfg.Pages != nil && cfg.Pages.CNAME != nil {
		if cname := trimURLScheme(*cfg.Pages.CNAME); cname != "" {
			return cname
		}
	}
	if cfg != nil && cfg.Repository != nil && cfg.Repository.Homepage != nil {
		if homepage := trimURLScheme(*cfg.Repository.Homepage); homepage != "" {
			return homepage
		}
	}
	if owner != "" && repo != "" {
		return "github.com/" + owner + "/" + repo
	}
	return ""
}

func trimURLScheme(raw string) string {
	s := strings.TrimSpace(raw)
	s = strings.TrimPrefix(s, "https://")
	s = strings.TrimPrefix(s, "http://")
	s = strings.TrimRight(s, "/")
	return s
}

// shellQuoteJustArg wraps value in POSIX single quotes and breaks any Just
// interpolation delimiter ("{{") across adjacent single-quoted segments so
// Just passes literal bytes unchanged to bash.
func shellQuoteJustArg(value string) string {
	quoted := "'" + strings.ReplaceAll(value, "'", `'\''`) + "'"
	for strings.Contains(quoted, "{{") {
		quoted = strings.ReplaceAll(quoted, "{{", "{''{")
	}
	return quoted
}
