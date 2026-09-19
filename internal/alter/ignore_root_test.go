package alter

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/wimpysworld/tailor/internal/config"
	"github.com/wimpysworld/tailor/internal/swatch"
	"github.com/wimpysworld/tailor/internal/testutil"
)

func newConfig(entries ...config.SwatchEntry) *config.Config {
	return &config.Config{Swatches: entries}
}

func ignoreEntry(mode swatch.AlterationMode) config.SwatchEntry {
	return config.SwatchEntry{Path: ignoreRootPath, Alteration: mode}
}

func writeOnDisk(t *testing.T, dir, rel string, data []byte) {
	t.Helper()
	path := filepath.Join(dir, rel)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
}

func symlinkOrSkip(t *testing.T, oldname, newname string) {
	t.Helper()
	if err := os.Symlink(oldname, newname); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
}

func mkfifoOrSkip(t *testing.T, path string) {
	t.Helper()
	if err := exec.CommandContext(t.Context(), "mkfifo", path).Run(); err != nil {
		t.Skipf("mkfifo unavailable: %v", err)
	}
}

func TestIgnoreRootPreservesExistingBytes(t *testing.T) {
	contents := []struct {
		name string
		data []byte
	}{
		{name: "LF", data: []byte("vendor/\nlocal.env\n")},
		{name: "CRLF", data: []byte("vendor/\r\nlocal.env\r\n")},
		{name: "unterminated", data: []byte("vendor/\nlocal.env")},
	}
	alterations := []swatch.AlterationMode{swatch.FirstFit, swatch.Always}
	modes := []ApplyMode{DryRun, Apply, Recut}

	for _, content := range contents {
		for _, alteration := range alterations {
			for _, mode := range modes {
				name := content.name + "/" + string(alteration) + "/" + applyModeName(mode)
				t.Run(name, func(t *testing.T) {
					dir := t.TempDir()
					writeOnDisk(t, dir, ".gitignore", content.data)
					cfg := newConfig(ignoreEntry(alteration))

					for range 2 {
						results, err := ProcessOrdinarySwatchesForTest(cfg, dir, mode, &TokenContext{})
						if err != nil {
							t.Fatal(err)
						}
						if len(results) != 1 || results[0].Category != NoChange || results[0].Reason != SkipManagedRootExists {
							t.Fatalf("result = %+v, want preserved ignore root", results)
						}
						got, err := os.ReadFile(filepath.Join(dir, ".gitignore"))
						if err != nil {
							t.Fatal(err)
						}
						if !bytes.Equal(got, content.data) {
							t.Fatalf("ignore bytes = %q, want %q", got, content.data)
						}
					}
				})
			}
		}
	}
}

func TestIgnoreRootMissingUsesStarterOrNeverVeto(t *testing.T) {
	const starter = ".direnv/\nresult/\n"
	alterations := []swatch.AlterationMode{swatch.FirstFit, swatch.Always, swatch.Never}
	modes := []ApplyMode{DryRun, Apply, Recut}

	for _, alteration := range alterations {
		for _, mode := range modes {
			t.Run(string(alteration)+"/"+applyModeName(mode), func(t *testing.T) {
				dir := t.TempDir()
				cfg := newConfig(ignoreEntry(alteration))
				results, err := ProcessOrdinarySwatchesForTest(cfg, dir, mode, &TokenContext{})
				if err != nil {
					t.Fatal(err)
				}
				path := filepath.Join(dir, ".gitignore")
				if alteration == swatch.Never {
					if results[0].Category != Skipped || results[0].Reason != SkipModeNever {
						t.Fatalf("result = %+v, want never veto", results[0])
					}
					if _, err := os.Lstat(path); !os.IsNotExist(err) {
						t.Fatalf("never destination error = %v, want missing", err)
					}
					return
				}
				if results[0].Category != WouldCopy {
					t.Fatalf("category = %q, want %q", results[0].Category, WouldCopy)
				}
				if mode == DryRun {
					if _, err := os.Lstat(path); !os.IsNotExist(err) {
						t.Fatalf("preview destination error = %v, want missing", err)
					}
					return
				}
				got, err := os.ReadFile(path)
				if err != nil {
					t.Fatal(err)
				}
				if string(got) != starter {
					t.Fatalf("starter = %q, want %q", got, starter)
				}
			})
		}
	}
}

func TestIgnoreRootPreservesFinalSymlinks(t *testing.T) {
	for _, kind := range []string{"internal", "external", "dangling"} {
		for _, mode := range []ApplyMode{DryRun, Apply, Recut} {
			t.Run(kind+"/"+applyModeName(mode), func(t *testing.T) {
				dir := t.TempDir()
				var target string
				switch kind {
				case "internal":
					target = "user.ignore"
					writeOnDisk(t, dir, target, []byte("inside\n"))
				case "external":
					target = filepath.Join(t.TempDir(), "outside.ignore")
					if err := os.WriteFile(target, []byte("outside\n"), 0o644); err != nil {
						t.Fatal(err)
					}
				case "dangling":
					target = "missing.ignore"
				}
				symlinkOrSkip(t, target, filepath.Join(dir, ".gitignore"))
				var targetBefore []byte
				if kind != "dangling" {
					var err error
					targetBefore, err = os.ReadFile(filepath.Join(dir, ".gitignore"))
					if err != nil {
						t.Fatal(err)
					}
				}

				cfg := newConfig(ignoreEntry(swatch.Always))
				results, err := ProcessOrdinarySwatchesForTest(cfg, dir, mode, &TokenContext{})
				if err != nil {
					t.Fatal(err)
				}
				if results[0].Category != NoChange || results[0].Reason != SkipManagedRootExists {
					t.Fatalf("result = %+v, want preserved symlink", results[0])
				}
				gotTarget, err := os.Readlink(filepath.Join(dir, ".gitignore"))
				if err != nil || gotTarget != target {
					t.Fatalf("symlink target = %q, error = %v, want %q", gotTarget, err, target)
				}
				if kind != "dangling" {
					targetAfter, err := os.ReadFile(filepath.Join(dir, ".gitignore"))
					if err != nil || !bytes.Equal(targetAfter, targetBefore) {
						t.Fatalf("target bytes = %q, error = %v, want %q", targetAfter, err, targetBefore)
					}
				}
			})
		}
	}
}

func TestIgnoreRootPublicationRacePreservesOrRejectsDestination(t *testing.T) {
	const userContent = "user content\n"
	for _, kind := range []string{"regular file", "final symlink", "directory", "fifo"} {
		t.Run(kind, func(t *testing.T) {
			dir := t.TempDir()
			destination := filepath.Join(dir, ignoreRootPath)
			outside := filepath.Join(t.TempDir(), "outside.ignore")
			if kind == "final symlink" {
				if err := os.WriteFile(outside, []byte(userContent), 0o600); err != nil {
					t.Fatal(err)
				}
			}
			root, err := os.OpenRoot(dir)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() {
				if err := root.Close(); err != nil {
					t.Error(err)
				}
			})

			result, err := processSwatch(root, ignoreEntry(swatch.Always), []byte("generated\n"), Apply, managedApplyHooks{
				beforeRename: func(path string) error {
					if path != ignoreRootPath {
						t.Fatalf("hook path = %q, want %q", path, ignoreRootPath)
					}
					switch kind {
					case "regular file":
						return os.WriteFile(destination, []byte(userContent), 0o640)
					case "final symlink":
						symlinkOrSkip(t, outside, destination)
					case "directory":
						return os.Mkdir(destination, 0o750)
					case "fifo":
						mkfifoOrSkip(t, destination)
					}
					return nil
				},
			})

			if kind == "regular file" || kind == "final symlink" {
				if err != nil {
					t.Fatal(err)
				}
				if result.Category != NoChange || result.Reason != SkipManagedRootExists {
					t.Fatalf("result = %+v, want preserved ignore root", result)
				}
				if kind == "regular file" {
					data, readErr := os.ReadFile(destination)
					if readErr != nil || string(data) != userContent {
						t.Fatalf("destination = %q, error = %v", data, readErr)
					}
				} else {
					target, readErr := os.Readlink(destination)
					if readErr != nil || target != outside {
						t.Fatalf("symlink target = %q, error = %v, want %q", target, readErr, outside)
					}
					data, readErr := os.ReadFile(outside)
					if readErr != nil || string(data) != userContent {
						t.Fatalf("outside target = %q, error = %v", data, readErr)
					}
				}
			} else {
				if err == nil {
					t.Fatalf("%s race accepted, result = %+v", kind, result)
				}
				want := "is a directory"
				if kind == "fifo" {
					want = "is not a regular file or symlink"
				}
				if !strings.Contains(err.Error(), want) {
					t.Fatalf("error = %q, want %q", err, want)
				}
				info, statErr := os.Lstat(destination)
				if statErr != nil {
					t.Fatal(statErr)
				}
				if kind == "directory" && !info.IsDir() {
					t.Fatalf("destination mode = %v, want directory", info.Mode())
				}
				if kind == "fifo" && info.Mode()&os.ModeNamedPipe == 0 {
					t.Fatalf("destination mode = %v, want named pipe", info.Mode())
				}
			}

			matches, globErr := filepath.Glob(destination + ".tmp-*")
			if globErr != nil || len(matches) != 0 {
				t.Fatalf("temporary files = %v, error = %v", matches, globErr)
			}
		})
	}
}

func TestIgnoreRootRejectsUnsafeTypesUnlessNever(t *testing.T) {
	for _, kind := range []string{"directory", "fifo"} {
		t.Run(kind, func(t *testing.T) {
			dir := t.TempDir()
			path := filepath.Join(dir, ".gitignore")
			if kind == "directory" {
				if err := os.Mkdir(path, 0o755); err != nil {
					t.Fatal(err)
				}
			} else {
				mkfifoOrSkip(t, path)
			}

			cfg := newConfig(ignoreEntry(swatch.Always))
			if _, err := ProcessOrdinarySwatchesForTest(cfg, dir, Apply, &TokenContext{}); err == nil {
				t.Fatal("unsafe ignore root accepted")
			}
			cfg = newConfig(ignoreEntry(swatch.Never))
			results, err := ProcessOrdinarySwatchesForTest(cfg, dir, Recut, &TokenContext{})
			if err != nil {
				t.Fatal(err)
			}
			if results[0].Category != Skipped || results[0].Reason != SkipModeNever {
				t.Fatalf("never result = %+v", results[0])
			}
		})
	}
}

func TestInvalidIgnoreRootBlocksExecutionBeforeSideEffects(t *testing.T) {
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		requests.Add(1)
	}))
	t.Cleanup(server.Close)
	client := testutil.NewTestClient(t, server)

	for _, kind := range []string{"directory", "fifo"} {
		t.Run(kind, func(t *testing.T) {
			dir := t.TempDir()
			configBytes := []byte("license: none\n")
			retiredBytes := []byte("retired\n")
			writeOnDisk(t, dir, ".tailor.yml", configBytes)
			writeOnDisk(t, dir, ".github/workflows/tailor.yml", retiredBytes)
			path := filepath.Join(dir, ".gitignore")
			if kind == "directory" {
				if err := os.Mkdir(path, 0o755); err != nil {
					t.Fatal(err)
				}
			} else {
				mkfifoOrSkip(t, path)
			}
			cfg := &config.Config{License: "none", Swatches: []config.SwatchEntry{
				{Path: ".github/workflows/tailor.yml", Alteration: swatch.Always},
				{Path: ".gitignore", Alteration: swatch.Always},
			}}

			_, err := Execute(cfg, dir, Apply, client, io.Discard, Options{})
			if err == nil || !strings.Contains(err.Error(), "checking protected ignore root") {
				t.Fatalf("Execute() error = %v, want protected ignore error", err)
			}
			if requests.Load() != 0 {
				t.Fatalf("HTTP requests = %d, want 0", requests.Load())
			}
			gotConfig, err := os.ReadFile(filepath.Join(dir, ".tailor.yml"))
			if err != nil || !bytes.Equal(gotConfig, configBytes) {
				t.Fatalf("config bytes = %q, error = %v", gotConfig, err)
			}
			gotRetired, err := os.ReadFile(filepath.Join(dir, ".github/workflows/tailor.yml"))
			if err != nil || !bytes.Equal(gotRetired, retiredBytes) {
				t.Fatalf("retired bytes = %q, error = %v", gotRetired, err)
			}
		})
	}
}

func applyModeName(mode ApplyMode) string {
	switch mode {
	case DryRun:
		return "preview"
	case Apply:
		return "apply"
	case Recut:
		return "recut"
	default:
		return "unknown"
	}
}
