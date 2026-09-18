package swatch_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/wimpysworld/tailor/internal/swatch"
)

func TestRenderedJustfileRelease(t *testing.T) {
	for _, program := range []string{"just", "git", "bash"} {
		if _, err := exec.LookPath(program); err != nil {
			t.Skip(program + " is required for offline release tests")
		}
	}
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	t.Setenv("GIT_CONFIG_GLOBAL", os.DevNull)
	t.Setenv("GIT_AUTHOR_NAME", "Release Test")
	t.Setenv("GIT_AUTHOR_EMAIL", "release@example.invalid")
	t.Setenv("GIT_COMMITTER_NAME", "Release Test")
	t.Setenv("GIT_COMMITTER_EMAIL", "release@example.invalid")
	for _, tt := range []struct {
		name      string
		includeGo bool
	}{
		{name: "base"},
		{name: "go", includeGo: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			if err := os.Mkdir(filepath.Join(dir, "just"), 0o755); err != nil {
				t.Fatal(err)
			}
			loader := string(mustSwatchContent(t, "just/loader.just"))
			const placeholder = "[[TAILOR_MANAGED_IMPORTS]]"
			if count := strings.Count(loader, placeholder); count != 1 {
				t.Fatalf("just/loader.just imports placeholders = %d, want 1", count)
			}
			// Keep these fixture imports local. The alter package tests production registry coverage.
			const fixtureImports = "import? \"tailor.just\"\nimport? \"go.just\"\nimport? \"pages.just\""
			loader = strings.Replace(loader, placeholder, fixtureImports, 1)
			files := map[string][]byte{
				"justfile":         mustSwatchContent(t, "justfile"),
				"just/loader.just": []byte(loader),
				"just/tailor.just": mustSwatchContent(t, "just/tailor.just"),
			}
			if tt.includeGo {
				files["just/go.just"] = mustSwatchContent(t, "just/go.just")
			}
			for name, content := range files {
				if err := os.WriteFile(filepath.Join(dir, name), content, 0o644); err != nil {
					t.Fatal(err)
				}
			}
			run := func(program string, args ...string) (string, error) {
				t.Helper()
				command := exec.CommandContext(t.Context(), program, args...) // #nosec G204 -- Isolated local release fixtures.
				command.Dir = dir
				output, err := command.CombinedOutput()
				return strings.TrimSpace(string(output)), err
			}
			git := func(args ...string) string {
				t.Helper()
				output, err := run("git", args...)
				if err != nil {
					t.Fatalf("git %v: %v\n%s", args, err, output)
				}
				return output
			}
			git("init")
			git("add", ".")
			git("commit", "-m", "Add just recipes")
			for _, version := range []string{
				"invalid",
				"$(touch VERSION_MARKER)invalid",
				"`touch VERSION_MARKER`invalid",
				`"; touch VERSION_MARKER; #`,
			} {
				output, err := run("just", "release", version)
				if err == nil || !strings.Contains(output, "VERSION must be in format x.y.z") {
					t.Errorf("release %q: %v\n%s", version, err, output)
				}
				if _, err := os.Stat(filepath.Join(dir, "VERSION_MARKER")); !os.IsNotExist(err) {
					t.Fatalf("release %q executed the version argument: %v", version, err)
				}
			}
			if tags := git("tag", "--list"); tags != "" {
				t.Fatalf("invalid versions created tags: %s", tags)
			}
			if output, err := run("just", "release", "1.2.3"); err != nil {
				t.Fatalf("valid release: %v\n%s", err, output)
			}
			if kind := git("cat-file", "-t", "refs/tags/v1.2.3"); kind != "tag" {
				t.Fatalf("release tag type = %q, want annotated tag", kind)
			}
			tag := git("rev-parse", "refs/tags/v1.2.3")
			if output, err := run("just", "release", "1.2.3"); err == nil || !strings.Contains(output, "Tag v1.2.3 already exists") {
				t.Fatalf("duplicate release: %v\n%s", err, output)
			}
			if got := git("rev-parse", "refs/tags/v1.2.3"); got != tag {
				t.Fatalf("duplicate release changed tag from %s to %s", tag, got)
			}
		})
	}
}

func mustSwatchContent(t *testing.T, name string) []byte {
	t.Helper()
	content, err := swatch.Content(name)
	if err != nil {
		t.Fatal(err)
	}
	return content
}
