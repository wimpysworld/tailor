package alter

import (
	"slices"
	"strings"

	"github.com/wimpysworld/tailor/internal/config"
	"github.com/wimpysworld/tailor/internal/output"
)

func appendManagedGuidance(report *Report, guidance []string) {
	for _, text := range guidance {
		if text == "" || slices.ContainsFunc(report.Document.Guidance, func(item output.Guidance) bool { return item.Text == text }) {
			continue
		}
		appendGuidance(report, text+"\n", output.GuidanceRequired)
	}
}

func managedRootGuidance(cfg *config.Config, dir, destination string) []string {
	if destination != "justfile" && destination != "flake.nix" {
		return nil
	}
	content, cause := readManagedInspection(dir, destination)
	if cause != "" {
		return []string{"Verify Tailor's loader setup in `" + destination + "`: " + cause + ". Tailor left the file untouched."}
	}
	if destination == "justfile" {
		inspection := inspectManagedJust(content)
		switch inspection.state {
		case managedSetupMissing:
			return []string{"Add `import 'just/loader.just'` to the preserved `justfile`."}
		case managedSetupUnknown:
			return []string{"Verify `import 'just/loader.just'` in `justfile`: " + inspection.cause + "."}
		default:
			return nil
		}
	}
	inspection := inspectManagedNix(content)
	packageEdit := "Append `++ import ./nix/loader.nix { inherit pkgs; }` to the existing package list in `flake.nix`."
	if inspection.packages.cause == "no packages attribute" {
		packageEdit = "Add `packages = import ./nix/loader.nix { inherit pkgs; };` to the development shell in `flake.nix`. Use the same overridden `pkgs` for both loaders."
	}
	hookEdit := "Add `shellHook = import ./nix/hooks.nix { inherit pkgs; };` to the development shell in `flake.nix`."
	if inspection.hooks.cause == "literal custom hook" {
		hookEdit = "Prepend `(import ./nix/hooks.nix { inherit pkgs; }) + \"\\n\" +` to the existing `shellHook` expression in `flake.nix`. Use the same overridden `pkgs` for both loaders and keep custom hooks last."
	}
	checks := []struct {
		name   string
		result managedSetupInspection
		edit   string
	}{
		{"package loader", inspection.packages, packageEdit},
		{"core hook", inspection.hooks, hookEdit},
	}
	if cfg.EbitengineAndroidEnabled() {
		checks = append(checks, struct {
			name   string
			result managedSetupInspection
			edit   string
		}{
			"android output", inspection.android,
			"Add `android = import ./nix/shells/go-ebitengine-android.nix { inherit pkgs; };` to the x86_64-linux devShells set in `flake.nix`. Review SDK licence consent before shell entry.",
		})
	}
	if cfg.BlenderEnabled() {
		checks = append(checks, struct {
			name   string
			result managedSetupInspection
			edit   string
		}{
			"blender-agent-bridge-extension output", inspection.blender,
			"Add `blender-agent-bridge-extension = (import ./nix/lib/blender.nix { inherit pkgs; }).extension;` to the per-system packages in `flake.nix`. Guard the set with `pkgs.lib.optionalAttrs (builtins.pathExists ./nix/lib/blender.nix)`. Install the ZIP separately after approval.",
		})
	}
	var guidance, unknown []string
	for _, check := range checks {
		switch check.result.state {
		case managedSetupMissing:
			guidance = append(guidance, check.edit)
		case managedSetupUnknown:
			unknown = append(unknown, check.name+": "+check.result.cause)
		}
	}
	if len(unknown) != 0 {
		guidance = append(guidance, "Verify `flake.nix` ("+strings.Join(unknown, "; ")+"). Use the same overridden `pkgs` for both loaders and keep custom hooks last. Syntax inspection does not establish runtime compatibility.")
	}
	return guidance
}
