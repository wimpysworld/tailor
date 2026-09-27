package alter

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path"

	"github.com/wimpysworld/tailor/internal/config"
	"github.com/wimpysworld/tailor/internal/swatch"
)

// ApplyLocal refreshes explicitly selected, existing owned files without remote access.
func ApplyLocal(cfg *config.Config, dir string, paths []string) (updated []string, retErr error) {
	if cfg == nil || len(paths) == 0 {
		return nil, fmt.Errorf("local-only alteration requires a config and at least one path")
	}
	if err := validateConfig(cfg); err != nil {
		return nil, err
	}
	seen := make(map[string]bool)
	for _, name := range paths {
		switch name {
		case dependabotPath, "just/tailor.just", "nix/hooks/go.nix":
		default:
			return nil, fmt.Errorf("path %q is not supported by local-only alteration", name)
		}
		if seen[name] {
			return nil, fmt.Errorf("duplicate local-only path %q", name)
		}
		seen[name] = true
	}
	info, err := os.Lstat(dir)
	if err != nil {
		return nil, err
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("local-only project root must be a directory, not a symlink")
	}
	root, err := os.OpenRoot(dir)
	if err != nil {
		return nil, err
	}
	defer func() { retErr = errors.Join(retErr, root.Close()) }()

	files := make([]localFile, 0, len(paths))
	defer func() {
		for _, file := range files {
			retErr = errors.Join(retErr, file.parent.Close())
		}
	}()
	for _, name := range paths {
		file, err := prepareLocalFile(root, cfg, dir, name)
		if err != nil {
			return nil, err
		}
		files = append(files, file)
	}
	for _, file := range files {
		if bytes.Equal(file.before.Content, file.content) {
			continue
		}
		check := func(string) error { return file.recheck(root) }
		if err := check(file.name); err != nil {
			return updated, err
		}
		changed, err := writeManagedFile(file.parent, path.Base(file.name), file.content, false, managedApplyHooks{beforeRename: check})
		if changed {
			updated = append(updated, file.name)
		}
		if err != nil {
			return updated, err
		}
	}
	return updated, nil
}

type localFile struct {
	name       string
	parent     *os.Root
	parentInfo os.FileInfo
	before     dependabotDestinationSnapshot
	content    []byte
	dependabot *dependabotPlan
}

func prepareLocalFile(root *os.Root, cfg *config.Config, dir, name string) (file localFile, retErr error) {
	file.name = name
	if name == dependabotPath {
		selection := dependabotSelectionFromConfig(cfg, Apply)
		if !selection.Present || selection.Mode == swatch.Never {
			return file, fmt.Errorf("local-only destination %q is not enabled in the config", name)
		}
	}
	if err := checkParents(root, name, "local-only destination parent"); err != nil {
		return file, err
	}
	var err error
	file.parent, err = root.OpenRoot(path.Dir(name))
	if err != nil {
		return file, err
	}
	defer func() {
		if retErr != nil {
			retErr = errors.Join(retErr, file.parent.Close())
		}
	}()
	file.parentInfo, err = file.parent.Stat(".")
	if err != nil {
		return file, err
	}
	file.before, err = inspectDependabotEntry(file.parent, path.Base(name), true, dependabotInspectionHooks{})
	if err != nil {
		return file, err
	}
	if file.before.Kind != dependabotDestinationRegular || !hasManagedMarker(file.before.Content, name) {
		return file, fmt.Errorf("local-only destination %q must be an existing regular file owned by Tailor", name)
	}
	if name == dependabotPath {
		plan, err := prepareDependabotExecution(cfg, dir)
		if err != nil {
			return file, err
		}
		if plan.Outcome != dependabotOutcomeReplace && plan.Outcome != dependabotOutcomeUnchanged {
			return file, fmt.Errorf("local-only destination %q is not enabled in the config", name)
		}
		file.content, file.dependabot = plan.Output, &plan
		return file, nil
	}
	selections, err := selectManagedFiles(cfg)
	if err != nil {
		return file, err
	}
	for _, selection := range selections {
		if selection.Entry.Path != name || !selection.Enabled {
			continue
		}
		rendered, err := renderManagedFiles(cfg, []managedSelection{selection})
		if err != nil {
			return file, err
		}
		file.content = rendered[name]
		return file, nil
	}
	return file, fmt.Errorf("local-only destination %q is not enabled in the config", name)
}

func (file localFile) recheck(root *os.Root) error {
	if err := checkParents(root, file.name, "local-only destination parent"); err != nil {
		return err
	}
	info, err := root.Lstat(path.Dir(file.name))
	if err != nil || !os.SameFile(info, file.parentInfo) {
		return fmt.Errorf("local-only parent for %q changed after preflight", file.name)
	}
	current, err := inspectDependabotEntry(file.parent, path.Base(file.name), true, dependabotInspectionHooks{})
	if err != nil {
		return err
	}
	if current.Kind != dependabotDestinationRegular || !os.SameFile(current.Identity, file.before.Identity) || !bytes.Equal(current.Content, file.before.Content) {
		return fmt.Errorf("local-only destination %q changed after preflight", file.name)
	}
	if file.dependabot != nil {
		return recheckDependabotSnapshot(root, file.dependabot.Snapshot, nil)
	}
	return nil
}
