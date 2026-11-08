package alter_test

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os/exec"
	"reflect"
	"strings"
	"testing"

	"github.com/wimpysworld/tailor/internal/alter"
	"github.com/wimpysworld/tailor/internal/gh"
	"github.com/wimpysworld/tailor/internal/ghfake"
	"github.com/wimpysworld/tailor/internal/output"
	"github.com/wimpysworld/tailor/internal/swatch"
	"github.com/wimpysworld/tailor/internal/testutil"
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

// GitHub answers "not found" for an enabled public wiki without pages, so the run continues with guidance.
func TestWikiEnablementContinuesWhenWikiIsEmpty(t *testing.T) {
	for _, mode := range []alter.ApplyMode{alter.Apply, alter.Recut} {
		t.Run(fmt.Sprint(mode), func(t *testing.T) {
			s, client := newPagesAcceptanceAPI(t)
			dir := t.TempDir()
			writeOnDisk(t, dir, ".tailor.yml", []byte("license: none\nrepository:\n  has_wiki: true\n  description: changed\nswatches:\n  - path: wiki/Home.md\n    alteration: first-fit\n"))
			writeOnDisk(t, dir, ".github/workflows/tailor.yml", []byte("retired"))
			called := false
			t.Cleanup(gh.SetInspectWikiFunc(func(_, remote, baseline string) error {
				called = true
				if len(s.writes) != 1 || s.writes[0].Method != http.MethodPatch || s.writes[0].Body != `{"has_wiki":true}` {
					t.Fatalf("readiness preceded enablement or other changes occurred: %v", s.writes)
				}
				if remote != "https://github.com/testowner/testrepo.wiki.git" || baseline != "" {
					t.Fatalf("remote=%q baseline=%q", remote, baseline)
				}
				return &gh.WikiAccessError{Reason: gh.WikiAccessUnknown, State: "wiki remote is unavailable; it is not possible to distinguish a missing wiki from denied access"}
			}))
			var stdout, stderr strings.Builder
			err := alter.Run(loadTestConfig(t, dir), dir, mode, client, &stdout, &stderr)
			if !called || err != nil {
				t.Fatalf("called=%t error=%v", called, err)
			}
			for _, want := range []string{"The wiki has no pages.", "Create and save the first page at:\n   https://github.com/testowner/testrepo/wiki"} {
				if !strings.Contains(stdout.String(), want) {
					t.Errorf("output lacks %q: %s", want, stdout.String())
				}
			}
			if strings.Contains(stdout.String(), "wiki is not ready") || strings.Count(stderr.String(), "set: repository.has_wiki = true\n") != 1 {
				t.Fatalf("stdout=%q stderr=%q", stdout.String(), stderr.String())
			}
			if len(s.writes) != 2 || s.writes[1].Method != http.MethodPatch || !strings.Contains(s.writes[1].Body, `"description":"changed"`) {
				t.Fatalf("other declared settings were not applied: %v", s.writes)
			}
			after := pagesAcceptanceSnapshot(t, dir)
			for _, name := range []string{"wiki/Home.md", swatch.WikiDestination} {
				if _, ok := after[name]; !ok {
					t.Errorf("%s was not created", name)
				}
			}
			if _, ok := after[".github/workflows/tailor.yml"]; ok {
				t.Error("retired workflow was not removed")
			}
		})
	}
}

func TestWikiDisabledPreviewIncludesBlockerAndOtherChanges(t *testing.T) {
	s, client := newPagesAcceptanceAPI(t)
	t.Cleanup(gh.SetReadWikiSnapshotFunc(func(string, string, string) (*gh.WikiSnapshot, error) {
		t.Error("disabled preview read a remote snapshot")
		return nil, errors.New("unexpected")
	}))
	t.Cleanup(gh.SetRecheckWikiHeadFunc(func(string, string) error {
		t.Error("disabled preview rechecked remote HEAD")
		return errors.New("unexpected")
	}))
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
	wantNextSteps := `The wiki is off. tailor alter will turn it on.

1. Run: tailor alter
   Tailor imports existing wiki pages automatically.

2. If the wiki has no pages, create and save the first page at:
   https://github.com/testowner/testrepo/wiki
   Then run tailor alter again.
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

func TestWikiAdoptionPreviewLineBoundary(t *testing.T) {
	t.Setenv("TERM", "xterm-256color")
	ghfake.FakeRepo(t, "testowner", "testrepo")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/user":
			fmt.Fprint(w, `{"login":"testuser"}`)
		case r.Method == http.MethodGet && r.URL.Path == "/repos/testowner/testrepo":
			fmt.Fprint(w, `{"private":false,"has_wiki":true,"default_branch":"main","permissions":{"admin":true},"security_and_analysis":{}}`)
		case r.Method == http.MethodGet && r.URL.Path == "/repos/testowner/testrepo/actions/permissions/workflow":
			fmt.Fprint(w, `{"default_workflow_permissions":"read","can_approve_pull_request_reviews":false}`)
		case r.Method == http.MethodGet && (r.URL.Path == "/repos/testowner/testrepo/private-vulnerability-reporting" || r.URL.Path == "/repos/testowner/testrepo/automated-security-fixes"):
			fmt.Fprint(w, `{"enabled":false}`)
		case r.Method == http.MethodGet && r.URL.Path == "/repos/testowner/testrepo/vulnerability-alerts":
			w.WriteHeader(http.StatusNotFound)
		default:
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusInternalServerError)
		}
	}))
	t.Cleanup(server.Close)
	client := testutil.NewTestClient(t, server)
	head := strings.Repeat("a", 40)
	readiness := &gh.WikiReadinessError{Reason: gh.WikiBaselineMissing, Err: errors.New("missing baseline")}
	t.Cleanup(gh.SetInspectWikiFunc(func(string, string, string) error { return readiness }))
	t.Cleanup(gh.SetReadWikiSnapshotFunc(func(string, string, string) (*gh.WikiSnapshot, error) {
		return &gh.WikiSnapshot{
			Head: head, Readiness: readiness,
			Files: []gh.WikiFile{{Path: "Home.md", Mode: 0o644, Content: []byte("remote home\n")}},
		}, nil
	}))
	dir := t.TempDir()
	writeOnDisk(t, dir, ".tailor.yml", []byte("license: none\nrepository:\n  has_wiki: true\nswatches:\n  - path: wiki/Home.md\n    alteration: first-fit\n"))
	before := pagesAcceptanceSnapshot(t, dir)
	const guidance = "Review the wiki import preview. tailor alter imports the wiki automatically."
	const boundary = guidance + "\nwould import: wiki/Home.md\n"
	var legacy strings.Builder
	if err := alter.Run(loadTestConfig(t, dir), dir, alter.DryRun, client, &legacy, io.Discard); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(legacy.String(), boundary) {
		t.Fatalf("legacy output lacks exact line boundary: %q", legacy.String())
	}
	report, err := alter.Execute(loadTestConfig(t, dir), dir, alter.DryRun, client, io.Discard, alter.Options{})
	if err != nil {
		t.Fatal(err)
	}
	var plain strings.Builder
	output.New(&plain, io.Discard, output.Plain).Print(report.Document, report.Plain)
	if plain.String() != legacy.String() || !strings.Contains(plain.String(), boundary) {
		t.Fatalf("plain policy output differs: %q", plain.String())
	}
	found := false
	for _, g := range report.Document.Guidance {
		if g.Kind == output.GuidanceRequired && g.Text == guidance {
			found = true
		}
	}
	if !found {
		t.Fatal("typed adoption guidance changed")
	}
	for _, tty := range []bool{false, true} {
		t.Run(fmt.Sprintf("auto/tty=%t", tty), func(t *testing.T) {
			var current, previous strings.Builder
			options := []output.Option{output.WithTTY(tty), output.WithWidth(80), output.WithColor(output.ColorNever)}
			output.New(&current, io.Discard, output.Auto, options...).Print(report.Document, report.Plain)
			oldPlain := strings.Replace(report.Plain, boundary, guidance+"would import: wiki/Home.md\n", 1)
			output.New(&previous, io.Discard, output.Auto, options...).Print(report.Document, oldPlain)
			if current.String() != previous.String() {
				t.Fatal("plain newline changed automatic rendering")
			}
		})
	}
	if !reflect.DeepEqual(before, pagesAcceptanceSnapshot(t, dir)) {
		t.Fatal("preview wrote files")
	}

	cmd := exec.CommandContext(t.Context(), "git", "init", "--template=", "-b", "main", dir)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("initialising recovery fixture: %v: %s", err, out)
	}
	rechecks := 0
	t.Cleanup(gh.SetRecheckWikiHeadFunc(func(string, string) error {
		rechecks++
		if rechecks == 2 {
			return errors.New("remote changed before baseline")
		}
		return nil
	}))
	report, err = alter.Execute(loadTestConfig(t, dir), dir, alter.Apply, client, io.Discard, alter.Options{})
	if err == nil || !strings.Contains(err.Error(), "remote changed before baseline") {
		t.Fatalf("expected interrupted import, got %v", err)
	}
	const recoveryEnd = "Run tailor alter to resume the pinned import.\n"
	if !strings.HasSuffix(report.Plain, recoveryEnd) {
		t.Fatalf("recovery guidance lacks one final newline: %q", report.Plain)
	}
	head = strings.Repeat("b", 40)
	report, err = alter.Execute(loadTestConfig(t, dir), dir, alter.DryRun, client, io.Discard, alter.Options{})
	if err != nil {
		t.Fatal(err)
	}
	found = false
	for _, g := range report.Document.Guidance {
		if strings.HasPrefix(g.Text, "wiki changed since the interrupted import") {
			found = strings.Contains(report.Plain, g.Text+"\nwould import: wiki/Home.md\n")
		}
	}
	if !found || !strings.HasSuffix(report.Plain, recoveryEnd) {
		t.Fatalf("failed resume guidance has incorrect boundaries: %q", report.Plain)
	}
	head = strings.Repeat("a", 40)
	report, err = alter.Execute(loadTestConfig(t, dir), dir, alter.Apply, client, io.Discard, alter.Options{})
	if err != nil {
		t.Fatal(err)
	}
	found = false
	for _, g := range report.Document.Guidance {
		if strings.HasPrefix(g.Text, "Wiki adoption completed.") {
			found = strings.HasSuffix(report.Plain, g.Text+"\n") && !strings.HasSuffix(g.Text, "\n")
		}
	}
	if !found {
		t.Fatalf("completion guidance has incorrect boundary: %q", report.Plain)
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
