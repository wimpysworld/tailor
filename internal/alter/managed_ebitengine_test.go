package alter

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/wimpysworld/tailor/internal/config"
)

func ebitengineConfig() *config.Config {
	return &config.Config{Languages: &config.LanguageSettings{Go: new(true), GoOptions: &config.GoOptions{CGO: new(false), Ebitengine: &config.EbitengineOptions{Enabled: true, Mapping: true, Android: &config.EbitengineAndroidOptions{Enabled: true}}}}}
}

func TestEbitengineLifecycleAndLoaders(t *testing.T) {
	cfg := ebitengineConfig()
	files := renderSelectedManagedFiles(t, cfg)
	paths := []string{"nix/go-ebitengine.nix", "nix/lib/go-ebitengine-runtime.nix", "nix/hooks/go-ebitengine.nix", "nix/shells/go-ebitengine-android.nix"}
	for _, path := range paths {
		if len(files[path]) == 0 {
			t.Fatalf("missing %s", path)
		}
	}
	for _, loader := range []string{"nix/loader.nix", "nix/hooks.nix"} {
		if strings.Contains(string(files[loader]), "./shells/") || strings.Contains(string(files[loader]), "./lib/") {
			t.Fatalf("typed loader imports support or shell: %s", loader)
		}
	}
	for _, tc := range []struct {
		name           string
		cfg            *config.Config
		count, enabled int
	}{
		{"absent", &config.Config{}, 0, 0},
		{"Go false", &config.Config{Languages: &config.LanguageSettings{Go: new(false)}}, 4, 0},
		{"all", cfg, 4, 4},
		{"desktop", &config.Config{Languages: &config.LanguageSettings{Go: new(true), GoOptions: &config.GoOptions{Ebitengine: &config.EbitengineOptions{Enabled: true}}}}, 4, 3},
	} {
		t.Run(tc.name, func(t *testing.T) {
			selections, err := selectManagedFiles(tc.cfg)
			if err != nil {
				t.Fatal(err)
			}
			count, enabled := 0, 0
			for _, s := range selections {
				if strings.Contains(s.Entry.Path, "ebitengine") {
					count++
					if s.Enabled {
						enabled++
					}
				}
			}
			if count != tc.count || enabled != tc.enabled {
				t.Fatalf("selection %d/%d", count, enabled)
			}
		})
	}
}

func TestEbitengineMobileRunner(t *testing.T) {
	nix, err := exec.LookPath("nix")
	if err != nil {
		t.Skip("nix required")
	}
	root := testTempDir(t)
	files := renderSelectedManagedFiles(t, ebitengineConfig())
	writeManagedRenderFixture(t, root, "nix/go-ebitengine.nix", files["nix/go-ebitengine.nix"])
	expr := `let pkgs = rec { bash="bash"; coreutils="coreutils"; findutils="findutils"; gnused="sed"; gnugrep="grep"; gawk="awk"; git="git"; curl="curl"; gzip="gzip"; zip="zip"; unzip="unzip"; brotli="brotli"; miniserve="miniserve"; go="selected-go"; go-tools.override = _: "staticcheck"; writeShellApplication = x: x.text; lib.optionals = _: _: []; }; in builtins.elemAt (import ` + strconv.Quote(filepath.Join(root, "nix/go-ebitengine.nix")) + ` {inherit pkgs;}) 12`
	output, err := exec.CommandContext(t.Context(), nix, "eval", "--offline", "--impure", "--raw", "--expr", expr).CombinedOutput()
	if err != nil {
		t.Fatalf("runner eval: %v %s", err, output)
	}
	runner := string(output)
	for _, forbidden := range []string{"go install", "@latest"} {
		if strings.Contains(runner, forbidden) {
			t.Fatal(forbidden)
		}
	}
	module := filepath.Join(root, "replacement with spaces")
	if err := os.MkdirAll(filepath.Join(module, "cmd/ebitenmobile"), 0o700); err != nil {
		t.Fatal(err)
	}
	bin := filepath.Join(root, "bin")
	writeManagedRenderFixture(t, root, "bin/go", []byte(`#!/usr/bin/env bash
set -eu
case "$1" in
 list) test "$2" = -mod=readonly; printf '%s\n' "$MODULE" ;;
 env) case "$2" in GOHOSTOS) echo linux;; GOHOSTARCH) echo amd64;; esac ;;
 -C)
 test "$2" = "$MODULE"
 test "$GOOS:$GOARCH:$GOWORK:$CGO_ENABLED:$GOFLAGS" = linux:amd64:off:0:
 test "$4" = -mod=readonly
 test "$7" = ./cmd/ebitenmobile
 test -z "${GOARM+x}${GOAMD64+x}${GOWASM+x}${GOEXPERIMENT+x}"
 if [ "$RESULT" = 23 ]; then exit 23; fi
 printf '%s' "$6" > "$TOOL_PATH"
 cat > "$6" <<'TOOL'
#!/usr/bin/env bash
set -eu
test "$CGO_ENABLED:$GOOS:$GOARCH:$GOFLAGS:$GOWORK" = '1:js:wasm:-tags=project:workspace'
test "$PWD" = "$CALLER"
test "$#" = 3
test "$1" = bind
test "$2" = '-o=output with spaces'
test "$3" = ./mobile
if [ "$RESULT" = 143 ]; then kill -TERM "$PPID"; exit 0; fi
exit "${RESULT:-0}"
TOOL
 chmod +x "$6"
 ;;
 *) exit 91 ;;
esac
`))
	if err := os.Chmod(filepath.Join(bin, "go"), 0o700); err != nil {
		t.Fatal(err)
	}
	tempRoot := t.TempDir()
	for _, result := range []string{"0", "7", "23", "143"} {
		cmd := exec.CommandContext(t.Context(), "bash", "-euc", runner, "runner", "bind", "-o=output with spaces", "./mobile")
		cmd.Dir = root
		toolPath := filepath.Join(root, "tool-path")
		cmd.Env = append(os.Environ(), "PATH="+bin+":"+os.Getenv("PATH"), "MODULE="+module, "CALLER="+root, "TOOL_PATH="+toolPath, "CGO_ENABLED=0", "GOOS=js", "GOARCH=wasm", "GOFLAGS=-tags=project", "GOWORK=workspace", "GOARM=7", "GOAMD64=v3", "GOWASM=satconv", "GOEXPERIMENT=project", "RESULT="+result, "TMPDIR="+tempRoot)
		out, err := cmd.CombinedOutput()
		if result == "0" && err != nil {
			t.Fatalf("runner: %v %s", err, out)
		}
		if result != "0" {
			var exit *exec.ExitError
			if !errors.As(err, &exit) || strconv.Itoa(exit.ExitCode()) != result {
				t.Fatalf("exit: %v %s", err, out)
			}
		}
		entries, err := os.ReadDir(tempRoot)
		if err != nil || len(entries) != 0 {
			t.Fatalf("temporary directory remains: %v %v", entries, err)
		}
	}
}
