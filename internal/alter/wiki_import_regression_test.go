package alter

import (
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/wimpysworld/tailor/internal/gh"
)

func TestWikiImportConflictBlocks(t *testing.T) {
	conflict := "<<<<<<< local\nours\n=======\ntheirs\n>>>>>>> remote\n"
	diff3 := "<<<<<<< local\nours\n||||||| ancestor\nbase\n=======\ntheirs\n>>>>>>> remote\n"
	for _, tc := range []struct {
		name, content string
		blocked       bool
	}{
		{"setext", "Title\n=======\n", false},
		{"separator", "=======\n", false},
		{"incomplete", "<<<<<<< local\nours\n=======\n", false},
		{"reversed", ">>>>>>> remote\n=======\n<<<<<<< local\n", false},
		{"backticks", "```diff\n" + conflict + "```\n", false},
		{"tildes diff3", "~~~diff\n" + diff3 + "~~~\n", false},
		{"long fence", "   ````diff\n```\n" + conflict + "   ````` \t\n", false},
		{"conflict", conflict, true},
		{"diff3", diff3, true},
		{"crlf diff3", strings.ReplaceAll(diff3, "\n", "\r\n"), true},
		{"unclosed fence", "```diff\n" + conflict, true},
		{"mismatched fence", "```diff\n" + conflict + "~~~\n", true},
		{"short closing fence", "````diff\n" + conflict + "```\n", true},
		{"invalid closing suffix", "```diff\n" + conflict + "```text\n", true},
		{"outside fence", "```diff\n" + conflict + "```\n" + diff3, true},
		{"binary", "\x00\xff" + conflict, false},
	} {
		for _, localOnly := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/local-only=%t", tc.name, localOnly), func(t *testing.T) {
				dir := wikiImportProject(t)
				snapshot := wikiImportSnapshot()
				name := "wiki/Home.md"
				if localOnly {
					name = "wiki/Local.md"
					wikiTestWrite(t, dir, name, tc.content)
				} else {
					snapshot.Files[0].Content = []byte(tc.content)
				}
				wikiImportHooks(t, snapshot)
				target, _ := wikiTestTarget(t, 200, `{"private":false,"has_wiki":true,"default_branch":"main"}`)
				p, err := preflightWiki(wikiTestConfig(new(true)), dir, Apply, target, true)
				if tc.blocked {
					if err == nil || !strings.Contains(err.Error(), "unresolved conflict") {
						t.Fatalf("blocked import: err=%v", err)
					}
					if _, err := os.Stat(filepath.Join(dir, ".git/tailor")); !errors.Is(err, fs.ErrNotExist) {
						t.Fatal("blocked import wrote recovery state")
					}
					return
				}
				if err != nil || p.importPlan == nil || !p.importPlan.Complete {
					t.Fatalf("valid import: plan=%+v err=%v", p, err)
				}
				data, err := os.ReadFile(filepath.Join(dir, name))
				if err != nil || !bytes.Equal(data, []byte(tc.content)) {
					t.Fatalf("content changed: %q, %v", data, err)
				}
			})
		}
	}
}

func TestWikiImportUnmergedIndex(t *testing.T) {
	for _, name := range []string{"wiki/Home.md", "wiki/Local.md", "wiki/nested/Local.md", "README.md", "wiki-other/Home.md"} {
		for _, mode := range []ApplyMode{Apply, DryRun} {
			t.Run(fmt.Sprintf("%s/%v", name, mode), func(t *testing.T) {
				dir := wikiImportProject(t)
				wikiTestWrite(t, dir, name, "resolved worktree bytes\n")
				object := wikiImportGit(t, dir, "hash-object", "-w", "--", name)
				cmd := exec.CommandContext(t.Context(), "git", "-C", dir, "update-index", "--index-info")
				for _, entry := range os.Environ() {
					if !strings.HasPrefix(entry, "GIT_") {
						cmd.Env = append(cmd.Env, entry)
					}
				}
				cmd.Env = append(cmd.Env, "GIT_CONFIG_NOSYSTEM=1", "GIT_CONFIG_GLOBAL="+os.DevNull)
				cmd.Stdin = strings.NewReader(fmt.Sprintf("100644 %s 1\t%s\n100644 %s 2\t%s\n", object, name, object, name))
				if out, err := cmd.CombinedOutput(); err != nil {
					t.Fatalf("unmerged fixture: %v: %s", err, out)
				}
				index := filepath.Join(dir, ".git/index")
				before, err := os.ReadFile(index)
				if err != nil {
					t.Fatal(err)
				}
				t.Setenv("GIT_INDEX_FILE", filepath.Join(t.TempDir(), "ignored-index"))
				wikiImportHooks(t, wikiImportSnapshot())
				target, _ := wikiTestTarget(t, 200, `{"private":false,"has_wiki":true,"default_branch":"main"}`)
				p, err := preflightWiki(wikiTestConfig(new(true)), dir, mode, target, true)
				blocked := strings.HasPrefix(name, "wiki/")
				if blocked {
					message := p.nextSteps
					if err != nil {
						message = err.Error()
					}
					if !strings.Contains(message, "unmerged Git index") || p.importPlan != nil {
						t.Fatalf("index conflict not blocked: plan=%+v err=%v", p, err)
					}
					if _, err := os.Stat(filepath.Join(dir, ".git/tailor")); !errors.Is(err, fs.ErrNotExist) {
						t.Fatal("blocked import wrote recovery state")
					}
				} else if err != nil || p.importPlan == nil || mode == Apply && !p.importPlan.Complete {
					t.Fatalf("unrelated conflict blocked import: plan=%+v err=%v", p, err)
				}
				after, err := os.ReadFile(index)
				if err != nil || !bytes.Equal(before, after) {
					t.Fatalf("index changed: %v", err)
				}
			})
		}
	}
}

func TestWikiImportNonGitPreview(t *testing.T) {
	dir := t.TempDir()
	wikiTestWrite(t, dir, "wiki/Local.md", "Title\n=======\n")
	wikiImportHooks(t, wikiImportSnapshot())
	target, _ := wikiTestTarget(t, 200, `{"private":false,"has_wiki":true,"default_branch":"main"}`)
	p, err := preflightWiki(wikiTestConfig(new(true)), dir, DryRun, target, true)
	if err != nil || p.importPlan == nil {
		t.Fatalf("non-Git preview: plan=%+v err=%v", p, err)
	}
}

func TestWikiImportMovedCheckout(t *testing.T) {
	for _, complete := range []bool{true, false} {
		t.Run(fmt.Sprintf("complete=%t", complete), func(t *testing.T) {
			// Exercise temporary roots that use a symlink alias, as on macOS.
			dir := filepath.Join(t.TempDir(), "checkout")
			if err := os.Symlink(wikiImportProject(t), dir); err != nil {
				t.Fatal(err)
			}
			wikiTestWrite(t, dir, "wiki/Home.md", "original local bytes\n")
			snapshot := wikiImportSnapshot()
			wikiImportHooks(t, snapshot)
			if !complete {
				checks := 0
				t.Cleanup(gh.SetRecheckWikiHeadFunc(func(string, string) error {
					checks++
					if checks == 2 {
						return errors.New("interrupted")
					}
					return nil
				}))
			}
			target, _ := wikiTestTarget(t, 200, `{"private":false,"has_wiki":true,"default_branch":"main"}`)
			p, err := preflightWiki(wikiTestConfig(new(true)), dir, Apply, target, true)
			if (err == nil) != complete || p.importPlan == nil || p.importPlan.Complete != complete {
				t.Fatalf("initial import: plan=%+v err=%v", p, err)
			}
			dir, err = filepath.EvalSymlinks(dir)
			if err != nil {
				t.Fatal(err)
			}
			location, err := filepath.Rel(dir, p.importPlan.location)
			if err != nil {
				t.Fatal(err)
			}
			records := make(map[string]wikiFileState)
			root, err := os.OpenRoot(p.importPlan.location)
			if err != nil {
				t.Fatal(err)
			}
			err = fs.WalkDir(root.FS(), ".", func(name string, entry fs.DirEntry, err error) error {
				if err != nil || entry.IsDir() {
					return err
				}
				state, _, _, err := readWikiImportFile(root, name)
				records[name] = state
				return err
			})
			_ = root.Close()
			if err != nil {
				t.Fatal(err)
			}
			moved := filepath.Join(t.TempDir(), "moved")
			if err := os.Rename(dir, moved); err != nil {
				t.Fatal(err)
			}
			t.Cleanup(gh.SetInspectWikiFunc(func(string, string, string) error { return nil }))
			t.Cleanup(gh.SetReadWikiSnapshotFunc(func(string, string, string) (*gh.WikiSnapshot, error) {
				t.Fatal("moved checkout attempted reimport")
				return nil, nil
			}))
			p, err = preflightWiki(wikiTestConfig(new(true)), moved, Apply, target, true)
			if complete {
				if err != nil || p.importPlan != nil {
					t.Fatalf("completed checkout blocked: %v", err)
				}
			} else if err == nil || !strings.Contains(err.Error(), "identity mismatch") {
				t.Fatalf("incomplete checkout not blocked: %v", err)
			}
			for name, before := range records {
				filename := filepath.Join(moved, location, name)
				data, err := os.ReadFile(filename)
				info, statErr := os.Stat(filename)
				if err != nil || statErr != nil || wikiState(data, info.Mode()) != before {
					t.Fatalf("recovery record %s changed: %v, %v", name, err, statErr)
				}
			}
		})
	}
}
