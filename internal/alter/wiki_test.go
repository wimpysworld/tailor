package alter

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/wimpysworld/tailor/internal/config"
	"github.com/wimpysworld/tailor/internal/model"
	"github.com/wimpysworld/tailor/internal/swatch"
	"github.com/wimpysworld/tailor/internal/testutil"
)

func wikiTestConfig(enabled *bool) *config.Config {
	cfg := &config.Config{License: "none", Repository: &model.RepositorySettings{HasWiki: enabled}}
	for _, entry := range swatch.All() {
		if swatch.IsWiki(entry.Path) {
			cfg.Swatches = append(cfg.Swatches, config.SwatchEntry{Path: entry.Path, Alteration: entry.DefaultAlteration})
		}
	}
	return cfg
}

func wikiTestTarget(t *testing.T, status int, body string) (RepoTarget, *int) {
	t.Helper()
	reads := new(int)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		*reads++
		if r.Method != http.MethodGet || r.URL.Path != "/repos/owner/repo" {
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		fmt.Fprint(w, body)
	}))
	t.Cleanup(server.Close)
	return RepoTarget{Client: testutil.NewTestClient(t, server), Owner: "owner", Name: "repo", HasRepo: true}, reads
}

func wikiTestWrite(t *testing.T, dir, name, content string) {
	t.Helper()
	file := filepath.Join(dir, name)
	if err := os.MkdirAll(filepath.Dir(file), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(file, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestWikiLifecycle(t *testing.T) {
	for _, mode := range []ApplyMode{DryRun, Apply, Recut} {
		t.Run(fmt.Sprint(mode), func(t *testing.T) {
			dir := t.TempDir()
			cfg := wikiTestConfig(new(true))
			target, reads := wikiTestTarget(t, 200, `{"private":false,"default_branch":"main"}`)
			var stderr strings.Builder
			target.Stderr = &stderr
			wikiTestWrite(t, dir, "wiki/Home.md", "user home\n")
			wikiTestWrite(t, dir, "pages/index.html", "independent pages\n")
			p, err := preflightWiki(cfg, dir, mode, target, true)
			if err != nil {
				t.Fatal(err)
			}
			if *reads != 1 || !strings.Contains(stderr.String(), "wiki/.tailor-wiki-base") {
				t.Fatalf("reads=%d warning=%q", *reads, stderr.String())
			}
			_, results, err := processWiki(cfg, dir, mode, p)
			if err != nil || len(results) != 4 {
				t.Fatalf("results=%v err=%v", results, err)
			}
			for name, want := range map[string]string{"wiki/Home.md": "user home\n", "pages/index.html": "independent pages\n"} {
				data, err := os.ReadFile(filepath.Join(dir, name))
				if err != nil || string(data) != want {
					t.Fatalf("%s changed: %q, %v", name, data, err)
				}
			}
			_, err = os.Stat(filepath.Join(dir, swatch.WikiDestination))
			if mode == DryRun {
				if !os.IsNotExist(err) {
					t.Fatal("dry run wrote the workflow")
				}
				for _, name := range []string{"wiki/_Sidebar.md", "wiki/_Footer.md"} {
					if _, err := os.Stat(filepath.Join(dir, name)); !os.IsNotExist(err) {
						t.Fatalf("dry run wrote %s: %v", name, err)
					}
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			for _, name := range []string{"wiki/_Sidebar.md", "wiki/_Footer.md"} {
				want, err := swatch.Content(name)
				if err != nil {
					t.Fatal(err)
				}
				data, err := os.ReadFile(filepath.Join(dir, name))
				if err != nil || string(data) != string(want) {
					t.Fatalf("%s = %q, want %q, err=%v", name, data, want, err)
				}
			}
			_, repeated, err := processWiki(cfg, dir, mode, p)
			if err != nil {
				t.Fatal(err)
			}
			for _, result := range repeated {
				if result.Category != NoChange && result.Category != Skipped {
					t.Fatalf("repeated run changed %s: %v", result.Path, result)
				}
			}
		})
	}
}

func TestWikiDisabledAndOmitted(t *testing.T) {
	for _, mode := range []ApplyMode{DryRun, Apply, Recut} {
		for _, declared := range []bool{false, true} {
			for _, owned := range []bool{false, true} {
				t.Run(fmt.Sprintf("%d/%t/%t", mode, declared, owned), func(t *testing.T) {
					dir := t.TempDir()
					content := "name: user workflow\n"
					if owned {
						content = swatch.WikiMarker + "\n" + content
					}
					wikiTestWrite(t, dir, swatch.WikiDestination, content)
					wikiTestWrite(t, dir, "wiki/Home.md", "keep")
					cfg := wikiTestConfig(new(false))
					p, err := preflightWiki(cfg, dir, mode, RepoTarget{}, declared)
					if err != nil {
						t.Fatal(err)
					}
					_, _, err = processWiki(cfg, dir, mode, p)
					if err != nil {
						t.Fatal(err)
					}
					data, err := os.ReadFile(filepath.Join(dir, swatch.WikiDestination))
					if declared && owned && mode.ShouldWrite() {
						if !os.IsNotExist(err) {
							t.Fatal("owned workflow was not removed")
						}
					} else if err != nil || string(data) != content {
						t.Fatalf("workflow changed: %q %v", data, err)
					}
					data, err = os.ReadFile(filepath.Join(dir, "wiki/Home.md"))
					if err != nil || string(data) != "keep" {
						t.Fatal("source changed")
					}
				})
			}
		}
	}
}

func TestWikiMetadataSkips(t *testing.T) {
	for _, tc := range []struct {
		name   string
		status int
		body   string
	}{
		{"private", 200, `{"private":true,"default_branch":"main"}`},
		{"missing privacy", 200, `{"default_branch":"main"}`},
		{"missing branch", 200, `{"private":false}`},
		{"denied", 403, `{"message":"Forbidden"}`},
		{"missing", 404, `{"message":"Not Found"}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			target, _ := wikiTestTarget(t, tc.status, tc.body)
			cfg := wikiTestConfig(new(true))
			p, err := preflightWiki(cfg, dir, Apply, target, true)
			if err != nil {
				t.Fatal(err)
			}
			results, files, err := processWiki(cfg, dir, Apply, p)
			if err != nil || len(results) != 1 || len(files) != 0 {
				t.Fatalf("%v %v %v", results, files, err)
			}
			entries, _ := os.ReadDir(dir)
			if len(entries) != 0 {
				t.Fatal("skipped wiki wrote files")
			}
		})
	}
}

func TestWikiRejectsUnsafeSourceBeforeMetadata(t *testing.T) {
	for _, kind := range []string{"source symlink", "nested symlink", "git metadata", "invalid baseline", "workflow symlink", "workflow directory", "ownership", "workflow parent symlink"} {
		t.Run(kind, func(t *testing.T) {
			dir, outside := t.TempDir(), t.TempDir()
			switch kind {
			case "source symlink":
				if err := os.Symlink(outside, filepath.Join(dir, "wiki")); err != nil {
					t.Fatal(err)
				}
			case "nested symlink":
				wikiTestWrite(t, dir, "wiki/Home.md", "keep")
				if err := os.Symlink(outside, filepath.Join(dir, "wiki/link")); err != nil {
					t.Fatal(err)
				}
			case "git metadata":
				wikiTestWrite(t, dir, "wiki/.git/config", "keep")
			case "invalid baseline":
				wikiTestWrite(t, dir, swatch.WikiBaseline, "abc")
			case "workflow symlink":
				wikiTestWrite(t, dir, ".github/workflows/keep", "keep")
				if err := os.Symlink(outside, filepath.Join(dir, swatch.WikiDestination)); err != nil {
					t.Fatal(err)
				}
			case "workflow directory":
				if err := os.MkdirAll(filepath.Join(dir, swatch.WikiDestination), 0o755); err != nil {
					t.Fatal(err)
				}
			case "ownership":
				wikiTestWrite(t, dir, swatch.WikiDestination, "name: user workflow\n")
			case "workflow parent symlink":
				if err := os.Symlink(outside, filepath.Join(dir, ".github")); err != nil {
					t.Fatal(err)
				}
			}
			_, err := preflightWiki(wikiTestConfig(new(true)), dir, Recut, RepoTarget{}, true)
			if err == nil {
				t.Fatal("unsafe source or workflow accepted")
			}
		})
	}
}

func TestWikiGenericSwatchesRemainInactive(t *testing.T) {
	cfg := wikiTestConfig(new(true))
	results, err := ProcessSwatches(cfg, t.TempDir(), Recut, nil)
	if err != nil || len(results) != 0 {
		t.Fatalf("wiki escaped conditional stage: %v %v", results, err)
	}
}
