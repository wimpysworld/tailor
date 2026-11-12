package alter

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"runtime"

	"github.com/wimpysworld/tailor/internal/config"
	"github.com/wimpysworld/tailor/internal/swatch"
)

type ShellPolicy struct {
	Reason           string
	AdoptionRequired bool
	Present          bool
}

func shellPolicyWarning(path, cause string) ShellPolicy {
	return ShellPolicy{AdoptionRequired: true, Reason: fmt.Sprintf("Shell manual adoption required for %s: %s. Existing files stay unchanged", path, cause)}
}

// InspectShellPolicy checks native policy locations without parsing or executing configuration.
func InspectShellPolicy(dir, destination string) (ShellPolicy, error) {
	if destination != ".shellcheckrc" && destination != ".editorconfig" {
		return ShellPolicy{}, nil
	}
	project, err := filepath.Abs(dir)
	if err != nil {
		return ShellPolicy{}, err
	}
	resolved, err := filepath.EvalSymlinks(project)
	if err != nil {
		return shellPolicyWarning(project, "cannot resolve project directory"), nil
	}
	project = resolved
	for current := project; ; current = filepath.Dir(current) {
		names := []string{destination}
		if destination == ".shellcheckrc" {
			names = append(names, "shellcheckrc")
		}
		for _, name := range names {
			path := filepath.Join(current, name)
			present, content, cause := inspectShellPolicyFile(path)
			if cause != "" {
				return shellPolicyWarning(path, cause), nil
			}
			if !present {
				continue
			}
			if destination == ".editorconfig" && current != project {
				continue
			}
			policy := ShellPolicy{Present: true, Reason: "existing Shell policy preserved: " + path}
			if destination == ".editorconfig" {
				starter, err := swatch.Content(destination)
				if err != nil {
					return ShellPolicy{}, err
				}
				if !bytes.Equal(content, starter) {
					policy = shellPolicyWarning(path, "shared EditorConfig needs manual review of the Shell section")
					policy.Present = true
				}
			}
			return policy, nil
		}
		if current == filepath.Dir(current) {
			break
		}
	}
	if destination == ".editorconfig" {
		return ShellPolicy{}, nil
	}
	if runtime.GOOS == "windows" || os.Getenv("SHELLCHECK_OPTS") != "" {
		return shellPolicyWarning(destination, "native rc lookup needs manual verification on Windows or with SHELLCHECK_OPTS"), nil
	}
	home := os.Getenv("HOME")
	if !filepath.IsAbs(home) {
		return shellPolicyWarning(destination, "HOME is absent or not absolute"), nil
	}
	xdg := os.Getenv("XDG_CONFIG_HOME")
	if !filepath.IsAbs(xdg) {
		xdg = filepath.Join(home, ".config")
	}
	for _, path := range []string{filepath.Join(home, ".shellcheckrc"), filepath.Join(xdg, "shellcheckrc")} {
		present, _, cause := inspectShellPolicyFile(path)
		if cause != "" {
			return shellPolicyWarning(path, cause), nil
		}
		if present {
			return ShellPolicy{Present: true, Reason: "existing Shell policy preserved: " + path}, nil
		}
	}
	return ShellPolicy{}, nil
}

func inspectShellPolicyFile(path string) (bool, []byte, string) {
	base := filepath.VolumeName(path) + string(filepath.Separator)
	name, err := filepath.Rel(base, path)
	if err != nil {
		return false, nil, "cannot resolve policy path"
	}
	root, err := os.OpenRoot(base)
	if err != nil {
		return false, nil, "cannot open policy filesystem"
	}
	defer root.Close()
	if err := checkParents(root, name, "Shell policy parent"); err != nil {
		return false, nil, err.Error()
	}
	for parent := filepath.Dir(name); parent != "."; parent = filepath.Dir(parent) {
		info, err := root.Lstat(parent)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil || info.Mode().Perm()&0o111 == 0 {
			return false, nil, "cannot safely inspect an accessible policy parent"
		}
	}
	info, err := root.Lstat(name)
	if os.IsNotExist(err) {
		return false, nil, ""
	}
	if err != nil || !info.Mode().IsRegular() || info.Mode().Perm()&0o444 == 0 {
		return false, nil, "cannot safely inspect a readable regular file"
	}
	content, cause := readManagedInspection(base, filepath.ToSlash(name))
	return true, content, cause
}

func processShellStarter(root *os.Root, entry config.SwatchEntry, content []byte, mode ApplyMode) (SwatchResult, error) {
	policy, err := InspectShellPolicy(root.Name(), entry.Path)
	if err != nil {
		return SwatchResult{}, err
	}
	if policy.Reason != "" {
		return SwatchResult{Path: entry.Path, Category: Skipped, Reason: SwatchReason(policy.Reason)}, nil
	}
	if !mode.ShouldWrite() {
		return SwatchResult{Path: entry.Path, Category: WouldCopy}, nil
	}
	changed, err := writeManagedFile(root, entry.Path, content, true, managedApplyHooks{})
	if err != nil {
		return SwatchResult{}, err
	}
	if changed {
		return SwatchResult{Path: entry.Path, Category: WouldCopy}, nil
	}
	return SwatchResult{Path: entry.Path, Category: Skipped, Reason: SkipManagedSharedExists}, nil
}
