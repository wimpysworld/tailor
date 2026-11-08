package gh

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"golang.org/x/text/unicode/norm"
)

const (
	WikiSnapshotMaxFiles     = 10000
	WikiSnapshotMaxBlobBytes = 16 << 20
	WikiSnapshotMaxBytes     = 64 << 20
	WikiSnapshotMaxTreeBytes = 8 << 20
	WikiSnapshotMaxPathBytes = 4096
	WikiSnapshotMaxPathDepth = 64
)

type WikiFile struct {
	Path    string
	Mode    fs.FileMode
	Content []byte
}

type WikiSnapshot struct {
	Head      string
	Files     []WikiFile
	Readiness error
}

var wikiObjectID = regexp.MustCompile(`^[0-9a-f]{40}$`)

var (
	readWikiSnapshot = readWikiSnapshotGit
	recheckWikiHead  = recheckWikiHeadGit
)

// ReadWikiSnapshot reads tracked files without checkout filters or project writes.
// Readiness contains adoption blockers, not remote access or unsafe import errors.
func ReadWikiSnapshot(dir, remoteURL, baseline string, repository WikiRepository) (*WikiSnapshot, error) {
	snapshot, err := readWikiSnapshot(dir, remoteURL, baseline)
	if err != nil {
		return nil, confirmEmptyWiki(err, repository)
	}
	if err := ValidateWikiSnapshot(snapshot); err != nil {
		return nil, err
	}
	return snapshot, nil
}

// ValidateWikiSnapshot also validates snapshots supplied by test hooks or recovery data.
func ValidateWikiSnapshot(snapshot *WikiSnapshot) error {
	if snapshot == nil || !wikiObjectID.MatchString(snapshot.Head) || len(snapshot.Files) == 0 || len(snapshot.Files) > WikiSnapshotMaxFiles {
		return wikiSnapshotError(WikiUnsafeFile, "", "invalid or empty wiki snapshot")
	}
	paths := make(map[string]string)
	total := 0
	for _, file := range snapshot.Files {
		if err := validateWikiSnapshotPath(file.Path, paths); err != nil {
			return err
		}
		if file.Mode != 0o644 && file.Mode != 0o755 {
			return wikiSnapshotError(WikiUnsafeFile, file.Path, "unsupported wiki file mode for %q", file.Path)
		}
		if len(file.Content) > WikiSnapshotMaxBlobBytes || len(file.Content) > WikiSnapshotMaxBytes-total {
			return wikiSnapshotError(WikiUnsafeFile, file.Path, "wiki snapshot exceeds size limits")
		}
		total += len(file.Content)
	}
	return nil
}

// RecheckWikiHead rejects a remote that no longer has the expected default HEAD.
func RecheckWikiHead(remoteURL, expected string) error {
	return recheckWikiHead(remoteURL, expected)
}

func readWikiSnapshotGit(dir, remoteURL, baseline string) (*WikiSnapshot, error) {
	var snapshot *WikiSnapshot
	err := withWikiRemote(remoteURL, func(ctx context.Context, remote, head string) error {
		tree := &wikiLimitedBuffer{limit: WikiSnapshotMaxTreeBytes}
		command := wikiGitCommand(ctx, remote, "ls-tree", "-r", "-z", head)
		command.Stdout = tree
		if err := command.Run(); err != nil {
			if tree.exceeded {
				return wikiSnapshotError(WikiReadFailed, "", "wiki tree exceeds the %d-byte limit", WikiSnapshotMaxTreeBytes)
			}
			return classifyWikiAccess(ctx, err)
		}
		files, objects, err := parseWikiSnapshotTree(tree.buffer.Bytes())
		if err != nil {
			return err
		}
		if err := readWikiBlobs(ctx, remote, files, objects); err != nil {
			return err
		}
		readiness := inspectWikiClone(ctx, dir, remote, head, baseline)
		if ctx.Err() != nil {
			return classifyWikiAccess(ctx, ctx.Err())
		}
		var blocker *WikiReadinessError
		if readiness != nil && (!errors.As(readiness, &blocker) || blocker.Reason == WikiInspectionFailed) {
			return readiness
		}
		snapshot = &WikiSnapshot{Head: head, Files: files, Readiness: readiness}
		return nil
	})
	return snapshot, err
}

func recheckWikiHeadGit(remoteURL, expected string) error {
	if !wikiObjectID.MatchString(expected) {
		return wikiSnapshotError(WikiInspectionFailed, "", "wiki expected HEAD is invalid")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	temporary, err := os.MkdirTemp("", "tailor-wiki-check-")
	if err != nil {
		return fmt.Errorf("creating temporary wiki inspection directory: %w", err)
	}
	defer os.RemoveAll(temporary)
	head, err := readWikiRemoteHead(ctx, temporary, remoteURL)
	if err != nil {
		return err
	}
	if head != expected {
		return wikiSnapshotError(WikiChangedOutside, "", "wiki HEAD changed during adoption; review the current wiki and retry")
	}
	return nil
}

func readWikiRemoteHead(ctx context.Context, dir, remoteURL string) (string, error) {
	scratch, err := filepath.Abs(dir)
	if err != nil {
		return "", wikiSnapshotError(WikiInspectionFailed, "", "cannot isolate wiki reference inspection")
	}
	scratch, err = filepath.EvalSymlinks(scratch)
	if err != nil {
		return "", wikiSnapshotError(WikiInspectionFailed, "", "cannot isolate wiki reference inspection")
	}
	ceiling := filepath.Dir(scratch)
	if strings.ContainsRune(ceiling, os.PathListSeparator) {
		return "", wikiSnapshotError(WikiInspectionFailed, "", "wiki temporary path cannot form a Git discovery ceiling")
	}
	refs := &wikiLimitedBuffer{limit: WikiSnapshotMaxTreeBytes}
	diagnostics := &wikiLimitedBuffer{limit: 64 << 10}
	// All refs distinguish an empty wiki from a nonempty wiki with a dangling HEAD.
	command := wikiGitCommand(ctx, scratch, "ls-remote", "--", remoteURL)
	// Git ignores a ceiling equal to its starting directory, so exclude its parent.
	command.Env = append(command.Env, "GIT_CEILING_DIRECTORIES="+ceiling)
	command.Stdout, command.Stderr = refs, diagnostics
	err = command.Run()
	if ctx.Err() != nil {
		return "", classifyWikiAccess(ctx, ctx.Err())
	}
	if refs.exceeded || diagnostics.exceeded {
		return "", wikiSnapshotError(WikiReadFailed, "", "wiki reference output exceeds its byte limit")
	}
	if err != nil {
		if exitErr, ok := errors.AsType[*exec.ExitError](err); ok {
			exitErr.Stderr = diagnostics.buffer.Bytes()
		}
		return "", classifyWikiAccess(ctx, err)
	}
	return parseWikiRemoteHead(refs.buffer.Bytes())
}

func parseWikiRemoteHead(refs []byte) (string, error) {
	inspectionFailed := func() (string, error) {
		return "", wikiSnapshotError(WikiInspectionFailed, "", "wiki default branch cannot be inspected; check the wiki repository HEAD")
	}
	if len(refs) > WikiSnapshotMaxTreeBytes {
		return "", wikiSnapshotError(WikiReadFailed, "", "wiki reference output exceeds its byte limit")
	}
	if len(refs) == 0 {
		return "", &WikiAccessError{Reason: WikiEmpty, State: "wiki has no commits"}
	}
	if refs[len(refs)-1] != '\n' {
		return inspectionFailed()
	}
	head := ""
	seen := make(map[string]bool)
	for line := range bytes.SplitSeq(refs[:len(refs)-1], []byte{'\n'}) {
		object, name, ok := bytes.Cut(line, []byte{'\t'})
		ref := string(name)
		if !ok || !wikiObjectID.Match(object) || !validWikiRemoteRef(ref) || seen[ref] {
			return inspectionFailed()
		}
		seen[ref] = true
		if ref == "HEAD" {
			head = string(object)
		}
	}
	if head == "" {
		return inspectionFailed()
	}
	return head, nil
}

func validWikiRemoteRef(ref string) bool {
	if ref == "HEAD" {
		return true
	}
	ref = strings.TrimSuffix(ref, "^{}")
	if !strings.HasPrefix(ref, "refs/") || strings.HasSuffix(ref, ".") || strings.Contains(ref, "..") || strings.Contains(ref, "@{") ||
		strings.ContainsAny(ref, "~^:?*[\\") || strings.ContainsFunc(ref, func(r rune) bool { return r <= ' ' || r == 0x7f }) {
		return false
	}
	for component := range strings.SplitSeq(ref, "/") {
		if component == "" || strings.HasPrefix(component, ".") || strings.HasSuffix(component, ".lock") {
			return false
		}
	}
	return true
}

func wikiSnapshotError(reason WikiReadinessReason, path, format string, args ...any) error {
	return &WikiReadinessError{Reason: reason, Path: path, Err: fmt.Errorf(format, args...)}
}

type wikiLimitedBuffer struct {
	buffer   bytes.Buffer
	limit    int
	exceeded bool
}

func (b *wikiLimitedBuffer) Write(data []byte) (int, error) {
	if len(data) > b.limit-b.buffer.Len() {
		b.exceeded = true
		return 0, errors.New("wiki output exceeds its byte limit")
	}
	return b.buffer.Write(data)
}

func parseWikiSnapshotTree(tree []byte) ([]WikiFile, []string, error) {
	if len(tree) > WikiSnapshotMaxTreeBytes {
		return nil, nil, wikiSnapshotError(WikiReadFailed, "", "wiki tree exceeds the %d-byte limit", WikiSnapshotMaxTreeBytes)
	}
	if len(tree) > 0 && tree[len(tree)-1] != 0 {
		return nil, nil, wikiSnapshotError(WikiReadFailed, "", "wiki tree is incomplete")
	}
	var files []WikiFile
	var objects []string
	paths := make(map[string]string)
	for entry := range bytes.SplitSeq(tree, []byte{0}) {
		if len(entry) == 0 {
			continue
		}
		if len(files) == WikiSnapshotMaxFiles {
			return nil, nil, wikiSnapshotError(WikiReadFailed, "", "wiki exceeds the %d-file limit", WikiSnapshotMaxFiles)
		}
		metadata, name, ok := bytes.Cut(entry, []byte{'\t'})
		fields := strings.Fields(string(metadata))
		path := string(name)
		if !ok || len(fields) != 3 || fields[1] != "blob" || (fields[0] != "100644" && fields[0] != "100755") {
			return nil, nil, wikiSnapshotError(WikiUnsafeFile, path, "wiki file %q has an unsupported Git mode or type", path)
		}
		if err := validateWikiSnapshotPath(path, paths); err != nil {
			return nil, nil, err
		}
		mode := fs.FileMode(0o644)
		if fields[0] == "100755" {
			mode = 0o755
		}
		files = append(files, WikiFile{Path: path, Mode: mode})
		objects = append(objects, fields[2])
	}
	return files, objects, nil
}

func validateWikiSnapshotPath(path string, paths map[string]string) error {
	if path == "" || len(path) > WikiSnapshotMaxPathBytes || !utf8.ValidString(path) {
		return wikiSnapshotError(WikiUnsafeFile, path, "wiki file %q has an invalid path", path)
	}
	components := strings.Split(path, "/")
	if len(components) > WikiSnapshotMaxPathDepth {
		return wikiSnapshotError(WikiUnsafeFile, path, "wiki file %q exceeds the %d-component path limit", path, WikiSnapshotMaxPathDepth)
	}
	for i, component := range components {
		lower := strings.ToLower(component)
		invalid := component == "" || len(component) > 255 || component == "." || component == ".." ||
			strings.HasSuffix(component, ".") || strings.HasSuffix(component, " ") ||
			strings.ContainsAny(component, "\\:<>\"|?*\x00") ||
			strings.ContainsFunc(component, func(r rune) bool { return unicode.IsControl(r) || unicode.Is(unicode.Cf, r) }) ||
			lower == ".git" || lower == "git~1" || strings.HasPrefix(lower, ".tailor-wiki-")
		stem, _, _ := strings.Cut(strings.ToUpper(component), ".")
		switch stem {
		case "CON", "PRN", "AUX", "NUL", "CLOCK$", "CONIN$", "CONOUT$",
			"COM1", "COM2", "COM3", "COM4", "COM5", "COM6", "COM7", "COM8", "COM9",
			"LPT1", "LPT2", "LPT3", "LPT4", "LPT5", "LPT6", "LPT7", "LPT8", "LPT9",
			"COM¹", "COM²", "COM³", "LPT¹", "LPT²", "LPT³":
			invalid = true
		}
		if invalid {
			return wikiSnapshotError(WikiUnsafeFile, path, "wiki file %q has an unsafe or reserved path", path)
		}
		prefix := strings.Join(components[:i+1], "/")
		key := strings.Map(func(r rune) rune {
			minimum := r
			for folded := unicode.SimpleFold(r); folded != r; folded = unicode.SimpleFold(folded) {
				if folded < minimum {
					minimum = folded
				}
			}
			return minimum
		}, norm.NFC.String(prefix))
		value := prefix
		if i < len(components)-1 {
			value = prefix + "/"
		}
		if previous, exists := paths[key]; exists && (previous != value || i == len(components)-1) {
			return wikiSnapshotError(WikiUnsafeFile, path, "wiki path %q collides with %q on another platform", path, previous)
		}
		paths[key] = value
	}
	return nil
}

func readWikiBlobs(ctx context.Context, remote string, files []WikiFile, objects []string) error {
	if len(files) == 0 {
		return nil
	}
	command := wikiGitCommand(ctx, remote, "cat-file", "--batch")
	command.Stdin = strings.NewReader(strings.Join(objects, "\n") + "\n")
	stdout, err := command.StdoutPipe()
	if err != nil {
		return wikiSnapshotError(WikiReadFailed, "", "cannot open wiki blob reader")
	}
	if err := command.Start(); err != nil {
		return classifyWikiAccess(ctx, err)
	}
	defer func() {
		_ = command.Process.Kill()
		_ = command.Wait()
	}()
	if err := decodeWikiBlobs(bufio.NewReader(stdout), files, objects); err != nil {
		if ctx.Err() != nil {
			return classifyWikiAccess(ctx, ctx.Err())
		}
		return err
	}
	if err := command.Wait(); err != nil {
		return classifyWikiAccess(ctx, err)
	}
	return nil
}

func decodeWikiBlobs(reader *bufio.Reader, files []WikiFile, objects []string) error {
	total := int64(0)
	for i := range files {
		header, err := reader.ReadSlice('\n')
		fields := strings.Fields(string(header))
		if err != nil || len(fields) != 3 || fields[0] != objects[i] || fields[1] != "blob" {
			return wikiSnapshotError(WikiReadFailed, files[i].Path, "cannot read wiki blob %q", files[i].Path)
		}
		size, err := strconv.ParseInt(fields[2], 10, 64)
		if err != nil || size < 0 {
			return wikiSnapshotError(WikiReadFailed, files[i].Path, "wiki blob %q has an invalid size", files[i].Path)
		}
		if size > WikiSnapshotMaxBlobBytes {
			return wikiSnapshotError(WikiReadFailed, files[i].Path, "wiki blob %q exceeds the %d-byte limit", files[i].Path, WikiSnapshotMaxBlobBytes)
		}
		if size > WikiSnapshotMaxBytes-total {
			return wikiSnapshotError(WikiReadFailed, files[i].Path, "wiki exceeds the %d-byte total limit", WikiSnapshotMaxBytes)
		}
		files[i].Content = make([]byte, size)
		if _, err := io.ReadFull(reader, files[i].Content); err != nil {
			return wikiSnapshotError(WikiReadFailed, files[i].Path, "cannot read wiki blob %q", files[i].Path)
		}
		if terminator, err := reader.ReadByte(); err != nil || terminator != '\n' {
			return wikiSnapshotError(WikiReadFailed, files[i].Path, "wiki blob %q is incomplete", files[i].Path)
		}
		total += size
	}
	return nil
}
