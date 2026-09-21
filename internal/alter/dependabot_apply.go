package alter

import (
	"bytes"
	"crypto/rand"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"

	"github.com/wimpysworld/tailor/internal/config"
	"github.com/wimpysworld/tailor/internal/swatch"
)

type dependabotInspectionHooks struct {
	beforeRootOpen        func(string) error
	beforeDestinationOpen func(string) error
	readDestination       func(string, io.Reader, int64) ([]byte, error)
}

func prepareDependabotExecution(cfg *config.Config, dir string) (dependabotPlan, error) {
	return prepareDependabotExecutionWithHooks(cfg, dir, dependabotInspectionHooks{})
}

func prepareDependabotExecutionWithHooks(cfg *config.Config, dir string, hooks dependabotInspectionHooks) (plan dependabotPlan, retErr error) {
	selection := dependabotSelectionFromConfig(cfg, DryRun)
	if !selection.Present || selection.Mode == swatch.Never {
		return planDependabot(selection, dependabotSnapshot{}, dependabotBodies{})
	}

	if err := runDependabotHook(hooks.beforeRootOpen, dir); err != nil {
		return dependabotPlan{}, fmt.Errorf("opening Dependabot project root %q: %w", dir, err)
	}
	root, err := os.OpenRoot(dir)
	if err != nil {
		return dependabotPlan{}, fmt.Errorf("opening Dependabot project root %q: %w", dir, err)
	}
	defer func() {
		if err := root.Close(); err != nil {
			retErr = errors.Join(retErr, fmt.Errorf("closing Dependabot project root %q: %w", dir, err))
		}
	}()

	snapshot, err := inspectDependabotSnapshot(root, hooks)
	if err != nil {
		return dependabotPlan{}, err
	}
	bodies, err := renderDependabotBodies()
	if err != nil {
		return dependabotPlan{}, err
	}
	return planDependabot(selection, snapshot, bodies)
}

func inspectDependabotSnapshot(root *os.Root, hooks dependabotInspectionHooks) (dependabotSnapshot, error) {
	snapshot := dependabotSnapshot{
		Root:        dependabotIdentitySnapshot{Path: ".", Exists: true},
		Parent:      dependabotIdentitySnapshot{Path: path.Dir(dependabotPath)},
		Alternate:   dependabotDestinationSnapshot{Path: dependabotAlternatePath, Kind: dependabotDestinationMissing},
		Destination: dependabotDestinationSnapshot{Path: dependabotPath, Kind: dependabotDestinationMissing},
	}

	rootInfo, err := root.Stat(".")
	if err != nil {
		return dependabotSnapshot{}, dependabotConflict("the project root cannot be inspected")
	}
	if !rootInfo.IsDir() {
		return dependabotSnapshot{}, dependabotConflict("the project root is not a directory")
	}
	snapshot.Root.Identity = rootInfo

	if err := checkParents(root, dependabotPath, "Dependabot destination parent"); err != nil {
		return dependabotSnapshot{}, dependabotConflict(err.Error())
	}
	parentInfo, err := root.Lstat(snapshot.Parent.Path)
	if err != nil {
		if !errors.Is(err, fs.ErrNotExist) {
			return dependabotSnapshot{}, dependabotConflict("the destination parent cannot be inspected")
		}
	} else {
		snapshot.Parent.Exists = true
		snapshot.Parent.Identity = parentInfo
	}

	snapshot.Alternate, err = inspectDependabotEntry(root, dependabotAlternatePath, false, hooks)
	if err != nil {
		return dependabotSnapshot{}, err
	}
	if snapshot.Alternate.Kind != dependabotDestinationMissing {
		return snapshot, nil
	}

	snapshot.Destination, err = inspectDependabotEntry(root, dependabotPath, true, hooks)
	if err != nil {
		return dependabotSnapshot{}, err
	}
	return snapshot, nil
}

func inspectDependabotEntry(root *os.Root, destination string, readContent bool, hooks dependabotInspectionHooks) (dependabotDestinationSnapshot, error) {
	snapshot := dependabotDestinationSnapshot{Path: destination, Kind: dependabotDestinationMissing}
	info, err := root.Lstat(destination)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return snapshot, nil
		}
		return dependabotDestinationSnapshot{}, dependabotConflict("a Dependabot destination cannot be inspected")
	}
	snapshot.Kind = dependabotKind(info)
	snapshot.Identity = info
	if snapshot.Kind != dependabotDestinationRegular || !readContent {
		return snapshot, nil
	}

	if err := runDependabotHook(hooks.beforeDestinationOpen, destination); err != nil {
		return dependabotDestinationSnapshot{}, dependabotConflict("the Dependabot destination changed before it was opened")
	}
	file, err := openDependabotFile(root, destination)
	if err != nil {
		return dependabotDestinationSnapshot{}, dependabotConflict("the Dependabot destination cannot be opened")
	}
	content, inspectErr := inspectOpenedDependabotFile(destination, file, info, hooks)
	closeErr := file.Close()
	if inspectErr != nil {
		return dependabotDestinationSnapshot{}, inspectErr
	}
	if closeErr != nil {
		return dependabotDestinationSnapshot{}, dependabotConflict("the Dependabot destination cannot be closed after inspection")
	}
	snapshot.Content = content
	return snapshot, nil
}

func inspectOpenedDependabotFile(destination string, file *os.File, inspected os.FileInfo, hooks dependabotInspectionHooks) ([]byte, error) {
	opened, err := file.Stat()
	if err != nil {
		return nil, dependabotConflict("the opened Dependabot destination cannot be inspected")
	}
	if !opened.Mode().IsRegular() || !os.SameFile(inspected, opened) {
		return nil, dependabotConflict("the Dependabot destination changed while it was opened")
	}

	const readLimit = int64(dependabotMaxBytes + 1)
	var content []byte
	if hooks.readDestination != nil {
		content, err = hooks.readDestination(destination, io.LimitReader(file, readLimit), readLimit)
	} else {
		content, err = io.ReadAll(io.LimitReader(file, readLimit))
	}
	if err != nil {
		return nil, dependabotConflict("the Dependabot destination cannot be read")
	}
	if len(content) > dependabotMaxBytes {
		return nil, dependabotConflict("the Dependabot destination exceeds 1048576 bytes")
	}
	return bytes.Clone(content), nil
}

func dependabotKind(info os.FileInfo) dependabotDestinationKind {
	switch {
	case info.Mode()&os.ModeSymlink != 0:
		return dependabotDestinationSymlink
	case info.IsDir():
		return dependabotDestinationDirectory
	case info.Mode().IsRegular():
		return dependabotDestinationRegular
	default:
		return dependabotDestinationSpecial
	}
}

func applyDependabotPlan(dir string, plan dependabotPlan) (bool, error) {
	return applyDependabotPlanWithParentAndHooks(dir, plan, nil, managedApplyHooks{})
}

func applyDependabotPlanWithHooks(dir string, plan dependabotPlan, hooks managedApplyHooks) (bool, error) {
	return applyDependabotPlanWithParentAndHooks(dir, plan, nil, hooks)
}

func applyDependabotPlanWithParentAndHooks(dir string, plan dependabotPlan, confirmedParent *dependabotCreatedParent, hooks managedApplyHooks) (bool, error) {
	return applyDependabotPlanWithParentCloseHook(dir, plan, confirmedParent, hooks, nil)
}

//nolint:gocyclo // The publication sequence keeps each safety check next to its filesystem operation.
func applyDependabotPlanWithParentCloseHook(dir string, plan dependabotPlan, confirmedParent *dependabotCreatedParent, hooks managedApplyHooks, parentClose func(*dependabotParentCreation) error) (published bool, retErr error) {
	if plan.Outcome != dependabotOutcomeCreate && plan.Outcome != dependabotOutcomeReplace {
		return false, nil
	}
	if err := validateDependabotPublicationPlan(plan); err != nil {
		return false, err
	}

	root, err := os.OpenRoot(dir)
	if err != nil {
		return false, fmt.Errorf("opening Dependabot project root %q: %w", dir, err)
	}
	defer func() {
		if err := root.Close(); err != nil {
			retErr = errors.Join(retErr, fmt.Errorf("closing Dependabot project root %q: %w", dir, err))
		}
	}()

	createdParentIdentity := dependabotParentIdentity(confirmedParent)
	if err := recheckDependabotSnapshot(root, plan.Snapshot, createdParentIdentity); err != nil {
		return false, err
	}

	var localParent *dependabotParentCreation
	if !plan.Snapshot.Parent.Exists && confirmedParent == nil {
		if plan.Outcome != dependabotOutcomeCreate {
			return false, dependabotConflict("the destination parent was absent for a replacement")
		}
		localParent = newDependabotParentCreation(plan)
		defer func() {
			retErr = errors.Join(retErr, closeDependabotParent(localParent, parentClose))
		}()
		created, err := localParent.publish(root, plan.Snapshot.Parent.Path, 0o755)
		if err != nil {
			return false, err
		}
		if !created {
			return false, dependabotConflict("the destination parent was not created for publication")
		}
		confirmedParent = localParent.confirmedParent()
		createdParentIdentity = dependabotParentIdentity(confirmedParent)
	}

	mode := os.FileMode(0o644)
	if plan.Outcome == dependabotOutcomeReplace {
		mode = plan.Snapshot.Destination.Identity.Mode().Perm()
	}

	parentRoot := (*os.Root)(nil)
	closeParentRoot := false
	if confirmedParent != nil {
		parentRoot = confirmedParent.root
	} else {
		parentRoot, err = root.OpenRoot(plan.Snapshot.Parent.Path)
		if err != nil {
			return false, dependabotConflict("the destination parent cannot be opened for publication")
		}
		closeParentRoot = true
	}
	if parentRoot == nil {
		return false, dependabotConflict("the created destination parent handle is unavailable")
	}
	if closeParentRoot {
		defer func() {
			if err := parentRoot.Close(); err != nil {
				retErr = errors.Join(retErr, fmt.Errorf("closing Dependabot destination parent %q: %w", plan.Snapshot.Parent.Path, err))
			}
		}()
	}
	parentInfo, err := parentRoot.Stat(".")
	expectedParent := plan.Snapshot.Parent.Identity
	if createdParentIdentity != nil {
		expectedParent = createdParentIdentity
	}
	if err != nil || expectedParent == nil || !parentInfo.IsDir() || !os.SameFile(expectedParent, parentInfo) {
		return false, dependabotConflict("the destination parent changed while it was opened for publication")
	}

	if err := runManagedHook(hooks.beforeTempCreate, dependabotPath); err != nil {
		return false, fmt.Errorf("creating temporary Dependabot file for %q: %w", dependabotPath, err)
	}
	tempName := path.Base(dependabotPath) + ".tmp-" + rand.Text()
	tempPath := path.Join(plan.Snapshot.Parent.Path, tempName)
	temp, err := parentRoot.OpenFile(tempName, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return false, fmt.Errorf("creating temporary Dependabot file for %q: %w", dependabotPath, err)
	}
	tempOpen := true
	defer func() {
		if tempOpen {
			if err := temp.Close(); err != nil {
				retErr = errors.Join(retErr, fmt.Errorf("closing temporary Dependabot file for %q: %w", dependabotPath, err))
			}
		}
		if tempName != "" {
			if err := parentRoot.Remove(tempName); err != nil && !errors.Is(err, fs.ErrNotExist) {
				retErr = errors.Join(retErr, fmt.Errorf("removing temporary Dependabot file for %q: %w", dependabotPath, err))
			}
		}
	}()

	if err := runManagedHook(hooks.beforeTempWrite, dependabotPath); err != nil {
		return false, fmt.Errorf("writing temporary Dependabot file for %q: %w", dependabotPath, err)
	}
	written, err := temp.Write(plan.Output)
	if err != nil {
		return false, fmt.Errorf("writing temporary Dependabot file for %q: %w", dependabotPath, err)
	}
	if written != len(plan.Output) {
		return false, fmt.Errorf("writing temporary Dependabot file for %q: %w", dependabotPath, io.ErrShortWrite)
	}
	if err := runManagedHook(hooks.beforeTempChmod, dependabotPath); err != nil {
		return false, fmt.Errorf("setting temporary Dependabot file permissions for %q: %w", dependabotPath, err)
	}
	if err := temp.Chmod(mode); err != nil {
		return false, fmt.Errorf("setting temporary Dependabot file permissions for %q: %w", dependabotPath, err)
	}
	if err := runManagedHook(hooks.beforeTempSync, dependabotPath); err != nil {
		return false, fmt.Errorf("syncing temporary Dependabot file for %q: %w", dependabotPath, err)
	}
	if err := temp.Sync(); err != nil {
		return false, fmt.Errorf("syncing temporary Dependabot file for %q: %w", dependabotPath, err)
	}
	if err := runManagedHook(hooks.beforeTempClose, dependabotPath); err != nil {
		return false, fmt.Errorf("closing temporary Dependabot file for %q: %w", dependabotPath, err)
	}
	if err := temp.Close(); err != nil {
		tempOpen = false
		return false, fmt.Errorf("closing temporary Dependabot file for %q: %w", dependabotPath, err)
	}
	tempOpen = false

	if err := recheckDependabotSnapshot(root, plan.Snapshot, createdParentIdentity); err != nil {
		return false, err
	}
	if err := runManagedHook(hooks.beforeRename, dependabotPath); err != nil {
		return false, fmt.Errorf("publishing Dependabot destination %q: %w", dependabotPath, err)
	}

	if plan.Outcome == dependabotOutcomeCreate {
		if err := root.Link(tempPath, dependabotPath); err != nil {
			if errors.Is(err, fs.ErrExist) {
				return false, dependabotConflict("the destination appeared during protected creation")
			}
			return false, fmt.Errorf("creating protected Dependabot destination %q: %w", dependabotPath, err)
		}
		if err := runManagedHook(hooks.beforeRemove, dependabotPath); err != nil {
			return true, fmt.Errorf("removing temporary Dependabot file for %q: %w", dependabotPath, err)
		}
		if err := parentRoot.Remove(tempName); err != nil {
			return true, fmt.Errorf("removing temporary Dependabot file for %q: %w", dependabotPath, err)
		}
	} else {
		if err := root.Rename(tempPath, dependabotPath); err != nil {
			return false, fmt.Errorf("replacing Dependabot destination %q: %w", dependabotPath, err)
		}
	}
	tempName = ""

	if err := syncManagedDirectory(root, dependabotPath, hooks); err != nil {
		return true, err
	}
	return true, nil
}

func validateDependabotPublicationPlan(plan dependabotPlan) error {
	if len(plan.Output) > dependabotMaxBytes {
		return dependabotConflict("the planned output exceeds 1048576 bytes")
	}
	if !hasManagedMarker(plan.Output, dependabotPath) {
		return dependabotConflict("the planned output lacks the ownership marker")
	}
	switch plan.Outcome {
	case dependabotOutcomeCreate:
		if plan.Snapshot.Destination.Kind != dependabotDestinationMissing {
			return dependabotConflict("the create plan does not have a missing destination snapshot")
		}
	case dependabotOutcomeReplace:
		if plan.Snapshot.Destination.Kind != dependabotDestinationRegular || plan.Snapshot.Destination.Identity == nil {
			return dependabotConflict("the replace plan does not have a regular destination snapshot")
		}
	default:
		return dependabotConflict("the publication plan has no write outcome")
	}
	if plan.Snapshot.Root.Identity == nil {
		return dependabotConflict("the publication plan lacks a project root identity")
	}
	if plan.Snapshot.Parent.Exists && plan.Snapshot.Parent.Identity == nil {
		return dependabotConflict("the publication plan lacks a destination parent identity")
	}
	return nil
}

func recheckDependabotSnapshot(root *os.Root, snapshot dependabotSnapshot, createdParent os.FileInfo) error {
	rootInfo, err := root.Stat(".")
	if err != nil || !rootInfo.IsDir() || !os.SameFile(snapshot.Root.Identity, rootInfo) {
		return dependabotConflict("the project root changed after inspection")
	}
	if err := recheckDependabotParent(root, snapshot.Parent, createdParent); err != nil {
		return err
	}

	alternate, err := inspectDependabotEntry(root, dependabotAlternatePath, false, dependabotInspectionHooks{})
	if err != nil {
		return err
	}
	if !sameDependabotEntry(snapshot.Alternate, alternate, false) {
		return dependabotConflict("the alternate .github/dependabot.yaml entry changed after inspection")
	}

	destination, err := inspectDependabotEntry(root, dependabotPath, true, dependabotInspectionHooks{})
	if err != nil {
		return err
	}
	if snapshot.Destination.Kind != destination.Kind {
		return dependabotConflict("the Dependabot destination type changed after inspection")
	}
	if snapshot.Destination.Kind == dependabotDestinationRegular {
		if !os.SameFile(snapshot.Destination.Identity, destination.Identity) {
			return dependabotConflict("the Dependabot destination identity changed after inspection")
		}
		oldMarker := hasManagedMarker(snapshot.Destination.Content, dependabotPath)
		newMarker := hasManagedMarker(destination.Content, dependabotPath)
		if oldMarker != newMarker {
			return dependabotConflict("the Dependabot ownership marker changed after inspection")
		}
		if !bytes.Equal(snapshot.Destination.Content, destination.Content) {
			return dependabotConflict("the Dependabot destination bytes changed after inspection")
		}
	}
	return nil
}

func recheckDependabotParent(root *os.Root, snapshot dependabotIdentitySnapshot, created os.FileInfo) error {
	info, err := root.Lstat(snapshot.Path)
	if !snapshot.Exists && created == nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil
		}
		if err != nil {
			return dependabotConflict("the destination parent cannot be reinspected")
		}
		return dependabotConflict("the destination parent appeared after inspection")
	}
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return dependabotConflict("the destination parent disappeared after inspection")
		}
		return dependabotConflict("the destination parent cannot be reinspected")
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return dependabotConflict("the destination parent type changed after inspection")
	}
	expected := snapshot.Identity
	if created != nil {
		expected = created
	}
	if expected == nil || !os.SameFile(expected, info) {
		return dependabotConflict("the destination parent identity changed after inspection")
	}
	return nil
}

func sameDependabotEntry(want, got dependabotDestinationSnapshot, compareContent bool) bool {
	if want.Kind != got.Kind {
		return false
	}
	if want.Kind == dependabotDestinationMissing {
		return true
	}
	if want.Identity == nil || got.Identity == nil || !os.SameFile(want.Identity, got.Identity) {
		return false
	}
	return !compareContent || bytes.Equal(want.Content, got.Content)
}

func runDependabotHook(hook func(string) error, value string) error {
	if hook == nil {
		return nil
	}
	return hook(value)
}
