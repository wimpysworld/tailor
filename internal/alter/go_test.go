package alter_test

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/wimpysworld/tailor/internal/alter"
	"github.com/wimpysworld/tailor/internal/config"
	"github.com/wimpysworld/tailor/internal/swatch"
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
