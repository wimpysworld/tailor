package alter

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"runtime"
	"slices"
	"strings"
	"testing"

	"github.com/cli/go-gh/v2/pkg/api"
	"github.com/wimpysworld/tailor/internal/config"
	"github.com/wimpysworld/tailor/internal/model"
	"github.com/wimpysworld/tailor/internal/swatch"
	"github.com/wimpysworld/tailor/internal/testutil"
)

func socialTestClient(t *testing.T, description string) *api.RESTClient {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/user":
			_, _ = io.WriteString(w, `{"login":"tailor"}`)
		case r.Method == http.MethodGet && r.URL.Path == "/repos/wimpysworld/tailor":
			_ = json.NewEncoder(w).Encode(map[string]any{"description": description})
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)
	return testutil.NewTestClient(t, server)
}

func initSocialTestRepo(t *testing.T, dir, remoteURL string) {
	t.Helper()
	git := requireManagedExecutable(t, "git")
	managedGit(t, git, dir, "init")
	managedGit(t, git, dir, "remote", "add", "origin", remoteURL)
}

func TestSocialEnabledCreatesUpdatesAndPreservesOGImage(t *testing.T) {
	bespokeOG := []byte("bespoke-og-png-bytes\n")

	for _, mode := range []ApplyMode{DryRun, Apply, Recut} {
		t.Run(managedModeName(mode), func(t *testing.T) {
			dir := t.TempDir()
			initSocialTestRepo(t, dir, "https://github.com/wimpysworld/tailor.git")
			writeManagedTestFile(t, dir, "pages/og.png", bespokeOG)
			writeManagedTestFile(t, dir, "pages/style.css", []byte(":root { --mu-primary: #ff6600; }\n"))

			cfg := &config.Config{
				License: "none",
				Repository: &model.RepositorySettings{
					Description: new("Ready-to-wear project templates 👔"),
				},
				Pages: &model.PagesSettings{
					CNAME: new("tailor.wimpys.world"),
				},
				Social: &model.SocialSettings{
					Enabled:    new(true),
					Theme:      new("light"),
					Accent:     new("#ff6600"),
					Background: new("#0f172a"),
					Platforms:  new("Linux, macOS"),
					OG: &model.SocialOGSettings{
						Output:         new("pages/og.png"),
						IncludePreview: new(true),
						Image:          new("pages/demo.png"),
						Terminal:       &[]string{"$ tailor fit .", "Done 'ok'"},
					},
					Live: &model.SocialLiveSettings{
						Branch:   new("live-social"),
						Schedule: new("30 6 * * 1"),
					},
				},
			}

			client := socialTestClient(t, *cfg.Repository.Description)
			before := snapshotManagedTree(t, dir)
			report, err := Execute(cfg, dir, mode, client, io.Discard, Options{})
			if err != nil {
				t.Fatalf("Execute(%s) error: %v", managedModeName(mode), err)
			}

			for _, path := range []string{"nix/og.nix", "just/og.just", socialWorkflowDestination} {
				if !managedReportHasPath(report, path) {
					t.Errorf("report lacks social path %q:\n%s", path, report.Plain)
				}
			}

			if mode == DryRun {
				for _, path := range []string{"nix/og.nix", "just/og.just", socialWorkflowDestination} {
					assertManagedMissing(t, filepath.Join(dir, filepath.FromSlash(path)))
					if !strings.Contains(report.Plain, "would copy:") {
						t.Errorf("DryRun report lacks would copy:\n%s", report.Plain)
					}
				}
				if after := snapshotManagedTree(t, dir); !reflect.DeepEqual(after, before) {
					t.Fatalf("DryRun modified files: before=%v after=%v", before, after)
				}
				return
			}

			// Bespoke pages/og.png must never be overwritten, even under --recut.
			assertManagedBytes(t, filepath.Join(dir, "pages", "og.png"), bespokeOG)

			// Verify loaders reference og.just and og.nix.
			justLoader, err := os.ReadFile(filepath.Join(dir, "just", "loader.just"))
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(string(justLoader), `import? "og.just"`) {
				t.Errorf("just/loader.just lacks og.just import:\n%s", justLoader)
			}
			nixLoader, err := os.ReadFile(filepath.Join(dir, "nix", "loader.nix"))
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(string(nixLoader), "builtins.pathExists ./og.nix") {
				t.Errorf("nix/loader.nix lacks og.nix import:\n%s", nixLoader)
			}

			// Verify nix/og.nix matches embedded template.
			wantNix, err := swatch.Content("nix/og.nix")
			if err != nil {
				t.Fatal(err)
			}
			assertManagedBytes(t, filepath.Join(dir, "nix", "og.nix"), wantNix)

			// Verify just/og.just rendered arguments.
			ogJustBytes, err := os.ReadFile(filepath.Join(dir, "just", "og.just"))
			if err != nil {
				t.Fatal(err)
			}
			ogJust := string(ogJustBytes)
			if strings.Contains(ogJust, managedOGArgsPlaceholder) {
				t.Fatalf("just/og.just contains unresolved placeholder:\n%s", ogJust)
			}
			for _, want := range []string{
				"--output 'pages/og.png'",
				"--theme 'light'",
				"--style-css 'pages/style.css'",
				"--accent '#ff6600'",
				"--background '#0f172a'",
				"--title 'tailor'",
				"--tagline 'Ready-to-wear project templates'",
				"--footer 'tailor.wimpys.world'",
				"--platforms 'Linux, macOS'",
				"--include-preview",
				"--image 'pages/demo.png'",
				"--terminal-line '$ tailor fit .'",
				`--terminal-line 'Done '\''ok'\'''`,
			} {
				if !strings.Contains(ogJust, want) {
					t.Errorf("just/og.just lacks %q:\n%s", want, ogJust)
				}
			}

			// Verify .github/workflows/tailor-social.yml rendered schedule and branch.
			workflowBytes, err := os.ReadFile(filepath.Join(dir, filepath.FromSlash(socialWorkflowDestination)))
			if err != nil {
				t.Fatal(err)
			}
			workflow := string(workflowBytes)
			if !strings.HasPrefix(workflow, socialWorkflowMarker+"\n") {
				t.Errorf("social workflow lacks marker:\n%s", workflow)
			}
			if !strings.Contains(workflow, "cron: '30 6 * * 1'") {
				t.Errorf("social workflow lacks rendered schedule:\n%s", workflow)
			}
			if !strings.Contains(workflow, "branch='live-social'") {
				t.Errorf("social workflow lacks rendered branch:\n%s", workflow)
			}
			if strings.Contains(workflow, "{{SCHEDULE}}") || strings.Contains(workflow, "{{BRANCH}}") {
				t.Errorf("social workflow contains unresolved placeholders:\n%s", workflow)
			}

			// Idempotent re-run reports no change.
			secondReport, err := Execute(cfg, dir, mode, client, io.Discard, Options{})
			if err != nil {
				t.Fatalf("second Execute error: %v", err)
			}
			for _, path := range []string{"nix/og.nix", "just/og.just", socialWorkflowDestination} {
				line := findPlainLine(secondReport.Plain, path)
				if !strings.HasPrefix(line, "no change:") {
					t.Errorf("second run for %q = %q, want no change", path, line)
				}
			}

			// Updating config previews overwrite in DryRun and updates in Apply.
			cfg.Social.Live.Schedule = new("15 2 * * *")
			cfg.Social.Theme = new("dark")
			dryReport, err := Execute(cfg, dir, DryRun, client, io.Discard, Options{})
			if err != nil {
				t.Fatalf("DryRun update error: %v", err)
			}
			for _, path := range []string{"just/og.just", socialWorkflowDestination} {
				line := findPlainLine(dryReport.Plain, path)
				if !strings.HasPrefix(line, "would overwrite:") {
					t.Errorf("DryRun update for %q = %q, want would overwrite", path, line)
				}
			}
			// Disk still has old workflow after DryRun.
			assertManagedBytes(t, filepath.Join(dir, filepath.FromSlash(socialWorkflowDestination)), workflowBytes)

			applyReport, err := Execute(cfg, dir, Apply, client, io.Discard, Options{})
			if err != nil {
				t.Fatalf("Apply update error: %v", err)
			}
			for _, path := range []string{"just/og.just", socialWorkflowDestination} {
				line := findPlainLine(applyReport.Plain, path)
				if !strings.HasPrefix(line, "overwritten:") {
					t.Errorf("Apply update for %q = %q, want overwritten", path, line)
				}
			}
			updatedWorkflow, err := os.ReadFile(filepath.Join(dir, filepath.FromSlash(socialWorkflowDestination)))
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(string(updatedWorkflow), "cron: '15 2 * * *'") {
				t.Errorf("updated workflow lacks new schedule:\n%s", updatedWorkflow)
			}
		})
	}
}

func TestSocialDisabledRemovesOwnedFilesAndPreservesUserAssets(t *testing.T) {
	ogPNG := []byte("user-og-png")
	heroSVG := []byte("<svg>hero</svg>\n")
	readme := []byte("# Project\n<!-- tailor:social-header:start -->\n<!-- tailor:social-header:end -->\n")

	for _, mode := range []ApplyMode{DryRun, Apply, Recut} {
		t.Run(managedModeName(mode)+"/marked-workflow", func(t *testing.T) {
			dir := t.TempDir()
			writeManagedTestFile(t, dir, "nix/og.nix", managedContent("nix/og.nix", "[]"))
			writeManagedTestFile(t, dir, "just/og.just", managedContent("just/og.just", "og:"))
			writeManagedTestFile(t, dir, socialWorkflowDestination, []byte(socialWorkflowMarker+"\nname: Social assets\n"))
			writeManagedTestFile(t, dir, "pages/og.png", ogPNG)
			writeManagedTestFile(t, dir, "pages/social/hero-dark.svg", heroSVG)
			writeManagedTestFile(t, dir, "README.md", readme)

			cfg := &config.Config{
				License: "none",
				Social:  &model.SocialSettings{Enabled: new(false)},
			}
			report, err := Execute(cfg, dir, mode, managedProductionClient(t), io.Discard, Options{})
			if err != nil {
				t.Fatalf("Execute error: %v", err)
			}

			// Authored assets and README are always preserved.
			assertManagedBytes(t, filepath.Join(dir, "pages", "og.png"), ogPNG)
			assertManagedBytes(t, filepath.Join(dir, "pages", "social", "hero-dark.svg"), heroSVG)
			assertManagedBytes(t, filepath.Join(dir, "README.md"), readme)

			if mode == DryRun {
				for _, path := range []string{"nix/og.nix", "just/og.just", socialWorkflowDestination} {
					line := findPlainLine(report.Plain, path)
					if !strings.HasPrefix(line, "would remove:") {
						t.Errorf("DryRun line for %q = %q, want would remove", path, line)
					}
					if _, err := os.Lstat(filepath.Join(dir, filepath.FromSlash(path))); err != nil {
						t.Errorf("DryRun removed %q: %v", path, err)
					}
				}
				return
			}

			for _, path := range []string{"nix/og.nix", "just/og.just", socialWorkflowDestination} {
				line := findPlainLine(report.Plain, path)
				if !strings.HasPrefix(line, "removed:") {
					t.Errorf("Apply line for %q = %q, want removed", path, line)
				}
				assertManagedMissing(t, filepath.Join(dir, filepath.FromSlash(path)))
			}
		})

		t.Run(managedModeName(mode)+"/user-owned-workflow-preserved", func(t *testing.T) {
			dir := t.TempDir()
			userWorkflow := []byte("name: Custom social workflow\non: workflow_dispatch\n")
			writeManagedTestFile(t, dir, "nix/og.nix", managedContent("nix/og.nix", "[]"))
			writeManagedTestFile(t, dir, "just/og.just", managedContent("just/og.just", "og:"))
			writeManagedTestFile(t, dir, socialWorkflowDestination, userWorkflow)

			cfg := &config.Config{
				License: "none",
				Social:  &model.SocialSettings{Enabled: new(false)},
			}
			report, err := Execute(cfg, dir, mode, managedProductionClient(t), io.Discard, Options{})
			if err != nil {
				t.Fatalf("Execute error: %v", err)
			}

			assertManagedBytes(t, filepath.Join(dir, filepath.FromSlash(socialWorkflowDestination)), userWorkflow)
			line := findPlainLine(report.Plain, socialWorkflowDestination)
			if !strings.HasPrefix(line, "skipped:") {
				t.Errorf("unmarked workflow line = %q, want skipped", line)
			}
			if mode.ShouldWrite() {
				assertManagedMissing(t, filepath.Join(dir, "nix", "og.nix"))
				assertManagedMissing(t, filepath.Join(dir, "just", "og.just"))
			}
		})

		t.Run(managedModeName(mode)+"/already-absent", func(t *testing.T) {
			dir := t.TempDir()
			cfg := &config.Config{
				License: "none",
				Social:  &model.SocialSettings{Enabled: new(false)},
			}
			report, err := Execute(cfg, dir, mode, managedProductionClient(t), io.Discard, Options{})
			if err != nil {
				t.Fatalf("Execute error: %v", err)
			}
			if managedReportHasPath(report, socialWorkflowDestination) {
				t.Errorf("absent workflow should not be reported:\n%s", report.Plain)
			}
		})
	}
}

func TestSocialOmittedLeavesFilesUntouched(t *testing.T) {
	cases := []struct {
		name   string
		social *model.SocialSettings
	}{
		{name: "nil-section", social: nil},
		{name: "nil-enabled", social: &model.SocialSettings{Theme: new("light")}},
	}

	for _, tc := range cases {
		for _, mode := range []ApplyMode{DryRun, Apply, Recut} {
			t.Run(tc.name+"/"+managedModeName(mode), func(t *testing.T) {
				dir := t.TempDir()
				unmanagedNix := []byte("unmanaged nix/og.nix\n")
				unmanagedJust := []byte("unmanaged just/og.just\n")
				unmanagedWorkflow := []byte("unmanaged tailor-social.yml\n")
				ogPNG := []byte("user-og-png")

				writeManagedTestFile(t, dir, "nix/og.nix", unmanagedNix)
				writeManagedTestFile(t, dir, "just/og.just", unmanagedJust)
				writeManagedTestFile(t, dir, socialWorkflowDestination, unmanagedWorkflow)
				writeManagedTestFile(t, dir, "pages/og.png", ogPNG)

				cfg := &config.Config{License: "none", Social: tc.social}
				report, err := Execute(cfg, dir, mode, managedProductionClient(t), io.Discard, Options{})
				if err != nil {
					t.Fatalf("Execute error: %v", err)
				}

				assertManagedBytes(t, filepath.Join(dir, "nix", "og.nix"), unmanagedNix)
				assertManagedBytes(t, filepath.Join(dir, "just", "og.just"), unmanagedJust)
				assertManagedBytes(t, filepath.Join(dir, filepath.FromSlash(socialWorkflowDestination)), unmanagedWorkflow)
				assertManagedBytes(t, filepath.Join(dir, "pages", "og.png"), ogPNG)

				for _, path := range []string{"nix/og.nix", "just/og.just", socialWorkflowDestination} {
					if managedReportHasPath(report, path) {
						t.Errorf("omitted social reported %q:\n%s", path, report.Plain)
					}
				}
			})
		}
	}
}

func TestSocialWorkflowOwnershipConflictAndSymlinkRejection(t *testing.T) {
	t.Run("unmarked-workflow-blocks-enabled", func(t *testing.T) {
		dir := t.TempDir()
		userWorkflow := []byte("name: User owned\n")
		writeManagedTestFile(t, dir, socialWorkflowDestination, userWorkflow)

		cfg := &config.Config{
			License: "none",
			Social:  &model.SocialSettings{Enabled: new(true)},
		}
		_, err := Execute(cfg, dir, Apply, managedProductionClient(t), io.Discard, Options{})
		if err == nil || !strings.Contains(err.Error(), "social workflow ownership conflict") {
			t.Fatalf("Execute error = %v, want ownership conflict", err)
		}
		assertManagedBytes(t, filepath.Join(dir, filepath.FromSlash(socialWorkflowDestination)), userWorkflow)
		assertManagedMissing(t, filepath.Join(dir, "nix", "og.nix"))
		assertManagedMissing(t, filepath.Join(dir, "just", "og.just"))
	})

	t.Run("symlink-workflow-rejected", func(t *testing.T) {
		dir := t.TempDir()
		target := filepath.Join(t.TempDir(), "outside.yml")
		if err := os.WriteFile(target, []byte(socialWorkflowMarker+"\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := os.MkdirAll(filepath.Join(dir, ".github", "workflows"), 0o755); err != nil {
			t.Fatal(err)
		}
		managedSymlinkOrSkip(t, target, filepath.Join(dir, filepath.FromSlash(socialWorkflowDestination)))

		cfg := &config.Config{
			License: "none",
			Social:  &model.SocialSettings{Enabled: new(true)},
		}
		_, err := prepareSocialWorkflow(cfg, dir)
		if err == nil || !strings.Contains(err.Error(), "must be a regular file") {
			t.Fatalf("prepareSocialWorkflow error = %v, want regular file error", err)
		}
	})
}

func TestRenderManagedOGFooterAndStyleFallbacks(t *testing.T) {
	template, err := swatch.Content("just/og.just")
	if err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name      string
		cfg       *config.Config
		ctx       managedRenderContext
		setupDir  func(t *testing.T, dir string)
		wantParts []string
		omitParts []string
	}{
		{
			name: "cname beats homepage and repo",
			cfg: &config.Config{
				Pages:      &model.PagesSettings{Enabled: new(true), CNAME: new("https://tailor.wimpys.world/")},
				Repository: &model.RepositorySettings{Homepage: new("https://fallback.example")},
				Social:     &model.SocialSettings{Enabled: new(true)},
			},
			ctx: managedRenderContext{Owner: "wimpysworld", Repo: "tailor"},
			wantParts: []string{
				"--style-css 'pages/style.css'",
				"--footer 'tailor.wimpys.world'",
				"--title 'tailor'",
			},
			omitParts: []string{"fallback.example", "github.com/wimpysworld/tailor", "--include-preview"},
		},
		{
			name: "homepage beats github fallback",
			cfg: &config.Config{
				Repository: &model.RepositorySettings{Homepage: new("https://docs.example.org/")},
				Social:     &model.SocialSettings{Enabled: new(true)},
			},
			ctx:       managedRenderContext{Owner: "wimpysworld", Repo: "demo"},
			wantParts: []string{"--footer 'docs.example.org'", "--title 'demo'"},
			omitParts: []string{"--style-css", "github.com/wimpysworld/demo"},
		},
		{
			name: "github fallback when cname and homepage absent",
			cfg: &config.Config{
				Social: &model.SocialSettings{Enabled: new(true)},
			},
			ctx:       managedRenderContext{Owner: "wimpysworld", Repo: "demo"},
			wantParts: []string{"--footer 'github.com/wimpysworld/demo'"},
			omitParts: []string{"--style-css"},
		},
		{
			name: "style-css detected on disk when pages disabled",
			cfg: &config.Config{
				Pages:  &model.PagesSettings{Path: new("site")},
				Social: &model.SocialSettings{Enabled: new(true)},
			},
			setupDir: func(t *testing.T, dir string) {
				t.Helper()
				writeManagedTestFile(t, dir, "site/style.css", []byte("body{}\n"))
			},
			wantParts: []string{"--style-css 'site/style.css'"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := tt.ctx
			if tt.setupDir != nil {
				ctx.Dir = t.TempDir()
				tt.setupDir(t, ctx.Dir)
			}
			rendered, err := renderManagedOG(tt.cfg, template, ctx)
			if err != nil {
				t.Fatalf("renderManagedOG error: %v", err)
			}
			text := string(rendered)
			for _, part := range tt.wantParts {
				if !strings.Contains(text, part) {
					t.Errorf("rendered just/og.just lacks %q:\n%s", part, text)
				}
			}
			for _, part := range tt.omitParts {
				if strings.Contains(text, part) {
					t.Errorf("rendered just/og.just unexpectedly contains %q:\n%s", part, text)
				}
			}
		})
	}
}

func TestManagedOGRecipePassesSafelyQuotedArgsToStub(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("managed just recipes require unix shell")
	}
	just := requireManagedExecutable(t, "just")
	root := t.TempDir()
	argsFile := filepath.Join(root, "tailor-og.args")
	stub := filepath.Join(root, "bin", "tailor-og")
	writeManagedExecutable(t, stub, "#!/usr/bin/env bash\nset -eu\nprintf '%s\\0' \"$@\" > \"$TAILOR_OG_ARGS_FILE\"\n")

	cfg := &config.Config{
		License: "none",
		Repository: &model.RepositorySettings{
			Description: new(`Quotes 'single' "double" $HOME ` + "`id`" + ` {{expr}} ✨`),
		},
		Social: &model.SocialSettings{
			Enabled:   new(true),
			Platforms: new("Linux & macOS; BSD"),
			OG: &model.SocialOGSettings{
				IncludePreview: new(true),
				Terminal:       &[]string{"$ echo '{{hello}}'", "it's safe"},
			},
		},
		Swatches: []config.SwatchEntry{{Path: "justfile", Alteration: swatch.FirstFit}},
	}
	selections, err := selectManagedFiles(cfg)
	if err != nil {
		t.Fatal(err)
	}
	rendered, err := renderManagedFilesWithContext(cfg, selections, managedRenderContext{
		Dir:   root,
		Owner: "wimpysworld",
		Repo:  "tailor",
	})
	if err != nil {
		t.Fatal(err)
	}
	writeManagedJustFixture(t, root, rendered)

	cmd := exec.CommandContext(t.Context(), just, "--justfile", "justfile", "og", "--force") // #nosec G204 -- controlled test executable
	cmd.Dir = root
	cmd.Env = append(managedMiniserveEnvironment(filepath.Dir(stub), "TAILOR_OG_ARGS_FILE"), "TAILOR_OG_ARGS_FILE="+argsFile)
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("just og --force: %v\n%s", err, output)
	}

	raw, err := os.ReadFile(argsFile)
	if err != nil {
		t.Fatal(err)
	}
	got := strings.Split(strings.TrimSuffix(string(raw), "\x00"), "\x00")
	want := []string{
		"--output", "pages/og.png",
		"--theme", "dark",
		"--title", "tailor",
		"--tagline", `Quotes 'single' "double" $HOME ` + "`id`" + ` {{expr}}`,
		"--footer", "github.com/wimpysworld/tailor",
		"--platforms", "Linux & macOS; BSD",
		"--include-preview",
		"--terminal-line", "$ echo '{{hello}}'",
		"--terminal-line", "it's safe",
		"--force",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("tailor-og argv = %#v, want %#v", got, want)
	}
}

func TestSocialOGNotInSwatchRegistry(t *testing.T) {
	if slices.Contains(swatch.Paths(), "pages/og.png") {
		t.Fatal("pages/og.png must not be registered in swatch.Paths()")
	}
	if slices.Contains(swatch.PagesStarterPaths, "pages/og.png") {
		t.Fatal("pages/og.png must not be registered in swatch.PagesStarterPaths")
	}
}

func findPlainLine(plain, needle string) string {
	for line := range strings.SplitSeq(plain, "\n") {
		if strings.Contains(line, needle) {
			return strings.TrimSpace(line)
		}
	}
	return ""
}
