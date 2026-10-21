package alter

import (
	"fmt"
	"io/fs"
	"path"
	"strings"

	"github.com/wimpysworld/tailor/internal/config"
	"github.com/wimpysworld/tailor/internal/swatch"
)

type managedCapability uint8

const (
	managedCapabilityNone managedCapability = iota
	managedCapabilityGo
	managedCapabilityPages
	managedCapabilityPlaywright
	managedCapabilityGoFFmpegStatigo
	managedCapabilityRust
	managedCapabilityGoEbitengine
	managedCapabilityGoEbitengineAndroid
)

type managedPolicy uint8

const (
	managedPolicyRoot managedPolicy = iota
	managedPolicyLoader
	managedPolicyCore
	managedPolicyFragment
	managedPolicySharedStarter
)

type managedNixKind uint8

const (
	managedNixList managedNixKind = iota
	managedNixHook
	managedNixSupport
	managedNixShell
)

type managedRegistryEntry struct {
	NixKind    managedNixKind
	Path       string
	Policy     managedPolicy
	Capability managedCapability
	BuildBody  managedQualityBodyName
	LintBody   managedQualityBodyName
	TestBody   managedQualityBodyName
	SetupBody  managedQualityBodyName
}

type managedRecipeOperation string

const (
	managedBuild managedRecipeOperation = "build"
	managedTest  managedRecipeOperation = "test"
	managedLint  managedRecipeOperation = "lint"
	managedSetup managedRecipeOperation = "setup"
)

func managedOperations() []managedRecipeOperation {
	return []managedRecipeOperation{managedBuild, managedTest, managedLint, managedSetup}
}

type managedQualityBodyName string

func (operation managedRecipeOperation) body(entry managedRegistryEntry) managedQualityBodyName {
	switch operation {
	case managedBuild:
		return entry.BuildBody
	case managedTest:
		return entry.TestBody
	case managedLint:
		return entry.LintBody
	case managedSetup:
		return entry.SetupBody
	default:
		return ""
	}
}

type managedSelection struct {
	Entry   managedRegistryEntry
	Enabled bool
}

func fixedManagedRegistry() []managedRegistryEntry {
	return []managedRegistryEntry{
		{Path: "justfile", Policy: managedPolicyRoot},
		{Path: "flake.nix", Policy: managedPolicyRoot},
		{Path: "just/loader.just", Policy: managedPolicyLoader},
		{Path: "nix/loader.nix", Policy: managedPolicyLoader},
		{Path: "nix/hooks.nix", Policy: managedPolicyLoader},
		{Path: "nix/hooks/core.nix", Policy: managedPolicyCore, NixKind: managedNixHook},
		{Path: "nix/hooks/go.nix", Policy: managedPolicyFragment, Capability: managedCapabilityGo, NixKind: managedNixHook},
		{Path: "nix/hooks/go-ffmpeg-statigo.nix", Policy: managedPolicyFragment, Capability: managedCapabilityGoFFmpegStatigo, NixKind: managedNixHook},
		{Path: "just/tailor.just", Policy: managedPolicyCore},
		{Path: "just/go.just", Policy: managedPolicyFragment, Capability: managedCapabilityGo, BuildBody: "build-go", TestBody: "test-go", LintBody: "lint-go"},
		{Path: "nix/go.nix", Policy: managedPolicyFragment, Capability: managedCapabilityGo},
		{Path: "nix/go-ebitengine.nix", Policy: managedPolicyFragment, Capability: managedCapabilityGoEbitengine},
		{Path: "nix/lib/go-ebitengine-runtime.nix", Policy: managedPolicyFragment, Capability: managedCapabilityGoEbitengine, NixKind: managedNixSupport},
		{Path: "nix/hooks/go-ebitengine.nix", Policy: managedPolicyFragment, Capability: managedCapabilityGoEbitengine, NixKind: managedNixHook},
		{Path: "nix/shells/go-ebitengine-android.nix", Policy: managedPolicyFragment, Capability: managedCapabilityGoEbitengineAndroid, NixKind: managedNixShell},
		{Path: "just/rust.just", Policy: managedPolicyFragment, Capability: managedCapabilityRust, BuildBody: "build-rust", TestBody: "test-rust", LintBody: "lint-rust"},
		{Path: "nix/rust.nix", Policy: managedPolicyFragment, Capability: managedCapabilityRust},
		{Path: "just/go-ffmpeg-statigo.just", Policy: managedPolicyFragment, Capability: managedCapabilityGoFFmpegStatigo, SetupBody: "go-ffmpeg-statigo"},
		{Path: "nix/go-ffmpeg-statigo.nix", Policy: managedPolicyFragment, Capability: managedCapabilityGoFFmpegStatigo},
		{Path: "just/pages.just", Policy: managedPolicyFragment, Capability: managedCapabilityPages},
		{Path: "nix/pages.nix", Policy: managedPolicyFragment, Capability: managedCapabilityPages},
		{Path: "nix/playwright.nix", Policy: managedPolicyFragment, Capability: managedCapabilityPlaywright},
		{Path: ".mcp.json", Policy: managedPolicySharedStarter},
		{Path: ".codex/config.toml", Policy: managedPolicySharedStarter},
		{Path: "opencode.json", Policy: managedPolicySharedStarter},
		{Path: ".pi/mcp.json", Policy: managedPolicySharedStarter},
	}
}

func validateManagedRegistry(registry []managedRegistryEntry) error {
	seen := make(map[string]struct{}, len(registry))
	recipes := make(map[string]string)
	capabilities := make(map[managedRecipeOperation]map[managedCapability]string)
	for _, operation := range managedOperations() {
		capabilities[operation] = make(map[managedCapability]string)
	}
	for _, entry := range registry {
		if err := validateManagedPath(entry.Path); err != nil {
			return fmt.Errorf("managed registry: %w", err)
		}
		if _, exists := seen[entry.Path]; exists {
			return fmt.Errorf("managed registry contains duplicate destination %q", entry.Path)
		}
		seen[entry.Path] = struct{}{}
		if entry.NixKind > managedNixShell || entry.NixKind != managedNixList && (path.Ext(entry.Path) != ".nix" || (entry.Policy != managedPolicyCore && entry.Policy != managedPolicyFragment)) {
			return fmt.Errorf("managed registry destination %q has an invalid Nix kind", entry.Path)
		}

		switch entry.Policy {
		case managedPolicyRoot, managedPolicyLoader, managedPolicyCore:
			if entry.Capability != managedCapabilityNone {
				return fmt.Errorf("managed registry destination %q has an invalid capability", entry.Path)
			}
		case managedPolicyFragment:
			if !entry.Capability.validDeclaration() {
				return fmt.Errorf("managed registry fragment %q requires a capability", entry.Path)
			}
		case managedPolicySharedStarter:
			if entry.Capability != managedCapabilityNone {
				return fmt.Errorf("managed registry shared starter %q has an invalid capability", entry.Path)
			}
		default:
			return fmt.Errorf("managed registry destination %q has an invalid policy", entry.Path)
		}

		for _, operation := range managedOperations() {
			name, kind := string(operation.body(entry)), string(operation)+" body"
			if err := validateManagedRecipeMetadata(entry, kind, name, recipes, capabilities[operation]); err != nil {
				return err
			}
			if name != "" {
				if _, err := operation.contribution(name); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

func validateManagedRecipeMetadata(entry managedRegistryEntry, kind, recipe string, recipes map[string]string, capabilities map[managedCapability]string) error {
	if recipe == "" {
		return nil
	}
	if entry.Policy != managedPolicyFragment || path.Dir(entry.Path) != "just" || path.Ext(entry.Path) != ".just" {
		return fmt.Errorf("managed registry %s %q must belong to a Just capability fragment", kind, recipe)
	}
	if !validManagedRecipeName(recipe) {
		return fmt.Errorf("managed registry destination %q has invalid %s %q", entry.Path, kind, recipe)
	}
	if previous, exists := recipes[recipe]; exists {
		return fmt.Errorf("managed registry %s %q is duplicated by %q and %q", kind, recipe, previous, entry.Path)
	}
	recipes[recipe] = entry.Path
	if previous, exists := capabilities[entry.Capability]; exists {
		return fmt.Errorf("managed capability %q has multiple %s contributions in %q and %q", entry.Capability.canonicalName(), kind, previous, entry.Path)
	}
	capabilities[entry.Capability] = entry.Path
	return nil
}

func validateManagedPath(name string) error {
	if name == "" || name == "." || !fs.ValidPath(name) || path.Clean(name) != name || strings.Contains(name, `\`) {
		return fmt.Errorf("unsafe managed destination %q", name)
	}
	for _, r := range name {
		if r < 0x20 || r == 0x7f {
			return fmt.Errorf("unsafe managed destination %q", name)
		}
	}
	return nil
}

func (capability managedCapability) validDeclaration() bool {
	return capability == managedCapabilityGoEbitengine || capability == managedCapabilityGoEbitengineAndroid || capability == managedCapabilityRust || capability == managedCapabilityGo || capability == managedCapabilityPages || capability == managedCapabilityPlaywright || capability == managedCapabilityGoFFmpegStatigo
}

func (capability managedCapability) canonicalName() string {
	switch capability {
	case managedCapabilityGoEbitengine:
		return "go-ebitengine"
	case managedCapabilityGoEbitengineAndroid:
		return "go-ebitengine-android"
	case managedCapabilityRust:
		return "rust"
	case managedCapabilityGo:
		return "go"
	case managedCapabilityGoFFmpegStatigo:
		return "go-ffmpeg-statigo"
	case managedCapabilityPages:
		return "pages"
	case managedCapabilityPlaywright:
		return "playwright"
	default:
		return ""
	}
}

func (policy managedPolicy) marked() bool {
	return policy == managedPolicyLoader || policy == managedPolicyCore || policy == managedPolicyFragment
}

func (policy managedPolicy) protected() bool {
	return policy == managedPolicyRoot || policy == managedPolicySharedStarter
}

func selectManagedFiles(cfg *config.Config) ([]managedSelection, error) {
	selected, err := selectManagedFilesFromRegistries(cfg, fixedManagedRegistry(), fixedManagedMCPRegistry())
	if err != nil {
		return nil, fmt.Errorf("selecting managed files: %w", err)
	}
	return selected, nil
}

func selectManagedFilesFromRegistries(cfg *config.Config, registry []managedRegistryEntry, mcpRegistry []managedMCPServerDefinition) ([]managedSelection, error) {
	if cfg == nil {
		return nil, fmt.Errorf("managed file selection requires a config")
	}
	if err := validateManagedRegistry(registry); err != nil {
		return nil, err
	}
	if err := validateManagedMCPRegistry(mcpRegistry); err != nil {
		return nil, err
	}

	selected := make([]managedSelection, 0, len(registry))
	for _, entry := range registry {
		switch entry.Policy {
		case managedPolicyRoot:
			enabled, err := managedRootBootstrapEnabled(cfg, entry.Path)
			if err != nil {
				return nil, err
			}
			if enabled {
				selected = append(selected, managedSelection{Entry: entry, Enabled: true})
			}
		case managedPolicyLoader, managedPolicyCore:
			selected = append(selected, managedSelection{Entry: entry, Enabled: true})
		case managedPolicyFragment:
			declared, enabled := managedCapabilityState(cfg, entry.Capability)
			if declared {
				selected = append(selected, managedSelection{Entry: entry, Enabled: enabled})
			}
		case managedPolicySharedStarter:
			if managedMCPDestinationEnabled(cfg, entry.Path, mcpRegistry) {
				selected = append(selected, managedSelection{Entry: entry, Enabled: true})
			}
		}
	}
	return selected, nil
}

func managedRootBootstrapEnabled(cfg *config.Config, destination string) (bool, error) {
	found := false
	enabled := false
	for _, entry := range cfg.Swatches {
		if entry.Path != destination {
			continue
		}
		if found {
			return false, fmt.Errorf("duplicate root bootstrap entry %q", destination)
		}
		found = true
		switch entry.Alteration {
		case swatch.Always, swatch.FirstFit:
			enabled = true
		case swatch.Never:
			enabled = false
		default:
			return false, fmt.Errorf("root bootstrap entry %q has invalid alteration %q", destination, entry.Alteration)
		}
	}
	return found && enabled, nil
}

func managedCapabilityState(cfg *config.Config, capability managedCapability) (declared, enabled bool) {
	switch capability {
	case managedCapabilityRust:
		return cfg.RustDeclared(), cfg.RustEnabled()
	case managedCapabilityGo:
		return cfg.GoDeclared(), cfg.GoEnabled()
	case managedCapabilityGoEbitengine:
		return cfg.EbitengineDeclared() || cfg.GoDeclared() && !cfg.GoEnabled(), cfg.EbitengineEnabled()
	case managedCapabilityGoEbitengineAndroid:
		return cfg.EbitengineDeclared() || cfg.GoDeclared() && !cfg.GoEnabled(), cfg.EbitengineAndroidEnabled()
	case managedCapabilityGoFFmpegStatigo:
		return cfg.FFmpegStatigoDeclared() || cfg.GoDeclared() && !cfg.GoEnabled(), cfg.FFmpegStatigoEnabled()
	case managedCapabilityPages:
		return cfg.PagesDeclared(), cfg.PagesEnabled()
	case managedCapabilityPlaywright:
		return cfg.PlaywrightDeclared(), cfg.PlaywrightEnabled()
	default:
		return false, false
	}
}
