package gh

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

var inspectWiki = inspectWikiGit

// WikiAccessError separates remote availability from adoption problems.
type WikiAccessError struct {
	State string
}

func (e *WikiAccessError) Error() string { return e.State }

// InspectWiki checks adoption and publication history without changing the project or remote.
func InspectWiki(dir, remoteURL, baseline string) error {
	return inspectWiki(dir, remoteURL, baseline)
}

var wikiSourceTrailer = regexp.MustCompile(`(?m)^Tailor-Wiki-Source: ([0-9a-f]{40})$`)

func inspectWikiGit(dir, remoteURL, baseline string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	temporary, err := os.MkdirTemp("", "tailor-wiki-check-")
	if err != nil {
		return fmt.Errorf("creating temporary wiki inspection directory: %w", err)
	}
	defer os.RemoveAll(temporary)
	remote := filepath.Join(temporary, "wiki.git")
	if _, err := wikiGit(ctx, temporary, "clone", "--bare", "--", remoteURL, remote); err != nil {
		return classifyWikiAccess(ctx, err)
	}
	head, err := wikiGit(ctx, remote, "rev-parse", "--verify", "HEAD")
	if err != nil {
		refs, refsErr := wikiGit(ctx, remote, "for-each-ref", "--format=%(refname)")
		if refsErr == nil && len(bytes.TrimSpace(refs)) == 0 {
			return &WikiAccessError{State: "wiki has no commits"}
		}
		return errors.New("wiki default branch cannot be inspected; check the wiki repository HEAD")
	}
	if baseline == "" {
		return errors.New("wiki exists but wiki/.tailor-wiki-base is missing; import the wiki before adoption")
	}
	remoteTree, err := wikiGit(ctx, remote, "ls-tree", "-r", "-z", "HEAD")
	if err != nil {
		return errors.New("wiki tree cannot be inspected")
	}
	if !safeWikiTree(remoteTree) {
		return errors.New("wiki contains an unsafe file; review remote files before adoption")
	}
	if strings.TrimSpace(string(head)) == baseline {
		root, err := os.OpenRoot(dir)
		if err != nil {
			return err
		}
		defer root.Close()
		for _, entry := range bytes.Split(remoteTree, []byte{0}) {
			if len(entry) == 0 {
				continue
			}
			metadata, name, ok := bytes.Cut(entry, []byte{'\t'})
			if !ok || (!bytes.HasPrefix(metadata, []byte("100644 blob ")) && !bytes.HasPrefix(metadata, []byte("100755 blob "))) || strings.EqualFold(string(name), ".tailor-wiki-base") {
				return errors.New("wiki contains an unsupported file; review the remote files before adoption")
			}
			info, err := root.Lstat("wiki/" + string(name))
			if err != nil || !info.Mode().IsRegular() {
				return fmt.Errorf("wiki import is incomplete: %q is missing from wiki/; import every remote file before adoption", name)
			}
		}
		return nil
	}
	message, err := wikiGit(ctx, remote, "log", "-1", "--format=%B")
	if err != nil {
		return errors.New("wiki history cannot be inspected")
	}
	previous := wikiSourceTrailer.FindAllSubmatch(message, -1)
	if len(previous) != 1 {
		return errors.New("wiki baseline differs from the remote HEAD and the remote changed outside the publisher; import it again")
	}
	source := string(previous[0][1])
	if _, err := wikiGit(ctx, dir, "merge-base", "--is-ancestor", source, "HEAD"); err != nil {
		return errors.New("wiki published source is not in local HEAD history; fetch the source history and use the current project branch, or review and reimport the wiki")
	}
	sourceTree, err := wikiGit(ctx, dir, "ls-tree", "-r", "-z", source+":wiki")
	if err != nil {
		return errors.New("wiki published source tree is unavailable; fetch the source history or review and reimport the wiki")
	}
	if !safeWikiTree(sourceTree) {
		return errors.New("wiki published source contains an unsafe file; review and reimport the wiki")
	}
	var filtered []byte
	for _, entry := range bytes.Split(sourceTree, []byte{0}) {
		if len(entry) == 0 {
			continue
		}
		_, name, _ := bytes.Cut(entry, []byte{'\t'})
		if string(name) != ".tailor-wiki-base" {
			filtered = append(filtered, entry...)
			filtered = append(filtered, 0)
		}
	}
	if !bytes.Equal(remoteTree, filtered) {
		return errors.New("wiki remote differs from its published source; import it again")
	}
	return nil
}

func safeWikiTree(tree []byte) bool {
	for _, entry := range bytes.Split(tree, []byte{0}) {
		if len(entry) == 0 {
			continue
		}
		metadata, name, ok := bytes.Cut(entry, []byte{'\t'})
		if !ok || (!bytes.HasPrefix(metadata, []byte("100644 blob ")) && !bytes.HasPrefix(metadata, []byte("100755 blob "))) {
			return false
		}
		for _, component := range strings.Split(string(name), "/") {
			if strings.EqualFold(component, ".git") {
				return false
			}
		}
	}
	return true
}

func classifyWikiAccess(ctx context.Context, err error) error {
	if ctx.Err() != nil {
		return &WikiAccessError{State: "wiki inspection timed out; check network access and retry tailor alter"}
	}
	var exitErr *exec.ExitError
	if !errors.As(err, &exitErr) {
		return &WikiAccessError{State: "git could not start; install git and retry tailor alter"}
	}
	message := strings.ToLower(string(exitErr.Stderr))
	switch {
	case strings.Contains(message, "authentication failed"), strings.Contains(message, "403"), strings.Contains(message, "401"):
		return &WikiAccessError{State: "wiki access was denied; check repository access and retry tailor alter"}
	case strings.Contains(message, "not found"), strings.Contains(message, "could not read username"):
		return &WikiAccessError{State: "wiki remote is unavailable; it is not possible to distinguish a missing wiki from denied access"}
	default:
		return &WikiAccessError{State: "wiki remote could not be read; check network, TLS and repository access, then retry tailor alter"}
	}
}

func wikiGit(ctx context.Context, dir string, args ...string) ([]byte, error) {
	command := exec.CommandContext(ctx, "git", append([]string{"-c", "credential.helper=", "-c", "core.askPass=", "-C", dir}, args...)...) // #nosec G204 -- Fixed Git operations, separate arguments, no shell.
	command.WaitDelay = 2 * time.Second
	for _, entry := range os.Environ() {
		if !strings.HasPrefix(entry, "GIT_") {
			command.Env = append(command.Env, entry)
		}
	}
	command.Env = append(command.Env, "GIT_TERMINAL_PROMPT=0", "GIT_CONFIG_NOSYSTEM=1", "GIT_CONFIG_GLOBAL="+os.DevNull)
	return command.Output()
}
