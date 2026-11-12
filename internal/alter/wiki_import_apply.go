package alter

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/wimpysworld/tailor/internal/gh"
)

var unlockWikiImport = func(*os.File) error { return nil }

func applyWikiImport(dir string, p *wikiImportPlan) error {
	gitDir, _, err := wikiGitDirectory(dir)
	if err != nil {
		return err
	}
	storage, err := wikiStateRoot(gitDir, true)
	if err != nil {
		return err
	}
	defer storage.Close()
	if info, err := storage.Lstat("lock"); err == nil && !info.Mode().IsRegular() {
		return errors.New("unsafe wiki recovery lock")
	} else if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	lock, err := storage.OpenFile("lock", os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return err
	}
	defer lock.Close()
	if err := lockWikiImport(lock); err != nil {
		return fmt.Errorf("another wiki import is active: %w", err)
	}
	defer func() { _ = unlockWikiImport(lock) }()
	pending, err := findWikiImport(dir, p.Remote)
	if err != nil {
		return err
	}
	if pending != nil && pending.location != p.location {
		return fmt.Errorf("another wiki import needs recovery at %q", pending.location)
	}
	if pending == nil && p.location != "" {
		return errors.New("wiki recovery state changed before import")
	}
	if pending != nil {
		// Do not reuse an approval after another process advances the journal.
		if !sameWikiJournal(p, pending) {
			return errors.New("wiki recovery journal changed before import")
		}
	}
	root, err := os.OpenRoot(dir)
	if err != nil {
		return err
	}
	defer root.Close()
	if err := recheckWikiImport(root, p, false); err != nil {
		return err
	}
	if err := gh.RecheckWikiHead(p.Remote, p.Head); err != nil {
		return err
	}
	if err := stageWikiImport(dir, p); err != nil {
		return err
	}
	if err := recheckWikiImport(root, p, false); err != nil {
		return err
	}
	transaction, err := storage.OpenRoot(filepath.Base(p.location))
	if err != nil {
		return err
	}
	defer transaction.Close()
	for i := range p.Files {
		file := &p.Files[i]
		if i == len(p.Files)-1 {
			if err := recheckWikiImport(root, p, true); err != nil {
				return err
			}
			if err := gh.RecheckWikiHead(p.Remote, p.Head); err != nil {
				return err
			}
		}
		current, _, _, err := readWikiImportFile(root, file.Path)
		if err != nil {
			return err
		}
		if sameWikiFileState(current, file.After) {
			file.Installed = true
			continue
		}
		if !sameWikiFileState(current, file.Before) || file.Installed {
			return fmt.Errorf("wiki destination %q changed; recovery data is at %q", file.Path, p.location)
		}
		if err := createWikiImportParents(root, p, path.Dir(file.Path)); err != nil {
			return err
		}
		changed, err := publishWikiFile(root, file.Path, p.source[i], file.After.Mode, !file.Before.Exists, func() error {
			return recheckWikiImportDestination(root, p, *file)
		}, p.hooks)
		if changed {
			file.Installed = true
			info, statErr := root.Lstat(file.Path)
			if statErr != nil {
				return statErr
			}
			p.identities[file.Path] = info
		}
		if err != nil {
			return err
		}
		if err := saveWikiJournal(transaction, p); err != nil {
			return err
		}
	}
	if err := recheckWikiImport(root, p, true); err != nil {
		return err
	}
	p.Complete = true
	if err := saveWikiJournal(transaction, p); err != nil {
		p.Complete = false
		return err
	}
	return nil
}

func sameWikiJournal(a, b *wikiImportPlan) bool {
	if a.Head != b.Head || a.Project != b.Project || a.Remote != b.Remote || a.Prepared != b.Prepared || a.Complete != b.Complete || len(a.Files) != len(b.Files) {
		return false
	}
	for i := range a.Files {
		if a.Files[i] != b.Files[i] {
			return false
		}
	}
	return true
}

func recheckWikiImport(root *os.Root, p *wikiImportPlan, installed bool) error {
	for i, f := range p.Files {
		if installed && i == len(p.Files)-1 && !f.Installed {
			continue
		}
		if err := recheckWikiImportDestination(root, p, f); err != nil {
			return err
		}
		if installed {
			state, _, _, err := readWikiImportFile(root, f.Path)
			if err != nil {
				return err
			}
			if !sameWikiFileState(state, f.After) {
				return fmt.Errorf("wiki import verification failed for %q", f.Path)
			}
		}
	}
	return nil
}

func recheckWikiImportDestination(root *os.Root, p *wikiImportPlan, f wikiImportFile) error {
	for name := f.Path; ; name = path.Dir(name) {
		current, err := root.Lstat(name)
		if err != nil && !errors.Is(err, fs.ErrNotExist) {
			return err
		}
		expected := p.identities[name]
		if (current == nil) != (expected == nil) || current != nil && (!os.SameFile(current, expected) || effectiveWikiMode(current.Mode()) != effectiveWikiMode(expected.Mode())) {
			return fmt.Errorf("wiki destination or parent %q changed after planning", name)
		}
		if name == "." {
			break
		}
	}
	state, _, _, err := readWikiImportFile(root, f.Path)
	if err != nil {
		return err
	}
	if f.Installed || p.Prepared && sameWikiFileState(state, f.After) {
		if !sameWikiFileState(state, f.After) {
			return fmt.Errorf("wiki destination %q changed after import", f.Path)
		}
	} else if !sameWikiFileState(state, f.Before) {
		return fmt.Errorf("wiki destination %q changed after planning", f.Path)
	}
	return nil
}

func createWikiImportParents(root *os.Root, p *wikiImportPlan, parent string) error {
	current := ""
	for part := range strings.SplitSeq(parent, "/") {
		current = path.Join(current, part)
		info, err := root.Lstat(current)
		if errors.Is(err, fs.ErrNotExist) {
			if p.identities[current] != nil {
				return fmt.Errorf("wiki parent %q disappeared", current)
			}
			if err := root.Mkdir(current, 0o755); err != nil {
				return err
			}
			info, err = root.Lstat(current)
			if err != nil {
				return err
			}
			p.identities[current] = info
			if err := syncManagedDirectory(root, current, managedApplyHooks{}); err != nil {
				return err
			}
		} else if err != nil {
			return err
		}
		if !info.IsDir() || p.identities[current] == nil || !os.SameFile(info, p.identities[current]) {
			return fmt.Errorf("wiki parent %q changed", current)
		}
	}
	return nil
}
