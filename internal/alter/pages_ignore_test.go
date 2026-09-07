package alter

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/wimpysworld/tailor/internal/config"
	"github.com/wimpysworld/tailor/internal/swatch"
)

func TestProcessPagesIgnore(t *testing.T) {
	for _, tt := range []struct {
		name, generator, path, initial, want string
		exists                               bool
		mode                                 ApplyMode
		category                             SwatchCategory
	}{
		{name: "new Hugo", generator: "hugo", path: "pages", mode: Apply, want: "/pages/public/\n", category: WouldCopy},
		{name: "new Jekyll", generator: "jekyll", path: "docs", mode: Apply, want: "/docs/_site/\n", category: WouldCopy},
		{name: "preserve bytes", generator: "hugo", path: "pages", exists: true, initial: "# user rules\r\nsecret\n", mode: Apply, want: "# user rules\r\nsecret\n/pages/public/\n", category: WouldOverwrite},
		{name: "missing newline", generator: "hugo", path: "pages", exists: true, initial: "secret", mode: Apply, want: "secret\n/pages/public/\n", category: WouldOverwrite},
		{name: "existing exact rule", generator: "hugo", path: "pages", exists: true, initial: "/pages/public/\nsecret\n", mode: Apply, want: "/pages/public/\nsecret\n", category: NoChange},
		{name: "existing CRLF rule", generator: "hugo", path: "pages", exists: true, initial: "/pages/public/\r\nsecret\r\n", mode: Apply, want: "/pages/public/\r\nsecret\r\n", category: NoChange},
		{name: "existing unterminated rule", generator: "hugo", path: "pages", exists: true, initial: "/pages/public/", mode: Apply, want: "/pages/public/", category: NoChange},
		{name: "escape patterns", generator: "hugo", path: "docs [draft]/*?!#", mode: Apply, want: "/docs\\ \\[draft\\]/\\*\\?\\!\\#/public/\n", category: WouldCopy},
		{name: "root source", generator: "hugo", path: ".", mode: Apply, want: "/public/\n", category: WouldCopy},
		{name: "recut stays additive", generator: "jekyll", path: "pages", exists: true, initial: "secret\n", mode: Recut, want: "secret\n/pages/_site/\n", category: WouldOverwrite},
		{name: "preview existing", generator: "hugo", path: "pages", exists: true, initial: "secret", mode: DryRun, want: "secret", category: WouldOverwrite},
		{name: "preview missing", generator: "hugo", path: "pages", mode: DryRun, category: WouldCopy},
	} {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			file := filepath.Join(dir, ".gitignore")
			if tt.exists {
				if err := os.WriteFile(file, []byte(tt.initial), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			prepared := &pagesPreparation{Generator: tt.generator, Path: tt.path}
			result, err := processPagesIgnore(&config.Config{}, dir, tt.mode, prepared)
			if err != nil {
				t.Fatal(err)
			}
			if result == nil || result.Path != ".gitignore" || result.Category != tt.category {
				t.Fatalf("result = %+v, want %s", result, tt.category)
			}
			got, err := os.ReadFile(file)
			if tt.mode == DryRun && !tt.exists {
				if !os.IsNotExist(err) {
					t.Fatalf("preview created a file: %v", err)
				}
			} else if err != nil || string(got) != tt.want {
				t.Fatalf("content = %q, error = %v, want %q", got, err, tt.want)
			}
			if tt.mode.ShouldWrite() {
				result, err = processPagesIgnore(&config.Config{}, dir, tt.mode, prepared)
				if err != nil || result == nil || result.Category != NoChange {
					t.Fatalf("second apply = %+v, error = %v", result, err)
				}
			}
		})
	}
}

func TestProcessPagesIgnoreDisabledStaticAndNever(t *testing.T) {
	for _, mode := range []ApplyMode{DryRun, Apply, Recut} {
		for _, prepared := range []*pagesPreparation{nil, {Generator: "static", Path: "pages"}} {
			result, err := processPagesIgnore(&config.Config{}, "missing-root", mode, prepared)
			if err != nil || result != nil {
				t.Fatalf("disabled/static: result = %+v, error = %v", result, err)
			}
		}
		cfg := &config.Config{Swatches: []config.SwatchEntry{{Path: ".gitignore", Alteration: swatch.Never}}}
		result, err := processPagesIgnore(cfg, "missing-root", mode, &pagesPreparation{Generator: "hugo", Path: "pages"})
		if err != nil || result == nil || result.Category != Skipped || result.Reason != SkipModeNever {
			t.Fatalf("never: result = %+v, error = %v", result, err)
		}
	}
}

func TestProcessPagesIgnoreDestinationSafety(t *testing.T) {
	for _, mode := range []ApplyMode{DryRun, Apply, Recut} {
		t.Run(map[ApplyMode]string{DryRun: "preview", Apply: "apply", Recut: "recut"}[mode], func(t *testing.T) {
			dir, outside := t.TempDir(), t.TempDir()
			target := filepath.Join(outside, "ignore")
			if err := os.WriteFile(target, []byte("outside\n"), 0o644); err != nil {
				t.Fatal(err)
			}
			file := filepath.Join(dir, ".gitignore")
			if err := os.Symlink(target, file); err != nil {
				t.Fatal(err)
			}
			result, err := processPagesIgnore(&config.Config{}, dir, mode, &pagesPreparation{Generator: "hugo", Path: "pages"})
			if err != nil || result == nil || result.Category != WouldCopy {
				t.Fatalf("result = %+v, error = %v", result, err)
			}
			got, err := os.ReadFile(target)
			if err != nil || string(got) != "outside\n" {
				t.Fatalf("outside file changed: %q, %v", got, err)
			}
			info, err := os.Lstat(file)
			if err != nil {
				t.Fatal(err)
			}
			if (info.Mode()&os.ModeSymlink != 0) != (mode == DryRun) {
				t.Fatalf("unexpected destination mode: %v", info.Mode())
			}
			if mode.ShouldWrite() {
				got, err = os.ReadFile(file)
				if err != nil || string(got) != "/pages/public/\n" {
					t.Fatalf("destination = %q, error = %v", got, err)
				}
			}
		})
	}
	for _, mode := range []ApplyMode{DryRun, Apply, Recut} {
		dir := t.TempDir()
		if err := os.Mkdir(filepath.Join(dir, ".gitignore"), 0o755); err != nil {
			t.Fatal(err)
		}
		if _, err := processPagesIgnore(&config.Config{}, dir, mode, &pagesPreparation{Generator: "hugo", Path: "pages"}); err == nil {
			t.Fatal("expected directory destination error")
		}
	}
}
