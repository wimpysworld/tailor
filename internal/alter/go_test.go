package alter_test

import (
	"bytes"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/wimpysworld/tailor/internal/alter"
	"github.com/wimpysworld/tailor/internal/config"
	"github.com/wimpysworld/tailor/internal/ghfake"
	"github.com/wimpysworld/tailor/internal/model"
	"github.com/wimpysworld/tailor/internal/swatch"
	"github.com/wimpysworld/tailor/internal/testutil"
)

func goConfig(entries ...config.SwatchEntry) *config.Config {
	enabled := true
	return &config.Config{Languages: &config.LanguageSettings{Go: &enabled}, Swatches: entries}
}

func TestExecuteGoBuilderDefaultBranch(t *testing.T) {
	for _, noRepo := range []bool{false, true} {
		t.Run(map[bool]string{false: "repository", true: "local"}[noRepo], func(t *testing.T) {
			options := []testOption{WithRepoSettings(repoJSON{DefaultBranch: "trunk"})}
			if noRepo {
				options = append(options, WithNoRepo())
			}
			tc := setupAlterTest(t, "languages:\n  go: true\nswatches: []\n", options...)
			writeOnDisk(t, tc.Dir, "go.mod", []byte("module example.com/demo\n\ngo 1.26\n"))
			writeOnDisk(t, tc.Dir, "main.go", []byte("package main\nfunc main() {}\n"))
			cfg := goConfig(entry(".golangci.yml", swatch.FirstFit), entry(".goreleaser.yaml", swatch.FirstFit), entry(".github/workflows/build-go.yml", swatch.FirstFit))
			if _, err := alter.Execute(cfg, tc.Dir, alter.Apply, tc.Client, nil, alter.Options{}); err != nil {
				t.Fatal(err)
			}
			data, err := os.ReadFile(filepath.Join(tc.Dir, ".github/workflows/build-go.yml"))
			if err != nil || bytes.Contains(data, []byte("[[TAILOR_")) {
				t.Fatalf("builder unresolved: %v", err)
			}
			if !noRepo && !bytes.Contains(data, []byte("trunk")) {
				t.Fatal("builder does not use repository default branch")
			}
			for _, path := range []string{".golangci.yml", ".goreleaser.yaml"} {
				if _, err := os.Stat(filepath.Join(tc.Dir, path)); err != nil {
					t.Fatalf("dependency %s missing: %v", path, err)
				}
			}
		})
	}
}

func TestExecuteGoBuilderMetadata(t *testing.T) {
	for _, tt := range []struct {
		name   string
		status int
		body   string
		header string
		fatal  bool
		branch string
	}{
		{name: "forbidden", status: 403, body: `{"message":"Forbidden"}`},
		{name: "not found", status: 404, body: `{"message":"Not Found"}`},
		{name: "private branch", status: 200, body: `{"private":true,"default_branch":"trunk"}`, branch: "trunk"},
		{name: "unproved scope branch", status: 200, body: `{"default_branch":"trunk"}`, branch: "trunk"},
		{name: "primary rate limit", status: 403, body: `{"message":"Forbidden"}`, header: "X-RateLimit-Remaining", fatal: true},
		{name: "secondary rate limit", status: 403, body: `{"message":"Forbidden"}`, header: "Retry-After", fatal: true},
		{name: "message rate limit", status: 403, body: `{"message":"API rate limit exceeded"}`, fatal: true},
		{name: "too many requests", status: 429, body: `{}`, fatal: true},
		{name: "server failure", status: 500, body: `{}`, fatal: true},
		{name: "invalid json", status: 200, body: `{`, fatal: true},
		{name: "missing branch", status: 200, body: `{}`, fatal: true},
		{name: "transport failure", fatal: true},
	} {
		for _, pagesEnabled := range []bool{false, true} {
			for _, mode := range []alter.ApplyMode{alter.DryRun, alter.Apply} {
				t.Run(fmt.Sprintf("%s/pages=%v/mode=%v", tt.name, pagesEnabled, mode), func(t *testing.T) {
					ghfake.FakeRepo(t, "testowner", "testrepo")
					reads := 0
					server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
						if r.Method != http.MethodGet {
							t.Errorf("unexpected write: %s %s", r.Method, r.URL.Path)
						}
						w.Header().Set("Content-Type", "application/json")
						switch r.URL.Path {
						case "/user":
							fmt.Fprint(w, `{"login":"testuser"}`)
						case "/repos/testowner/testrepo":
							reads++
							if tt.status == 0 {
								conn, _, err := w.(http.Hijacker).Hijack()
								if err != nil {
									t.Error(err)
									return
								}
								_ = conn.Close()
								return
							}
							if tt.header != "" {
								w.Header().Set(tt.header, "0")
							}
							w.WriteHeader(tt.status)
							fmt.Fprint(w, tt.body)
						default:
							t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
							w.WriteHeader(http.StatusNotFound)
						}
					}))
					defer server.Close()
					dir := t.TempDir()
					writeOnDisk(t, dir, ".tailor.yml", []byte("languages:\n  go: true\nswatches: []\n"))
					writeOnDisk(t, dir, "go.mod", []byte("module example.com/demo\n\ngo 1.26\n"))
					writeOnDisk(t, dir, "main.go", []byte("package main\nfunc main() {}\n"))
					writeOnDisk(t, dir, "pages/index.html", []byte("<h1>Existing site</h1>"))
					cfg := goConfig(entry(".golangci.yml", swatch.FirstFit), entry(".goreleaser.yaml", swatch.FirstFit), entry(".github/workflows/build-go.yml", swatch.FirstFit))
					cfg.Pages = &model.PagesSettings{Enabled: &pagesEnabled}
					before := pagesAcceptanceSnapshot(t, dir)
					report, err := alter.Execute(cfg, dir, mode, testutil.NewTestClient(t, server), nil, alter.Options{})
					if (err != nil) != tt.fatal {
						t.Fatalf("error = %v, want fatal = %v", err, tt.fatal)
					}
					if tt.status != 0 && reads != 1 {
						t.Fatalf("repository reads = %d, want 1", reads)
					}
					if tt.fatal || mode == alter.DryRun {
						if !reflect.DeepEqual(before, pagesAcceptanceSnapshot(t, dir)) {
							t.Fatal("preview or failed preflight changed local files")
						}
					}
					if tt.fatal {
						return
					}
					for _, s := range cfg.Swatches {
						if !strings.Contains(report.Plain, s.Path) {
							t.Fatalf("report omits %s: %s", s.Path, report.Plain)
						}
						if mode == alter.Apply {
							if _, err := os.Stat(filepath.Join(dir, s.Path)); err != nil {
								t.Fatalf("Go swatch %s missing: %v", s.Path, err)
							}
						}
					}
					if mode == alter.Apply {
						data, err := os.ReadFile(filepath.Join(dir, ".github/workflows/build-go.yml"))
						want := tt.branch
						if want == "" {
							want = "github.event.repository.default_branch"
						}
						if err != nil || !bytes.Contains(data, []byte(want)) || bytes.Contains(data, []byte("[[TAILOR_")) {
							t.Fatalf("builder lacks resolved branch or runtime guard: %v", err)
						}
					}
				})
			}
		}
	}
}

func TestGoSwatchModes(t *testing.T) {
	for _, mode := range []alter.ApplyMode{alter.DryRun, alter.Apply, alter.Recut} {
		t.Run(map[alter.ApplyMode]string{alter.DryRun: "preview", alter.Apply: "apply", alter.Recut: "recut"}[mode], func(t *testing.T) {
			dir := t.TempDir()
			writeOnDisk(t, dir, "go.mod", []byte("module example.com/demo\n\ngo 1.26\n"))
			writeOnDisk(t, dir, "cmd/demo/main.go", []byte("package main\nfunc main() {}\n"))
			cfg := goConfig(entry(".golangci.yml", swatch.FirstFit), entry(".goreleaser.yaml", swatch.FirstFit), entry(".github/workflows/build-go.yml", swatch.FirstFit))
			results, err := alter.ProcessSwatches(cfg, dir, mode, &alter.TokenContext{DefaultBranch: "trunk"})
			if err != nil || len(results) != 3 {
				t.Fatalf("results = %v, error = %v", results, err)
			}
			data, err := os.ReadFile(filepath.Join(dir, ".github/workflows/build-go.yml"))
			if mode == alter.DryRun {
				if !os.IsNotExist(err) {
					t.Fatalf("preview wrote builder: %v", err)
				}
				return
			}
			if err != nil || !bytes.Contains(data, []byte("trunk")) {
				t.Fatalf("builder lacks default branch: %v", err)
			}
			data, err = os.ReadFile(filepath.Join(dir, ".goreleaser.yaml"))
			if err != nil || !bytes.Contains(data, []byte("./cmd/demo")) {
				t.Fatalf("release config lacks discovered build: %s, %v", data, err)
			}
		})
	}
}

func TestGoInactivePreservesDestinations(t *testing.T) {
	for _, declared := range []bool{false, true} {
		cfg := goConfig(entry(".golangci.yml", swatch.Always), entry(".goreleaser.yaml", swatch.Always), entry(".github/workflows/build-go.yml", swatch.Always))
		if declared {
			*cfg.Languages.Go = false
		} else {
			cfg.Languages = nil
		}
		dir := t.TempDir()
		for _, s := range cfg.Swatches {
			writeOnDisk(t, dir, s.Path, []byte("custom"))
		}
		results, err := alter.ProcessSwatches(cfg, dir, alter.Recut, nil)
		if err != nil || len(results) != 0 {
			t.Fatalf("inactive results = %v, error = %v", results, err)
		}
		for _, s := range cfg.Swatches {
			data, err := os.ReadFile(filepath.Join(dir, s.Path))
			if err != nil || string(data) != "custom" {
				t.Fatalf("inactive file changed: %s", s.Path)
			}
		}
	}
}

func TestGoProtectedFilesDoNotNeedDiscovery(t *testing.T) {
	for _, mode := range []swatch.AlterationMode{swatch.FirstFit, swatch.Never} {
		dir := t.TempDir()
		cfg := goConfig(entry(".goreleaser.yaml", mode), entry(".github/workflows/build-go.yml", mode))
		if mode == swatch.FirstFit {
			for _, s := range cfg.Swatches {
				writeOnDisk(t, dir, s.Path, []byte("custom"))
			}
		}
		results, err := alter.ProcessSwatches(cfg, dir, alter.Apply, nil)
		if err != nil || len(results) != 2 {
			t.Fatalf("protected results = %v, error = %v", results, err)
		}
		for _, result := range results {
			if result.Category != alter.Skipped {
				t.Fatalf("protected result = %v", result)
			}
		}
	}
}

func TestGoRecutRequiresDiscoveryBeforeReplacingFirstFit(t *testing.T) {
	dir := t.TempDir()
	writeOnDisk(t, dir, ".goreleaser.yaml", []byte("custom"))
	cfg := goConfig(entry(".goreleaser.yaml", swatch.FirstFit))
	if _, err := alter.ProcessSwatches(cfg, dir, alter.Recut, nil); err == nil {
		t.Fatal("recut without a Go module succeeded")
	}
	data, err := os.ReadFile(filepath.Join(dir, ".goreleaser.yaml"))
	if err != nil || string(data) != "custom" {
		t.Fatalf("failed recut changed first-fit file: %s, %v", data, err)
	}
}

func TestGoPreflightBlocksBeforeWrites(t *testing.T) {
	for _, mode := range []alter.ApplyMode{alter.DryRun, alter.Apply, alter.Recut} {
		dir := t.TempDir()
		cfg := goConfig(entry(".gitignore", swatch.FirstFit), entry(".goreleaser.yaml", swatch.FirstFit))
		_, err := alter.Execute(cfg, dir, mode, nil, nil, alter.Options{})
		if err == nil || !strings.Contains(err.Error(), "go.mod") {
			t.Fatalf("error = %v, want local discovery error before authentication", err)
		}
		files, err := os.ReadDir(dir)
		if err != nil || len(files) != 0 {
			t.Fatalf("preflight changed project: %v, %v", files, err)
		}
	}
}

func TestGoBuilderDependencies(t *testing.T) {
	for _, existing := range []bool{false, true} {
		dir := t.TempDir()
		cfg := goConfig(entry(".github/workflows/build-go.yml", swatch.FirstFit), entry(".goreleaser.yaml", swatch.Never), entry(".golangci.yml", swatch.Never))
		if existing {
			writeOnDisk(t, dir, ".goreleaser.yaml", []byte("custom release configuration"))
			writeOnDisk(t, dir, ".golangci.yml", []byte("custom lint configuration"))
		}
		_, err := alter.ProcessSwatches(cfg, dir, alter.Apply, &alter.TokenContext{DefaultBranch: "trunk"})
		if existing && err != nil {
			t.Fatalf("existing custom dependencies rejected: %v", err)
		}
		if !existing && (err == nil || !strings.Contains(err.Error(), "requires .golangci.yml")) {
			t.Fatalf("missing dependency error = %v", err)
		}
	}
}

func TestGoBuilderRejectsSymlinkDependency(t *testing.T) {
	dir := t.TempDir()
	writeOnDisk(t, dir, "custom-lint.yml", []byte("custom"))
	symlinkOrSkip(t, "custom-lint.yml", filepath.Join(dir, ".golangci.yml"))
	cfg := goConfig(entry(".github/workflows/build-go.yml", swatch.FirstFit))
	if _, err := alter.ProcessSwatches(cfg, dir, alter.Apply, &alter.TokenContext{DefaultBranch: "trunk"}); err == nil {
		t.Fatal("symlink dependency accepted")
	}
	if _, err := os.Stat(filepath.Join(dir, ".github/workflows/build-go.yml")); !os.IsNotExist(err) {
		t.Fatalf("builder created after failed preflight: %v", err)
	}
}

func TestGoDependabotVariantsAndFirstFit(t *testing.T) {
	for _, declaration := range []string{"absent", "false", "true"} {
		dir := t.TempDir()
		cfg := goConfig(entry(".github/dependabot.yml", swatch.Always))
		if declaration == "absent" {
			cfg.Languages = nil
		} else {
			*cfg.Languages.Go = declaration == "true"
		}
		if _, err := alter.ProcessSwatches(cfg, dir, alter.Apply, nil); err != nil {
			t.Fatal(err)
		}
		data, err := os.ReadFile(filepath.Join(dir, ".github/dependabot.yml"))
		if err != nil || bytes.Contains(data, []byte("gomod")) != (declaration != "false") {
			t.Fatalf("incorrect Dependabot variant for %s: %v", declaration, err)
		}
		results, err := alter.ProcessSwatches(cfg, dir, alter.DryRun, nil)
		if err != nil || results[0].Category != alter.NoChange {
			t.Fatalf("resolved-content comparison = %v, %v", results, err)
		}
		cfg.Swatches[0].Alteration = swatch.FirstFit
		cfg.Languages = &config.LanguageSettings{Go: new(bool)}
		if _, err := alter.ProcessSwatches(cfg, dir, alter.Apply, nil); err != nil {
			t.Fatal(err)
		}
		after, err := os.ReadFile(filepath.Join(dir, ".github/dependabot.yml"))
		if err != nil || !bytes.Equal(data, after) {
			t.Fatal("language change overwrote first-fit Dependabot")
		}
	}
}
