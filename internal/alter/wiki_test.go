package alter

import (
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/wimpysworld/tailor/internal/config"
	"github.com/wimpysworld/tailor/internal/gh"
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
	t.Cleanup(gh.SetInspectWikiFunc(func(string, string, string) error { return nil }))
	for _, mode := range []ApplyMode{DryRun, Apply, Recut} {
		t.Run(fmt.Sprint(mode), func(t *testing.T) {
			dir := t.TempDir()
			cfg := wikiTestConfig(new(true))
			target, reads := wikiTestTarget(t, 200, `{"private":false,"has_wiki":true,"default_branch":"main"}`)
			var stderr strings.Builder
			target.Stderr = &stderr
			wikiTestWrite(t, dir, "wiki/Home.md", "user home\n")
			wikiTestWrite(t, dir, "pages/index.html", "independent pages\n")
			p, err := preflightWiki(cfg, dir, mode, target, true)
			if err != nil {
				t.Fatal(err)
			}
			if *reads != 1 || stderr.Len() != 0 {
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
			if tc.name != "private" {
				if err == nil || !strings.Contains(err.Error(), "wiki is not ready") {
					t.Fatalf("metadata did not block readiness: %v", err)
				}
				return
			}
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
	results, err := processSwatches(cfg, t.TempDir(), Recut, nil, managedExcludedPaths())
	if err != nil || len(results) != 0 {
		t.Fatalf("wiki escaped conditional stage: %v %v", results, err)
	}
}

func TestWikiGuidanceByReadinessState(t *testing.T) {
	for _, tc := range []struct {
		name       string
		err        error
		want       string
		wantImport bool
		wantPage   bool
	}{
		{"empty", &gh.WikiAccessError{State: "wiki has no commits"}, "Import the wiki:", true, true},
		{"unavailable", &gh.WikiAccessError{State: "cannot distinguish a missing wiki from denied access"}, "If you cannot open the wiki, check your access.", false, true},
		{"denied", &gh.WikiAccessError{State: "wiki access was denied"}, "Check your repository access.", false, false},
		{"network", &gh.WikiAccessError{State: "wiki remote could not be read"}, "Check your network connection", false, false},
		{"timeout", &gh.WikiAccessError{State: "wiki inspection timed out"}, "Check your network connection", false, false},
		{"git missing", &gh.WikiAccessError{State: "git could not start"}, "Check that Git is installed", false, false},
		{"metadata", errors.New("reading wiki repository metadata: forbidden"), "Check GitHub authentication", false, false},
		{"incomplete metadata", errors.New("repository metadata is incomplete"), "Check GitHub authentication", false, false},
		{"enablement failed", errors.New("enabling repository.has_wiki through the API: forbidden"), "Check GitHub authentication", false, false},
		{"missing baseline", errors.New("wiki/.tailor-wiki-base is missing"), "1. Import the wiki:", true, false},
		{"invalid baseline", errors.New("wiki/.tailor-wiki-base must contain one full 40-character commit ID"), "full 40-character ID of the imported wiki version", true, false},
		{"incomplete import", errors.New("wiki import is incomplete: \"Guide.md\" is missing from wiki/; import every remote file before adoption"), "\"Guide.md\" is missing from wiki/", true, false},
		{"unsafe file", errors.New("wiki contains an unsafe file; review remote files before adoption"), "Review the wiki files before importing them", true, false},
		{"missing history", errors.New("wiki published source is not in local HEAD history; fetch the source history and use the current project branch, or review and reimport the wiki"), "Fetch the project history and switch to the current project branch", true, false},
		{"missing source tree", errors.New("wiki published source tree is unavailable; fetch the source history or review and reimport the wiki"), "If the history is still unavailable, review and import the wiki again", true, false},
		{"independent edits", errors.New("wiki remote changed outside the publisher; import it again"), "Review the wiki files, then import the wiki again", true, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			wikiURL := "https://git.example.net/another/project/wiki"
			remoteURL := "https://git.example.net/another/project.wiki.git"
			got := wikiPreviewGuidance(tc.err, wikiURL, remoteURL, false)
			if applied := wikiReadinessGuidance(tc.err, wikiURL, remoteURL, false); applied != "wiki is not ready\n"+strings.TrimSuffix(got, "\n") {
				t.Fatalf("alter and baste guidance differ: %s", applied)
			}
			for _, want := range []string{tc.want, "Next steps:", wikiRecheckGuidance, "Other changes will wait until it is ready."} {
				if !strings.Contains(got, want) {
					t.Errorf("guidance lacks %q: %s", want, got)
				}
			}
			if strings.Contains(got, "git clone "+remoteURL) != tc.wantImport {
				t.Errorf("import guidance mismatch: %s", got)
			}
			if strings.Contains(strings.ToLower(got), "create and save") != tc.wantPage || strings.Contains(got, wikiURL) != tc.wantPage {
				t.Errorf("page guidance mismatch: %s", got)
			}
			for _, jargon := range []string{"readiness", "preflight", "adoption", "remote HEAD", "publisher"} {
				if strings.Contains(got, jargon) {
					t.Errorf("guidance contains %q: %s", jargon, got)
				}
			}
		})
	}
}

func TestWikiUnavailableGuidance(t *testing.T) {
	err := &gh.WikiAccessError{State: "wiki remote is unavailable; it is not possible to distinguish a missing wiki from denied access"}
	got := wikiReadinessGuidance(err, "https://github.com/owner/repo/wiki", "https://github.com/owner/repo.wiki.git", false)
	want := `wiki is not ready

Next steps:
Tailor could not open the wiki. Other changes will wait until it is ready.

1. Open https://github.com/owner/repo/wiki
   If the wiki has no pages, create and save one.
   If you cannot open the wiki, check your access.

2. Run tailor baste again. When the wiki checks pass, run tailor alter.`
	if got != want {
		t.Fatalf("guidance=%s, want %s", got, want)
	}
}

func TestWikiTypedGuidanceMatchesLegacy(t *testing.T) {
	for _, tc := range []struct {
		name   string
		reason gh.WikiReadinessReason
		legacy error
		path   string
	}{
		{"empty", gh.WikiEmpty, &gh.WikiAccessError{State: "wiki has no commits"}, ""},
		{"unavailable", gh.WikiAccessUnknown, &gh.WikiAccessError{State: "cannot distinguish a missing wiki from denied access"}, ""},
		{"denied", gh.WikiAccessDenied, &gh.WikiAccessError{State: "wiki access was denied"}, ""},
		{"network", gh.WikiReadFailed, &gh.WikiAccessError{State: "wiki remote could not be read"}, ""},
		{"timeout", gh.WikiTimedOut, &gh.WikiAccessError{State: "wiki inspection timed out"}, ""},
		{"git missing", gh.WikiGitUnavailable, &gh.WikiAccessError{State: "git could not start"}, ""},
		{"metadata", gh.WikiMetadataFailed, errors.New("reading wiki repository metadata: forbidden"), ""},
		{"incomplete metadata", gh.WikiMetadataFailed, errors.New("repository metadata is incomplete"), ""},
		{"enablement failed", gh.WikiMetadataFailed, errors.New("enabling repository.has_wiki through the API: forbidden"), ""},
		{"disabled", gh.WikiDisabled, errors.New("wiki is disabled"), ""},
		{"missing baseline", gh.WikiBaselineMissing, errors.New("wiki/.tailor-wiki-base is missing"), ""},
		{"invalid baseline", gh.WikiBaselineInvalid, errors.New("wiki/.tailor-wiki-base must contain one full 40-character commit ID"), ""},
		{"incomplete import", gh.WikiImportIncomplete, errors.New("wiki import is incomplete: \"nested/Guide.md\" is missing from wiki/; import every remote file before adoption"), "nested/Guide.md"},
		{"incomplete import semicolon", gh.WikiImportIncomplete, errors.New("wiki import is incomplete: \"nested/Guide;extra.md\" is missing from wiki/; import every remote file before adoption"), "nested/Guide;extra.md"},
		{"unsafe file", gh.WikiUnsafeFile, errors.New("wiki contains an unsafe file; review remote files before adoption"), ""},
		{"unsupported file", gh.WikiUnsafeFile, errors.New("wiki contains an unsupported file; review the remote files before adoption"), ""},
		{"unsafe source", gh.WikiUnsafeFile, errors.New("wiki published source contains an unsafe file; review and reimport the wiki"), ""},
		{"missing history", gh.WikiHistoryMissing, errors.New("wiki published source is not in local HEAD history"), ""},
		{"missing source tree", gh.WikiHistoryMissing, errors.New("wiki published source tree is unavailable"), ""},
		{"independent edits", gh.WikiChangedOutside, errors.New("wiki remote changed outside the publisher; import it again"), ""},
		{"source mismatch", gh.WikiChangedOutside, errors.New("wiki remote differs from its published source; import it again"), ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			message := "wiki is disabled; unrelated diagnostic"
			if tc.reason == gh.WikiDisabled {
				message = "repository metadata is incomplete"
			}
			var typed error = &gh.WikiReadinessError{Reason: tc.reason, Path: tc.path, Err: errors.New(message)}
			if _, ok := errors.AsType[*gh.WikiAccessError](tc.legacy); ok {
				typed = &gh.WikiAccessError{Reason: tc.reason, State: message}
			}
			for _, hasSource := range []bool{false, true} {
				for _, wrapped := range []bool{false, true} {
					err := typed
					if wrapped {
						err = fmt.Errorf("outer diagnostic: %w", err)
					}
					wikiURL := "https://git.example.net/another/project/wiki"
					remoteURL := "https://git.example.net/another/project.wiki.git"
					want := wikiPreviewGuidance(tc.legacy, wikiURL, remoteURL, hasSource)
					if got := wikiPreviewGuidance(err, wikiURL, remoteURL, hasSource); got != want {
						t.Fatalf("hasSource=%t wrapped=%t: guidance=%s, want %s", hasSource, wrapped, got, want)
					}
					if got := wikiReadinessGuidance(err, wikiURL, remoteURL, hasSource); got != "wiki is not ready\n"+strings.TrimSuffix(want, "\n") {
						t.Fatalf("apply guidance=%s", got)
					}
				}
			}
		})
	}
}

func TestWikiGuidanceFallbacks(t *testing.T) {
	for _, tc := range []struct {
		name string
		err  error
		want string
	}{
		{"untyped", errors.New("unexpected diagnostic"), "Tailor could not check the wiki: unexpected diagnostic."},
		{"typed unknown", &gh.WikiReadinessError{Reason: "future", Err: errors.New("wiki is disabled")}, "Tailor could not check the wiki: wiki is disabled."},
		{"typed zero", &gh.WikiReadinessError{Err: errors.New("wiki/.tailor-wiki-base is missing")}, "Tailor could not check the wiki: wiki/.tailor-wiki-base is missing."},
		{"typed inspection", &gh.WikiReadinessError{Reason: gh.WikiInspectionFailed, Err: errors.New("unsafe file")}, "Tailor could not check the wiki: unsafe file."},
		{"access unknown reason", &gh.WikiAccessError{Reason: "future", State: "wiki has no commits"}, "Tailor could not read the wiki."},
		{"access unknown state", &gh.WikiAccessError{State: "unexpected diagnostic"}, "Tailor could not read the wiki."},
		{"wrapped legacy access", fmt.Errorf("outer: %w", &gh.WikiAccessError{State: "wiki has no commits"}), "The wiki has no pages."},
		{"wrapped legacy prefix", fmt.Errorf("outer: %w", errors.New("wiki is disabled")), "Tailor could not check the wiki: outer: wiki is disabled."},
		{"legacy disabled access", &gh.WikiAccessError{State: "wiki is disabled"}, "The wiki is off."},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := wikiPreviewGuidance(tc.err, "https://github.com/owner/repo/wiki", "https://github.com/owner/repo.wiki.git", false)
			if !strings.Contains(got, tc.want) {
				t.Fatalf("guidance=%s, want %q", got, tc.want)
			}
		})
	}
}

func TestWikiBaselineTypedReason(t *testing.T) {
	for _, content := range []string{"abc", strings.Repeat("a", 129)} {
		dir := t.TempDir()
		wikiTestWrite(t, dir, swatch.WikiBaseline, content)
		root, err := os.OpenRoot(dir)
		if err != nil {
			t.Fatal(err)
		}
		defer root.Close()
		_, err = readWikiBaseline(root)
		var readiness *gh.WikiReadinessError
		if !errors.As(err, &readiness) || readiness.Reason != gh.WikiBaselineInvalid || err.Error() != "wiki/.tailor-wiki-base must contain one full 40-character commit ID" {
			t.Fatalf("baseline error=%v", err)
		}
	}
}

func TestWikiImportCopyCommands(t *testing.T) {
	if _, err := exec.LookPath("rsync"); err != nil {
		t.Skipf("copy guidance requires rsync: %v", err)
	}
	for _, existing := range []bool{false, true} {
		t.Run(fmt.Sprintf("existing=%t", existing), func(t *testing.T) {
			guidance := wikiImportGuidance("https://github.com/owner/repo.wiki.git", existing)
			for _, forbidden := range []string{"&&", ";", "|", "$", "if [", "mkdir", "mktemp", "printf", "cp -a"} {
				if strings.Contains(guidance, forbidden) {
					t.Fatalf("guidance contains %q: %s", forbidden, guidance)
				}
			}
			root := t.TempDir()
			project := filepath.Join(root, "project")
			if err := os.Mkdir(project, 0o755); err != nil {
				t.Fatal(err)
			}
			write := func(name, content string) {
				t.Helper()
				filename := filepath.Join(root, name)
				if err := os.MkdirAll(filepath.Dir(filename), 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(filename, []byte(content), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			for name, content := range map[string]string{
				"Home.md": "remote", ".hidden": "hidden", "nested/Guide.md": "guide", ".git/config": "metadata",
			} {
				write("tailor-wiki-import/"+name, content)
			}
			if existing {
				write("project/wiki/Home.md", "local!")
				write("project/wiki/Local.md", "local only")
				write("project/wiki/.tailor-wiki-base", "old baseline")
				remoteInfo, err := os.Stat(filepath.Join(root, "tailor-wiki-import/Home.md"))
				if err != nil {
					t.Fatal(err)
				}
				if err := os.Chtimes(filepath.Join(project, "wiki/Home.md"), remoteInfo.ModTime(), remoteInfo.ModTime()); err != nil {
					t.Fatal(err)
				}
			}
			var comparison string
			commands := 0
			for line := range strings.SplitSeq(guidance, "\n") {
				if !strings.HasPrefix(line, "     ") {
					continue
				}
				commands++
				args := strings.Fields(line)
				if args[0] != "rsync" {
					continue
				}
				cmd := exec.CommandContext(t.Context(), "rsync", args[1:]...) // #nosec G204 -- Arguments come from fixed guidance with a literal test URL.
				cmd.Dir = project
				output, err := cmd.CombinedOutput()
				if err != nil {
					t.Fatalf("command failed: %v\n%s", err, output)
				}
				if args[1] == "-naci" {
					comparison = string(output)
				}
			}
			wantCommands := 3
			if existing {
				wantCommands = 4
			}
			if commands != wantCommands {
				t.Fatalf("command count = %d, want %d", commands, wantCommands)
			}
			check := func(name, want string) {
				t.Helper()
				got, err := os.ReadFile(filepath.Join(root, name))
				if err != nil || string(got) != want {
					t.Fatalf("%s = %q, %v; want %q", name, got, err, want)
				}
			}
			if existing {
				check("project/wiki/Home.md", "local!")
				if !strings.Contains(comparison, "Home.md") || strings.Contains(comparison, "Guide.md") {
					t.Fatalf("comparison did not isolate remaining conflicts: %s", comparison)
				}
			} else {
				check("project/wiki/Home.md", "remote")
			}
			check("project/wiki/.hidden", "hidden")
			check("project/wiki/nested/Guide.md", "guide")
			if _, err := os.Stat(filepath.Join(project, "wiki/.git")); !errors.Is(err, os.ErrNotExist) {
				t.Fatalf("copy included .git: %v", err)
			}
			if existing {
				check("project/wiki/Local.md", "local only")
				check("project/wiki/.tailor-wiki-base", "old baseline")
			} else if _, err := os.Stat(filepath.Join(project, "wiki/.tailor-wiki-base")); !errors.Is(err, os.ErrNotExist) {
				t.Fatalf("copy recorded the baseline before review: %v", err)
			}
		})
	}
}

func TestWikiImportGuidanceUsesLocalSourceState(t *testing.T) {
	t.Cleanup(gh.SetInspectWikiFunc(func(string, string, string) error {
		return errors.New("wiki/.tailor-wiki-base is missing")
	}))
	for _, existing := range []bool{false, true} {
		for _, mode := range []ApplyMode{DryRun, Apply} {
			t.Run(fmt.Sprintf("existing=%t/mode=%d", existing, mode), func(t *testing.T) {
				dir := t.TempDir()
				if existing {
					wikiTestWrite(t, dir, "wiki/Home.md", "local page")
				}
				target, _ := wikiTestTarget(t, 200, `{"private":false,"has_wiki":true,"default_branch":"main"}`)
				result, err := preflightWiki(wikiTestConfig(new(true)), dir, mode, target, true)
				var guidance string
				if mode == DryRun {
					if err != nil {
						t.Fatal(err)
					}
					guidance = result.nextSteps
				} else {
					if err == nil {
						t.Fatal("expected missing baseline error")
					}
					guidance = err.Error()
				}
				if strings.Contains(guidance, "--ignore-existing") != existing {
					t.Fatalf("wrong guidance for existing=%t: %s", existing, guidance)
				}
			})
		}
	}
}
