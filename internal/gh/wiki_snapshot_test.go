package gh

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
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
	snapshot, err := ReadWikiSnapshot(project, bare, "")
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
			snapshot, err := ReadWikiSnapshot(project, remote, tc.baseline)
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
	snapshot, err := ReadWikiSnapshot(project, remote, strings.Repeat("a", 40))
	if err != nil || snapshot.Readiness != nil {
		t.Fatalf("snapshot=%+v, error=%v", snapshot, err)
	}
}

func TestWikiSnapshotAvailabilityAndRecheck(t *testing.T) {
	remote := t.TempDir()
	wikiFixtureGit(t, remote, "init", "-b", "main")
	var access *WikiAccessError
	if snapshot, err := ReadWikiSnapshot(t.TempDir(), remote, ""); snapshot != nil || !errors.As(err, &access) || access.Reason != WikiEmpty {
		t.Fatalf("empty remote: snapshot=%v, error=%v", snapshot, err)
	}
	if err := RecheckWikiHead(remote, strings.Repeat("a", 40)); !errors.As(err, &access) || access.Reason != WikiEmpty {
		t.Fatalf("empty recheck: %v", err)
	}
	if _, err := ReadWikiSnapshot(t.TempDir(), filepath.Join(t.TempDir(), "missing.git"), ""); !errors.As(err, &access) || access.Reason == WikiEmpty {
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
				if err := os.Symlink("page.md", filepath.Join(remote, name)); err != nil {
					t.Fatal(err)
				}
			case "submodule":
				head := wikiFixtureGit(t, remote, "rev-parse", "HEAD")
				wikiFixtureGit(t, remote, "update-index", "--add", "--cacheinfo", "160000,"+head+",submodule")
			case "Home.md":
				wikiFixtureWrite(t, remote, name, "upper")
				wikiFixtureWrite(t, remote, "home.md", "lower")
			default:
				wikiFixtureWrite(t, remote, name, "reserved")
			}
			if name != "submodule" {
				wikiFixtureGit(t, remote, "add", ".")
			}
			wikiFixtureGit(t, remote, "commit", "-m", "Unsafe")
			snapshot, err := ReadWikiSnapshot(t.TempDir(), remote, "")
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
		snapshot, err := ReadWikiSnapshot(t.TempDir(), remote, "")
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
	snapshot, err := ReadWikiSnapshot(t.TempDir(), remote, "")
	if err != nil || len(snapshot.Files) != 1 {
		t.Fatalf("configuration isolation failed: snapshot=%v, error=%v", snapshot, err)
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
	if err := InspectWiki("", "", ""); err != nil {
		t.Fatal(err)
	}
	if _, err := ReadWikiSnapshot("", "", ""); err == nil || !strings.Contains(err.Error(), "test hook is required") {
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
	if got, err := ReadWikiSnapshot("project", "remote", "baseline"); got != want || err != nil {
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
	if _, err := ReadWikiSnapshot("", "", ""); err == nil {
		t.Fatal("snapshot hook did not restore isolation")
	}
	if err := RecheckWikiHead("", ""); err == nil {
		t.Fatal("recheck hook did not restore isolation")
	}
}
