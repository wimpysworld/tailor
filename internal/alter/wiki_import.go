package alter

import (
	"bytes"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"runtime"
	"slices"
	"strings"

	"github.com/wimpysworld/tailor/internal/gh"
	"github.com/wimpysworld/tailor/internal/output"
	"github.com/wimpysworld/tailor/internal/swatch"
	"github.com/wimpysworld/tailor/internal/termtext"
)

type wikiFileState struct {
	Exists bool
	Mode   fs.FileMode
	Hash   string
}

type wikiImportFile struct {
	Path      string
	Before    wikiFileState
	After     wikiFileState
	Installed bool
}

type wikiImportPlan struct {
	Version  int
	Project  string
	Remote   string
	Head     string
	Files    []wikiImportFile
	Prepared bool
	Complete bool

	location   string
	source     [][]byte
	backup     [][]byte
	identities map[string]fs.FileInfo
	hooks      managedApplyHooks
}

func wikiState(data []byte, mode fs.FileMode) wikiFileState {
	return wikiFileState{Exists: true, Mode: mode, Hash: fmt.Sprintf("%x", sha256.Sum256(data))}
}

func effectiveWikiMode(mode fs.FileMode) fs.FileMode {
	if runtime.GOOS != "windows" {
		return mode
	}
	if mode.IsDir() {
		if mode&0o200 == 0 {
			return fs.ModeDir | 0o555
		}
		return fs.ModeDir | 0o777
	}
	if mode.IsRegular() {
		if mode&0o200 == 0 {
			return 0o444
		}
		return 0o666
	}
	return mode
}

func sameWikiFileState(a, b wikiFileState) bool {
	if runtime.GOOS != "windows" {
		return a == b
	}
	return a.Exists == b.Exists && a.Hash == b.Hash && (!a.Exists || effectiveWikiMode(a.Mode) == effectiveWikiMode(b.Mode))
}

func readWikiImportFile(root *os.Root, name string) (wikiFileState, []byte, fs.FileInfo, error) {
	if err := checkParents(root, name, "wiki import parent"); err != nil {
		return wikiFileState{}, nil, nil, err
	}
	info, err := root.Lstat(name)
	if errors.Is(err, fs.ErrNotExist) {
		return wikiFileState{}, nil, nil, nil
	}
	if err != nil {
		return wikiFileState{}, nil, nil, err
	}
	if !info.Mode().IsRegular() || info.Mode() & ^fs.FileMode(0o777) != 0 || info.Size() > gh.WikiSnapshotMaxBlobBytes {
		return wikiFileState{}, nil, nil, fmt.Errorf("unsafe wiki import file %q", name)
	}
	file, err := openManagedInspectionFile(root, name)
	if err != nil {
		return wikiFileState{}, nil, nil, err
	}
	defer file.Close()
	opened, err := file.Stat()
	if err != nil || !os.SameFile(info, opened) {
		return wikiFileState{}, nil, nil, fmt.Errorf("wiki import file %q changed while opening", name)
	}
	data, err := io.ReadAll(io.LimitReader(file, gh.WikiSnapshotMaxBlobBytes+1))
	if err != nil {
		return wikiFileState{}, nil, nil, err
	}
	after, err := file.Stat()
	if err != nil || len(data) > gh.WikiSnapshotMaxBlobBytes || after.Mode() != info.Mode() || after.Size() != info.Size() || !after.ModTime().Equal(info.ModTime()) {
		return wikiFileState{}, nil, nil, fmt.Errorf("wiki import file %q changed while reading", name)
	}
	return wikiState(data, info.Mode()), data, info, nil
}

func planWikiImport(dir, remote string, snapshot *gh.WikiSnapshot) (*wikiImportPlan, error) {
	if err := gh.ValidateWikiSnapshot(snapshot); err != nil {
		return nil, err
	}
	if err := checkWikiUnmergedIndex(dir); err != nil {
		return nil, err
	}
	root, err := os.OpenRoot(dir)
	if err != nil {
		return nil, err
	}
	defer root.Close()
	if err := checkWikiSource(root); err != nil {
		return nil, err
	}
	if err := validateWikiImportLocal(root, snapshot); err != nil {
		return nil, err
	}
	p := &wikiImportPlan{Version: 1, Remote: remote, Head: snapshot.Head, identities: map[string]fs.FileInfo{}}
	files := slices.Clone(snapshot.Files)
	slices.SortFunc(files, func(a, b gh.WikiFile) int { return strings.Compare(a.Path, b.Path) })
	files = append(files, gh.WikiFile{Path: strings.TrimPrefix(swatch.WikiBaseline, "wiki/"), Mode: 0o644, Content: []byte(snapshot.Head + "\n")})
	total := 0
	for _, file := range files {
		name := "wiki/" + file.Path
		if unresolvedWikiConflict(file.Content) {
			return nil, fmt.Errorf("wiki file %q contains unresolved conflict markers", name)
		}
		before, data, info, err := readWikiImportFile(root, name)
		if err != nil {
			return nil, err
		}
		total += len(data)
		if total > gh.WikiSnapshotMaxBytes {
			return nil, fmt.Errorf("wiki backups exceed the size limit")
		}
		p.Files = append(p.Files, wikiImportFile{Path: name, Before: before, After: wikiState(file.Content, file.Mode)})
		p.source = append(p.source, slices.Clone(file.Content))
		p.backup = append(p.backup, data)
		p.identities[name] = info
		for parent := path.Dir(name); ; parent = path.Dir(parent) {
			info, err := root.Lstat(parent)
			if err != nil && !errors.Is(err, fs.ErrNotExist) {
				return nil, err
			}
			p.identities[parent] = info
			if parent == "." {
				break
			}
		}
	}
	return p, nil
}

func validateWikiImportLocal(root *os.Root, snapshot *gh.WikiSnapshot) error {
	combined := &gh.WikiSnapshot{Head: snapshot.Head, Files: slices.Clone(snapshot.Files)}
	remote := make(map[string]bool)
	for _, file := range snapshot.Files {
		remote[file.Path] = true
	}
	if _, err := root.Lstat("wiki"); errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	total := 0
	err := fs.WalkDir(root.FS(), "wiki", func(name string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if name == "wiki" || name == swatch.WikiBaseline {
			return nil
		}
		rel := strings.TrimPrefix(name, "wiki/")
		if entry.IsDir() {
			return nil
		}
		if remote[rel] {
			return nil
		}
		state, data, _, err := readWikiImportFile(root, name)
		if err != nil {
			return err
		}
		total += len(data)
		if total > gh.WikiSnapshotMaxBytes {
			return errors.New("local wiki files exceed the size limit")
		}
		if unresolvedWikiConflict(data) {
			return fmt.Errorf("local wiki file %q contains unresolved conflict markers", name)
		}
		mode := fs.FileMode(0o644)
		if state.Mode&0o111 != 0 {
			mode = 0o755
		}
		combined.Files = append(combined.Files, gh.WikiFile{Path: rel, Mode: mode})
		if len(combined.Files) > gh.WikiSnapshotMaxFiles {
			return errors.New("local wiki exceeds the file limit")
		}
		return nil
	})
	if err != nil {
		return err
	}
	return gh.ValidateWikiSnapshot(combined)
}

func unresolvedWikiConflict(data []byte) bool {
	if bytes.Contains(data, []byte{0}) {
		return false
	}
	state, fencedState := 0, 0
	var fence byte
	fenceLength := 0
	for line := range bytes.SplitSeq(data, []byte{'\n'}) {
		line = bytes.TrimSuffix(line, []byte{'\r'})
		marker, length, suffix := wikiMarkdownFence(line)
		if fence != 0 {
			if marker == fence && length >= fenceLength && len(bytes.Trim(suffix, " \t")) == 0 {
				fence = 0
				fencedState = 0
			} else {
				fencedState = wikiConflictState(line, fencedState)
			}
			continue
		}
		if length >= 3 && (marker != '`' || !bytes.ContainsRune(suffix, '`')) {
			fence, fenceLength = marker, length
			fencedState = state
			continue
		}
		state = wikiConflictState(line, state)
		if state == 3 {
			return true
		}
	}
	// Only a matched fence suppresses conflict examples.
	return fencedState == 3
}

func wikiMarkdownFence(line []byte) (byte, int, []byte) {
	trimmed := bytes.TrimLeft(line, " ")
	if len(line)-len(trimmed) > 3 || len(trimmed) == 0 || trimmed[0] != '`' && trimmed[0] != '~' {
		return 0, 0, nil
	}
	length := 0
	for length < len(trimmed) && trimmed[length] == trimmed[0] {
		length++
	}
	return trimmed[0], length, trimmed[length:]
}

func wikiConflictState(line []byte, state int) int {
	if state == 3 {
		return state
	}
	switch {
	case bytes.Equal(line, []byte("<<<<<<<")), bytes.HasPrefix(line, []byte("<<<<<<< ")):
		return 1
	case bytes.Equal(line, []byte("=======")) && state == 1:
		return 2
	case bytes.Equal(line, []byte(">>>>>>>")), bytes.HasPrefix(line, []byte(">>>>>>> ")):
		if state == 2 {
			return 3
		}
		return 0
	default:
		return state
	}
}

func appendWikiImportReporting(report *Report, p *wikiRun, mode ApplyMode) {
	if p == nil {
		return
	}
	if p.enabledViaAPI {
		report.Document.Items = append(report.Document.Items, output.Item{Domain: "Repository", Outcome: output.Applied, Action: "set", Name: "has wiki", After: "true", Provenance: "repository.has_wiki"})
		report.Plain += "set: repository.has_wiki = true\n"
	}
	if p.importPlan != nil {
		plan := p.importPlan
		for _, f := range plan.Files {
			if sameWikiFileState(f.Before, f.After) {
				continue
			}
			outcome, action := output.Alteration, "import"
			if f.Before.Exists {
				action = "replace with backup"
			}
			if mode.ShouldWrite() {
				if !f.Installed {
					continue
				}
				outcome = output.Applied
			}
			report.Document.Items = append(report.Document.Items, output.Item{Domain: "Wiki", Outcome: outcome, Action: action, Name: f.Path, Provenance: f.Path})
			prefix := "would "
			if outcome == output.Applied {
				prefix = ""
			}
			report.Plain += fmt.Sprintf("%s%s: %s\n", prefix, action, termtext.EscapeControlText(f.Path))
		}
		if plan.location != "" {
			text := fmt.Sprintf("Wiki recovery state and backups: %q. Keep these files. Run tailor alter to resume the pinned import.\n", plan.location)
			if plan.Complete {
				text = fmt.Sprintf("Wiki adoption completed. Backups remain at %q.\n", plan.location)
			}
			appendGuidance(report, text, output.GuidanceRequired)
		}
	}
	report.Document.Summary = output.Count(report.Document.Items)
}
