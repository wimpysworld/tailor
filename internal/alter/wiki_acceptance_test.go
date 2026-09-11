package alter_test

import (
	"errors"
	"fmt"
	"net/http"
	"reflect"
	"strings"
	"testing"

	"github.com/wimpysworld/tailor/internal/alter"
	"github.com/wimpysworld/tailor/internal/gh"
	"github.com/wimpysworld/tailor/internal/swatch"
)

func TestWikiConflictBlocksAllWrites(t *testing.T) {
	s, client := newPagesAcceptanceAPI(t)
	dir := t.TempDir()
	writeOnDisk(t, dir, ".tailor.yml", []byte("license: none\nrepository:\n  has_wiki: true\n  description: changed\nswatches:\n  - path: .tailor.yml\n    alteration: always\n"))
	writeOnDisk(t, dir, swatch.WikiDestination, []byte("name: user-owned\n"))
	before := pagesAcceptanceSnapshot(t, dir)
	var stdout, stderr strings.Builder
	err := alter.Run(loadTestConfig(t, dir), dir, alter.Apply, client, &stdout, &stderr)
	if err == nil || !strings.Contains(err.Error(), "ownership conflict") {
		t.Fatalf("error=%v", err)
	}
	if len(s.writes) != 0 || !reflect.DeepEqual(before, pagesAcceptanceSnapshot(t, dir)) {
		t.Fatal("conflict caused writes")
	}
}

func TestWikiEnablementStopsBeforeOtherWrites(t *testing.T) {
	for _, mode := range []alter.ApplyMode{alter.Apply, alter.Recut} {
		t.Run(fmt.Sprint(mode), func(t *testing.T) {
			s, client := newPagesAcceptanceAPI(t)
			dir := t.TempDir()
			writeOnDisk(t, dir, ".tailor.yml", []byte("license: none\nrepository:\n  has_wiki: true\n  description: changed\nswatches:\n  - path: .tailor.yml\n    alteration: always\n"))
			writeOnDisk(t, dir, ".github/workflows/tailor.yml", []byte("retired"))
			before := pagesAcceptanceSnapshot(t, dir)
			called := false
			t.Cleanup(gh.SetInspectWikiFunc(func(_, remote, baseline string) error {
				called = true
				if len(s.writes) != 1 || s.writes[0].Method != http.MethodPatch || s.writes[0].Body != `{"has_wiki":true}` {
					t.Fatalf("readiness preceded enablement or other changes occurred: %v", s.writes)
				}
				if remote != "https://github.com/testowner/testrepo.wiki.git" || baseline != "" {
					t.Fatalf("remote=%q baseline=%q", remote, baseline)
				}
				return &gh.WikiAccessError{State: "wiki has no commits"}
			}))
			var stdout, stderr strings.Builder
			err := alter.Run(loadTestConfig(t, dir), dir, mode, client, &stdout, &stderr)
			if !called || err == nil || !strings.Contains(err.Error(), "https://github.com/testowner/testrepo/wiki") || !strings.Contains(err.Error(), "Create and save") {
				t.Fatalf("called=%t error=%v", called, err)
			}
			if stderr.String() != "set: repository.has_wiki = true\n" {
				t.Fatalf("enablement output=%q", stderr.String())
			}
			if len(s.writes) != 1 || !reflect.DeepEqual(before, pagesAcceptanceSnapshot(t, dir)) {
				t.Fatal("readiness blocker allowed other writes")
			}
		})
	}
}

func TestWikiDisabledPreviewIncludesBlockerAndOtherChanges(t *testing.T) {
	s, client := newPagesAcceptanceAPI(t)
	t.Cleanup(gh.SetInspectWikiFunc(func(string, string, string) error {
		t.Error("disabled preview inspected remote wiki")
		return errors.New("unexpected")
	}))
	dir := t.TempDir()
	writeOnDisk(t, dir, ".tailor.yml", []byte("license: none\nrepository:\n  has_wiki: true\n  description: changed\nswatches:\n  - path: wiki/Home.md\n    alteration: first-fit\n  - path: .gitignore\n    alteration: always\n"))
	before := pagesAcceptanceSnapshot(t, dir)
	var stdout, stderr strings.Builder
	if err := alter.Run(loadTestConfig(t, dir), dir, alter.DryRun, client, &stdout, &stderr); err != nil {
		t.Fatal(err)
	}
	preview, nextSteps, found := strings.Cut(stdout.String(), "\nNext steps:\n")
	if !found || strings.Contains(nextSteps, "Next steps:") {
		t.Fatalf("expected one final Next steps section: %s", stdout.String())
	}
	for _, want := range []string{"wiki/Home.md", swatch.WikiDestination, ".gitignore", "repository.description", "repository.has_wiki"} {
		if !strings.Contains(preview, want) {
			t.Errorf("preview lacks %q: %s", want, stdout.String())
		}
	}
	wantNextSteps := `The wiki is off. Other changes will wait until it is ready.

1. Run: tailor alter

2. If the wiki has no pages, create and save one at:
   https://github.com/testowner/testrepo/wiki

3. If Tailor asks you to import the wiki:
   Requires rsync. Run each command from the project root, stopping if a command fails.
   Use an unused path for ../tailor-wiki-import.
     git clone https://github.com/testowner/testrepo.wiki.git ../tailor-wiki-import
     rsync -a --exclude=.git ../tailor-wiki-import/ wiki/
     git -C ../tailor-wiki-import rev-parse HEAD > wiki/.tailor-wiki-base

4. Run tailor baste again. When the wiki checks pass, run tailor alter.
`
	if nextSteps != wantNextSteps {
		t.Errorf("next steps mismatch:\ngot:\n%s\nwant:\n%s", nextSteps, wantNextSteps)
	}
	if strings.Contains(stderr.String(), "wiki readiness") || strings.Contains(preview, "The wiki is off") || strings.Contains(preview, "Run: tailor alter") {
		t.Fatalf("wiki guidance appeared before the final section: stdout=%s stderr=%s", stdout.String(), stderr.String())
	}
	if len(s.writes) != 0 || !reflect.DeepEqual(before, pagesAcceptanceSnapshot(t, dir)) {
		t.Fatal("preview wrote state")
	}
}

func TestWikiConfirmedEnablementIsNotRepeated(t *testing.T) {
	s, client := newPagesAcceptanceAPI(t)
	t.Cleanup(gh.SetInspectWikiFunc(func(string, string, string) error { return nil }))
	dir := t.TempDir()
	writeOnDisk(t, dir, ".tailor.yml", []byte("license: none\nrepository:\n  has_wiki: true\nswatches:\n  - path: wiki/Home.md\n    alteration: first-fit\n"))
	var stdout, stderr strings.Builder
	if err := alter.Run(loadTestConfig(t, dir), dir, alter.Apply, client, &stdout, &stderr); err != nil {
		t.Fatal(err)
	}
	if len(s.writes) != 1 || s.writes[0].Body != `{"has_wiki":true}` {
		t.Fatalf("confirmed enablement repeated with stale metadata: %v", s.writes)
	}
}

func TestWikiAndPagesPreviewTogether(t *testing.T) {
	s, client := newPagesAcceptanceAPI(t)
	dir := t.TempDir()
	writeOnDisk(t, dir, ".tailor.yml", []byte("license: none\nrepository:\n  has_wiki: true\npages:\n  enabled: true\nswatches:\n  - path: wiki/Home.md\n    alteration: first-fit\n"))
	writeOnDisk(t, dir, "pages/index.html", []byte("site"))
	before := pagesAcceptanceSnapshot(t, dir)
	output := captureAlterRun(t, loadTestConfig(t, dir), dir, alter.DryRun, client)
	for _, name := range []string{swatch.WikiDestination, swatch.PagesDestination, "wiki/Home.md"} {
		if !strings.Contains(output, name) {
			t.Fatalf("preview missing %s: %s", name, output)
		}
	}
	if len(s.writes) != 0 || !reflect.DeepEqual(before, pagesAcceptanceSnapshot(t, dir)) {
		t.Fatal("preview wrote files or remote state")
	}
}
