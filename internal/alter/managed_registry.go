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
)

type managedPolicy uint8

const (
	managedPolicyRoot managedPolicy = iota
	managedPolicyLoader
	managedPolicyCore
	managedPolicyFragment
	managedPolicySharedStarter
)

type managedRegistryEntry struct {
	Path       string
	Policy     managedPolicy
	Capability managedCapability
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
		{Path: "just/tailor.just", Policy: managedPolicyCore},
		{Path: "just/go.just", Policy: managedPolicyFragment, Capability: managedCapabilityGo},
		{Path: "nix/go.nix", Policy: managedPolicyFragment, Capability: managedCapabilityGo},
		{Path: "just/pages.just", Policy: managedPolicyFragment, Capability: managedCapabilityPages},
		{Path: "nix/pages.nix", Policy: managedPolicyFragment, Capability: managedCapabilityPages},
		{Path: "nix/playwright.nix", Policy: managedPolicyFragment, Capability: managedCapabilityPlaywright},
		{Path: ".mcp.json", Policy: managedPolicySharedStarter, Capability: managedCapabilityPlaywright},
		{Path: ".codex/config.toml", Policy: managedPolicySharedStarter, Capability: managedCapabilityPlaywright},
		{Path: "opencode.json", Policy: managedPolicySharedStarter, Capability: managedCapabilityPlaywright},
		{Path: ".pi/mcp.json", Policy: managedPolicySharedStarter, Capability: managedCapabilityPlaywright},
	}
}

func validateManagedRegistry(registry []managedRegistryEntry) error {
	seen := make(map[string]struct{}, len(registry))
	for _, entry := range registry {
		if err := validateManagedPath(entry.Path); err != nil {
			return fmt.Errorf("managed registry: %w", err)
		}
		if _, exists := seen[entry.Path]; exists {
			return fmt.Errorf("managed registry contains duplicate destination %q", entry.Path)
		}
		seen[entry.Path] = struct{}{}

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
			if entry.Capability != managedCapabilityPlaywright {
				return fmt.Errorf("managed registry shared starter %q requires the playwright capability", entry.Path)
			}
		default:
			return fmt.Errorf("managed registry destination %q has an invalid policy", entry.Path)
		}
	}
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
	return capability == managedCapabilityGo || capability == managedCapabilityPages || capability == managedCapabilityPlaywright
}

func (policy managedPolicy) marked() bool {
	return policy == managedPolicyLoader || policy == managedPolicyCore || policy == managedPolicyFragment
}

func (policy managedPolicy) protected() bool {
	return policy == managedPolicyRoot || policy == managedPolicySharedStarter
}

func selectManagedFiles(cfg *config.Config) ([]managedSelection, error) {
	selected, err := selectManagedFilesFromRegistry(cfg, fixedManagedRegistry())
	if err != nil {
		return nil, fmt.Errorf("selecting managed files: %w", err)
	}
	return selected, nil
}

func selectManagedFilesFromRegistry(cfg *config.Config, registry []managedRegistryEntry) ([]managedSelection, error) {
	if cfg == nil {
		return nil, fmt.Errorf("managed file selection requires a config")
	}
	if err := validateManagedRegistry(registry); err != nil {
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
			declared, enabled := managedCapabilityState(cfg, entry.Capability)
			if declared && enabled {
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
	case managedCapabilityGo:
		return cfg.GoDeclared(), cfg.GoEnabled()
	case managedCapabilityPages:
		return cfg.PagesDeclared(), cfg.PagesEnabled()
	case managedCapabilityPlaywright:
		return cfg.PlaywrightDeclared(), cfg.PlaywrightEnabled()
	default:
		return false, false
	}
}
