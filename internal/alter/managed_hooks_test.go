package alter

import (
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/wimpysworld/tailor/internal/config"
)

func evaluateManagedHooks(t *testing.T, cfg *config.Config, linux bool) string {
	t.Helper()
	return evaluateManagedPlatformHooks(t, cfg, linux, true)
}

func evaluateManagedPlatformHooks(t *testing.T, cfg *config.Config, linux, x86 bool) string {
	t.Helper()
	nix, err := exec.LookPath("nix")
	if err != nil {
		t.Skip("nix is required")
	}
	root := t.TempDir()
	for name, content := range renderSelectedManagedFiles(t, cfg) {
		if strings.HasPrefix(name, "nix/") {
			writeManagedRenderFixture(t, root, name, content)
		}
	}
	expression := `let linux = ` + strconv.FormatBool(linux) + `; x86 = ` + strconv.FormatBool(x86) + `; pkgs = { stdenv.isLinux = linux; stdenv.hostPlatform.isx86_64 = x86; lib.optionalString = b: s: if b then s else ""; vulkan-loader = if linux then "/vulkan" else throw "Vulkan on Darwin"; intel-media-driver = if linux && x86 then "/intel" else throw "Intel on unsupported host"; vpl-gpu-rt = if linux && x86 then "/vpl" else throw "VPL on unsupported host"; }; in import ` + strconv.Quote(filepath.Join(root, "nix/hooks.nix")) + ` { inherit pkgs; }`
	cmd := exec.CommandContext(t.Context(), nix, "eval", "--impure", "--raw", "--expr", expression)
	var stderr strings.Builder
	cmd.Stderr = &stderr
	output, err := cmd.Output()
	if err != nil {
		t.Fatalf("hook evaluation: %v\n%s%s", err, output, stderr.String())
	}
	return string(output)
}

func TestManagedHookEvaluationExcludesStderr(t *testing.T) {
	root := t.TempDir()
	writeManagedRenderFixture(t, root, "nix", []byte("#!/bin/sh\nprintf 'export CGO_ENABLED=1\\n'\nprintf 'exit 37\\n' >&2\n"))
	if err := os.Chmod(filepath.Join(root, "nix"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", root+string(os.PathListSeparator)+os.Getenv("PATH"))
	if hook := evaluateManagedHooks(t, ffmpegConfig(), true); hook != "export CGO_ENABLED=1\n" {
		t.Fatalf("hook source includes unexpected output: %q", hook)
	}
}

func TestManagedHookRuntime(t *testing.T) {
	for _, x86 := range []bool{true, false} {
		t.Run("x86="+strconv.FormatBool(x86), func(t *testing.T) {
			testManagedHookRuntime(t, x86)
		})
	}
}

func testManagedHookRuntime(t *testing.T, x86 bool) {
	t.Helper()
	hook := evaluateManagedPlatformHooks(t, ffmpegConfig(), true, x86)
	for _, tc := range []struct {
		name, id, initial, check string
		drivers, dri             bool
	}{
		{"defaults", "ID=\"nixos\"", "", `test "$LD_LIBRARY_PATH" = "$host/lib:/vpl/lib:/intel/lib:/vulkan/lib"; test "$ONEVPL_SEARCH_PATH" = /vpl/lib; test "$LIBVA_DRIVERS_PATH" = "$host/lib/dri"; test "$VK_DRIVER_FILES" = "$host/share/vulkan/icd.d/a.json:$host/share/vulkan/icd.d/b [driver].json:$host/share/vulkan/icd.d/c linked.json:$host/share/vulkan/icd.d/z.json"; test -s "$find_log"`, true, true},
		{"preserve", "ID=nixos", `export LD_LIBRARY_PATH=:/existing:/vpl/lib:/existing: ONEVPL_SEARCH_PATH=chosen LIBVA_DRIVERS_PATH=chosen VK_DRIVER_FILES=chosen`, `test "$LD_LIBRARY_PATH" = "$host/lib:/vpl/lib:/intel/lib:/vulkan/lib:/existing"; test "$ONEVPL_SEARCH_PATH:$LIBVA_DRIVERS_PATH:$VK_DRIVER_FILES" = chosen:chosen:chosen`, true, true},
		{"empty", "ID=nixos", `export ONEVPL_SEARCH_PATH= LIBVA_DRIVERS_PATH= VK_DRIVER_FILES=`, `test "$ONEVPL_SEARCH_PATH:$LIBVA_DRIVERS_PATH:$VK_DRIVER_FILES" = ::`, true, true},
		{"fallback", "ID='nixos'", "", `test "$LD_LIBRARY_PATH" = /vpl/lib:/intel/lib:/vulkan/lib; test "$LIBVA_DRIVERS_PATH" = /intel/lib/dri`, false, false},
		{"empty DRI", "ID=nixos", "", `test "$LIBVA_DRIVERS_PATH" = /intel/lib/dri`, true, true},
		{"other driver", "ID=nixos", "", `test "$LIBVA_DRIVERS_PATH" = /intel/lib/dri`, true, true},
		{"unreadable driver", "ID=nixos", "", `test "$LIBVA_DRIVERS_PATH" = /intel/lib/dri`, true, true},
		{"directory driver", "ID=nixos", "", `test "$LIBVA_DRIVERS_PATH" = /intel/lib/dri`, true, true},
		{"dangling driver", "ID=nixos", "", `test "$LIBVA_DRIVERS_PATH" = /intel/lib/dri`, true, true},
		{"optout", "ID=nixos", `export TAILOR_NIXOS_DRIVERS=0`, `test "${LD_LIBRARY_PATH+x}${ONEVPL_SEARCH_PATH+x}${LIBVA_DRIVERS_PATH+x}${VK_DRIVER_FILES+x}" = ""`, true, true},
		{"other linux", "ID=ubuntu", "", `test "${LD_LIBRARY_PATH+x}${ONEVPL_SEARCH_PATH+x}${LIBVA_DRIVERS_PATH+x}${VK_DRIVER_FILES+x}" = ""`, true, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if !x86 {
				tc.check = strings.ReplaceAll(tc.check, "/vpl/lib:/intel/lib:", "")
				tc.check = strings.ReplaceAll(tc.check, `test "$ONEVPL_SEARCH_PATH" = /vpl/lib`, `test "${ONEVPL_SEARCH_PATH+x}" = ""`)
				tc.check = strings.ReplaceAll(tc.check, `test "$LIBVA_DRIVERS_PATH" = /intel/lib/dri`, `test "${LIBVA_DRIVERS_PATH+x}" = ""`)
				if tc.name == "other driver" {
					tc.check = `test "$LIBVA_DRIVERS_PATH" = "$host/lib/dri"`
				}
				if tc.name == "preserve" {
					tc.check = strings.ReplaceAll(tc.check, "/vulkan/lib:/existing", "/vulkan/lib:/existing:/vpl/lib")
				}
			}
			root := t.TempDir()
			host := filepath.Join(root, "driver")
			release := filepath.Join(root, "os-release")
			writeManagedRenderFixture(t, root, "os-release", []byte(tc.id+"\n"))
			if tc.drivers {
				if err := os.MkdirAll(filepath.Join(host, "lib"), 0o755); err != nil {
					t.Fatal(err)
				}
			}
			if tc.dri {
				if err := os.MkdirAll(filepath.Join(host, "lib/dri"), 0o755); err != nil {
					t.Fatal(err)
				}
				driver := filepath.Join(host, "lib/dri/iHD_drv_video.so")
				switch tc.name {
				case "empty DRI":
				case "other driver":
					writeManagedRenderFixture(t, host, "lib/dri/other_drv_video.so", []byte("driver"))
				case "unreadable driver":
					if os.Geteuid() == 0 {
						t.Skip("root can read files without read permission")
					}
					writeManagedRenderFixture(t, host, "lib/dri/iHD_drv_video.so", []byte("driver"))
					if err := os.Chmod(driver, 0); err != nil {
						t.Fatal(err)
					}
				case "directory driver":
					if err := os.Mkdir(driver, 0o755); err != nil {
						t.Fatal(err)
					}
				case "dangling driver":
					if err := os.Symlink("missing.so", driver); err != nil {
						t.Fatal(err)
					}
				default:
					writeManagedRenderFixture(t, host, "lib/driver.so", []byte("driver"))
					if err := os.Symlink("../driver.so", driver); err != nil {
						t.Fatal(err)
					}
				}
			}
			icds := filepath.Join(root, "icds")
			writeManagedRenderFixture(t, icds, "z.json", []byte("{}"))
			writeManagedRenderFixture(t, icds, "a.json", []byte("{}"))
			writeManagedRenderFixture(t, icds, "b [driver].json", []byte("{}"))
			if err := os.Symlink("a.json", filepath.Join(icds, "c linked.json")); err != nil {
				t.Fatal(err)
			}
			if err := os.Symlink("missing.json", filepath.Join(icds, "dangling.json")); err != nil {
				t.Fatal(err)
			}
			if err := os.Mkdir(filepath.Join(icds, "directory.json"), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.MkdirAll(filepath.Join(host, "share/vulkan"), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.Symlink(icds, filepath.Join(host, "share/vulkan/icd.d")); err != nil {
				t.Fatal(err)
			}
			script := strings.NewReplacer("/etc/os-release", release, "/run/opengl-driver", host).Replace(hook)
			portableFind := `
find() {
  printf 'invoked\n' >> "$find_log"
  for arg in "$@"; do
    if [ "$arg" = -readable ]; then
      printf 'find: -readable is not portable\n' >&2
      return 1
    fi
  done
  command find "$@"
}
`
			script = "set -eu\nunset LD_LIBRARY_PATH ONEVPL_SEARCH_PATH LIBVA_DRIVERS_PATH VK_DRIVER_FILES TAILOR_NIXOS_DRIVERS\nhost=" + strconv.Quote(host) + "\nfind_log=" + strconv.Quote(filepath.Join(root, "find.log")) + "\n" + portableFind + tc.initial + "\n" + script + "\n" + script + "\n" + tc.check + "\ntest \"$CGO_ENABLED\" = 1\n"
			cmd := exec.CommandContext(t.Context(), "bash")
			cmd.Stdin = strings.NewReader(script)
			output, err := cmd.CombinedOutput()
			if err != nil {
				t.Fatalf("runtime: %v\n%s", err, output)
			}
		})
	}
}

func TestManagedFFmpegPlatforms(t *testing.T) {
	nix, err := exec.LookPath("nix")
	if err != nil {
		t.Skip("nix is required")
	}
	root := t.TempDir()
	files := renderSelectedManagedFiles(t, ffmpegConfig())
	writeManagedRenderFixture(t, root, "ffmpeg.nix", files["nix/go-ffmpeg-statigo.nix"])
	for _, tc := range []struct {
		name       string
		linux, x86 bool
		want       string
	}{
		{"x86 Linux", true, true, `["pkg-config","git","curl","jq","vulkan","intel","vpl"]`},
		{"ARM Linux", true, false, `["pkg-config","git","curl","jq","vulkan"]`},
		{"Darwin", false, true, `["pkg-config","git","curl","jq"]`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			expr := `let linux = ` + strconv.FormatBool(tc.linux) + `; x86 = ` + strconv.FormatBool(tc.x86) + `; pkgs = { stdenv.isLinux = linux; stdenv.hostPlatform.isx86_64 = x86; lib.optionals = b: xs: if b then xs else []; pkg-config = "pkg-config"; git = "git"; curl = "curl"; jq = "jq"; vulkan-loader = if linux then "vulkan" else throw "Vulkan on Darwin"; intel-media-driver = if linux && x86 then "intel" else throw "Intel on unsupported host"; vpl-gpu-rt = if linux && x86 then "vpl" else throw "VPL on unsupported host"; }; in import ` + strconv.Quote(filepath.Join(root, "ffmpeg.nix")) + ` { inherit pkgs; }`
			cmd := exec.CommandContext(t.Context(), nix, "eval", "--offline", "--impure", "--json", "--expr", expr)
			var stderr strings.Builder
			cmd.Stderr = &stderr
			output, err := cmd.Output()
			if err != nil || strings.TrimSpace(string(output)) != tc.want {
				t.Fatalf("packages: %v\n%s%s", err, output, stderr.String())
			}
			hook := evaluateManagedPlatformHooks(t, ffmpegConfig(), tc.linux, tc.x86)
			if !tc.linux && strings.Contains(hook, "tailor_prepend_path LD_LIBRARY_PATH") {
				t.Fatal("Darwin graphics hook is not empty")
			}
		})
	}
}

func TestManagedHookCGOAndPlatform(t *testing.T) {
	for _, policy := range []*bool{nil, new(false), new(true)} {
		cfg := &config.Config{Languages: &config.LanguageSettings{Go: new(true), GoOptions: &config.GoOptions{CGO: policy}}}
		hook := evaluateManagedHooks(t, cfg, false)
		if strings.Contains(hook, "tailor_prepend_path LD_LIBRARY_PATH") {
			t.Fatal("Darwin graphics hook is not empty")
		}
		want := "chosen"
		if policy != nil {
			want = "0"
			if *policy {
				want = "1"
			}
		}
		cmd := exec.CommandContext(t.Context(), "bash")
		cmd.Stdin = strings.NewReader("set -eu\nexport CGO_ENABLED=chosen\n" + hook + "\ntest \"$CGO_ENABLED\" = " + want)
		output, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("CGO: %v\n%s", err, output)
		}
	}
}

func TestManagedGoHookHasNoUnusedBindings(t *testing.T) {
	deadnix, err := exec.LookPath("deadnix")
	if err != nil {
		t.Skip("deadnix is required")
	}
	for _, policy := range []*bool{nil, new(false), new(true)} {
		cfg := &config.Config{Languages: &config.LanguageSettings{Go: new(true), GoOptions: &config.GoOptions{CGO: policy}}}
		content := renderSelectedManagedFiles(t, cfg)["nix/hooks/go.nix"]
		cmd := exec.CommandContext(t.Context(), deadnix, "--fail")
		cmd.Stdin = strings.NewReader(string(content))
		if output, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("Go hook unused bindings: %v\n%s", err, output)
		}
	}
}

func TestManagedHooksPartialUpgrade(t *testing.T) {
	nix, err := exec.LookPath("nix")
	if err != nil {
		t.Skip("nix is required")
	}
	root := t.TempDir()
	files := renderSelectedManagedFiles(t, ffmpegConfig())
	writeManagedRenderFixture(t, root, "nix/hooks.nix", files["nix/hooks.nix"])
	writeManagedRenderFixture(t, root, "nix/loader.nix", files["nix/loader.nix"])
	writeManagedRenderFixture(t, root, "nix/go.nix", []byte(`{pkgs}: [ "legacy" ]`))
	expr := `let pkgs = {}; in { packages = import ` + strconv.Quote(filepath.Join(root, "nix/loader.nix")) + ` {inherit pkgs;}; hook = builtins.typeOf (import ` + strconv.Quote(filepath.Join(root, "nix/hooks.nix")) + ` {inherit pkgs;}); }`
	output, err := exec.CommandContext(t.Context(), nix, "eval", "--impure", "--json", "--expr", expr).CombinedOutput()
	if err != nil || strings.TrimSpace(string(output)) != `{"hook":"string","packages":["legacy"]}` {
		t.Fatalf("partial upgrade: %v %s", err, output)
	}
}
