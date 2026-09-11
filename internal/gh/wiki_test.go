package gh

import (
	"context"
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
	if err := inspectWikiGit(t.TempDir(), remote, ""); err == nil || !strings.Contains(err.Error(), "no commits") {
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
	}{
		{"missing baseline", "", "missing", false},
		{"baseline only", head, "incomplete", false},
		{"imported with local edits", head, "", true},
		{"mismatched baseline", strings.Repeat("a", 40), "outside the publisher", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			if tc.imported {
				wikiFixtureWrite(t, dir, "wiki/Home.md", "local edit")
				wikiFixtureWrite(t, dir, "wiki/Extra.md", "preserve this page")
			}
			err := inspectWikiGit(dir, remote, tc.baseline)
			if tc.want == "" && err != nil || tc.want != "" && (err == nil || !strings.Contains(err.Error(), tc.want)) {
				t.Fatalf("error=%v, want %q", err, tc.want)
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
	if err := inspectWikiGit(unknown, remote, strings.Repeat("a", 40)); err == nil || !strings.Contains(err.Error(), "local HEAD history") {
		t.Fatalf("unknown history: %v", err)
	}
	wikiFixtureWrite(t, remote, "Home.md", "independent edit")
	wikiFixtureGit(t, remote, "add", ".")
	wikiFixtureGit(t, remote, "commit", "-m", "Forged\n\nTailor-Wiki-Source: "+source)
	if err := inspectWikiGit(dir, remote, strings.Repeat("a", 40)); err == nil || !strings.Contains(err.Error(), "differs from its published source") {
		t.Fatalf("forged trailer: %v", err)
	}
}

func TestWikiAccessErrors(t *testing.T) {
	for _, tc := range []struct{ stderr, want string }{
		{"Authentication failed: secret", "access was denied"},
		{"Could not resolve host: secret", "network, TLS"},
		{"Repository not found: secret", "distinguish"},
	} {
		err := classifyWikiAccess(context.Background(), &exec.ExitError{Stderr: []byte(tc.stderr)})
		if !strings.Contains(err.Error(), tc.want) || strings.Contains(err.Error(), "secret") {
			t.Fatalf("error=%v", err)
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := classifyWikiAccess(ctx, context.DeadlineExceeded); !strings.Contains(err.Error(), "timed out") {
		t.Fatal(err)
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
