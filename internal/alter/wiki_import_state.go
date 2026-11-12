package alter

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/wimpysworld/tailor/internal/gh"
	"github.com/wimpysworld/tailor/internal/swatch"
)

const wikiJournalLimit = 8 << 20

// Git rev-parse reads worktree metadata without loading project helpers or changing configuration.
func wikiGitDirectory(dir string) (string, string, error) {
	project, err := filepath.Abs(dir)
	if err != nil {
		return "", "", err
	}
	project, err = filepath.EvalSymlinks(project)
	if err != nil {
		return "", "", err
	}
	out, err := readWikiGitMetadata(project, false)
	if err != nil {
		return "", "", fmt.Errorf("locating private wiki recovery storage: %w", err)
	}
	out = strings.ReplaceAll(out, "\r\n", "\n")
	lines := strings.Split(strings.TrimSuffix(out, "\n"), "\n")
	if len(lines) != 2 {
		return "", "", errors.New("wiki adoption requires the Git worktree root")
	}
	gitPath := filepath.FromSlash(lines[0])
	topLevel := filepath.Clean(filepath.FromSlash(lines[1]))
	if !filepath.IsAbs(gitPath) || topLevel != project {
		return "", "", errors.New("wiki adoption requires the Git worktree root")
	}
	gitDir := filepath.Clean(gitPath)
	clean, err := filepath.EvalSymlinks(gitDir)
	if err != nil || clean != gitDir {
		return "", "", errors.New("wiki Git metadata path contains a symlink")
	}
	return gitDir, project, nil
}

func readWikiGitMetadata(dir string, unmerged bool) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "git", "-C", dir, "rev-parse", "--absolute-git-dir", "--show-toplevel")
	if unmerged {
		cmd = exec.CommandContext(ctx, "git", "-C", dir, "ls-files", "--unmerged", "-z", "--", "wiki/")
	}
	for _, entry := range os.Environ() {
		if !strings.HasPrefix(entry, "GIT_") {
			cmd.Env = append(cmd.Env, entry)
		}
	}
	cmd.Env = append(cmd.Env, "GIT_CONFIG_NOSYSTEM=1", "GIT_CONFIG_GLOBAL="+os.DevNull, "GIT_OPTIONAL_LOCKS=0", "GIT_TERMINAL_PROMPT=0")
	var out wikiBoundedOutput
	cmd.Stdout = &out
	err := cmd.Run()
	return out.String(), err
}

func checkWikiUnmergedIndex(dir string) error {
	if _, err := os.Lstat(filepath.Join(dir, ".git")); errors.Is(err, fs.ErrNotExist) {
		return nil
	} else if err != nil {
		return err
	}
	out, err := readWikiGitMetadata(dir, true)
	if err != nil {
		return fmt.Errorf("checking wiki index conflicts: %w", err)
	}
	if out != "" {
		return errors.New("wiki has unmerged Git index entries; resolve them before adoption")
	}
	return nil
}

type wikiBoundedOutput struct{ bytes.Buffer }

func (b *wikiBoundedOutput) Write(data []byte) (int, error) {
	if len(data) > 16384-b.Len() {
		return 0, errors.New("git metadata output exceeds limit")
	}
	return b.Buffer.Write(data)
}

func wikiStateRoot(gitDir string, create bool) (*os.Root, error) {
	root, err := os.OpenRoot(gitDir)
	if err != nil {
		return nil, err
	}
	defer root.Close()
	if err := checkParents(root, "tailor/wiki-imports/journal", "wiki recovery storage"); err != nil {
		return nil, err
	}
	if create {
		if err := root.MkdirAll("tailor/wiki-imports", 0o700); err != nil {
			return nil, err
		}
	}
	for _, name := range []string{"tailor", "tailor/wiki-imports"} {
		info, err := root.Lstat(name)
		if err != nil {
			return nil, err
		}
		if !isPrivateWikiDir(info, false) {
			return nil, fmt.Errorf("wiki recovery directory %q must be private", name)
		}
	}
	return root.OpenRoot("tailor/wiki-imports")
}

func findWikiImport(dir, remote string) (*wikiImportPlan, error) {
	if _, err := os.Lstat(filepath.Join(dir, ".git")); errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	gitDir, project, err := wikiGitDirectory(dir)
	if err != nil {
		return nil, err
	}
	root, err := wikiStateRoot(gitDir, false)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	defer root.Close()
	directory, err := root.Open(".")
	if err != nil {
		return nil, err
	}
	defer directory.Close()
	entries, err := directory.ReadDir(1001)
	if err != nil && !errors.Is(err, io.EOF) {
		return nil, err
	}
	if len(entries) > 1000 {
		return nil, errors.New("too many wiki recovery transactions")
	}
	var pending *wikiImportPlan
	for _, entry := range entries {
		if entry.Name() == "lock" {
			continue
		}
		location := filepath.Join(gitDir, "tailor/wiki-imports", entry.Name())
		if !entry.IsDir() || entry.Type()&os.ModeSymlink != 0 {
			return nil, fmt.Errorf("unsafe wiki recovery entry %q", location)
		}
		transaction, err := root.OpenRoot(entry.Name())
		if err != nil {
			return nil, err
		}
		p, loadErr := loadWikiJournal(transaction)
		_ = transaction.Close()
		if loadErr != nil {
			return nil, fmt.Errorf("wiki recovery state needs review at %q: %w", location, loadErr)
		}
		if p.Complete {
			continue
		}
		if p.Project != project || p.Remote != remote {
			return nil, fmt.Errorf("wiki recovery identity mismatch at %q", location)
		}
		p.location = location
		if pending != nil {
			return nil, fmt.Errorf("multiple incomplete wiki imports at %q", filepath.Dir(location))
		}
		pending = p
	}
	return pending, nil
}

func loadWikiJournal(root *os.Root) (*wikiImportPlan, error) {
	info, err := root.Lstat("journal.json")
	if err != nil {
		return nil, err
	}
	if !isPrivateWikiFile(info) || info.Size() > wikiJournalLimit {
		return nil, errors.New("unsafe wiki import journal")
	}
	f, err := openManagedInspectionFile(root, "journal.json")
	if err != nil {
		return nil, err
	}
	defer f.Close()
	decoder := json.NewDecoder(io.LimitReader(f, wikiJournalLimit+1))
	decoder.DisallowUnknownFields()
	var p wikiImportPlan
	if err := decoder.Decode(&p); err != nil {
		return nil, err
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		return nil, errors.New("trailing wiki journal data")
	}
	if p.Version != 1 || !wikiCommit.MatchString(p.Head) || len(p.Files) < 2 || len(p.Files) > gh.WikiSnapshotMaxFiles+1 || p.Complete && !p.Prepared {
		return nil, errors.New("invalid wiki import journal")
	}
	snapshot := &gh.WikiSnapshot{Head: p.Head}
	for i, f := range p.Files {
		if !strings.HasPrefix(f.Path, "wiki/") || !validWikiState(f.Before) || !validWikiState(f.After) || !f.After.Exists || !p.Prepared && f.Installed {
			return nil, errors.New("invalid wiki journal file")
		}
		if i == len(p.Files)-1 {
			if f.Path != swatch.WikiBaseline || f.After != wikiState([]byte(p.Head+"\n"), 0o644) {
				return nil, errors.New("invalid wiki journal baseline")
			}
		} else {
			snapshot.Files = append(snapshot.Files, gh.WikiFile{Path: strings.TrimPrefix(f.Path, "wiki/"), Mode: f.After.Mode})
		}
	}
	if err := gh.ValidateWikiSnapshot(snapshot); err != nil {
		return nil, err
	}
	return &p, nil
}

func validWikiState(s wikiFileState) bool {
	if !s.Exists {
		return s == (wikiFileState{})
	}
	if s.Mode & ^fs.FileMode(0o777) != 0 || len(s.Hash) != 64 {
		return false
	}
	for _, c := range s.Hash {
		if (c < '0' || c > '9') && (c < 'a' || c > 'f') {
			return false
		}
	}
	return true
}

func saveWikiJournal(root *os.Root, p *wikiImportPlan) error {
	data, err := json.Marshal(p)
	if err != nil {
		return err
	}
	if len(data) > wikiJournalLimit {
		return errors.New("wiki journal exceeds size limit")
	}
	_, err = publishWikiFile(root, "journal.json", data, 0o600, false, nil, p.hooks)
	return err
}

func stageWikiImport(dir string, p *wikiImportPlan) error {
	gitDir, project, err := wikiGitDirectory(dir)
	if err != nil {
		return err
	}
	root, err := wikiStateRoot(gitDir, true)
	if err != nil {
		return err
	}
	defer root.Close()
	name := filepath.Base(p.location)
	newImport := p.location == ""
	if newImport {
		name = "import-" + rand.Text()
		if err := root.Mkdir(name, 0o700); err != nil {
			return err
		}
		p.location = filepath.Join(gitDir, "tailor/wiki-imports", name)
		p.Project = project
	}
	transaction, err := root.OpenRoot(name)
	if err != nil {
		return err
	}
	defer transaction.Close()
	if newImport {
		if err := saveWikiJournal(transaction, p); err != nil {
			_, journalErr := transaction.Lstat("journal.json")
			if !errors.Is(journalErr, fs.ErrNotExist) {
				return fmt.Errorf("saving initial wiki journal at %q: %w", p.location, errors.Join(err, journalErr))
			}
			opened, openedErr := transaction.Stat(".")
			current, currentErr := root.Lstat(name)
			var cleanupErr error
			if openedErr != nil || currentErr != nil || !current.IsDir() || !os.SameFile(opened, current) {
				cleanupErr = errors.Join(errors.New("wiki recovery directory changed before cleanup"), openedErr, currentErr)
			} else {
				cleanupErr = root.Remove(name)
			}
			if cleanupErr != nil {
				return fmt.Errorf("initial wiki journal failed; retain recovery state at %q: %w", p.location, errors.Join(err, cleanupErr))
			}
			p.location = ""
			return err
		}
	}
	if err := checkWikiStoredEntries(transaction, p); err != nil {
		return err
	}
	for _, name := range []string{"source", "backup"} {
		if err := transaction.Mkdir(name, 0o700); err != nil && !errors.Is(err, fs.ErrExist) {
			return err
		}
	}
	projectRoot, err := os.OpenRoot(dir)
	if err != nil {
		return err
	}
	defer projectRoot.Close()
	for i, f := range p.Files {
		if err := prepareWikiStoredData(transaction, "source/"+strconv.Itoa(i), p.source[i], wikiSourceState(f.After), p); err != nil {
			return err
		}
		if f.Before.Exists {
			if err := recheckWikiImportDestination(projectRoot, p, f); err != nil {
				return err
			}
			if err := prepareWikiStoredData(transaction, "backup/"+strconv.Itoa(i), p.backup[i], f.Before, p); err != nil {
				return err
			}
		}
	}
	if err := syncManagedDirectory(root, name, p.hooks); err != nil {
		return err
	}
	if !p.Prepared {
		p.Prepared = true
		if err := saveWikiJournal(transaction, p); err != nil {
			p.Prepared = false
			return err
		}
	}
	return nil
}

func restoreWikiImport(dir string, p *wikiImportPlan, snapshot *gh.WikiSnapshot) error {
	if snapshot == nil || snapshot.Head != p.Head {
		return fmt.Errorf("wiki changed since the interrupted import; keep recovery data at %q", p.location)
	}
	fresh, err := planWikiImport(dir, p.Remote, snapshot)
	if err != nil {
		return err
	}
	if len(fresh.Files) != len(p.Files) {
		return errors.New("wiki recovery plan differs from the pinned snapshot")
	}
	root, err := os.OpenRoot(p.location)
	if err != nil {
		return err
	}
	defer root.Close()
	if err := checkWikiStoredEntries(root, p); err != nil {
		return err
	}
	p.source, p.backup = nil, nil
	total := 0
	for i, f := range p.Files {
		if f.Path != fresh.Files[i].Path || f.After != fresh.Files[i].After {
			return errors.New("wiki recovery plan differs from the pinned snapshot")
		}
		current := fresh.Files[i].Before
		if !p.Prepared && (f.Installed || !sameWikiFileState(current, f.Before)) || p.Prepared && !sameWikiFileState(current, f.After) && (f.Installed || !sameWikiFileState(current, f.Before)) {
			return fmt.Errorf("wiki file %q changed after the import stopped; keep recovery data at %q", f.Path, p.location)
		}
		source, err := restoreWikiStoredData(root, "source/"+strconv.Itoa(i), wikiSourceState(f.After), fresh.source[i], !p.Prepared)
		if err != nil {
			return err
		}
		p.source = append(p.source, source)
		var backup []byte
		if f.Before.Exists {
			backup, err = restoreWikiStoredData(root, "backup/"+strconv.Itoa(i), f.Before, fresh.backup[i], !p.Prepared)
			if err != nil {
				return err
			}
		}
		total += len(backup)
		if total > gh.WikiSnapshotMaxBytes {
			return errors.New("wiki recovery backups exceed size limit")
		}
		p.backup = append(p.backup, backup)
	}
	p.identities = fresh.identities
	return nil
}

func checkWikiStoredEntries(root *os.Root, p *wikiImportPlan) error {
	allowed := map[string]bool{".": true, "journal.json": false, "source": true, "backup": true}
	for i, f := range p.Files {
		allowed["source/"+strconv.Itoa(i)] = false
		if f.Before.Exists {
			allowed["backup/"+strconv.Itoa(i)] = false
		}
	}
	for _, name := range []string{".", "source", "backup"} {
		info, err := root.Lstat(name)
		if errors.Is(err, fs.ErrNotExist) && !p.Prepared && name != "." {
			continue
		}
		if err != nil {
			return err
		}
		if !isPrivateWikiDir(info, true) {
			return fmt.Errorf("wiki recovery directory %q must be private", name)
		}
		directory, err := root.Open(name)
		if err != nil {
			return err
		}
		entries, readErr := directory.ReadDir(len(p.Files) + 4)
		closeErr := directory.Close()
		if readErr != nil && !errors.Is(readErr, io.EOF) {
			return readErr
		}
		if closeErr != nil {
			return closeErr
		}
		for _, entry := range entries {
			filename := path.Join(name, entry.Name())
			isDirectory, ok := allowed[filename]
			if !ok || entry.IsDir() != isDirectory || entry.Type()&os.ModeSymlink != 0 {
				return fmt.Errorf("unknown or unsafe wiki recovery entry %q; retain it for review", filename)
			}
		}
	}
	return nil
}

func isPrivateWikiDir(info fs.FileInfo, exact bool) bool {
	if !info.IsDir() {
		return false
	}
	perm := info.Mode().Perm()
	if runtime.GOOS == "windows" {
		return perm == 0o777 || perm == 0o555
	}
	if exact {
		return perm == 0o700
	}
	return perm&0o077 == 0
}

func isPrivateWikiFile(info fs.FileInfo) bool {
	if !info.Mode().IsRegular() {
		return false
	}
	perm := info.Mode().Perm()
	if runtime.GOOS == "windows" {
		return perm == 0o666 || perm == 0o444
	}
	return perm == 0o600
}

func prepareWikiStoredData(root *os.Root, name string, data []byte, expected wikiFileState, p *wikiImportPlan) error {
	state, _, _, err := readWikiImportFile(root, name)
	if err != nil {
		return err
	}
	if state.Exists {
		if !sameWikiFileState(state, expected) {
			return fmt.Errorf("wiki recovery data %q changed", name)
		}
		return syncManagedDirectory(root, name, p.hooks)
	}
	if p.Prepared || wikiState(data, expected.Mode) != expected {
		return fmt.Errorf("wiki recovery data %q is missing or changed", name)
	}
	_, err = publishWikiFile(root, name, data, expected.Mode, true, nil, p.hooks)
	return err
}

func wikiSourceState(state wikiFileState) wikiFileState {
	state.Mode = 0o600
	return state
}

func restoreWikiStoredData(root *os.Root, name string, expected wikiFileState, fallback []byte, allowMissing bool) ([]byte, error) {
	state, data, _, err := readWikiImportFile(root, name)
	if err != nil {
		return nil, err
	}
	if !state.Exists && allowMissing && wikiState(fallback, expected.Mode) == expected {
		return fallback, nil
	}
	if !sameWikiFileState(state, expected) {
		return nil, fmt.Errorf("wiki recovery data %q changed", name)
	}
	return data, nil
}

// publishWikiFile is atomic for one file, not for the whole import.
func publishWikiFile(root *os.Root, name string, data []byte, mode fs.FileMode, noClobber bool, recheck func() error, hooks managedApplyHooks) (changed bool, retErr error) {
	if err := checkParents(root, name, "wiki publication parent"); err != nil {
		return false, err
	}
	parent, err := root.OpenRoot(path.Dir(name))
	if err != nil {
		return false, err
	}
	defer parent.Close()
	tempName := ".tailor-wiki-tmp-" + rand.Text()
	temp, err := parent.OpenFile(tempName, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return false, err
	}
	tempOpen := true
	removeFailed := false
	defer func() {
		if tempOpen {
			retErr = errors.Join(retErr, temp.Close())
		}
		if tempName != "" {
			if err := parent.Remove(tempName); err != nil && !errors.Is(err, fs.ErrNotExist) && !removeFailed {
				retErr = errors.Join(retErr, err)
			}
		}
	}()
	if err := runManagedHook(hooks.beforeTempWrite, name); err != nil {
		return false, err
	}
	if _, err := temp.Write(data); err != nil {
		return false, err
	}
	if err := temp.Chmod(mode); err != nil {
		return false, err
	}
	if err := runManagedHook(hooks.beforeTempSync, name); err != nil {
		return false, err
	}
	if err := temp.Sync(); err != nil {
		return false, err
	}
	if err := runManagedHook(hooks.beforeTempClose, name); err != nil {
		return false, err
	}
	if err := temp.Close(); err != nil {
		tempOpen = false
		return false, err
	}
	tempOpen = false
	if err := runManagedHook(hooks.beforeRename, name); err != nil {
		return false, err
	}
	if recheck != nil {
		if err := recheck(); err != nil {
			return false, err
		}
	}
	opened, err := parent.Stat(".")
	if err != nil {
		return false, err
	}
	current, err := root.Lstat(path.Dir(name))
	if err != nil || !current.IsDir() || !os.SameFile(opened, current) {
		return false, errors.New("wiki publication parent changed")
	}
	if info, err := parent.Lstat(path.Base(name)); err == nil && !info.Mode().IsRegular() {
		return false, errors.New("unsafe wiki publication destination")
	} else if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return false, err
	}
	if noClobber {
		if err := parent.Link(tempName, path.Base(name)); err != nil {
			return false, err
		}
		if err := parent.Remove(tempName); err != nil && !errors.Is(err, fs.ErrNotExist) {
			removeFailed = true
			return true, err
		}
	} else if err := parent.Rename(tempName, path.Base(name)); err != nil {
		return false, err
	}
	tempName = ""
	return true, syncManagedDirectory(parent, path.Base(name), hooks)
}
