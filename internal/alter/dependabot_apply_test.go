package alter

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/wimpysworld/tailor/internal/config"
	"github.com/wimpysworld/tailor/internal/output"
	"github.com/wimpysworld/tailor/internal/swatch"
)

func TestPrepareDependabotExecutionSkipsBeforeOpeningRoot(t *testing.T) {
	for _, tt := range []struct {
		name string
		cfg  *config.Config
	}{
		{name: "omitted", cfg: &config.Config{}},
		{name: "never", cfg: dependabotTestConfig(swatch.Never, true, true)},
	} {
		t.Run(tt.name, func(t *testing.T) {
			opened := false
			plan, err := prepareDependabotExecutionWithHooks(tt.cfg, filepath.Join(t.TempDir(), "missing"), dependabotInspectionHooks{
				beforeRootOpen: func(string) error {
					opened = true
					return errors.New("root must not open")
				},
				readDestination: func(string, io.Reader, int64) ([]byte, error) {
					t.Fatal("destination must not be read")
					return nil, nil
				},
			})
			if err != nil || plan.Outcome != dependabotOutcomeSkip || opened {
				t.Fatalf("plan = %#v, opened = %v, error = %v", plan, opened, err)
			}
		})
	}
}

func TestPrepareDependabotExecutionUsesBoundedRootedRead(t *testing.T) {
	for _, tt := range []struct {
		name    string
		size    int
		wantErr bool
	}{
		{name: "at limit", size: dependabotMaxBytes},
		{name: "over limit", size: dependabotMaxBytes + 1, wantErr: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			writeDependabotTestFile(t, dir, dependabotPath, bytes.Repeat([]byte{'x'}, tt.size))
			var gotPath string
			var gotLimit int64
			plan, err := prepareDependabotExecutionWithHooks(dependabotTestConfig(swatch.FirstFit, true, true), dir, dependabotInspectionHooks{
				readDestination: func(name string, reader io.Reader, limit int64) ([]byte, error) {
					gotPath, gotLimit = name, limit
					return io.ReadAll(reader)
				},
			})
			if gotPath != dependabotPath || gotLimit != dependabotMaxBytes+1 {
				t.Fatalf("rooted read = %q with limit %d", gotPath, gotLimit)
			}
			if tt.wantErr {
				assertDependabotConflict(t, err)
			} else if err != nil || plan.Outcome != dependabotOutcomePreserve {
				t.Fatalf("plan = %#v, error = %v", plan, err)
			}
		})
	}
}

func TestPrepareDependabotExecutionReportsInjectedReadFailure(t *testing.T) {
	dir := t.TempDir()
	writeDependabotTestFile(t, dir, dependabotPath, []byte("unmarked\n"))
	injected := errors.New("injected read failure")
	_, err := prepareDependabotExecutionWithHooks(dependabotTestConfig(swatch.Always, true, true), dir, dependabotInspectionHooks{
		readDestination: func(name string, _ io.Reader, limit int64) ([]byte, error) {
			if name != dependabotPath || limit != dependabotMaxBytes+1 {
				t.Fatalf("read hook = %q, %d", name, limit)
			}
			return nil, injected
		},
	})
	assertDependabotConflict(t, err)
	if strings.Contains(err.Error(), "unmarked") {
		t.Fatal("read error disclosed destination bytes")
	}
}

func TestPrepareDependabotExecutionChecksOpenedIdentityBeforeReading(t *testing.T) {
	dir := t.TempDir()
	writeDependabotTestFile(t, dir, dependabotPath, []byte("original\n"))
	read := false
	_, err := prepareDependabotExecutionWithHooks(dependabotTestConfig(swatch.Always, true, true), dir, dependabotInspectionHooks{
		beforeDestinationOpen: func(string) error {
			full := filepath.Join(dir, filepath.FromSlash(dependabotPath))
			if err := os.Remove(full); err != nil {
				t.Fatal(err)
			}
			writeDependabotTestFile(t, dir, dependabotPath, []byte("replacement\n"))
			return nil
		},
		readDestination: func(string, io.Reader, int64) ([]byte, error) {
			read = true
			return nil, nil
		},
	})
	assertDependabotConflict(t, err)
	if read {
		t.Fatal("changed opened file was read")
	}
}

func TestPrepareDependabotExecutionRejectsFIFOReplacementWithoutBlocking(t *testing.T) {
	dir := t.TempDir()
	writeDependabotTestFile(t, dir, dependabotPath, []byte("original\n"))
	replacement := filepath.Join(dir, "replacement.fifo")
	managedFIFOOrSkip(t, replacement)

	result := make(chan error, 1)
	go func() {
		_, err := prepareDependabotExecutionWithHooks(dependabotTestConfig(swatch.Always, true, true), dir, dependabotInspectionHooks{
			beforeDestinationOpen: func(string) error {
				destination := filepath.Join(dir, filepath.FromSlash(dependabotPath))
				if err := os.Remove(destination); err != nil {
					return err
				}
				return os.Rename(replacement, destination)
			},
		})
		result <- err
	}()

	select {
	case err := <-result:
		assertDependabotConflict(t, err)
	case <-time.After(2 * time.Second):
		t.Fatal("Dependabot inspection blocked while opening a replacement FIFO")
	}
	info, err := os.Lstat(filepath.Join(dir, filepath.FromSlash(dependabotPath)))
	if err != nil || info.Mode()&os.ModeNamedPipe == 0 {
		t.Fatalf("replacement type = %v, error = %v", info, err)
	}
}

func TestPrepareDependabotExecutionChecksAlternateBeforeDestinationContent(t *testing.T) {
	for _, tt := range []struct {
		name        string
		kind        string
		destination bool
	}{
		{name: "alternate only", kind: "file"},
		{name: "both names", kind: "file", destination: true},
		{name: "directory", kind: "directory", destination: true},
		{name: "special", kind: "special", destination: true},
		{name: "symlink", kind: "symlink", destination: true},
		{name: "dangling symlink", kind: "dangling symlink", destination: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			if tt.destination {
				writeDependabotTestFile(t, dir, dependabotPath, []byte("destination must not be read"))
			} else if err := os.MkdirAll(filepath.Join(dir, ".github"), 0o755); err != nil {
				t.Fatal(err)
			}
			alternate := filepath.Join(dir, filepath.FromSlash(dependabotAlternatePath))
			switch tt.kind {
			case "file":
				writeDependabotTestFile(t, dir, dependabotAlternatePath, []byte("alternate"))
			case "directory":
				if err := os.MkdirAll(alternate, 0o755); err != nil {
					t.Fatal(err)
				}
			case "special":
				managedFIFOOrSkip(t, alternate)
			case "symlink":
				if err := os.Symlink(filepath.Join(dir, filepath.FromSlash(dependabotPath)), alternate); err != nil {
					t.Skipf("symlink unavailable: %v", err)
				}
			case "dangling symlink":
				if err := os.Symlink(filepath.Join(dir, "missing"), alternate); err != nil {
					t.Skipf("symlink unavailable: %v", err)
				}
			}
			_, err := prepareDependabotExecutionWithHooks(dependabotTestConfig(swatch.FirstFit, false, false), dir, dependabotInspectionHooks{
				readDestination: func(string, io.Reader, int64) ([]byte, error) {
					t.Fatal("destination content read before alternate conflict")
					return nil, nil
				},
			})
			assertDependabotConflict(t, err)
		})
	}
}

func TestPrepareDependabotExecutionRejectsUnsafeDestinationAndParentTypes(t *testing.T) {
	for _, tt := range []struct {
		name  string
		setup func(*testing.T, string)
	}{
		{name: "destination symlink", setup: func(t *testing.T, dir string) {
			if err := os.MkdirAll(filepath.Join(dir, ".github"), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.Symlink(filepath.Join(dir, "missing"), filepath.Join(dir, filepath.FromSlash(dependabotPath))); err != nil {
				t.Skipf("symlink unavailable: %v", err)
			}
		}},
		{name: "destination directory", setup: func(t *testing.T, dir string) {
			if err := os.MkdirAll(filepath.Join(dir, filepath.FromSlash(dependabotPath)), 0o755); err != nil {
				t.Fatal(err)
			}
		}},
		{name: "destination special", setup: func(t *testing.T, dir string) {
			if err := os.MkdirAll(filepath.Join(dir, ".github"), 0o755); err != nil {
				t.Fatal(err)
			}
			managedFIFOOrSkip(t, filepath.Join(dir, filepath.FromSlash(dependabotPath)))
		}},
		{name: "linked parent", setup: func(t *testing.T, dir string) {
			if err := os.Symlink(t.TempDir(), filepath.Join(dir, ".github")); err != nil {
				t.Skipf("symlink unavailable: %v", err)
			}
		}},
		{name: "non-directory parent", setup: func(t *testing.T, dir string) {
			if err := os.WriteFile(filepath.Join(dir, ".github"), []byte("file"), 0o644); err != nil {
				t.Fatal(err)
			}
		}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			tt.setup(t, dir)
			_, err := prepareDependabotExecution(dependabotTestConfig(swatch.FirstFit, true, true), dir)
			assertDependabotConflict(t, err)
		})
	}
}

func TestApplyDependabotPlanCreatesAndReplacesAtomically(t *testing.T) {
	t.Run("create missing parent", func(t *testing.T) {
		dir := t.TempDir()
		plan := mustPrepareDependabotPlan(t, dir, dependabotTestConfig(swatch.FirstFit, true, false))
		published, err := applyDependabotPlan(dir, plan)
		if err != nil || !published {
			t.Fatalf("published = %v, error = %v", published, err)
		}
		assertDependabotFile(t, dir, plan.Output, 0o644)
		assertNoDependabotTemps(t, dir)
	})

	t.Run("replace preserves permissions", func(t *testing.T) {
		dir := t.TempDir()
		bodies := mustDependabotBodies(t)
		writeDependabotTestFile(t, dir, dependabotPath, ownedDependabot("\n", bodies.Enabled))
		if err := os.Chmod(filepath.Join(dir, filepath.FromSlash(dependabotPath)), 0o640); err != nil {
			t.Fatal(err)
		}
		plan := mustPrepareDependabotPlan(t, dir, dependabotTestConfig(swatch.Always, true, false))
		published, err := applyDependabotPlan(dir, plan)
		if err != nil || !published {
			t.Fatalf("published = %v, error = %v", published, err)
		}
		assertDependabotFile(t, dir, plan.Output, 0o640)
		assertNoDependabotTemps(t, dir)
	})
}

func TestApplyDependabotPlanDoesNotInspectNonWriteOutcomes(t *testing.T) {
	for _, outcome := range []dependabotOutcome{dependabotOutcomeSkip, dependabotOutcomePreserve, dependabotOutcomeUnchanged} {
		published, err := applyDependabotPlan(filepath.Join(t.TempDir(), "missing"), dependabotPlan{Outcome: outcome})
		if err != nil || published {
			t.Fatalf("outcome %d: published = %v, error = %v", outcome, published, err)
		}
	}
}

func TestTailorCustomDependabotFileIsPreservedInEveryActiveMode(t *testing.T) {
	fixture, err := os.ReadFile(filepath.Join("..", "..", ".github", "dependabot.yml"))
	if err != nil {
		t.Fatal(err)
	}
	if hasManagedMarker(fixture, dependabotPath) {
		t.Fatal("Tailor's custom Dependabot fixture must remain unmarked")
	}
	for _, mode := range []swatch.AlterationMode{swatch.FirstFit, swatch.Always} {
		for _, applyMode := range []ApplyMode{Apply, Recut} {
			t.Run(string(mode)+"/"+fmt.Sprintf("%d", applyMode), func(t *testing.T) {
				dir := t.TempDir()
				writeDependabotTestFile(t, dir, dependabotPath, fixture)
				inspected := mustPrepareDependabotPlan(t, dir, dependabotTestConfig(mode, true, true))
				plan, err := planDependabot(
					dependabotSelection{Present: true, Mode: mode, Recut: applyMode == Recut, GoDeclared: true, GoEnabled: true},
					inspected.Snapshot,
					mustDependabotBodies(t),
				)
				if err != nil || plan.Outcome != dependabotOutcomePreserve {
					t.Fatalf("plan = %#v, error = %v, want preserve", plan, err)
				}
				published, err := applyDependabotPlan(dir, plan)
				if err != nil || published {
					t.Fatalf("published = %v, error = %v", published, err)
				}
				assertDependabotFile(t, dir, fixture, 0o644)
			})
		}
	}
}

func TestApplyDependabotPlanRejectsExactSnapshotDrift(t *testing.T) {
	bodies := mustDependabotBodies(t)
	for _, tt := range []struct {
		name   string
		setup  func(*testing.T, string)
		mutate func(*testing.T, string)
	}{
		{name: "bytes", setup: func(t *testing.T, dir string) {
			writeDependabotTestFile(t, dir, dependabotPath, ownedDependabot("\n", bodies.Enabled))
		}, mutate: func(t *testing.T, dir string) {
			writeDependabotTestFile(t, dir, dependabotPath, ownedDependabot("\n", append(bytes.Clone(bodies.Enabled), '\n')))
		}},
		{name: "marker", setup: func(t *testing.T, dir string) {
			writeDependabotTestFile(t, dir, dependabotPath, ownedDependabot("\n", bodies.Enabled))
		}, mutate: func(t *testing.T, dir string) { writeDependabotTestFile(t, dir, dependabotPath, bodies.Enabled) }},
		{name: "identity", setup: func(t *testing.T, dir string) {
			writeDependabotTestFile(t, dir, dependabotPath, ownedDependabot("\n", bodies.Enabled))
		}, mutate: func(t *testing.T, dir string) {
			full := filepath.Join(dir, filepath.FromSlash(dependabotPath))
			content, err := os.ReadFile(full)
			if err != nil {
				t.Fatal(err)
			}
			if err := os.Remove(full); err != nil {
				t.Fatal(err)
			}
			writeDependabotTestFile(t, dir, dependabotPath, content)
		}},
		{name: "present destination disappears", setup: func(t *testing.T, dir string) {
			writeDependabotTestFile(t, dir, dependabotPath, ownedDependabot("\n", bodies.Enabled))
		}, mutate: func(t *testing.T, dir string) {
			if err := os.Remove(filepath.Join(dir, filepath.FromSlash(dependabotPath))); err != nil {
				t.Fatal(err)
			}
		}},
		{name: "type", setup: func(t *testing.T, dir string) {
			writeDependabotTestFile(t, dir, dependabotPath, ownedDependabot("\n", bodies.Enabled))
		}, mutate: func(t *testing.T, dir string) {
			if err := os.Remove(filepath.Join(dir, filepath.FromSlash(dependabotPath))); err != nil {
				t.Fatal(err)
			}
			if err := os.Mkdir(filepath.Join(dir, filepath.FromSlash(dependabotPath)), 0o755); err != nil {
				t.Fatal(err)
			}
		}},
		{name: "alternate", setup: func(*testing.T, string) {}, mutate: func(t *testing.T, dir string) {
			writeDependabotTestFile(t, dir, dependabotAlternatePath, []byte("race"))
		}},
		{name: "missing destination appears", setup: func(*testing.T, string) {}, mutate: func(t *testing.T, dir string) { writeDependabotTestFile(t, dir, dependabotPath, []byte("user")) }},
		{name: "parent identity", setup: func(t *testing.T, dir string) {
			if err := os.Mkdir(filepath.Join(dir, ".github"), 0o755); err != nil {
				t.Fatal(err)
			}
		}, mutate: func(t *testing.T, dir string) {
			if err := os.Rename(filepath.Join(dir, ".github"), filepath.Join(dir, ".github-old")); err != nil {
				t.Fatal(err)
			}
			if err := os.Mkdir(filepath.Join(dir, ".github"), 0o755); err != nil {
				t.Fatal(err)
			}
		}},
		{name: "root identity", setup: func(*testing.T, string) {}, mutate: func(t *testing.T, dir string) {
			old := dir + "-old"
			if err := os.Rename(dir, old); err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = os.RemoveAll(old) })
			if err := os.Mkdir(dir, 0o755); err != nil {
				t.Fatal(err)
			}
		}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			tt.setup(t, dir)
			plan := mustPrepareDependabotPlan(t, dir, dependabotTestConfig(swatch.FirstFit, true, false))
			tt.mutate(t, dir)
			published, err := applyDependabotPlan(dir, plan)
			if published {
				t.Fatal("drift was published")
			}
			assertDependabotConflict(t, err)
			assertNoDependabotTemps(t, dir)
		})
	}
}

func TestApplyDependabotPlanRejectsNoClobberRace(t *testing.T) {
	dir := t.TempDir()
	plan := mustPrepareDependabotPlan(t, dir, dependabotTestConfig(swatch.FirstFit, true, true))
	userContent := []byte("user race\n")
	published, err := applyDependabotPlanWithHooks(dir, plan, managedApplyHooks{
		beforeRename: func(string) error {
			writeDependabotTestFile(t, dir, dependabotPath, userContent)
			return nil
		},
	})
	if published {
		t.Fatal("no-clobber race was published")
	}
	assertDependabotConflict(t, err)
	content, readErr := os.ReadFile(filepath.Join(dir, filepath.FromSlash(dependabotPath)))
	if readErr != nil || !bytes.Equal(content, userContent) {
		t.Fatalf("race file = %q, error = %v", content, readErr)
	}
	assertNoDependabotTemps(t, dir)
}

func TestApplyDependabotPlanCleansTempsAfterPublicationHookFailures(t *testing.T) {
	injected := errors.New("injected publication failure")
	for _, tt := range []struct {
		name string
		set  func(*managedApplyHooks)
	}{
		{name: "create", set: func(h *managedApplyHooks) { h.beforeTempCreate = func(string) error { return injected } }},
		{name: "write", set: func(h *managedApplyHooks) { h.beforeTempWrite = func(string) error { return injected } }},
		{name: "chmod", set: func(h *managedApplyHooks) { h.beforeTempChmod = func(string) error { return injected } }},
		{name: "sync", set: func(h *managedApplyHooks) { h.beforeTempSync = func(string) error { return injected } }},
		{name: "close", set: func(h *managedApplyHooks) { h.beforeTempClose = func(string) error { return injected } }},
		{name: "publish", set: func(h *managedApplyHooks) { h.beforeRename = func(string) error { return injected } }},
	} {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			plan := mustPrepareDependabotPlan(t, dir, dependabotTestConfig(swatch.FirstFit, true, true))
			var hooks managedApplyHooks
			tt.set(&hooks)
			published, err := applyDependabotPlanWithHooks(dir, plan, hooks)
			if published || !errors.Is(err, injected) {
				t.Fatalf("published = %v, error = %v", published, err)
			}
			if _, statErr := os.Lstat(filepath.Join(dir, filepath.FromSlash(dependabotPath))); !errors.Is(statErr, os.ErrNotExist) {
				t.Fatalf("destination exists after failure: %v", statErr)
			}
			assertNoDependabotTemps(t, dir)
		})
	}
}

func TestApplyDependabotPlanCleansTempFromRenamedParent(t *testing.T) {
	dir := t.TempDir()
	parent := filepath.Join(dir, ".github")
	if err := os.Mkdir(parent, 0o755); err != nil {
		t.Fatal(err)
	}
	plan := mustPrepareDependabotPlan(t, dir, dependabotTestConfig(swatch.FirstFit, true, true))
	renamedParent := filepath.Join(dir, ".github-renamed")
	keepPath := filepath.Join(parent, "keep")
	keepContent := []byte("user content\n")

	published, err := applyDependabotPlanWithHooks(dir, plan, managedApplyHooks{
		beforeTempWrite: func(string) error {
			if err := os.Rename(parent, renamedParent); err != nil {
				return err
			}
			if err := os.Mkdir(parent, 0o755); err != nil {
				return err
			}
			return os.WriteFile(keepPath, keepContent, 0o644)
		},
	})
	if published {
		t.Fatal("renamed parent was published")
	}
	assertDependabotConflict(t, err)

	oldEntries, err := os.ReadDir(renamedParent)
	if err != nil || len(oldEntries) != 0 {
		t.Fatalf("renamed parent entries = %v, error = %v", oldEntries, err)
	}
	newEntries, err := os.ReadDir(parent)
	if err != nil || len(newEntries) != 1 || newEntries[0].Name() != "keep" {
		t.Fatalf("new parent entries = %v, error = %v", newEntries, err)
	}
	content, err := os.ReadFile(keepPath)
	if err != nil || !bytes.Equal(content, keepContent) {
		t.Fatalf("new parent content = %q, error = %v", content, err)
	}
}

func TestApplyDependabotPlanCleansTempThroughTrackedParentHandle(t *testing.T) {
	dir := t.TempDir()
	plan := mustPrepareDependabotPlan(t, dir, dependabotTestConfig(swatch.FirstFit, true, true))
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

	parent := filepath.Join(dir, ".github")
	renamedParent := filepath.Join(dir, ".github-created")
	keepPath := filepath.Join(parent, "keep")
	keepContent := []byte("user content\n")
	published, err := applyDependabotPlanWithParentAndHooks(dir, plan, creation.confirmedParent(), managedApplyHooks{
		beforeTempWrite: func(string) error {
			if err := os.Rename(parent, renamedParent); err != nil {
				return err
			}
			if err := os.Mkdir(parent, 0o755); err != nil {
				return err
			}
			return os.WriteFile(keepPath, keepContent, 0o644)
		},
	})
	if published {
		t.Fatal("replaced tracked parent was published")
	}
	assertDependabotConflict(t, err)
	if matches, err := filepath.Glob(filepath.Join(renamedParent, "dependabot.yml.tmp-*")); err != nil || len(matches) != 0 {
		t.Fatalf("tracked parent temporary files = %v, error = %v", matches, err)
	}
	content, err := os.ReadFile(keepPath)
	if err != nil || !bytes.Equal(content, keepContent) {
		t.Fatalf("replacement parent content = %q, error = %v", content, err)
	}
}

func TestApplyDependabotPlanRechecksAfterTemporaryWrite(t *testing.T) {
	dir := t.TempDir()
	plan := mustPrepareDependabotPlan(t, dir, dependabotTestConfig(swatch.FirstFit, true, true))
	published, err := applyDependabotPlanWithHooks(dir, plan, managedApplyHooks{
		beforeTempClose: func(string) error {
			writeDependabotTestFile(t, dir, dependabotAlternatePath, []byte("late alternate"))
			return nil
		},
	})
	if published {
		t.Fatal("late alternate was published")
	}
	assertDependabotConflict(t, err)
	assertNoDependabotTemps(t, dir)
}

func TestExecuteReportsPublishedDependabotOnCleanupAndSyncFailures(t *testing.T) {
	for _, tt := range []struct {
		name string
		set  func(*managedApplyHooks, error)
	}{
		{name: "temporary cleanup", set: func(h *managedApplyHooks, err error) { h.beforeRemove = func(string) error { return err } }},
		{name: "directory sync", set: func(h *managedApplyHooks, err error) { h.beforeDirectorySync = func(string) error { return err } }},
	} {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			cfg := dependabotTestConfig(swatch.FirstFit, true, false)
			cfg.License = "none"
			injected := errors.New("injected post-publication failure")
			var applyHooks managedApplyHooks
			tt.set(&applyHooks, injected)

			report, err := executeWithHooks(cfg, dir, Apply, managedProductionClient(t), io.Discard, Options{}, managedTestRenderer, executionHooks{dependabotApply: applyHooks})
			if !errors.Is(err, injected) {
				t.Fatalf("execution error = %v, want injected failure", err)
			}
			assertDependabotFile(t, dir, mustPrepareDependabotPlan(t, dir, cfg).Output, 0o644)
			structured := 0
			for _, item := range report.Document.Items {
				if item.Name == dependabotPath {
					structured++
					if item.Outcome != output.Applied || item.Action != "copy" {
						t.Errorf("Dependabot result = %#v", item)
					}
				}
			}
			if structured != 1 || strings.Count(report.Plain, dependabotPath) != 1 {
				t.Fatalf("Dependabot result counts: structured=%d, plain=%d\n%s", structured, strings.Count(report.Plain, dependabotPath), report.Plain)
			}
		})
	}
}

func TestExecuteJoinsDependabotParentCloseFailureWithPartialReport(t *testing.T) {
	for _, tt := range []struct {
		name      string
		priorFail bool
	}{
		{name: "close failure"},
		{name: "prior and close failures", priorFail: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			cfg := dependabotTestConfig(swatch.FirstFit, true, false)
			cfg.License = "none"
			priorErr := errors.New("injected post-publication failure")
			closeErr := errors.New("injected parent close failure")
			hooks := executionHooks{
				dependabotParentClose: func(creation *dependabotParentCreation) error {
					return errors.Join(creation.close(), closeErr)
				},
			}
			if tt.priorFail {
				hooks.dependabotApply.beforeDirectorySync = func(string) error { return priorErr }
			}

			report, err := executeWithHooks(cfg, dir, Apply, managedProductionClient(t), io.Discard, Options{}, managedTestRenderer, hooks)
			if !errors.Is(err, closeErr) {
				t.Fatalf("execution error = %v, want close failure", err)
			}
			if errors.Is(err, priorErr) != tt.priorFail {
				t.Fatalf("prior error retained = %v, error = %v", errors.Is(err, priorErr), err)
			}
			assertDependabotFile(t, dir, mustPrepareDependabotPlan(t, dir, cfg).Output, 0o644)
			assertOneDependabotItemInInternalReport(t, report, output.Applied)
		})
	}
}

func assertOneDependabotItemInInternalReport(t *testing.T, report Report, want output.Outcome) {
	t.Helper()
	count := 0
	for _, item := range report.Document.Items {
		if item.Name != dependabotPath {
			continue
		}
		count++
		if item.Outcome != want {
			t.Errorf("Dependabot outcome = %q, want %q", item.Outcome, want)
		}
	}
	if count != 1 || strings.Count(report.Plain, dependabotPath) != 1 {
		t.Fatalf("Dependabot result counts: structured=%d, plain=%d\n%s", count, strings.Count(report.Plain, dependabotPath), report.Plain)
	}
}

func TestApplyDependabotPlanPreservesPublishedResultOnCleanupAndSyncFailures(t *testing.T) {
	for _, tt := range []struct {
		name string
		set  func(*managedApplyHooks, error)
	}{
		{name: "temporary cleanup", set: func(h *managedApplyHooks, err error) { h.beforeRemove = func(string) error { return err } }},
		{name: "directory sync", set: func(h *managedApplyHooks, err error) { h.beforeDirectorySync = func(string) error { return err } }},
	} {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			plan := mustPrepareDependabotPlan(t, dir, dependabotTestConfig(swatch.FirstFit, true, true))
			injected := errors.New("injected post-publication failure")
			var hooks managedApplyHooks
			tt.set(&hooks, injected)
			published, err := applyDependabotPlanWithHooks(dir, plan, hooks)
			if !published || !errors.Is(err, injected) {
				t.Fatalf("published = %v, error = %v", published, err)
			}
			assertDependabotFile(t, dir, plan.Output, 0o644)
			assertNoDependabotTemps(t, dir)
		})
	}
}

func TestApplyDependabotPlanJoinsCreatedParentCloseFailure(t *testing.T) {
	for _, tt := range []struct {
		name      string
		priorFail bool
	}{
		{name: "close failure"},
		{name: "prior and close failures", priorFail: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			plan := mustPrepareDependabotPlan(t, dir, dependabotTestConfig(swatch.FirstFit, true, true))
			priorErr := errors.New("injected post-publication failure")
			closeErr := errors.New("injected parent close failure")
			var hooks managedApplyHooks
			if tt.priorFail {
				hooks.beforeDirectorySync = func(string) error { return priorErr }
			}
			parentClose := func(creation *dependabotParentCreation) error {
				return errors.Join(creation.close(), closeErr)
			}

			published, err := applyDependabotPlanWithParentCloseHook(dir, plan, nil, hooks, parentClose)
			if !published || !errors.Is(err, closeErr) {
				t.Fatalf("published = %v, error = %v", published, err)
			}
			if errors.Is(err, priorErr) != tt.priorFail {
				t.Fatalf("prior error retained = %v, error = %v", errors.Is(err, priorErr), err)
			}
			assertDependabotFile(t, dir, plan.Output, 0o644)
			assertNoDependabotTemps(t, dir)
		})
	}
}

func dependabotTestConfig(mode swatch.AlterationMode, declared, enabled bool) *config.Config {
	cfg := &config.Config{Swatches: []config.SwatchEntry{{Path: dependabotPath, Alteration: mode}}}
	if declared {
		cfg.Languages = &config.LanguageSettings{Go: &enabled}
	}
	return cfg
}

func mustPrepareDependabotPlan(t *testing.T, dir string, cfg *config.Config) dependabotPlan {
	t.Helper()
	plan, err := prepareDependabotExecution(cfg, dir)
	if err != nil {
		t.Fatal(err)
	}
	return plan
}

func writeDependabotTestFile(t *testing.T, dir, name string, content []byte) {
	t.Helper()
	full := filepath.Join(dir, filepath.FromSlash(name))
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		t.Fatal(err)
	}
	// #nosec G703 -- Test paths are controlled and stay in the temporary fixture.
	if err := os.WriteFile(full, content, 0o644); err != nil {
		t.Fatal(err)
	}
}

func assertDependabotFile(t *testing.T, dir string, content []byte, mode os.FileMode) {
	t.Helper()
	full := filepath.Join(dir, filepath.FromSlash(dependabotPath))
	got, err := os.ReadFile(full)
	if err != nil || !bytes.Equal(got, content) {
		t.Fatalf("destination = %q, error = %v", got, err)
	}
	info, err := os.Stat(full)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != mode {
		t.Fatalf("destination mode = %v, want %v", info.Mode().Perm(), mode)
	}
}

func assertNoDependabotTemps(t *testing.T, dir string) {
	t.Helper()
	matches, err := filepath.Glob(filepath.Join(dir, ".github", "dependabot.yml.tmp-*"))
	if err != nil || !reflect.DeepEqual(matches, []string(nil)) {
		t.Fatalf("temporary files = %v, error = %v", matches, err)
	}
}
