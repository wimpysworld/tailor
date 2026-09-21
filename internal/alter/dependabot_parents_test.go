package alter

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/wimpysworld/tailor/internal/swatch"
)

func TestDependabotParentCreationCapturesIdentityBeforePublication(t *testing.T) {
	dir := t.TempDir()
	plan := mustPrepareDependabotPlan(t, dir, dependabotTestConfig(swatch.FirstFit, true, false))
	var privateIdentity os.FileInfo
	creation := newDependabotParentCreationWithHooks(plan, dependabotParentCreationHooks{
		beforePublish: func(root *os.Root, temporary, destination string) error {
			if _, err := root.Lstat(destination); !errors.Is(err, os.ErrNotExist) {
				t.Fatalf("destination exists before publication: %v", err)
			}
			var err error
			privateIdentity, err = root.Lstat(temporary)
			return err
		},
	})
	if creation == nil {
		t.Fatal("missing parent creation tracker")
	}
	defer creation.close()
	root, err := os.OpenRoot(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()

	if err := writeFileWithParentObserver(root, ".github/workflows/tailor-pages.yml", []byte("pages\n"), creation.observer()); err != nil {
		t.Fatal(err)
	}
	if err := writeFileWithParentObserver(root, ".github/workflows/tailor-wiki.yml", []byte("wiki\n"), creation.observer()); err != nil {
		t.Fatal(err)
	}
	parent := creation.confirmedParent()
	if privateIdentity == nil || parent == nil || parent.root == nil {
		t.Fatalf("private identity = %v, parent = %#v", privateIdentity, parent)
	}
	info, err := root.Lstat(plan.Snapshot.Parent.Path)
	if err != nil || !os.SameFile(privateIdentity, info) || !os.SameFile(parent.identity, info) {
		t.Fatalf("published identity changed: %v, %v", info, err)
	}
}

func TestDependabotParentCreationDoesNotReplaceConcurrentDestination(t *testing.T) {
	dir := t.TempDir()
	plan := mustPrepareDependabotPlan(t, dir, dependabotTestConfig(swatch.FirstFit, true, false))
	creation := newDependabotParentCreationWithHooks(plan, dependabotParentCreationHooks{
		beforePublish: func(root *os.Root, _, destination string) error {
			if err := root.Mkdir(destination, 0o755); err != nil {
				return err
			}
			return root.WriteFile(filepath.Join(destination, "user"), []byte("keep\n"), 0o644)
		},
	})
	defer creation.close()
	root, err := os.OpenRoot(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()

	err = writeFileWithParentObserver(root, ".github/workflows/tailor-pages.yml", []byte("pages\n"), creation.observer())
	assertDependabotConflict(t, err)
	if creation.confirmedParent() != nil {
		t.Fatal("concurrent parent identity was accepted")
	}
	content, err := root.ReadFile(".github/user")
	if err != nil || string(content) != "keep\n" {
		t.Fatalf("concurrent destination content = %q, error = %v", content, err)
	}
	if matches, err := filepath.Glob(filepath.Join(dir, ".tailor-dependabot-parent-*")); err != nil || len(matches) != 0 {
		t.Fatalf("private parent directories = %v, error = %v", matches, err)
	}
}

func TestApplyDependabotPlanRejectsReplacedTrackedParent(t *testing.T) {
	dir := t.TempDir()
	plan := mustPrepareDependabotPlan(t, dir, dependabotTestConfig(swatch.FirstFit, true, false))
	creation := newDependabotParentCreation(plan)
	defer creation.close()
	root, err := os.OpenRoot(dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := writeFileWithParentObserver(root, ".github/workflows/tailor-pages.yml", []byte("pages\n"), creation.observer()); err != nil {
		t.Fatal(err)
	}
	if err := root.Close(); err != nil {
		t.Fatal(err)
	}

	if err := os.Rename(filepath.Join(dir, ".github"), filepath.Join(dir, ".github-created")); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(dir, ".github"), 0o755); err != nil {
		t.Fatal(err)
	}
	published, err := applyDependabotPlanWithParentAndHooks(dir, plan, creation.confirmedParent(), managedApplyHooks{})
	if published {
		t.Fatal("Dependabot was published through a replaced tracked parent")
	}
	assertDependabotConflict(t, err)
	if _, err := os.Stat(filepath.Join(dir, filepath.FromSlash(dependabotPath))); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("Dependabot exists after tracked parent replacement: %v", err)
	}
}
