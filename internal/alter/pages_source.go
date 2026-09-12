package alter

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"github.com/wimpysworld/tailor/internal/config"
	"github.com/wimpysworld/tailor/internal/swatch"
	"gopkg.in/yaml.v3"
)

func preparePagesSource(cfg *config.Config, dir string, _ ApplyMode) (*pagesPreparation, error) {
	if cfg.Pages == nil || cfg.Pages.Enabled == nil || !*cfg.Pages.Enabled {
		return nil, nil
	}
	p := &pagesPreparation{Generator: "static", Path: "pages", Entry: config.SwatchEntry{Path: swatch.PagesDestination, Alteration: swatch.Always}}
	if cfg.Pages.Generator != nil {
		p.Generator = *cfg.Pages.Generator
	}
	if cfg.Pages.Path != nil {
		p.Path = *cfg.Pages.Path
	}
	if cfg.Pages.Branch != nil {
		p.Branch = *cfg.Pages.Branch
	}
	for _, entry := range cfg.Swatches {
		if entry.Path == swatch.PagesDestination {
			p.Entry = entry
		}
	}
	if _, err := swatch.PagesContent(p.Generator, p.Path, "main"); err != nil {
		return nil, err
	}
	root, err := os.OpenRoot(dir)
	if err != nil {
		return nil, fmt.Errorf("opening pages project: %w", err)
	}
	defer root.Close()
	if err := checkParents(root, path.Join(p.Path, "index.html"), "pages source parent"); err != nil {
		return nil, err
	}
	if p.Generator == "static" {
		empty, err := pagesStarterEmpty(root, p.Path)
		if err != nil {
			return nil, err
		}
		if empty {
			p.Starter = true
			p.Navigation = true
			if _, err := processPagesStarter(cfg, dir, DryRun, p); err != nil {
				return nil, err
			}
			return p, nil
		}
	}
	info, err := root.Lstat(p.Path)
	if err != nil {
		return nil, fmt.Errorf("pages source directory %q: %w", p.Path, err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("pages source %q is not a directory", p.Path)
	}
	if err := fs.WalkDir(root.FS(), p.Path, func(name string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.Type()&os.ModeSymlink != 0 {
			return fmt.Errorf("pages source %q is a symlink", name)
		}
		return nil
	}); err != nil {
		return nil, fmt.Errorf("checking pages source: %w", err)
	}
	switch p.Generator {
	case "static":
		var data []byte
		data, err = pagesReadFile(root, path.Join(p.Path, "index.html"))
		p.Navigation = hasPagesRepositoryLinks(data)
	case "hugo":
		err = validateHugoSource(root, dir, p.Path)
	case "jekyll":
		err = validateJekyllSource(root, p.Path)
	}
	if err != nil {
		return nil, fmt.Errorf("checking %s pages source: %w", p.Generator, err)
	}
	if _, err := processStaticPages(cfg, dir, DryRun, p); err != nil {
		return nil, err
	}
	return p, nil
}

func pagesReadFile(root *os.Root, name string) ([]byte, error) {
	if err := checkParents(root, name, "pages input parent"); err != nil {
		return nil, err
	}
	info, err := root.Lstat(name)
	if err != nil {
		return nil, fmt.Errorf("reading %q: %w", name, err)
	}
	if !info.Mode().IsRegular() || info.Size() > 1024*1024 {
		return nil, fmt.Errorf("pages input %q must be a regular file no larger than 1 MiB", name)
	}
	return root.ReadFile(name)
}

var (
	hugoThemeAssignment = regexp.MustCompile(`(?m)^\s*theme\s*=\s*(.+?)\s*$`)
	hugoModuleVersion   = regexp.MustCompile(`^v[0-9]+\.[0-9]+\.[0-9]+(?:[-+][A-Za-z0-9.+-]+)?$`)
)

func hugoConfigFiles(root *os.Root, source string) ([]string, error) {
	var configs []string
	for _, base := range []string{"hugo", "config", "config/_default/hugo", "config/_default/config"} {
		for _, ext := range []string{".toml", ".yaml", ".yml", ".json"} {
			name := path.Join(source, base+ext)
			if _, err := root.Lstat(name); err == nil {
				configs = append(configs, name)
			} else if !errors.Is(err, os.ErrNotExist) {
				return nil, err
			}
		}
	}
	if len(configs) == 0 {
		return nil, fmt.Errorf("a recognised hugo or config file is required")
	}
	if info, err := root.Stat(path.Join(source, "config")); err == nil && info.IsDir() {
		if err := fs.WalkDir(root.FS(), path.Join(source, "config"), func(name string, entry fs.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			if !entry.IsDir() {
				switch path.Ext(name) {
				case ".toml", ".yaml", ".yml", ".json":
					configs = append(configs, name)
				}
			}
			return nil
		}); err != nil {
			return nil, err
		}
	}
	return configs, nil
}

func validateHugoSource(root *os.Root, dir, source string) error {
	configs, err := hugoConfigFiles(root, source)
	if err != nil {
		return err
	}
	var themes []string
	modules := false
	for _, name := range configs {
		data, err := pagesReadFile(root, name)
		if err != nil {
			return err
		}
		var config struct {
			Theme any `yaml:"theme"`
		}
		modules = modules || strings.HasPrefix(path.Base(name), "module.")
		if strings.HasSuffix(name, ".toml") {
			if match := hugoThemeAssignment.FindSubmatch(data); match != nil {
				if err := yaml.Unmarshal(match[1], &config.Theme); err != nil {
					return fmt.Errorf("reading hugo theme: %w", err)
				}
			}
			modules = modules || strings.Contains(string(data), "module.imports")
		} else {
			if err := yaml.Unmarshal(data, &config); err != nil {
				return fmt.Errorf("reading hugo config: %w", err)
			}
			var values map[string]any
			if err := yaml.Unmarshal(data, &values); err != nil {
				return err
			}
			modules = modules || values["module"] != nil
		}
		switch theme := config.Theme.(type) {
		case string:
			themes = append(themes, theme)
		case []any:
			for _, value := range theme {
				name, ok := value.(string)
				if !ok {
					return fmt.Errorf("hugo theme names must be strings")
				}
				themes = append(themes, name)
			}
		case nil:
		default:
			return fmt.Errorf("hugo theme must be a name or list of names")
		}
	}
	if _, err := root.Lstat(path.Join(source, "go.mod")); err == nil {
		modules = true
	}
	var modulePaths []string
	if modules {
		modulePaths, err = validateHugoModules(root, source)
		if err != nil {
			return err
		}
	}
	for _, theme := range themes {
		if theme == "" || path.IsAbs(theme) || path.Clean(theme) != theme || theme == ".." || strings.HasPrefix(theme, "../") {
			return fmt.Errorf("unsafe hugo theme %q", theme)
		}
		if slices.Contains(modulePaths, theme) {
			continue
		}
		name := path.Join(source, "themes", theme)
		entries, err := fs.ReadDir(root.FS(), name)
		if err == nil && len(entries) > 0 {
			continue
		}
		if err := validateHugoSubmodule(root, dir, name); err != nil {
			return fmt.Errorf("hugo theme %q must contain local files or a pinned submodule: %w", theme, err)
		}
	}
	return nil
}

func validateHugoModules(root *os.Root, source string) ([]string, error) {
	mod, err := pagesReadFile(root, path.Join(source, "go.mod"))
	if err != nil {
		return nil, err
	}
	sums, err := pagesReadFile(root, path.Join(source, "go.sum"))
	if err != nil {
		return nil, err
	}
	var modulePaths []string
	inRequire := false
	for line := range strings.SplitSeq(string(mod), "\n") {
		fields := strings.Fields(strings.SplitN(line, "//", 2)[0])
		if len(fields) == 0 {
			continue
		}
		if fields[0] == "replace" {
			return nil, fmt.Errorf("hugo module replacements are not supported, use pinned requirements")
		}
		if fields[0] == "require" {
			fields = fields[1:]
			if len(fields) == 0 {
				return nil, fmt.Errorf("hugo go.mod has an empty require declaration")
			}
			if len(fields) == 1 && fields[0] == "(" {
				inRequire = true
				continue
			}
		} else if !inRequire {
			continue
		}
		if fields[0] == ")" {
			inRequire = false
			continue
		}
		if len(fields) != 2 || !hugoModuleVersion.MatchString(fields[1]) || !strings.Contains(string(sums), fields[0]+" "+fields[1]+" h1:") {
			return nil, fmt.Errorf("hugo modules require pinned versions and matching go.sum entries")
		}
		modulePaths = append(modulePaths, fields[0])
	}
	if len(modulePaths) == 0 {
		return nil, fmt.Errorf("hugo modules require at least one pinned dependency")
	}
	return modulePaths, nil
}

func validateHugoSubmodule(root *os.Root, dir, name string) error {
	data, err := pagesReadFile(root, ".gitmodules")
	if err != nil {
		return err
	}
	declared := false
	for line := range strings.SplitSeq(string(data), "\n") {
		key, value, ok := strings.Cut(strings.TrimSpace(line), "=")
		if ok && strings.TrimSpace(key) == "path" && strings.Trim(strings.TrimSpace(value), `"`) == name {
			declared = true
		}
	}
	if !declared {
		return fmt.Errorf("theme is not declared in .gitmodules")
	}
	cmd := exec.CommandContext(context.Background(), "git", "-C", dir, "ls-files", "--stage", "--", name)
	output, err := cmd.Output()
	if err != nil {
		return fmt.Errorf("checking pinned submodule: %w", err)
	}
	fields := strings.Fields(string(output))
	if len(fields) < 4 || fields[0] != "160000" || len(fields[1]) != 40 || fields[2] != "0" {
		return fmt.Errorf("theme submodule has no pinned gitlink")
	}
	return nil
}

var (
	jekyllGemDeclaration  = regexp.MustCompile(`(?m)^\s*gem\s+['"]([^'"]+)['"]([^\r\n]*)`)
	jekyllGemConstraint   = regexp.MustCompile(`^\s*,\s*(?:"([^"]*)"|'([^']*)')`)
	jekyllGemContinuation = regexp.MustCompile(`^(?:[^'"#]|"(?:\\.|[^"\\])*"|'(?:\\.|[^'\\])*')*[,\\]\s*(?:#.*)?$`)
	jekyllRequirement     = regexp.MustCompile(`^(=|!=|~>|>=|<=|>|<)?\s*([0-9]+(?:\.[0-9]+)*)$`)
	jekyllLockedGem       = regexp.MustCompile(`(?m)^    ([A-Za-z0-9_.-]+) \(([^)]+)\)$`)
	jekyllLockDependency  = regexp.MustCompile(`(?m)^      ([A-Za-z0-9_.-]+)(?: \([^\n]+\))?$`)
	jekyllGitRevision     = regexp.MustCompile(`(?m)^  revision: [a-f0-9]{40}$`)
)

func validateJekyllSource(root *os.Root, source string) error {
	data, err := pagesReadFile(root, path.Join(source, "_config.yml"))
	if err != nil {
		return err
	}
	var values map[string]any
	if err := yaml.Unmarshal(data, &values); err != nil {
		return fmt.Errorf("reading jekyll config: %w", err)
	}
	gemfile, err := pagesReadFile(root, path.Join(source, "Gemfile"))
	if err != nil {
		return err
	}
	lock, err := pagesReadFile(root, path.Join(source, "Gemfile.lock"))
	if err != nil {
		return err
	}
	locked := make(map[string]string)
	for _, match := range jekyllLockedGem.FindAllSubmatch(lock, -1) {
		locked[string(match[1])] = string(match[2])
	}
	if locked["jekyll"] != "4.4.1" {
		return fmt.Errorf("jekyll must be locked to 4.4.1 in Gemfile.lock")
	}
	for _, match := range jekyllLockDependency.FindAllSubmatch(lock, -1) {
		if locked[string(match[1])] == "" {
			return fmt.Errorf("missing transitive dependency %q in Gemfile.lock", match[1])
		}
	}
	for section := range strings.SplitSeq(string(lock), "\n\n") {
		if strings.HasPrefix(section, "GIT\n") && !jekyllGitRevision.MatchString(section) {
			return fmt.Errorf("git dependencies in Gemfile.lock require a pinned commit")
		}
		if strings.HasPrefix(section, "PATH\n") {
			for line := range strings.SplitSeq(section, "\n") {
				if name, ok := strings.CutPrefix(line, "  remote: "); ok {
					if path.IsAbs(name) || name == ".." || strings.HasPrefix(path.Clean(name), "../") {
						return fmt.Errorf("path dependency in Gemfile.lock must stay inside the site source")
					}
					info, err := root.Stat(path.Join(source, name))
					if err != nil || !info.IsDir() {
						return fmt.Errorf("path dependency %q in Gemfile.lock is missing", name)
					}
				}
			}
		}
	}
	declared := false
	for _, match := range jekyllGemDeclaration.FindAllSubmatch(gemfile, -1) {
		name := string(match[1])
		declared = declared || name == "jekyll"
		if name == "jekyll" {
			if err := validateJekyllRequirements(string(match[2])); err != nil {
				return err
			}
		}
		if locked[name] == "" {
			return fmt.Errorf("dependency %q from Gemfile is missing from Gemfile.lock", name)
		}
	}
	if !declared || !strings.Contains(string(lock), "\nDEPENDENCIES\n") {
		return fmt.Errorf("jekyll must be declared in Gemfile and Gemfile.lock must contain dependencies")
	}
	for _, name := range []string{"jekyll-sass-converter", "jekyll-watch", "kramdown", "liquid", "rouge"} {
		if locked[name] == "" {
			return fmt.Errorf("missing jekyll dependency %q in Gemfile.lock", name)
		}
	}
	return nil
}

func validateJekyllRequirements(arguments string) error {
	if jekyllGemContinuation.MatchString(arguments) {
		return fmt.Errorf("jekyll version requirements in Gemfile must be on a single line")
	}
	for {
		argument := jekyllGemConstraint.FindStringSubmatch(arguments)
		if argument == nil {
			return nil
		}
		arguments = arguments[len(argument[0]):]
		requirement := strings.TrimSpace(argument[1] + argument[2])
		match := jekyllRequirement.FindStringSubmatch(requirement)
		if match == nil {
			return fmt.Errorf("unsupported jekyll version requirement %q in Gemfile", requirement)
		}
		parts := strings.Split(match[2], ".")
		version := make([]int, max(3, len(parts)))
		pinned := make([]int, len(version))
		copy(pinned, []int{4, 4, 1})
		for i, part := range parts {
			value, err := strconv.Atoi(part)
			if err != nil {
				return fmt.Errorf("invalid jekyll version requirement %q in Gemfile: %w", requirement, err)
			}
			version[i] = value
		}
		comparison := slices.Compare(pinned, version)
		compatible := false
		switch match[1] {
		case "", "=":
			compatible = comparison == 0
		case "!=":
			compatible = comparison != 0
		case ">":
			compatible = comparison > 0
		case ">=":
			compatible = comparison >= 0
		case "<":
			compatible = comparison < 0
		case "<=":
			compatible = comparison <= 0
		case "~>":
			prefix := max(1, len(parts)-1)
			compatible = comparison >= 0 && slices.Equal(pinned[:prefix], version[:prefix])
		}
		if !compatible {
			return fmt.Errorf("jekyll version requirement %q in Gemfile does not allow pinned version 4.4.1", requirement)
		}
	}
}
