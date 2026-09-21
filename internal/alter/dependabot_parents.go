package alter

import (
	"crypto/rand"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

type parentCreationObserver struct {
	beforeCreate func(root *os.Root, path string, perm os.FileMode) (bool, error)
	afterInspect func(path string, identity os.FileInfo, created bool) error
}

type dependabotCreatedParent struct {
	identity os.FileInfo
	root     *os.Root
}

type dependabotParentCreationHooks struct {
	beforePublish func(root *os.Root, temporary, destination string) error
}

type dependabotParentCreation struct {
	path         string
	rootIdentity os.FileInfo
	parent       *dependabotCreatedParent
	hooks        dependabotParentCreationHooks
}

func newDependabotParentCreation(plan dependabotPlan) *dependabotParentCreation {
	return newDependabotParentCreationWithHooks(plan, dependabotParentCreationHooks{})
}

func newDependabotParentCreationWithHooks(plan dependabotPlan, hooks dependabotParentCreationHooks) *dependabotParentCreation {
	if plan.Outcome != dependabotOutcomeCreate || plan.Snapshot.Parent.Exists {
		return nil
	}
	return &dependabotParentCreation{
		path:         filepath.Clean(plan.Snapshot.Parent.Path),
		rootIdentity: plan.Snapshot.Root.Identity,
		hooks:        hooks,
	}
}

func (creation *dependabotParentCreation) observer() *parentCreationObserver {
	if creation == nil {
		return nil
	}
	return &parentCreationObserver{
		beforeCreate: creation.publish,
		afterInspect: creation.observe,
	}
}

//nolint:gocyclo // The private creation and exclusive publication checks must remain one sequence.
func (creation *dependabotParentCreation) publish(root *os.Root, name string, perm os.FileMode) (_ bool, retErr error) {
	if creation == nil || filepath.Clean(name) != creation.path {
		return false, nil
	}
	if creation.parent != nil {
		current, err := root.Lstat(creation.path)
		if errors.Is(err, fs.ErrNotExist) {
			return false, dependabotConflict("the tracked destination parent disappeared after creation")
		}
		if err != nil {
			return false, dependabotConflict("the tracked destination parent cannot be inspected after creation")
		}
		if err := creation.observe(creation.path, current, true); err != nil {
			return false, err
		}
		return true, nil
	}

	rootInfo, err := root.Stat(".")
	if err != nil || creation.rootIdentity == nil || !rootInfo.IsDir() || !os.SameFile(creation.rootIdentity, rootInfo) {
		return false, dependabotConflict("the project root changed before destination parent creation")
	}
	if _, err := root.Lstat(creation.path); err == nil {
		return false, dependabotConflict("the destination parent appeared before its tracked creation")
	} else if !errors.Is(err, fs.ErrNotExist) {
		return false, dependabotConflict("the destination parent cannot be inspected before creation")
	}

	temporary := ".tailor-dependabot-parent-" + rand.Text()
	if err := root.Mkdir(temporary, perm); err != nil {
		return false, fmt.Errorf("creating private Dependabot parent %q: %w", temporary, err)
	}
	temporaryInfo, err := root.Lstat(temporary)
	if err != nil || temporaryInfo.Mode()&os.ModeSymlink != 0 || !temporaryInfo.IsDir() {
		_ = root.Remove(temporary)
		return false, dependabotConflict("the private destination parent cannot be verified")
	}
	temporaryRoot, err := root.OpenRoot(temporary)
	if err != nil {
		_ = root.Remove(temporary)
		return false, fmt.Errorf("opening private Dependabot parent %q: %w", temporary, err)
	}
	published := false
	defer func() {
		if published {
			return
		}
		retErr = errors.Join(retErr, temporaryRoot.Close())
		current, statErr := root.Lstat(temporary)
		if statErr == nil && os.SameFile(temporaryInfo, current) {
			retErr = errors.Join(retErr, root.Remove(temporary))
		} else if statErr != nil && !errors.Is(statErr, fs.ErrNotExist) {
			retErr = errors.Join(retErr, statErr)
		}
	}()

	handleInfo, err := temporaryRoot.Stat(".")
	if err != nil || !os.SameFile(temporaryInfo, handleInfo) {
		return false, dependabotConflict("the private destination parent changed while it was opened")
	}
	rootDirectory, err := os.Open(root.Name())
	if err != nil {
		return false, fmt.Errorf("opening project root for destination parent publication: %w", err)
	}
	defer func() { retErr = errors.Join(retErr, rootDirectory.Close()) }()
	rootDirectoryInfo, err := rootDirectory.Stat()
	if err != nil || !os.SameFile(rootInfo, rootDirectoryInfo) {
		return false, dependabotConflict("the project root changed before destination parent publication")
	}
	if creation.hooks.beforePublish != nil {
		if err := creation.hooks.beforePublish(root, temporary, creation.path); err != nil {
			return false, err
		}
	}
	current, err := root.Lstat(temporary)
	if err != nil || !os.SameFile(temporaryInfo, current) {
		return false, dependabotConflict("the private destination parent changed before publication")
	}
	if err := publishDirectoryNoReplace(rootDirectory, temporary, creation.path); err != nil {
		if _, destinationErr := root.Lstat(creation.path); destinationErr == nil {
			return false, dependabotConflict("the destination parent appeared during protected creation")
		}
		return false, fmt.Errorf("publishing protected Dependabot parent %q: %w", creation.path, err)
	}
	published = true
	creation.parent = &dependabotCreatedParent{identity: temporaryInfo, root: temporaryRoot}

	finalInfo, err := root.Lstat(creation.path)
	if err != nil || !os.SameFile(temporaryInfo, finalInfo) {
		return false, dependabotConflict("the published destination parent identity changed")
	}
	return true, nil
}

func (creation *dependabotParentCreation) observe(path string, identity os.FileInfo, created bool) error {
	if creation == nil || filepath.Clean(path) != creation.path {
		return nil
	}
	if identity == nil || identity.Mode()&os.ModeSymlink != 0 || !identity.IsDir() {
		return dependabotConflict("the destination parent type changed after inspection")
	}
	if creation.parent == nil {
		return dependabotConflict("the destination parent appeared before its tracked creation")
	}
	if !os.SameFile(creation.parent.identity, identity) {
		return dependabotConflict("the tracked destination parent identity changed after creation")
	}
	if !created {
		return dependabotConflict("the tracked destination parent was not published by this execution")
	}
	return nil
}

func (creation *dependabotParentCreation) confirmedParent() *dependabotCreatedParent {
	if creation == nil {
		return nil
	}
	return creation.parent
}

func dependabotParentIdentity(parent *dependabotCreatedParent) os.FileInfo {
	if parent == nil {
		return nil
	}
	return parent.identity
}

func (creation *dependabotParentCreation) close() error {
	if creation == nil || creation.parent == nil || creation.parent.root == nil {
		return nil
	}
	err := creation.parent.root.Close()
	creation.parent.root = nil
	return err
}

func closeDependabotParent(creation *dependabotParentCreation, hook func(*dependabotParentCreation) error) error {
	if hook != nil {
		return hook(creation)
	}
	return creation.close()
}

func rootedMkdirAll(root *os.Root, directory string, perm os.FileMode, observer *parentCreationObserver) error {
	if !filepath.IsLocal(directory) {
		return fmt.Errorf("directory %q is not local", directory)
	}
	directory = filepath.Clean(directory)
	if directory == "." {
		return nil
	}

	current := ""
	for component := range strings.SplitSeq(directory, string(filepath.Separator)) {
		if component == "" || component == "." {
			continue
		}
		current = filepath.Join(current, component)
		created := false
		if observer != nil && observer.beforeCreate != nil {
			var err error
			created, err = observer.beforeCreate(root, current, perm)
			if err != nil {
				return err
			}
		}
		if !created {
			if err := root.Mkdir(current, perm); err != nil {
				if !errors.Is(err, fs.ErrExist) {
					return err
				}
			} else {
				created = true
			}
		}
		identity, err := root.Lstat(current)
		if err != nil {
			return err
		}
		if observer != nil && observer.afterInspect != nil {
			if err := observer.afterInspect(current, identity, created); err != nil {
				return err
			}
		}
		if identity.Mode()&os.ModeSymlink != 0 || !identity.IsDir() {
			return fmt.Errorf("directory %q is not a directory", current)
		}
	}
	return nil
}
