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
	testEbitengineMobileRunner(t, "bash")
}

func TestEbitengineMobileRunnerWithBashThree(t *testing.T) {
	for _, candidate := range []string{"bash3", "bash32", "bash-3.2", "/bin/bash", "bash"} {
		path, err := exec.LookPath(candidate)
		if err != nil {
			continue
		}
		output, err := exec.CommandContext(t.Context(), path, "-c", `printf '%s' "${BASH_VERSINFO[0]}"`).Output()
		if err == nil && string(output) == "3" {
			testEbitengineMobileRunner(t, path)
			return
		}
	}
	t.Skip("Bash 3 executable is not available")
}

func testEbitengineMobileRunner(t *testing.T, bash string) {
	t.Helper()
	nix, err := exec.LookPath("nix")
	if err != nil {
		t.Skip("nix required")
	}
	root := testTempDir(t)
	files := renderSelectedManagedFiles(t, ebitengineConfig())
	writeManagedRenderFixture(t, root, "nix/go-ebitengine.nix", files["nix/go-ebitengine.nix"])
	expr := `let pkgs = rec { bash="bash"; coreutils="coreutils"; findutils="findutils"; gnused="sed"; gnugrep="grep"; gawk="awk"; git="git"; curl="curl"; gzip="gzip"; zip="zip"; unzip="unzip"; brotli="brotli"; miniserve="miniserve"; go="selected-go"; writeShellApplication = x: x.text; lib.optionals = _: _: []; }; in builtins.elemAt (import ` + strconv.Quote(filepath.Join(root, "nix/go-ebitengine.nix")) + ` {inherit pkgs;}) 11`
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
 env) case "$2" in GOHOSTOS) echo "${HOST_OS:-linux}";; GOHOSTARCH) echo amd64;; esac ;;
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
case "$TARGET_FORM" in
 equals) test "$1" = -target=; shift ;;
 separate) test "$1" = -target; test "$2" = ''; shift 2 ;;
 android) test "$1" = -target=android/arm64; shift ;;
esac
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
	guard, _, ok := strings.Cut(runner, "module_dir=$(go list")
	if !ok {
		t.Fatal("missing module resolution boundary")
	}
	writeManagedRenderFixture(t, root, "bin/xcrun", []byte("#!/usr/bin/env bash\nexit \"${XCRUN_STATUS:-0}\"\n"))
	if err := os.Chmod(filepath.Join(bin, "xcrun"), 0o700); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name, target, host, xcrun, want string
		missingSDK                      bool
	}{
		{name: "empty target", target: ""},
		{name: "android", target: "android"},
		{name: "android architectures", target: "android/arm64,android/amd64"},
		{name: "ios", target: "ios", want: "iOS binding requires"},
		{name: "ios architectures", target: "ios/arm64,ios/amd64", want: "iOS binding requires"},
		{name: "android first", target: "android,ios", want: "iOS binding requires"},
		{name: "ios first", target: "ios,android", want: "iOS binding requires"},
		{name: "mixed architectures", target: "android/arm64,ios/arm64", want: "iOS binding requires"},
		{name: "simulator", target: "iossimulator/arm64", want: "iOS binding requires"},
		{name: "darwin", target: "ios/arm64", host: "darwin"},
		{name: "missing clang", target: "ios/arm64", host: "darwin", xcrun: "1", want: "iOS binding requires"},
		{name: "missing SDK", target: "android/arm64,android/amd64", missingSDK: true, want: "Android binding requires"},
	} {
		for _, separate := range []bool{false, true} {
			t.Run(tc.name+"/separate="+strconv.FormatBool(separate), func(t *testing.T) {
				args := []string{"-euc", guard + "echo guard-passed", "runner", "bind"}
				if separate {
					args = append(args, "-target", tc.target)
				} else {
					args = append(args, "-target="+tc.target)
				}
				cmd := exec.CommandContext(t.Context(), bash, args...)
				sdk := root
				if tc.missingSDK {
					sdk = filepath.Join(root, "missing-sdk")
				}
				cmd.Env = append(os.Environ(), "PATH="+bin+":"+os.Getenv("PATH"), "HOST_OS="+tc.host, "XCRUN_STATUS="+tc.xcrun, "ANDROID_HOME="+sdk, "ANDROID_NDK_HOME="+sdk)
				out, err := cmd.CombinedOutput()
				if tc.want == "" {
					if err != nil || !strings.Contains(string(out), "guard-passed") {
						t.Fatalf("guard: %v %s", err, out)
					}
				} else if err == nil || !strings.Contains(string(out), tc.want) || strings.Contains(string(out), "guard-passed") {
					t.Fatalf("want rejection %q: %v %s", tc.want, err, out)
				}
			})
		}
	}
	tempRoot := t.TempDir()
	for _, targetForm := range []string{"none", "equals", "separate", "android"} {
		for _, result := range []string{"0", "7", "23", "143"} {
			t.Run(targetForm+"/result="+result, func(t *testing.T) {
				args := []string{"-euc", runner, "runner"}
				switch targetForm {
				case "equals":
					args = append(args, "-target=")
				case "separate":
					args = append(args, "-target", "")
				case "android":
					args = append(args, "-target=android/arm64")
				}
				args = append(args, "bind", "-o=output with spaces", "./mobile")
				cmd := exec.CommandContext(t.Context(), bash, args...)
				cmd.Dir = root
				toolPath := filepath.Join(root, "tool-path")
				cmd.Env = append(os.Environ(), "PATH="+bin+":"+os.Getenv("PATH"), "MODULE="+module, "CALLER="+root, "ANDROID_HOME="+root, "ANDROID_NDK_HOME="+root, "TOOL_PATH="+toolPath, "CGO_ENABLED=0", "GOOS=js", "GOARCH=wasm", "GOFLAGS=-tags=project", "GOWORK=workspace", "GOARM=7", "GOAMD64=v3", "GOWASM=satconv", "GOEXPERIMENT=project", "RESULT="+result, "TMPDIR="+tempRoot, "TARGET_FORM="+targetForm)
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
			})
		}
	}
}
