package alter_test

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/wimpysworld/tailor/internal/alter"
	"github.com/wimpysworld/tailor/internal/gh"
	"github.com/wimpysworld/tailor/internal/ghfake"
	"github.com/wimpysworld/tailor/internal/swatch"
	"github.com/wimpysworld/tailor/internal/testutil"
)

func TestWikiReadinessLocalFailuresPrecedeEnablement(t *testing.T) {
	for _, kind := range []string{"unsafe source", "Pages conflict", "retired directory", "swatch directory"} {
		t.Run(kind, func(t *testing.T) {
			s, client := newPagesAcceptanceAPI(t)
			dir := t.TempDir()
			content := "license: none\nrepository:\n  has_wiki: true\n  description: changed\nswatches:\n  - path: .tailor.yml\n    alteration: always\n  - path: .gitignore\n    alteration: always\n"
			want := "directory"
			switch kind {
			case "unsafe source":
				if err := os.Symlink(t.TempDir(), filepath.Join(dir, "wiki")); err != nil {
					t.Fatal(err)
				}
			case "Pages conflict":
				content += "pages:\n  enabled: true\n"
				writeOnDisk(t, dir, "pages/index.html", []byte("site"))
				writeOnDisk(t, dir, swatch.PagesDestination, []byte("name: user-owned\n"))
				want = "ownership conflict"
			case "retired directory":
				if err := os.MkdirAll(filepath.Join(dir, ".github/workflows/tailor.yml"), 0o755); err != nil {
					t.Fatal(err)
				}
			case "swatch directory":
				if err := os.Mkdir(filepath.Join(dir, ".gitignore"), 0o755); err != nil {
					t.Fatal(err)
				}
			}
			writeOnDisk(t, dir, ".tailor.yml", []byte(content))
			before := pagesAcceptanceSnapshot(t, dir)
			t.Cleanup(gh.SetInspectWikiFunc(func(string, string, string) error {
				t.Error("local failure allowed a remote wiki probe")
				return nil
			}))
			err := alter.Run(loadTestConfig(t, dir), dir, alter.Apply, client, io.Discard, io.Discard)
			if err == nil || !strings.Contains(err.Error(), want) {
				t.Fatalf("error=%v, want %q", err, want)
			}
			if len(s.writes) != 0 || !reflect.DeepEqual(before, pagesAcceptanceSnapshot(t, dir)) {
				t.Fatalf("local failure allowed writes: %v", s.writes)
			}
		})
	}
}

func TestWikiReadinessEnabledBlockerAndEnablementFailure(t *testing.T) {
	for _, tc := range []struct {
		name             string
		enabled, preview bool
	}{
		{"already enabled", true, false},
		{"enabled preview", true, true},
		{"enablement denied", false, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var writes []apiCall
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				if r.Method != http.MethodGet {
					body, _ := io.ReadAll(r.Body)
					writes = append(writes, apiCall{Method: r.Method, Path: r.URL.Path, Body: string(body)})
					w.WriteHeader(http.StatusForbidden)
					fmt.Fprint(w, `{"message":"Forbidden"}`)
					return
				}
				switch r.URL.Path {
				case "/user":
					fmt.Fprint(w, `{"login":"testuser"}`)
				case "/repos/testowner/testrepo":
					w.Header().Set("X-OAuth-Scopes", "repo")
					fmt.Fprintf(w, `{"private":false,"has_wiki":%t,"default_branch":"main","description":"old"}`, tc.enabled)
				default:
					fmt.Fprint(w, `{"enabled":false}`)
				}
			}))
			t.Cleanup(server.Close)
			ghfake.FakeRepo(t, "testowner", "testrepo")
			probes := 0
			t.Cleanup(gh.SetInspectWikiFunc(func(string, string, string) error {
				probes++
				return errors.New("wiki exists but wiki/.tailor-wiki-base is missing; import the wiki before adoption")
			}))
			dir := t.TempDir()
			writeOnDisk(t, dir, ".tailor.yml", []byte("license: none\nrepository:\n  has_wiki: true\n  description: changed\nswatches:\n  - path: .gitignore\n    alteration: always\n"))
			writeOnDisk(t, dir, ".github/workflows/tailor.yml", []byte("retired"))
			before := pagesAcceptanceSnapshot(t, dir)
			mode := alter.Apply
			if tc.preview {
				mode = alter.DryRun
			}
			var stdout, stderr strings.Builder
			err := alter.Run(loadTestConfig(t, dir), dir, mode, testutil.NewTestClient(t, server), &stdout, &stderr)
			if tc.preview {
				if err != nil || !strings.Contains(stdout.String(), "Tailor needs an imported copy of the wiki in wiki/.") {
					t.Fatalf("preview error=%v stdout=%q", err, stdout.String())
				}
				for _, want := range []string{"repository.description", ".gitignore", "would remove", swatch.WikiDestination} {
					if !strings.Contains(stdout.String(), want) {
						t.Errorf("preview lacks %q: %s", want, stdout.String())
					}
				}
			} else if err == nil || !strings.Contains(err.Error(), "wiki is not ready") {
				t.Fatalf("readiness did not block: %v", err)
			}
			if tc.enabled {
				if probes != 1 || len(writes) != 0 {
					t.Fatalf("probes=%d writes=%v", probes, writes)
				}
			} else if probes != 0 || len(writes) != 1 || writes[0].Method != http.MethodPatch || writes[0].Path != "/repos/testowner/testrepo" || writes[0].Body != `{"has_wiki":true}` {
				t.Fatalf("enablement failure continued: probes=%d writes=%v", probes, writes)
			}
			if !reflect.DeepEqual(before, pagesAcceptanceSnapshot(t, dir)) {
				t.Fatal("readiness blocker changed local files")
			}
		})
	}
}

func TestWikiReadinessFalseAndOmittedDoNotProbe(t *testing.T) {
	for _, declaration := range []string{"", "repository:\n  has_wiki: false\n"} {
		t.Run(fmt.Sprintf("declaration=%q", declaration), func(t *testing.T) {
			s, client := newPagesAcceptanceAPI(t)
			t.Cleanup(gh.SetInspectWikiFunc(func(string, string, string) error {
				t.Error("inactive wiki caused a remote probe")
				return errors.New("unexpected probe")
			}))
			dir := t.TempDir()
			writeOnDisk(t, dir, ".tailor.yml", []byte("license: none\n"+declaration))
			writeOnDisk(t, dir, "wiki/Home.md", []byte("keep"))
			before := pagesAcceptanceSnapshot(t, dir)
			if err := alter.Run(loadTestConfig(t, dir), dir, alter.Apply, client, io.Discard, io.Discard); err != nil {
				t.Fatal(err)
			}
			after := pagesAcceptanceSnapshot(t, dir)
			if len(s.writes) != 0 || !reflect.DeepEqual(pagesAcceptanceWithoutManagedPaths(before, managedCoreAcceptancePaths...), pagesAcceptanceWithoutManagedPaths(after, managedCoreAcceptancePaths...)) {
				t.Fatalf("inactive wiki caused writes outside the managed core: %v", s.writes)
			}
		})
	}
}
