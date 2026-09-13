package gh

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func wikiFixtureGit(t *testing.T, dir string, args ...string) string {
	t.Helper()
	command := exec.CommandContext(t.Context(), "git", append([]string{"-C", dir}, args...)...) // #nosec G204 -- Isolated local Git fixtures.
	command.Env = append(os.Environ(), "GIT_AUTHOR_NAME=Test", "GIT_AUTHOR_EMAIL=test@example.com", "GIT_COMMITTER_NAME=Test", "GIT_COMMITTER_EMAIL=test@example.com")
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v: %s", args, err, output)
	}
	return strings.TrimSpace(string(output))
}

func wikiFixtureWrite(t *testing.T, dir, name, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(filepath.Join(dir, name)), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestInspectWikiAdoption(t *testing.T) {
	remote := t.TempDir()
	wikiFixtureGit(t, remote, "init", "-b", "main")
	var access *WikiAccessError
	if err := inspectWikiGit(t.TempDir(), remote, ""); !errors.As(err, &access) || access.Reason != WikiEmpty || err.Error() != "wiki has no commits" {
		t.Fatalf("empty remote: %v", err)
	}
	wikiFixtureWrite(t, remote, "Home.md", "remote content")
	wikiFixtureWrite(t, remote, "Extra.md", "preserve this page")
	wikiFixtureGit(t, remote, "add", ".")
	wikiFixtureGit(t, remote, "commit", "-m", "Initial wiki")
	head := wikiFixtureGit(t, remote, "rev-parse", "HEAD")
	for _, tc := range []struct {
		name, baseline, want string
		imported             bool
		reason               WikiReadinessReason
	}{
		{"missing baseline", "", "wiki exists but wiki/.tailor-wiki-base is missing; import the wiki before adoption", false, WikiBaselineMissing},
		{"baseline only", head, "wiki import is incomplete: \"Extra.md\" is missing from wiki/; import every remote file before adoption", false, WikiImportIncomplete},
		{"imported with local edits", head, "", true, ""},
		{"mismatched baseline", strings.Repeat("a", 40), "wiki baseline differs from the remote HEAD and the remote changed outside the publisher; import it again", true, WikiChangedOutside},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			if tc.imported {
				wikiFixtureWrite(t, dir, "wiki/Home.md", "local edit")
				wikiFixtureWrite(t, dir, "wiki/Extra.md", "preserve this page")
			}
			err := inspectWikiGit(dir, remote, tc.baseline)
			if tc.want == "" && err != nil || tc.want != "" && (err == nil || err.Error() != tc.want) {
				t.Fatalf("error=%v, want %q", err, tc.want)
			}
			if tc.reason != "" {
				var readiness *WikiReadinessError
				if !errors.As(err, &readiness) || readiness.Reason != tc.reason {
					t.Fatalf("readiness=%+v, want reason %q", readiness, tc.reason)
				}
				if tc.reason == WikiImportIncomplete && readiness.Path != "Extra.md" {
					t.Fatalf("incomplete path=%q", readiness.Path)
				}
			}
		})
	}
}

func TestInspectWikiPublishedHistory(t *testing.T) {
	dir, remote := t.TempDir(), t.TempDir()
	for _, root := range []string{dir, remote} {
		wikiFixtureGit(t, root, "init", "-b", "main")
	}
	wikiFixtureWrite(t, dir, "wiki/Home.md", "published")
	wikiFixtureWrite(t, dir, "wiki/.tailor-wiki-base", strings.Repeat("a", 40))
	wikiFixtureGit(t, dir, "add", ".")
	wikiFixtureGit(t, dir, "commit", "-m", "Source")
	source := wikiFixtureGit(t, dir, "rev-parse", "HEAD")
	wikiFixtureWrite(t, remote, "Home.md", "published")
	wikiFixtureGit(t, remote, "add", ".")
	wikiFixtureGit(t, remote, "commit", "-m", "Publish\n\nTailor-Wiki-Source: "+source)
	wikiFixtureWrite(t, dir, "wiki/Home.md", "normal local edit")
	if err := inspectWikiGit(dir, remote, strings.Repeat("a", 40)); err != nil {
		t.Fatalf("normal edit: %v", err)
	}
	unknown := t.TempDir()
	wikiFixtureGit(t, unknown, "init", "-b", "main")
	var readiness *WikiReadinessError
	if err := inspectWikiGit(unknown, remote, strings.Repeat("a", 40)); !errors.As(err, &readiness) || readiness.Reason != WikiHistoryMissing || err.Error() != "wiki published source is not in local HEAD history; fetch the source history and use the current project branch, or review and reimport the wiki" {
		t.Fatalf("unknown history: %v", err)
	}
	wikiFixtureWrite(t, remote, "Home.md", "independent edit")
	wikiFixtureGit(t, remote, "add", ".")
	wikiFixtureGit(t, remote, "commit", "-m", "Forged\n\nTailor-Wiki-Source: "+source)
	if err := inspectWikiGit(dir, remote, strings.Repeat("a", 40)); !errors.As(err, &readiness) || readiness.Reason != WikiChangedOutside || err.Error() != "wiki remote differs from its published source; import it again" {
		t.Fatalf("forged trailer: %v", err)
	}
}

func TestWikiAccessErrors(t *testing.T) {
	for _, tc := range []struct {
		stderr, want string
		reason       WikiReadinessReason
	}{
		{"Authentication failed: secret", "wiki access was denied; check repository access and retry tailor alter", WikiAccessDenied},
		{"HTTP 403: secret", "wiki access was denied; check repository access and retry tailor alter", WikiAccessDenied},
		{"HTTP 401: secret", "wiki access was denied; check repository access and retry tailor alter", WikiAccessDenied},
		{"Could not resolve host: secret", "wiki remote could not be read; check network, TLS and repository access, then retry tailor alter", WikiReadFailed},
		{"Repository not found: secret", "wiki remote is unavailable; it is not possible to distinguish a missing wiki from denied access", WikiAccessUnknown},
		{"Could not read username: secret", "wiki remote is unavailable; it is not possible to distinguish a missing wiki from denied access", WikiAccessUnknown},
	} {
		err := classifyWikiAccess(context.Background(), &exec.ExitError{Stderr: []byte(tc.stderr)})
		var access *WikiAccessError
		if !errors.As(fmt.Errorf("wrapped: %w", err), &access) || access.Reason != tc.reason || err.Error() != tc.want {
			t.Fatalf("error=%v", err)
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	var access *WikiAccessError
	if err := classifyWikiAccess(ctx, context.DeadlineExceeded); !errors.As(err, &access) || access.Reason != WikiTimedOut || err.Error() != "wiki inspection timed out; check network access and retry tailor alter" {
		t.Fatal(err)
	}
	if err := classifyWikiAccess(context.Background(), exec.ErrNotFound); !errors.As(err, &access) || access.Reason != WikiGitUnavailable || err.Error() != "git could not start; install git and retry tailor alter" {
		t.Fatal(err)
	}
}

func TestInspectWikiFileReadinessReasons(t *testing.T) {
	for _, tc := range []struct {
		name, file, want string
		reason           WikiReadinessReason
		symlink          bool
	}{
		{"unsafe file", "link", "wiki contains an unsafe file; review remote files before adoption", WikiUnsafeFile, true},
		{"unsupported file", ".tailor-wiki-base", "wiki contains an unsupported file; review the remote files before adoption", WikiUnsafeFile, false},
		{"incomplete import", "nested/Guide: \"quoted\".md", "wiki import is incomplete: \"nested/Guide: \\\"quoted\\\".md\" is missing from wiki/; import every remote file before adoption", WikiImportIncomplete, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			remote := t.TempDir()
			wikiFixtureGit(t, remote, "init", "-b", "main")
			if tc.symlink {
				if err := os.Symlink("Home.md", filepath.Join(remote, tc.file)); err != nil {
					t.Fatal(err)
				}
			} else {
				wikiFixtureWrite(t, remote, tc.file, "page")
			}
			wikiFixtureGit(t, remote, "add", ".")
			wikiFixtureGit(t, remote, "commit", "-m", "Wiki")
			head := wikiFixtureGit(t, remote, "rev-parse", "HEAD")
			err := inspectWikiGit(t.TempDir(), remote, head)
			var readiness *WikiReadinessError
			if !errors.As(err, &readiness) || readiness.Reason != tc.reason || err.Error() != tc.want {
				t.Fatalf("error=%v, reason=%+v, want %q (%s)", err, readiness, tc.want, tc.reason)
			}
			if tc.reason == WikiImportIncomplete && readiness.Path != tc.file {
				t.Fatalf("path=%q, want %q", readiness.Path, tc.file)
			}
		})
	}
}

func TestInspectWikiSourceTreeReadinessReasons(t *testing.T) {
	for _, unsafe := range []bool{false, true} {
		t.Run(fmt.Sprintf("unsafe=%t", unsafe), func(t *testing.T) {
			dir, remote := t.TempDir(), t.TempDir()
			for _, root := range []string{dir, remote} {
				wikiFixtureGit(t, root, "init", "-b", "main")
			}
			wikiFixtureWrite(t, dir, "README.md", "project")
			wantReason := WikiHistoryMissing
			want := "wiki published source tree is unavailable; fetch the source history or review and reimport the wiki"
			if unsafe {
				wikiFixtureWrite(t, dir, "wiki/Home.md", "published")
				if err := os.Symlink("Home.md", filepath.Join(dir, "wiki/link")); err != nil {
					t.Fatal(err)
				}
				wantReason = WikiUnsafeFile
				want = "wiki published source contains an unsafe file; review and reimport the wiki"
			}
			wikiFixtureGit(t, dir, "add", ".")
			wikiFixtureGit(t, dir, "commit", "-m", "Source")
			source := wikiFixtureGit(t, dir, "rev-parse", "HEAD")
			wikiFixtureWrite(t, remote, "Home.md", "published")
			wikiFixtureGit(t, remote, "add", ".")
			wikiFixtureGit(t, remote, "commit", "-m", "Publish\n\nTailor-Wiki-Source: "+source)
			err := inspectWikiGit(dir, remote, strings.Repeat("a", 40))
			var readiness *WikiReadinessError
			if !errors.As(err, &readiness) || readiness.Reason != wantReason || err.Error() != want {
				t.Fatalf("error=%v, reason=%+v, want %q (%s)", err, readiness, want, wantReason)
			}
		})
	}
}

func TestWikiReadinessErrorUnwrap(t *testing.T) {
	cause := &os.PathError{Op: "read", Path: "wiki", Err: os.ErrPermission}
	err := &WikiReadinessError{Reason: WikiInspectionFailed, Err: fmt.Errorf("checking wiki: %w", cause)}
	wrapped := fmt.Errorf("wrapped: %w", err)
	var readiness *WikiReadinessError
	var pathErr *os.PathError
	if !errors.As(wrapped, &readiness) || readiness != err || !errors.As(wrapped, &pathErr) || pathErr != cause || !errors.Is(wrapped, os.ErrPermission) {
		t.Fatalf("error chain lost: %v", wrapped)
	}
	if err.Error() != "checking wiki: "+cause.Error() {
		t.Fatalf("message changed: %v", err)
	}
}

func TestSafeWikiTree(t *testing.T) {
	for _, tc := range []struct {
		entry string
		want  bool
	}{
		{"100644 blob abc\tHome.md\x00", true},
		{"100755 blob abc\tnested/page.md\x00", true},
		{"120000 blob abc\tlink\x00", false},
		{"160000 commit abc\tsubmodule\x00", false},
		{"100644 blob abc\tnested/.GIT/config\x00", false},
	} {
		if got := safeWikiTree([]byte(tc.entry)); got != tc.want {
			t.Errorf("safeWikiTree(%q)=%t, want %t", tc.entry, got, tc.want)
		}
	}
}
