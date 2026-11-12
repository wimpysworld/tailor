package gh

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestReadWikiSnapshotFidelity(t *testing.T) {
	remote, project := t.TempDir(), t.TempDir()
	wikiFixtureGit(t, remote, "init", "-b", "main")
	contents := map[string]string{
		"Home.md":         "page\r\n$Format:%H$\n",
		"assets/icon.bin": "\x00\xff\xfe\r\n\x00",
		".hidden":         "hidden file\n",
		"nested/run.sh":   "#!/bin/sh\nexit 0\n",
		".gitattributes":  "Home.md export-subst\nassets/* export-ignore\n*.bin filter=missing\n",
		"empty":           "",
		"Café/日本語.md":     "unicode page\n",
		"Café/another.md": "same directory\n",
	}
	for name, content := range contents {
		wikiFixtureWrite(t, remote, name, content)
	}
	wikiFixtureGit(t, remote, "add", ".")
	wikiFixtureGit(t, remote, "update-index", "--chmod=+x", "nested/run.sh")
	wikiFixtureGit(t, remote, "commit", "-m", "Wiki files")
	head := wikiFixtureGit(t, remote, "rev-parse", "HEAD")
	bare := filepath.Join(t.TempDir(), "wiki.git")
	wikiFixtureGit(t, remote, "clone", "--bare", "--", remote, bare)
	snapshot, err := ReadWikiSnapshot(project, bare, "", WikiRepository{})
	if err != nil {
		t.Fatal(err)
	}
	var readiness *WikiReadinessError
	if !errors.As(snapshot.Readiness, &readiness) || readiness.Reason != WikiBaselineMissing {
		t.Fatalf("readiness=%v", snapshot.Readiness)
	}
	if snapshot.Head != head || len(snapshot.Files) != len(contents) {
		t.Fatalf("snapshot head=%q, files=%d", snapshot.Head, len(snapshot.Files))
	}
	for _, file := range snapshot.Files {
		want, exists := contents[file.Path]
		if !exists || !bytes.Equal(file.Content, []byte(want)) {
			t.Errorf("file %q content=%q, want %q", file.Path, file.Content, want)
		}
		wantMode := os.FileMode(0o644)
		if file.Path == "nested/run.sh" {
			wantMode = 0o755
		}
		if file.Mode != wantMode {
			t.Errorf("file %q mode=%v, want %v", file.Path, file.Mode, wantMode)
		}
	}
	entries, err := os.ReadDir(project)
	if err != nil || len(entries) != 0 {
		t.Fatalf("project changed: %v, %v", entries, err)
	}
	if got := wikiFixtureGit(t, bare, "rev-parse", "HEAD"); got != head {
		t.Fatalf("remote changed to %q", got)
	}
}

func TestReadWikiSnapshotReadiness(t *testing.T) {
	remote, project := t.TempDir(), t.TempDir()
	wikiFixtureGit(t, remote, "init", "-b", "main")
	wikiFixtureWrite(t, remote, "Home.md", "remote")
	wikiFixtureGit(t, remote, "add", ".")
	wikiFixtureGit(t, remote, "commit", "-m", "Wiki")
	head := wikiFixtureGit(t, remote, "rev-parse", "HEAD")
	for _, tc := range []struct {
		name, baseline string
		imported       bool
		reason         WikiReadinessReason
	}{
		{"missing baseline", "", false, WikiBaselineMissing},
		{"incomplete import", head, false, WikiImportIncomplete},
		{"ready", head, true, ""},
		{"changed outside", strings.Repeat("a", 40), true, WikiChangedOutside},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if tc.imported {
				wikiFixtureWrite(t, project, "wiki/Home.md", "local edit")
			}
			snapshot, err := ReadWikiSnapshot(project, remote, tc.baseline, WikiRepository{})
			if err != nil || snapshot == nil {
				t.Fatalf("snapshot=%v, error=%v", snapshot, err)
			}
			if tc.reason == "" {
				if snapshot.Readiness != nil {
					t.Fatal(snapshot.Readiness)
				}
			} else {
				var readiness *WikiReadinessError
				if !errors.As(snapshot.Readiness, &readiness) || readiness.Reason != tc.reason {
					t.Fatalf("readiness=%v, want %s", snapshot.Readiness, tc.reason)
				}
			}
			if tc.imported {
				content, err := os.ReadFile(filepath.Join(project, "wiki/Home.md"))
				if err != nil || string(content) != "local edit" {
					t.Fatalf("local edit changed: %q, %v", content, err)
				}
			}
		})
	}
}

func TestReadWikiSnapshotPublishedReadiness(t *testing.T) {
	project, remote := t.TempDir(), t.TempDir()
	for _, dir := range []string{project, remote} {
		wikiFixtureGit(t, dir, "init", "-b", "main")
	}
	wikiFixtureWrite(t, project, "wiki/Home.md", "published")
	wikiFixtureWrite(t, project, "wiki/.tailor-wiki-base", strings.Repeat("a", 40))
	wikiFixtureGit(t, project, "add", ".")
	wikiFixtureGit(t, project, "commit", "-m", "Source")
	source := wikiFixtureGit(t, project, "rev-parse", "HEAD")
	wikiFixtureWrite(t, remote, "Home.md", "published")
	wikiFixtureGit(t, remote, "add", ".")
	wikiFixtureGit(t, remote, "commit", "-m", "Publish\n\nTailor-Wiki-Source: "+source)
	wikiFixtureWrite(t, project, "wiki/Home.md", "local edit")
	snapshot, err := ReadWikiSnapshot(project, remote, strings.Repeat("a", 40), WikiRepository{})
	if err != nil || snapshot.Readiness != nil {
		t.Fatalf("snapshot=%+v, error=%v", snapshot, err)
	}
}

func TestWikiSnapshotAvailabilityAndRecheck(t *testing.T) {
	remote := t.TempDir()
	wikiFixtureGit(t, remote, "init", "-b", "main")
	var access *WikiAccessError
	if snapshot, err := ReadWikiSnapshot(t.TempDir(), remote, "", WikiRepository{}); snapshot != nil || !errors.As(err, &access) || access.Reason != WikiEmpty {
		t.Fatalf("empty remote: snapshot=%v, error=%v", snapshot, err)
	}
	if err := RecheckWikiHead(remote, strings.Repeat("a", 40)); !errors.As(err, &access) || access.Reason != WikiEmpty {
		t.Fatalf("empty recheck: %v", err)
	}
	if _, err := ReadWikiSnapshot(t.TempDir(), filepath.Join(t.TempDir(), "missing.git"), "", WikiRepository{}); !errors.As(err, &access) || access.Reason == WikiEmpty {
		t.Fatalf("unavailable remote: %v", err)
	}
	wikiFixtureWrite(t, remote, "Home.md", "first")
	wikiFixtureGit(t, remote, "add", ".")
	wikiFixtureGit(t, remote, "commit", "-m", "First")
	head := wikiFixtureGit(t, remote, "rev-parse", "HEAD")
	if err := RecheckWikiHead(remote, head); err != nil {
		t.Fatal(err)
	}
	wikiFixtureWrite(t, remote, "Home.md", "second")
	wikiFixtureGit(t, remote, "commit", "-am", "Second")
	var readiness *WikiReadinessError
	if err := RecheckWikiHead(remote, head); !errors.As(err, &readiness) || readiness.Reason != WikiChangedOutside {
		t.Fatalf("remote movement: %v", err)
	}
	if got := wikiFixtureGit(t, remote, "show", "HEAD:Home.md"); got != "second" {
		t.Fatalf("remote content changed: %q", got)
	}
}

func TestWikiRemoteHeadMetadata(t *testing.T) {
	head, other := strings.Repeat("a", 40), strings.Repeat("b", 40)
	for _, tc := range []struct {
		name, output, want string
		reason             WikiReadinessReason
	}{
		{"matching HEAD", head + "\tHEAD\n" + head + "\trefs/heads/main\n", head, ""},
		{"detached HEAD", head + "\tHEAD\n", head, ""},
		{"exact HEAD only", other + "\trefs/heads/HEAD\n" + head + "\tHEAD\n", head, ""},
		{"peeled tag", head + "\tHEAD\n" + other + "\trefs/tags/v1\n" + head + "\trefs/tags/v1^{}\n", head, ""},
		{"internal trailing dot", head + "\tHEAD\n" + head + "\trefs/heads/release./main\n", head, ""},
		{"final trailing dot", head + "\tHEAD\n" + head + "\trefs/heads/release/main.\n", "", WikiInspectionFailed},
		{"empty", "", "", WikiEmpty},
		{"dangling HEAD", head + "\trefs/heads/main\n", "", WikiInspectionFailed},
		{"suffix is not HEAD", head + "\trefs/heads/HEAD\n", "", WikiInspectionFailed},
		{"duplicate HEAD", head + "\tHEAD\n" + head + "\tHEAD\n", "", WikiInspectionFailed},
		{"conflicting HEAD", head + "\tHEAD\n" + other + "\tHEAD\n", "", WikiInspectionFailed},
		{"duplicate ref", head + "\tHEAD\n" + head + "\trefs/heads/main\n" + head + "\trefs/heads/main\n", "", WikiInspectionFailed},
		{"short ID", "abc\tHEAD\n", "", WikiInspectionFailed},
		{"uppercase ID", strings.Repeat("A", 40) + "\tHEAD\n", "", WikiInspectionFailed},
		{"truncated", head + "\tHEAD", "", WikiInspectionFailed},
		{"blank line", head + "\tHEAD\n\n", "", WikiInspectionFailed},
		{"spaces", head + " HEAD\n", "", WikiInspectionFailed},
		{"extra field", head + "\tHEAD\textra\n", "", WikiInspectionFailed},
		{"symbolic record", "ref: refs/heads/main\tHEAD\n", "", WikiInspectionFailed},
		{"unsafe ref", head + "\trefs/heads/../HEAD\n", "", WikiInspectionFailed},
		{"control ref", head + "\trefs/heads/main\r\n", "", WikiInspectionFailed},
		{"oversized", strings.Repeat("x", WikiSnapshotMaxTreeBytes+1), "", WikiReadFailed},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := parseWikiRemoteHead([]byte(tc.output))
			if got != tc.want {
				t.Fatalf("HEAD=%q, want %q", got, tc.want)
			}
			if tc.reason == "" {
				if err != nil {
					t.Fatal(err)
				}
				return
			}
			var access *WikiAccessError
			var readiness *WikiReadinessError
			if errors.As(err, &access) && access.Reason == tc.reason || errors.As(err, &readiness) && readiness.Reason == tc.reason {
				return
			}
			t.Fatalf("error=%v, want reason %s", err, tc.reason)
		})
	}
}

func wikiMetadataGit(t *testing.T, script string) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("POSIX shell git stub tests do not run on Windows")
	}
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "git"), []byte("#!/bin/sh\n"+script), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
}

func TestRecheckWikiHeadMetadataCommand(t *testing.T) {
	head := strings.Repeat("a", 40)
	scratchRoot := t.TempDir()
	canonicalScratchRoot, err := filepath.EvalSymlinks(scratchRoot)
	if err != nil {
		t.Fatal(err)
	}
	assertScratchEmpty := func() {
		t.Helper()
		entries, err := os.ReadDir(scratchRoot)
		if err != nil || len(entries) != 0 {
			t.Fatalf("temporary storage remains: %v, %v", entries, err)
		}
	}
	log := filepath.Join(t.TempDir(), "command")
	t.Setenv("TAILOR_TEST_WIKI_COMMAND", log)
	t.Setenv("GIT_CONFIG_COUNT", "1")
	t.Setenv("GIT_DIR", "/untrusted")
	t.Setenv("SSH_AUTH_SOCK", "/untrusted")
	t.Setenv("SSH_ASKPASS", "/untrusted")
	wikiMetadataGit(t, `printf '%s\n' "$@" >> "$TAILOR_TEST_WIKI_COMMAND"
printf '%s\n' "$HOME" "$GIT_TERMINAL_PROMPT" "$GIT_CONFIG_NOSYSTEM" "$GIT_CONFIG_GLOBAL" "$GIT_CEILING_DIRECTORIES" > "$TAILOR_TEST_WIKI_COMMAND.env"
[ -z "${GIT_CONFIG_COUNT+x}${GIT_DIR+x}${SSH_AUTH_SOCK+x}${SSH_ASKPASS+x}" ] || exit 99
printf '%s\tHEAD\n' 'aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa'
`)
	t.Setenv("TMPDIR", scratchRoot)
	remote := "--untrusted-remote"
	if err := recheckWikiHeadGit(remote, head); err != nil {
		t.Fatal(err)
	}
	assertScratchEmpty()
	data, err := os.ReadFile(log)
	if err != nil {
		t.Fatal(err)
	}
	args := strings.Split(strings.TrimSuffix(string(data), "\n"), "\n")
	if len(args) != 15 {
		t.Fatalf("unexpected commands: %q", args)
	}
	temporary := args[11]
	if filepath.Dir(temporary) != canonicalScratchRoot {
		t.Fatalf("temporary parent=%q, want %q", filepath.Dir(temporary), canonicalScratchRoot)
	}
	want := []string{"-c", "credential.helper=", "-c", "core.askPass=", "-c", "protocol.allow=never", "-c", "protocol.https.allow=always", "-c", "protocol.file.allow=always", "-C", temporary, "ls-remote", "--", remote}
	if !reflect.DeepEqual(args, want) {
		t.Fatalf("commands=%q, want %q", args, want)
	}
	data, err = os.ReadFile(log + ".env")
	if err != nil || string(data) != temporary+"\n0\n1\n"+os.DevNull+"\n"+canonicalScratchRoot+"\n" {
		t.Fatalf("anonymous environment=%q, error=%v", data, err)
	}
	if err := recheckWikiHeadGit(remote, strings.Repeat("b", 40)); err == nil {
		t.Fatal("changed HEAD accepted")
	} else {
		var readiness *WikiReadinessError
		if !errors.As(err, &readiness) || readiness.Reason != WikiChangedOutside {
			t.Fatalf("changed HEAD: %v", err)
		}
	}
	assertScratchEmpty()
	before, err := os.ReadFile(log)
	if err != nil {
		t.Fatal(err)
	}
	if err := recheckWikiHeadGit(remote, "HEAD^{commit}"); err == nil {
		t.Fatal("invalid expected ID accepted")
	}
	assertScratchEmpty()
	after, err := os.ReadFile(log)
	if err != nil || !bytes.Equal(before, after) {
		t.Fatalf("invalid ID ran Git: %q, %v", after, err)
	}
}

func TestWikiRemoteHeadParentConfigurationIsolation(t *testing.T) {
	for _, symlinked := range []bool{false, true} {
		t.Run(fmt.Sprintf("symlinked=%t", symlinked), func(t *testing.T) {
			nativeGit, err := exec.LookPath("git")
			if err != nil {
				t.Fatal(err)
			}
			parent, remote, rewritten := t.TempDir(), t.TempDir(), t.TempDir()
			for i, dir := range []string{parent, remote, rewritten} {
				wikiFixtureGit(t, dir, "init", "-b", "main")
				wikiFixtureGit(t, dir, "commit", "--allow-empty", "-m", fmt.Sprintf("Fixture %d", i))
			}
			head := wikiFixtureGit(t, remote, "rev-parse", "HEAD")
			wikiFixtureGit(t, parent, "config", "--local", "url."+rewritten+".insteadOf", remote)
			wikiFixtureGit(t, parent, "config", "--local", "http.extraHeader", "X-Tailor-Test: harmless")
			probe := filepath.Join(t.TempDir(), "headers")
			t.Setenv("TAILOR_TEST_NATIVE_GIT", nativeGit)
			t.Setenv("TAILOR_TEST_HEADER_PROBE", probe)
			wikiMetadataGit(t, `scratch=
while [ "$#" -gt 0 ]; do
  if [ "$1" = -C ]; then scratch=$2; break; fi
  shift
done
[ -n "$scratch" ] || exit 98
"$TAILOR_TEST_NATIVE_GIT" -C "$scratch" config --get-all http.extraHeader > "$TAILOR_TEST_HEADER_PROBE"
status=$?
[ "$status" -eq 1 ] || exit 99
"$TAILOR_TEST_NATIVE_GIT" -C "$scratch" -c protocol.allow=never -c protocol.file.allow=always ls-remote -- "$TAILOR_TEST_REMOTE"
`)
			t.Setenv("TAILOR_TEST_REMOTE", remote)
			temporaryParent := parent
			if symlinked {
				temporaryParent = filepath.Join(t.TempDir(), "linked-parent")
				if err := os.Symlink(parent, temporaryParent); err != nil {
					t.Fatal(err)
				}
			}
			t.Setenv("TMPDIR", temporaryParent)
			if err := recheckWikiHeadGit(remote, head); err != nil {
				t.Fatalf("parent configuration affected metadata: %v", err)
			}
			data, err := os.ReadFile(probe)
			if err != nil || len(data) != 0 {
				t.Fatalf("parent extraheader escaped isolation: %q, %v", data, err)
			}
			entries, err := os.ReadDir(parent)
			if err != nil || len(entries) != 1 || entries[0].Name() != ".git" {
				t.Fatalf("scratch storage remains: %v, %v", entries, err)
			}
		})
	}
}

func TestWikiRemoteHeadRejectsAmbiguousCeiling(t *testing.T) {
	parent := filepath.Join(t.TempDir(), "ambiguous"+string(os.PathListSeparator)+"parent")
	scratch := filepath.Join(parent, "scratch")
	if err := os.MkdirAll(scratch, 0o700); err != nil {
		t.Fatal(err)
	}
	_, err := readWikiRemoteHead(t.Context(), scratch, "unused")
	var readiness *WikiReadinessError
	if !errors.As(err, &readiness) || readiness.Reason != WikiInspectionFailed {
		t.Fatalf("ambiguous discovery ceiling accepted: %v", err)
	}
}

func TestWikiRemoteHeadFailures(t *testing.T) {
	for _, tc := range []struct {
		name, script string
		reason       WikiReadinessReason
	}{
		{"denied", "printf 'Authentication failed: secret' >&2; exit 1\n", WikiAccessDenied},
		{"unknown", "printf 'Repository not found: secret' >&2; exit 1\n", WikiAccessUnknown},
		{"network", "printf 'Could not resolve host: secret' >&2; exit 1\n", WikiReadFailed},
		{"partial failure", "printf 'aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa\\tHEAD\\n'; exit 1\n", WikiReadFailed},
	} {
		t.Run(tc.name, func(t *testing.T) {
			wikiMetadataGit(t, tc.script)
			_, err := readWikiRemoteHead(t.Context(), t.TempDir(), "fixture")
			var access *WikiAccessError
			if !errors.As(err, &access) || access.Reason != tc.reason || strings.Contains(err.Error(), "secret") {
				t.Fatalf("error=%v, want %s without diagnostics", err, tc.reason)
			}
		})
	}
	t.Run("cancelled", func(t *testing.T) {
		ctx, cancel := context.WithCancel(t.Context())
		cancel()
		_, err := readWikiRemoteHead(ctx, t.TempDir(), "fixture")
		var access *WikiAccessError
		if !errors.As(err, &access) || access.Reason != WikiTimedOut {
			t.Fatalf("cancelled query: %v", err)
		}
	})
	t.Run("expired deadline", func(t *testing.T) {
		ctx, cancel := context.WithDeadline(t.Context(), time.Now().Add(-time.Second))
		defer cancel()
		_, err := readWikiRemoteHead(ctx, t.TempDir(), "fixture")
		var access *WikiAccessError
		if !errors.As(err, &access) || access.Reason != WikiTimedOut {
			t.Fatalf("expired query: %v", err)
		}
	})
	t.Run("missing git", func(t *testing.T) {
		t.Setenv("PATH", t.TempDir())
		_, err := readWikiRemoteHead(t.Context(), t.TempDir(), "fixture")
		var access *WikiAccessError
		if !errors.As(err, &access) || access.Reason != WikiGitUnavailable {
			t.Fatalf("missing git: %v", err)
		}
	})
}

func TestWikiRemoteHeadOutputLimits(t *testing.T) {
	for _, stream := range []string{"stdout", "stderr"} {
		t.Run(stream, func(t *testing.T) {
			redirect := ""
			if stream == "stderr" {
				redirect = " >&2"
			}
			wikiMetadataGit(t, "i=0; while [ \"$i\" -lt 32769 ]; do printf '%0256d' 0"+redirect+"; i=$((i+1)); done\n")
			ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
			defer cancel()
			_, err := readWikiRemoteHead(ctx, t.TempDir(), "fixture")
			var readiness *WikiReadinessError
			if !errors.As(err, &readiness) || readiness.Reason != WikiReadFailed || !strings.Contains(err.Error(), "byte limit") {
				t.Fatalf("unbounded %s: %v", stream, err)
			}
		})
	}
}

func TestRecheckWikiHeadDanglingAndUnsafeTransport(t *testing.T) {
	remote := t.TempDir()
	wikiFixtureGit(t, remote, "init", "-b", "main")
	wikiFixtureWrite(t, remote, "Home.md", "page")
	wikiFixtureGit(t, remote, "add", ".")
	wikiFixtureGit(t, remote, "commit", "-m", "Wiki")
	head := wikiFixtureGit(t, remote, "rev-parse", "HEAD")
	wikiFixtureGit(t, remote, "symbolic-ref", "HEAD", "refs/heads/missing")
	var readiness *WikiReadinessError
	if err := recheckWikiHeadGit(remote, head); !errors.As(err, &readiness) || readiness.Reason != WikiInspectionFailed {
		t.Fatalf("nonempty dangling HEAD: %v", err)
	}
	var access *WikiAccessError
	if err := recheckWikiHeadGit(filepath.Join(t.TempDir(), "missing.git"), head); !errors.As(err, &access) || access.Reason == WikiEmpty {
		t.Fatalf("missing remote classified as empty: %v", err)
	}
	marker := filepath.Join(t.TempDir(), "transport-ran")
	t.Setenv("TAILOR_TEST_WIKI_TRANSPORT", marker)
	helper := filepath.Join(t.TempDir(), "transport")
	if err := os.WriteFile(helper, []byte("#!/bin/sh\nprintf forbidden > \"$TAILOR_TEST_WIKI_TRANSPORT\"\nexit 1\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", filepath.Dir(helper)+string(os.PathListSeparator)+os.Getenv("PATH"))
	if err := os.WriteFile(filepath.Join(filepath.Dir(helper), "ssh"), []byte("#!/bin/sh\nprintf forbidden > \"$TAILOR_TEST_WIKI_TRANSPORT\"\nexit 1\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	for _, remote := range []string{"ext::" + helper, "ssh://localhost/wiki.git", "git://localhost/wiki.git"} {
		if err := recheckWikiHeadGit(remote, head); !errors.As(err, &access) || access.Reason != WikiReadFailed {
			t.Fatalf("unsafe transport %q: %v", remote, err)
		}
		if _, err := os.Lstat(marker); !os.IsNotExist(err) {
			t.Fatalf("unsafe transport ran: %v", err)
		}
	}
}

func TestWikiSnapshotUnsafeTrees(t *testing.T) {
	for _, paths := range [][]string{
		{"../escape"},
		{"/absolute"},
		{"a//b"},
		{"./Home.md"},
		{"a/../b"},
		{"a/"},
		{strings.Repeat("x", 256)},
		{strings.Repeat("x", WikiSnapshotMaxPathBytes+1)},
		{strings.Repeat("a/", WikiSnapshotMaxPathDepth) + "page"},
		{".git/config"},
		{"nested/.GiT/config"},
		{"GIT~1/config"},
		{".gi\u200ct/config"},
		{".TAILOR-WIKI-BASE"},
		{"nested/.tailor-wiki-adoption"},
		{".tailor-wiki-transaction"},
		{"C:\\escape"},
		{"bad\\path"},
		{"NUL.txt"},
		{"COM1.md"},
		{"LPT¹"},
		{"bad:name"},
		{"bad?name"},
		{"bad\nname"},
		{"bad\x7fname"},
		{"bad\xffname"},
		{"trailing."},
		{"trailing "},
		{"Home.md", "home.md"},
		{"Assets/a", "assets/b"},
		{"Café.md", "Cafe\u0301.md"},
		{"Σ.md", "ς.md"},
		{"same", "same"},
		{"parent", "parent/child"},
		{"parent/child", "parent"},
	} {
		t.Run(fmt.Sprintf("%q", paths), func(t *testing.T) {
			var tree strings.Builder
			for _, path := range paths {
				fmt.Fprintf(&tree, "100644 blob %s\t%s\x00", strings.Repeat("a", 40), path)
			}
			_, _, err := parseWikiSnapshotTree([]byte(tree.String()))
			var readiness *WikiReadinessError
			if !errors.As(err, &readiness) || readiness.Reason != WikiUnsafeFile {
				t.Fatalf("unsafe paths accepted: %v", err)
			}
		})
	}
	for _, mode := range []string{"120000 blob", "160000 commit", "040000 tree", "100664 blob"} {
		if _, _, err := parseWikiSnapshotTree([]byte(mode + " abc\tunsafe\x00")); err == nil {
			t.Errorf("mode %q accepted", mode)
		}
	}
}

func TestReadWikiSnapshotRejectsUnsafeRemote(t *testing.T) {
	for _, name := range []string{".tailor-wiki-base", ".tailor-wiki-adoption", "link", "submodule", "Home.md"} {
		t.Run(name, func(t *testing.T) {
			remote := t.TempDir()
			wikiFixtureGit(t, remote, "init", "-b", "main")
			wikiFixtureWrite(t, remote, "page.md", "page")
			wikiFixtureGit(t, remote, "add", ".")
			wikiFixtureGit(t, remote, "commit", "-m", "Initial")
			switch name {
			case "link":
				if runtime.GOOS == "windows" {
					t.Skip("POSIX Git symlink staging is unsupported on Windows")
				}
				if err := os.Symlink("page.md", filepath.Join(remote, name)); err != nil {
					t.Fatal(err)
				}
			case "submodule":
				head := wikiFixtureGit(t, remote, "rev-parse", "HEAD")
				wikiFixtureGit(t, remote, "update-index", "--add", "--cacheinfo", "160000,"+head+",submodule")
			case "Home.md":
				// Create case-distinct paths without case-sensitive filesystem writes.
				blob := wikiFixtureGit(t, remote, "rev-parse", "HEAD:page.md")
				for _, path := range []string{"Home.md", "home.md"} {
					wikiFixtureGit(t, remote, "update-index", "--add", "--cacheinfo", "100644,"+blob+","+path)
				}
			default:
				wikiFixtureWrite(t, remote, name, "reserved")
			}
			if name != "submodule" && name != "Home.md" {
				wikiFixtureGit(t, remote, "add", ".")
			}
			wikiFixtureGit(t, remote, "commit", "-m", "Unsafe")
			if name == "Home.md" {
				if paths := wikiFixtureGit(t, remote, "ls-tree", "--name-only", "HEAD"); paths != "Home.md\nhome.md\npage.md" {
					t.Fatalf("case-collision fixture paths=%q", paths)
				}
			}
			snapshot, err := ReadWikiSnapshot(t.TempDir(), remote, "", WikiRepository{})
			var readiness *WikiReadinessError
			if snapshot != nil || !errors.As(err, &readiness) || readiness.Reason != WikiUnsafeFile {
				t.Fatalf("unsafe remote: snapshot=%v, error=%v", snapshot, err)
			}
		})
	}
}

func TestWikiSnapshotLimits(t *testing.T) {
	t.Run("file count", func(t *testing.T) {
		var tree strings.Builder
		for i := range WikiSnapshotMaxFiles + 1 {
			fmt.Fprintf(&tree, "100644 blob abc\tpage-%d\x00", i)
		}
		if _, _, err := parseWikiSnapshotTree([]byte(tree.String())); err == nil || !strings.Contains(err.Error(), "file limit") {
			t.Fatalf("file limit: %v", err)
		}
	})
	t.Run("tree bytes", func(t *testing.T) {
		if _, _, err := parseWikiSnapshotTree(make([]byte, WikiSnapshotMaxTreeBytes+1)); err == nil || !strings.Contains(err.Error(), "byte limit") {
			t.Fatalf("tree limit: %v", err)
		}
		buffer := &wikiLimitedBuffer{limit: 4}
		if _, err := io.Copy(buffer, strings.NewReader("12345")); err == nil || !buffer.exceeded || buffer.buffer.Len() != 0 {
			t.Fatalf("bounded output: %v, %+v", err, buffer)
		}
	})
	t.Run("blob bytes", func(t *testing.T) {
		files := []WikiFile{{Path: "large"}}
		header := fmt.Sprintf("abc blob %d\n", WikiSnapshotMaxBlobBytes+1)
		err := decodeWikiBlobs(bufio.NewReader(strings.NewReader(header)), files, []string{"abc"})
		if err == nil || !strings.Contains(err.Error(), "byte limit") || files[0].Content != nil {
			t.Fatalf("blob limit: %v", err)
		}
	})
	t.Run("total bytes", func(t *testing.T) {
		var readers []io.Reader
		var files []WikiFile
		var objects []string
		blob := make([]byte, WikiSnapshotMaxBlobBytes)
		for i := range WikiSnapshotMaxBytes/WikiSnapshotMaxBlobBytes + 1 {
			name := fmt.Sprintf("file-%d", i)
			files = append(files, WikiFile{Path: name})
			objects = append(objects, name)
			readers = append(readers, strings.NewReader(fmt.Sprintf("%s blob %d\n", name, len(blob))), bytes.NewReader(blob), strings.NewReader("\n"))
		}
		err := decodeWikiBlobs(bufio.NewReader(io.MultiReader(readers...)), files, objects)
		if err == nil || !strings.Contains(err.Error(), "total limit") || files[len(files)-1].Content != nil {
			t.Fatalf("total limit: %v", err)
		}
	})
	t.Run("local oversized blob", func(t *testing.T) {
		remote := t.TempDir()
		wikiFixtureGit(t, remote, "init", "-b", "main")
		wikiFixtureWrite(t, remote, "large.bin", strings.Repeat("x", WikiSnapshotMaxBlobBytes+1))
		wikiFixtureGit(t, remote, "add", ".")
		wikiFixtureGit(t, remote, "commit", "-m", "Large")
		snapshot, err := ReadWikiSnapshot(t.TempDir(), remote, "", WikiRepository{})
		if snapshot != nil || err == nil || !strings.Contains(err.Error(), "byte limit") {
			t.Fatalf("large blob: snapshot=%v, error=%v", snapshot, err)
		}
	})
}

func TestWikiSnapshotMalformedBlobs(t *testing.T) {
	for _, data := range []string{"", "abc missing\n", "other blob 0\n", "abc tree 0\n", "abc blob -1\n", "abc blob invalid\n", "abc blob 2\nx", "abc blob 1\nx!", strings.Repeat("x", 8192)} {
		if err := decodeWikiBlobs(bufio.NewReader(strings.NewReader(data)), []WikiFile{{Path: "file"}}, []string{"abc"}); err == nil {
			t.Fatalf("malformed blob accepted: %q", data)
		}
	}
}

func TestWikiSnapshotGitIsolation(t *testing.T) {
	remote := t.TempDir()
	wikiFixtureGit(t, remote, "init", "-b", "main")
	wikiFixtureWrite(t, remote, "Home.md", "page")
	wikiFixtureGit(t, remote, "add", ".")
	wikiFixtureGit(t, remote, "commit", "-m", "Wiki")
	home := t.TempDir()
	wikiFixtureWrite(t, home, ".gitconfig", "invalid global configuration")
	t.Setenv("HOME", home)
	t.Setenv("GIT_CONFIG_COUNT", "1")
	t.Setenv("GIT_CONFIG_KEY_0", "protocol.file.allow")
	t.Setenv("GIT_CONFIG_VALUE_0", "never")
	t.Setenv("GIT_DIR", filepath.Join(t.TempDir(), "missing"))
	snapshot, err := ReadWikiSnapshot(t.TempDir(), remote, "", WikiRepository{})
	if err != nil || len(snapshot.Files) != 1 {
		t.Fatalf("configuration isolation failed: snapshot=%v, error=%v", snapshot, err)
	}
	if err := recheckWikiHeadGit(remote, snapshot.Head); err != nil {
		t.Fatalf("recheck configuration isolation failed: %v", err)
	}
	command := wikiGitCommand(t.Context(), remote, "rev-parse", "HEAD")
	for _, entry := range command.Env {
		if strings.HasPrefix(entry, "GIT_CONFIG_COUNT=") || strings.HasPrefix(entry, "GIT_DIR=") || entry == "HOME="+home {
			t.Fatalf("unsafe inherited environment: %q", entry)
		}
	}
	for _, setting := range []string{"credential.helper=", "core.askPass=", "protocol.allow=never", "protocol.https.allow=always", "protocol.file.allow=always"} {
		found := false
		for _, arg := range command.Args {
			found = found || arg == setting
		}
		if !found {
			t.Errorf("missing isolation setting %q", setting)
		}
	}
}

func TestWikiSnapshotHooks(t *testing.T) {
	restoreInspect := SetInspectWikiFunc(func(string, string, string) error { return nil })
	defer restoreInspect()
	if err := InspectWiki("", "", "", WikiRepository{}); err != nil {
		t.Fatal(err)
	}
	if _, err := ReadWikiSnapshot("", "", "", WikiRepository{}); err == nil || !strings.Contains(err.Error(), "test hook is required") {
		t.Fatalf("legacy snapshot isolation: %v", err)
	}
	if err := RecheckWikiHead("", ""); err == nil || !strings.Contains(err.Error(), "test hook is required") {
		t.Fatalf("legacy recheck isolation: %v", err)
	}
	want := &WikiSnapshot{Head: strings.Repeat("a", 40), Files: []WikiFile{{Path: "Home.md", Mode: 0o644, Content: []byte("page")}}}
	restoreSnapshot := SetReadWikiSnapshotFunc(func(dir, remote, baseline string) (*WikiSnapshot, error) {
		if !reflect.DeepEqual([]string{dir, remote, baseline}, []string{"project", "remote", "baseline"}) {
			t.Fatal("snapshot hook arguments changed")
		}
		return want, nil
	})
	defer restoreSnapshot()
	if got, err := ReadWikiSnapshot("project", "remote", "baseline", WikiRepository{}); got != want || err != nil {
		t.Fatalf("snapshot hook: %v, %v", got, err)
	}
	called := false
	restoreRecheck := SetRecheckWikiHeadFunc(func(remote, expected string) error {
		called = remote == "remote" && expected == "pinned"
		return nil
	})
	defer restoreRecheck()
	if err := RecheckWikiHead("remote", "pinned"); err != nil || !called {
		t.Fatalf("recheck hook: %v", err)
	}
	restoreSnapshot()
	restoreRecheck()
	if _, err := ReadWikiSnapshot("", "", "", WikiRepository{}); err == nil {
		t.Fatal("snapshot hook did not restore isolation")
	}
	if err := RecheckWikiHead("", ""); err == nil {
		t.Fatal("recheck hook did not restore isolation")
	}
}
