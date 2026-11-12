package alter

import (
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/wimpysworld/tailor/internal/gh"
	"github.com/wimpysworld/tailor/internal/swatch"
)

func wikiImportProject(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	wikiImportGit(t, dir, "init", "-b", "main")
	return dir
}

func wikiImportGit(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.CommandContext(t.Context(), "git", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GIT_CONFIG_NOSYSTEM=1", "GIT_CONFIG_GLOBAL="+os.DevNull, "GIT_AUTHOR_NAME=Test", "GIT_AUTHOR_EMAIL=test@example.com", "GIT_COMMITTER_NAME=Test", "GIT_COMMITTER_EMAIL=test@example.com")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v: %s", args, err, out)
	}
	return strings.TrimSpace(string(out))
}

func wikiImportSnapshot() *gh.WikiSnapshot {
	return &gh.WikiSnapshot{Head: strings.Repeat("a", 40), Readiness: &gh.WikiReadinessError{Reason: gh.WikiBaselineMissing, Err: errors.New("missing baseline")}, Files: []gh.WikiFile{
		{Path: "Home.md", Mode: 0o644, Content: []byte("remote home\n")},
		{Path: "nested/run.sh", Mode: 0o755, Content: []byte("#!/bin/sh\necho test\n")},
		{Path: "image.bin", Mode: 0o644, Content: []byte{0, 255, 1, 13, 10}},
	}}
}

func wikiImportHooks(t *testing.T, snapshot *gh.WikiSnapshot) {
	t.Helper()
	t.Cleanup(gh.SetInspectWikiFunc(func(string, string, string) error { return snapshot.Readiness }))
	t.Cleanup(gh.SetReadWikiSnapshotFunc(func(string, string, string) (*gh.WikiSnapshot, error) { return snapshot, nil }))
	t.Cleanup(gh.SetRecheckWikiHeadFunc(func(string, string) error { return nil }))
}

func TestWikiImportExactDataAndBackups(t *testing.T) {
	dir := wikiImportProject(t)
	snapshot := wikiImportSnapshot()
	wikiImportHooks(t, snapshot)
	wikiTestWrite(t, dir, "wiki/Home.md", "local home\n")
	if err := os.Chmod(filepath.Join(dir, "wiki/Home.md"), 0o640); err != nil {
		t.Fatal(err)
	}
	wikiTestWrite(t, dir, "wiki/Local.md", "local only\n")
	wikiTestWrite(t, dir, swatch.WikiBaseline, strings.Repeat("b", 40)+"\n")
	plan, err := planWikiImport(dir, "remote", snapshot)
	if err != nil {
		t.Fatal(err)
	}
	replacements, additions := 0, 0
	for _, f := range plan.Files {
		switch {
		case f.Before == f.After:
		case f.Before.Exists:
			replacements++
		default:
			additions++
		}
	}
	if replacements != 2 || additions != 2 {
		t.Fatalf("plan: %+v", plan.Files)
	}
	if err := applyWikiImport(dir, plan); err != nil {
		t.Fatal(err)
	}
	if !plan.Complete {
		t.Fatal("not complete")
	}
	for _, f := range snapshot.Files {
		data, err := os.ReadFile(filepath.Join(dir, "wiki", f.Path))
		info, statErr := os.Stat(filepath.Join(dir, "wiki", f.Path))
		if err != nil || statErr != nil || !bytes.Equal(data, f.Content) || effectiveWikiMode(info.Mode()) != effectiveWikiMode(f.Mode) {
			t.Fatalf("file %s: %q %v %v", f.Path, data, err, statErr)
		}
	}
	data, _ := os.ReadFile(filepath.Join(dir, "wiki/Local.md"))
	if string(data) != "local only\n" {
		t.Fatal("local-only file changed")
	}
	for i, f := range plan.Files {
		if f.Before.Exists {
			data, err := os.ReadFile(filepath.Join(plan.location, "backup", strconv.Itoa(i)))
			info, statErr := os.Stat(filepath.Join(plan.location, "backup", strconv.Itoa(i)))
			if err != nil || statErr != nil || !sameWikiFileState(wikiState(data, info.Mode()), f.Before) {
				t.Fatalf("backup: %q %v %v", data, err, statErr)
			}
		}
	}
	if pending, err := findWikiImport(dir, "remote"); err != nil || pending != nil {
		t.Fatalf("pending=%v err=%v", pending, err)
	}
	status := wikiImportGit(t, dir, "status", "--porcelain", "--untracked-files=all")
	if strings.Contains(status, "tailor/wiki-imports") || strings.Contains(status, "backup") {
		t.Fatalf("visible recovery data: %s", status)
	}
}

// An existing wiki imports without a prompt or flag. Recut changes nothing about the import.
func TestWikiImportAutomaticAndPreview(t *testing.T) {
	for _, mode := range []ApplyMode{DryRun, Apply, Recut} {
		t.Run(fmt.Sprint(mode), func(t *testing.T) {
			dir := wikiImportProject(t)
			snapshot := wikiImportSnapshot()
			wikiImportHooks(t, snapshot)
			target, _ := wikiTestTarget(t, 200, `{"private":false,"has_wiki":true,"default_branch":"main"}`)
			p, err := preflightWiki(wikiTestConfig(new(true)), dir, mode, target, true)
			if err != nil || p.importPlan == nil || p.importPlan.Head != snapshot.Head || len(p.importPlan.Files) != 4 {
				t.Fatalf("plan=%+v err=%v", p, err)
			}
			if mode == DryRun {
				if _, err := os.Stat(filepath.Join(dir, "wiki")); !errors.Is(err, fs.ErrNotExist) {
					t.Fatal("preview wrote wiki")
				}
				if _, err := os.Stat(filepath.Join(dir, ".git/tailor")); !errors.Is(err, fs.ErrNotExist) {
					t.Fatal("preview wrote state")
				}
				if !strings.Contains(p.nextSteps, "imports the wiki automatically") {
					t.Fatalf("preview guidance: %q", p.nextSteps)
				}
				report := Report{}
				appendWikiImportReporting(&report, p, DryRun)
				if len(report.Document.Items) != 4 || !strings.Contains(report.Plain, "would import") {
					t.Fatalf("preview: %+v", report)
				}
				return
			}
			if !p.importPlan.Complete {
				t.Fatal("automatic import did not complete")
			}
			for _, f := range snapshot.Files {
				data, err := os.ReadFile(filepath.Join(dir, "wiki", f.Path))
				if err != nil || !bytes.Equal(data, f.Content) {
					t.Fatalf("file %s: %q %v", f.Path, data, err)
				}
			}
			if _, err := os.Stat(filepath.Join(dir, swatch.WikiBaseline)); err != nil {
				t.Fatalf("baseline missing after import: %v", err)
			}
		})
	}
}

func TestWikiImportPreservesReadyDirtyFiles(t *testing.T) {
	dir := wikiImportProject(t)
	wikiTestWrite(t, dir, "wiki/Home.md", "dirty local")
	t.Cleanup(gh.SetInspectWikiFunc(func(string, string, string) error { return nil }))
	target, _ := wikiTestTarget(t, 200, `{"private":false,"has_wiki":true,"default_branch":"main"}`)
	p, err := preflightWiki(wikiTestConfig(new(true)), dir, Apply, target, true)
	if err != nil || p.importPlan != nil {
		t.Fatalf("ready import: %v", err)
	}
	data, _ := os.ReadFile(filepath.Join(dir, "wiki/Home.md"))
	if string(data) != "dirty local" {
		t.Fatal("dirty file changed")
	}
}

func TestWikiImportRemoteRaceAndResume(t *testing.T) {
	for _, stage := range []int{1, 2} {
		t.Run(fmt.Sprint(stage), func(t *testing.T) {
			dir := wikiImportProject(t)
			snapshot := wikiImportSnapshot()
			wikiImportHooks(t, snapshot)
			wikiTestWrite(t, dir, swatch.WikiBaseline, strings.Repeat("b", 40)+"\n")
			plan, err := planWikiImport(dir, "remote", snapshot)
			if err != nil {
				t.Fatal(err)
			}
			calls := 0
			restore := gh.SetRecheckWikiHeadFunc(func(string, string) error {
				calls++
				if calls == stage {
					return errors.New("remote changed")
				}
				return nil
			})
			err = applyWikiImport(dir, plan)
			restore()
			if err == nil {
				t.Fatal("race accepted")
			}
			data, _ := os.ReadFile(filepath.Join(dir, swatch.WikiBaseline))
			if string(data) != strings.Repeat("b", 40)+"\n" {
				t.Fatal("baseline changed before verification")
			}
			if stage == 1 {
				if plan.location != "" {
					t.Fatal("race before apply wrote state")
				}
				return
			}
			pending, err := findWikiImport(dir, "remote")
			if err != nil || pending == nil {
				t.Fatalf("pending=%v err=%v", pending, err)
			}
			if err := restoreWikiImport(dir, pending, snapshot); err != nil {
				t.Fatal(err)
			}
			if err := applyWikiImport(dir, pending); err != nil {
				t.Fatal(err)
			}
			data, _ = os.ReadFile(filepath.Join(dir, swatch.WikiBaseline))
			if string(data) != snapshot.Head+"\n" {
				t.Fatal("resume did not install baseline")
			}
		})
	}
}

func TestWikiImportInterruptedStateRejectsEditsAndTampering(t *testing.T) {
	for _, kind := range []string{"local bytes", "local mode", "parent symlink", "source bytes", "backup bytes", "journal path", "journal identity", "journal symlink", "remote"} {
		t.Run(kind, func(t *testing.T) {
			if runtime.GOOS == "windows" && kind == "local mode" {
				t.Skip("POSIX permission bits require unix")
			}
			dir := wikiImportProject(t)
			snapshot := wikiImportSnapshot()
			wikiImportHooks(t, snapshot)
			wikiTestWrite(t, dir, "wiki/Home.md", "original")
			plan, err := planWikiImport(dir, "remote", snapshot)
			if err != nil {
				t.Fatal(err)
			}
			calls := 0
			restore := gh.SetRecheckWikiHeadFunc(func(string, string) error {
				calls++
				if calls == 2 {
					return errors.New("interrupted")
				}
				return nil
			})
			if err := applyWikiImport(dir, plan); err == nil {
				t.Fatal("expected interruption")
			}
			restore()
			switch kind {
			case "local bytes":
				wikiTestWrite(t, dir, "wiki/Home.md", "later edit")
			case "local mode":
				if err := os.Chmod(filepath.Join(dir, "wiki/Home.md"), 0o600); err != nil {
					t.Fatal(err)
				}
			case "parent symlink":
				if err := os.Rename(filepath.Join(dir, "wiki/nested"), filepath.Join(dir, "kept")); err != nil {
					t.Fatal(err)
				}
				managedSymlinkOrSkip(t, filepath.Join(dir, "kept"), filepath.Join(dir, "wiki/nested"))
			case "source bytes":
				wikiTestWrite(t, plan.location, "source/0", "tampered")
			case "backup bytes":
				wikiTestWrite(t, plan.location, "backup/0", "tampered")
			case "journal path", "journal identity":
				root, err := os.OpenRoot(plan.location)
				if err != nil {
					t.Fatal(err)
				}
				defer root.Close()
				if kind == "journal path" {
					plan.Files[0].Path = "wiki/../../outside"
				} else {
					plan.Project = "/another/project"
				}
				if err := saveWikiJournal(root, plan); err != nil {
					t.Fatal(err)
				}
			case "journal symlink":
				filename := filepath.Join(plan.location, "journal.json")
				if err := os.Rename(filename, filename+".kept"); err != nil {
					t.Fatal(err)
				}
				managedSymlinkOrSkip(t, filename+".kept", filename)
			case "remote":
				snapshot.Head = strings.Repeat("c", 40)
			}
			pending, err := findWikiImport(dir, "remote")
			if err == nil {
				err = restoreWikiImport(dir, pending, snapshot)
			}
			if err == nil {
				t.Fatal("tampering accepted")
			}
		})
	}
}

func TestWikiImportUnsafePlans(t *testing.T) {
	for _, kind := range []string{"nil", "bad head", "traversal", "duplicate", "reserved", "symlink mode", "collision", "local symlink", "local reserved", "local case collision", "conflict markers"} {
		t.Run(kind, func(t *testing.T) {
			dir := t.TempDir()
			snapshot := wikiImportSnapshot()
			switch kind {
			case "nil":
				snapshot = nil
			case "bad head":
				snapshot.Head = "short"
			case "traversal":
				snapshot.Files[0].Path = "../outside"
			case "duplicate":
				snapshot.Files = append(snapshot.Files, snapshot.Files[0])
			case "reserved":
				snapshot.Files[0].Path = ".tailor-wiki-base"
			case "symlink mode":
				snapshot.Files[0].Mode = fs.ModeSymlink | 0o777
			case "collision":
				wikiTestWrite(t, dir, "wiki/nested", "file")
			case "local symlink":
				managedSymlinkOrSkip(t, t.TempDir(), filepath.Join(dir, "wiki"))
			case "local reserved":
				wikiTestWrite(t, dir, "wiki/.tailor-wiki-state", "user data")
			case "local case collision":
				wikiTestWrite(t, dir, "wiki/home.md", "case collision")
			case "conflict markers":
				snapshot.Files[0].Content = []byte("<<<<<<< local\nconflict\n=======\nother\n>>>>>>> remote\n")
			}
			if _, err := planWikiImport(dir, "remote", snapshot); err == nil {
				t.Fatal("unsafe plan accepted")
			}
		})
	}
}

func TestWikiImportRechecksConsentDestination(t *testing.T) {
	dir := wikiImportProject(t)
	snapshot := wikiImportSnapshot()
	wikiImportHooks(t, snapshot)
	wikiTestWrite(t, dir, "wiki/Home.md", "original")
	plan, err := planWikiImport(dir, "remote", snapshot)
	if err != nil {
		t.Fatal(err)
	}
	wikiTestWrite(t, dir, "wiki/Home.md", "changed during confirmation")
	if err := applyWikiImport(dir, plan); err == nil {
		t.Fatal("changed destination accepted")
	}
	data, _ := os.ReadFile(filepath.Join(dir, "wiki/Home.md"))
	if string(data) != "changed during confirmation" {
		t.Fatal("later edit lost")
	}
}

func TestWikiImportRetainsAbandonedPublicationTemps(t *testing.T) {
	for _, name := range []string{"wiki/.tailor-wiki-tmp-abandoned", "wiki/nested/.tailor-wiki-tmp-abandoned"} {
		t.Run(name, func(t *testing.T) {
			dir := wikiImportProject(t)
			snapshot := wikiImportSnapshot()
			wikiImportHooks(t, snapshot)
			plan, err := planWikiImport(dir, "remote", snapshot)
			if err != nil {
				t.Fatal(err)
			}
			if err := stageWikiImport(dir, plan); err != nil {
				t.Fatal(err)
			}
			wikiTestWrite(t, dir, name, "retained payload")
			pending, err := findWikiImport(dir, "remote")
			if err != nil {
				t.Fatal(err)
			}
			if err := restoreWikiImport(dir, pending, snapshot); err == nil {
				t.Fatal("abandoned temporary file accepted")
			}
			root, err := os.OpenRoot(dir)
			if err != nil {
				t.Fatal(err)
			}
			defer root.Close()
			if err := checkWikiSource(root); err == nil {
				t.Fatal("ready wiki accepted reserved artifact")
			}
			data, err := os.ReadFile(filepath.Join(dir, name))
			if err != nil || string(data) != "retained payload" {
				t.Fatal("unknown file removed or changed")
			}
		})
	}
}

func TestWikiImportInitialJournalFailure(t *testing.T) {
	for _, phase := range []string{"write", "sync", "rename"} {
		t.Run(phase, func(t *testing.T) {
			dir := wikiImportProject(t)
			snapshot := wikiImportSnapshot()
			wikiImportHooks(t, snapshot)
			wikiTestWrite(t, dir, "wiki/Home.md", "original")
			plan, err := planWikiImport(dir, "remote", snapshot)
			if err != nil {
				t.Fatal(err)
			}
			injected := errors.New("initial journal failed")
			var location string
			hook := func(name string) error {
				if name == "journal.json" {
					location = plan.location
					return injected
				}
				return nil
			}
			switch phase {
			case "write":
				plan.hooks.beforeTempWrite = hook
			case "sync":
				plan.hooks.beforeTempSync = hook
			case "rename":
				plan.hooks.beforeRename = hook
			}
			if err := applyWikiImport(dir, plan); !errors.Is(err, injected) {
				t.Fatalf("initial failure: %v", err)
			}
			if location == "" || plan.location != "" {
				t.Fatalf("transaction location not cleared: before=%q after=%q", location, plan.location)
			}
			if _, err := os.Lstat(location); !errors.Is(err, fs.ErrNotExist) {
				t.Fatalf("empty transaction retained: %v", err)
			}
			if pending, err := findWikiImport(dir, "remote"); err != nil || pending != nil {
				t.Fatalf("failed initial save blocks retry: pending=%+v err=%v", pending, err)
			}
			data, err := os.ReadFile(filepath.Join(dir, "wiki/Home.md"))
			if err != nil || string(data) != "original" {
				t.Fatalf("destination changed: %q, %v", data, err)
			}
			if _, err := os.Stat(filepath.Join(dir, swatch.WikiBaseline)); !errors.Is(err, fs.ErrNotExist) {
				t.Fatal("baseline installed after failed initial save")
			}
			plan.hooks = managedApplyHooks{}
			if err := applyWikiImport(dir, plan); err != nil || !plan.Complete {
				t.Fatalf("retry failed: complete=%t err=%v", plan.Complete, err)
			}
			data, err = os.ReadFile(filepath.Join(plan.location, "backup/0"))
			if err != nil || string(data) != "original" {
				t.Fatalf("retry lost original backup: %q, %v", data, err)
			}
		})
	}
}

func TestWikiImportInitialJournalFailurePreservesState(t *testing.T) {
	for _, kind := range []string{"unknown file", "changed directory"} {
		t.Run(kind, func(t *testing.T) {
			if runtime.GOOS == "windows" && kind == "changed directory" {
				t.Skip("renaming an open directory handle requires unix")
			}
			dir := wikiImportProject(t)
			plan, err := planWikiImport(dir, "remote", wikiImportSnapshot())
			if err != nil {
				t.Fatal(err)
			}
			injected := errors.New("initial journal failed")
			var location string
			var retained fs.FileInfo
			plan.hooks.beforeTempWrite = func(name string) error {
				if name != "journal.json" {
					return nil
				}
				location = plan.location
				if kind == "unknown file" {
					wikiTestWrite(t, location, "unknown", "retain me")
				} else {
					if err := os.Rename(location, location+".kept"); err != nil {
						t.Fatal(err)
					}
					if err := os.Mkdir(location, 0o700); err != nil {
						t.Fatal(err)
					}
				}
				retained, err = os.Lstat(location)
				if err != nil {
					t.Fatal(err)
				}
				_ = os.SameFile(retained, retained)
				return injected
			}
			err = stageWikiImport(dir, plan)
			if !errors.Is(err, injected) || location == "" || !strings.Contains(err.Error(), strconv.Quote(location)) || plan.location != location {
				t.Fatalf("failure lost error or location: location=%q plan=%q err=%v", location, plan.location, err)
			}
			current, err := os.Lstat(location)
			if err != nil || !os.SameFile(retained, current) {
				t.Fatalf("transaction entry changed: %v", err)
			}
			if kind == "unknown file" {
				data, err := os.ReadFile(filepath.Join(location, "unknown"))
				if err != nil || string(data) != "retain me" {
					t.Fatalf("unknown file changed: %q, %v", data, err)
				}
			} else if _, err := os.Stat(location + ".kept"); err != nil {
				t.Fatalf("original transaction removed: %v", err)
			}
			if _, err := findWikiImport(dir, "remote"); err == nil {
				t.Fatal("retained state did not block adoption")
			}
		})
	}
}

func TestWikiImportStagingResume(t *testing.T) {
	for _, failAt := range []string{"journal sync", "source/0", "backup/0", "source/1", "source/3", "prepared journal"} {
		t.Run(failAt, func(t *testing.T) {
			dir := wikiImportProject(t)
			snapshot := wikiImportSnapshot()
			wikiImportHooks(t, snapshot)
			wikiTestWrite(t, dir, "wiki/Home.md", "original")
			plan, err := planWikiImport(dir, "remote", snapshot)
			if err != nil {
				t.Fatal(err)
			}
			injected := errors.New("staging write failed")
			plan.hooks.beforeDirectorySync = func(name string) error {
				if failAt == "journal sync" && name == "journal.json" {
					return injected
				}
				return nil
			}
			plan.hooks.beforeTempWrite = func(name string) error {
				if name == failAt || failAt == "prepared journal" && name == "journal.json" && plan.Prepared {
					return injected
				}
				return nil
			}
			if err := applyWikiImport(dir, plan); !errors.Is(err, injected) {
				t.Fatalf("interruption: %v", err)
			}
			data, _ := os.ReadFile(filepath.Join(dir, "wiki/Home.md"))
			if string(data) != "original" {
				t.Fatal("destination changed before preparation")
			}
			if _, err := os.Stat(filepath.Join(dir, swatch.WikiBaseline)); !errors.Is(err, fs.ErrNotExist) {
				t.Fatal("baseline installed during staging")
			}
			pending, err := findWikiImport(dir, "remote")
			if err != nil || pending == nil || pending.Prepared {
				t.Fatalf("pending=%+v err=%v", pending, err)
			}
			var staged fs.FileInfo
			if failAt != "source/0" && failAt != "journal sync" {
				staged, err = os.Stat(filepath.Join(plan.location, "source/0"))
				if err != nil {
					t.Fatal(err)
				}
				_ = os.SameFile(staged, staged)
			}
			if err := restoreWikiImport(dir, pending, snapshot); err != nil {
				t.Fatal(err)
			}
			if pending.location == "" {
				t.Fatal("incomplete staging not reported as resume")
			}
			// Restoring for a preview must not prepare missing files or alter the journal.
			onDisk, err := findWikiImport(dir, "remote")
			if err != nil || !sameWikiJournal(pending, onDisk) {
				t.Fatalf("restore wrote state: %v", err)
			}
			if err := applyWikiImport(dir, pending); err != nil {
				t.Fatal(err)
			}
			if pending.location != plan.location || !pending.Complete {
				t.Fatal("resume did not use the recorded transaction")
			}
			if staged != nil {
				current, err := os.Stat(filepath.Join(plan.location, "source/0"))
				if err != nil || !os.SameFile(staged, current) {
					t.Fatal("verified staged content was replaced")
				}
			}
			data, _ = os.ReadFile(filepath.Join(plan.location, "backup/0"))
			if string(data) != "original" {
				t.Fatal("original backup lost")
			}
		})
	}
}

func TestWikiImportIncompleteStagingRejectsChanges(t *testing.T) {
	for _, kind := range []string{"source bytes", "source mode", "backup bytes", "backup mode", "local bytes", "local mode", "local after", "snapshot bytes", "remote", "orphan backup", "missing journal", "late local edit", "late stored edit"} {
		t.Run(kind, func(t *testing.T) {
			if runtime.GOOS == "windows" && strings.HasSuffix(kind, " mode") {
				t.Skip("POSIX permission bits require unix")
			}
			dir := wikiImportProject(t)
			snapshot := wikiImportSnapshot()
			wikiImportHooks(t, snapshot)
			wikiTestWrite(t, dir, "wiki/Home.md", "original")
			plan, err := planWikiImport(dir, "remote", snapshot)
			if err != nil {
				t.Fatal(err)
			}
			plan.hooks.beforeTempWrite = func(name string) error {
				if name == "source/1" {
					return errors.New("interrupted")
				}
				return nil
			}
			if err := applyWikiImport(dir, plan); err == nil {
				t.Fatal("expected interruption")
			}
			switch kind {
			case "source bytes":
				wikiTestWrite(t, plan.location, "source/0", "tampered")
			case "source mode":
				err = os.Chmod(filepath.Join(plan.location, "source/0"), 0o644)
			case "backup bytes":
				wikiTestWrite(t, plan.location, "backup/0", "tampered")
			case "backup mode":
				err = os.Chmod(filepath.Join(plan.location, "backup/0"), 0o600)
			case "local bytes":
				wikiTestWrite(t, dir, "wiki/Home.md", "later edit")
			case "local mode":
				err = os.Chmod(filepath.Join(dir, "wiki/Home.md"), 0o600)
			case "local after":
				wikiTestWrite(t, dir, "wiki/Home.md", string(snapshot.Files[0].Content))
			case "snapshot bytes":
				snapshot.Files[0].Content = []byte("different snapshot")
			case "remote":
				snapshot.Head = strings.Repeat("b", 40)
			case "orphan backup":
				wikiTestWrite(t, plan.location, "backup/unknown", "retain me")
			case "missing journal":
				err = os.Rename(filepath.Join(plan.location, "journal.json"), filepath.Join(plan.location, "journal.kept"))
			}
			if err != nil {
				t.Fatal(err)
			}
			pending, err := findWikiImport(dir, "remote")
			if err == nil {
				err = restoreWikiImport(dir, pending, snapshot)
			}
			if strings.HasPrefix(kind, "late ") {
				if err != nil {
					t.Fatal(err)
				}
				if kind == "late local edit" {
					wikiTestWrite(t, dir, "wiki/Home.md", "edit after restore")
				} else {
					wikiTestWrite(t, plan.location, "source/0", "tampered after restore")
				}
				err = applyWikiImport(dir, pending)
			}
			if err == nil {
				t.Fatal("changed state accepted")
			}
			if _, err := os.Stat(filepath.Join(plan.location, "backup/0")); err != nil {
				t.Fatal("backup not retained")
			}
			if _, err := os.Stat(filepath.Join(dir, swatch.WikiBaseline)); !errors.Is(err, fs.ErrNotExist) {
				t.Fatal("baseline installed after rejected recovery")
			}
		})
	}
}

func TestWikiImportDestinationWriteFailures(t *testing.T) {
	for _, failAt := range []string{"wiki/Home.md", "wiki/image.bin", swatch.WikiBaseline} {
		for _, phase := range []string{"write", "sync", "rename"} {
			t.Run(failAt+"/"+phase, func(t *testing.T) {
				dir := wikiImportProject(t)
				snapshot := wikiImportSnapshot()
				wikiImportHooks(t, snapshot)
				wikiTestWrite(t, dir, "wiki/Home.md", "original")
				oldBase := strings.Repeat("b", 40) + "\n"
				wikiTestWrite(t, dir, swatch.WikiBaseline, oldBase)
				plan, err := planWikiImport(dir, "remote", snapshot)
				if err != nil {
					t.Fatal(err)
				}
				injected := errors.New("destination failure")
				hook := func(name string) error {
					if !strings.HasPrefix(name, "wiki/") {
						return nil
					}
					for i, f := range plan.Files {
						if !f.Before.Exists {
							continue
						}
						data, err := os.ReadFile(filepath.Join(plan.location, "backup", strconv.Itoa(i)))
						info, statErr := os.Stat(filepath.Join(plan.location, "backup", strconv.Itoa(i)))
						if err != nil || statErr != nil || !sameWikiFileState(wikiState(data, info.Mode()), f.Before) {
							t.Fatal("destination write without verified backup")
						}
					}
					if name == swatch.WikiBaseline {
						for _, f := range plan.Files[:len(plan.Files)-1] {
							if !f.Installed {
								t.Fatal("baseline was not last")
							}
						}
					}
					if name == failAt {
						return injected
					}
					return nil
				}
				switch phase {
				case "write":
					plan.hooks.beforeTempWrite = hook
				case "sync":
					plan.hooks.beforeTempSync = hook
				case "rename":
					plan.hooks.beforeRename = hook
				}
				if err := applyWikiImport(dir, plan); !errors.Is(err, injected) {
					t.Fatalf("failure: %v", err)
				}
				data, _ := os.ReadFile(filepath.Join(dir, swatch.WikiBaseline))
				if string(data) != oldBase {
					t.Fatal("failed import advanced baseline")
				}
				pending, err := findWikiImport(dir, "remote")
				if err != nil {
					t.Fatal(err)
				}
				if err := restoreWikiImport(dir, pending, snapshot); err != nil {
					t.Fatal(err)
				}
				if err := applyWikiImport(dir, pending); err != nil {
					t.Fatal(err)
				}
			})
		}
	}
}

func TestWikiImportLinkedWorktree(t *testing.T) {
	dir := wikiImportProject(t)
	wikiTestWrite(t, dir, "README.md", "project")
	wikiImportGit(t, dir, "add", "README.md")
	wikiImportGit(t, dir, "commit", "-m", "initial")
	linked := filepath.Join(t.TempDir(), "linked")
	wikiImportGit(t, dir, "worktree", "add", "-b", "linked", linked)
	snapshot := wikiImportSnapshot()
	wikiImportHooks(t, snapshot)
	plan, err := planWikiImport(linked, "remote", snapshot)
	if err != nil {
		t.Fatal(err)
	}
	if err := applyWikiImport(linked, plan); err != nil {
		t.Fatal(err)
	}
	gitDir, _, err := wikiGitDirectory(linked)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(plan.location, filepath.Join(gitDir, "tailor/wiki-imports")) {
		t.Fatalf("wrong storage: %s", plan.location)
	}
	if slices.Contains(strings.Fields(wikiImportGit(t, linked, "status", "--porcelain")), "backup") {
		t.Fatal("visible backups")
	}
}
