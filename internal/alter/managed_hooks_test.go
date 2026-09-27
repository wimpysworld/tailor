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
	expression := `let pkgs = { stdenv.isLinux = ` + strconv.FormatBool(linux) + `; lib.optionalString = b: s: if b then s else ""; vulkan-loader = "/vulkan"; intel-media-driver = "/intel"; vpl-gpu-rt = "/vpl"; }; in import ` + strconv.Quote(filepath.Join(root, "nix/hooks.nix")) + ` { inherit pkgs; }`
	output, err := exec.CommandContext(t.Context(), nix, "eval", "--impure", "--raw", "--expr", expression).CombinedOutput()
	if err != nil {
		t.Fatalf("hook evaluation: %v\n%s", err, output)
	}
	return string(output)
}

func TestManagedHookRuntime(t *testing.T) {
	hook := evaluateManagedHooks(t, ffmpegConfig(), true)
	for _, tc := range []struct {
		name, id, initial, check string
		drivers, dri             bool
	}{
		{"defaults", "ID=\"nixos\"", "", `test "$LD_LIBRARY_PATH" = "$host/lib:/vpl/lib:/intel/lib:/vulkan/lib"; test "$ONEVPL_SEARCH_PATH" = /vpl/lib; test "$LIBVA_DRIVERS_PATH" = "$host/lib/dri"; test "$VK_DRIVER_FILES" = "$host/share/vulkan/icd.d/a.json:$host/share/vulkan/icd.d/z.json"`, true, true},
		{"preserve", "ID=nixos", `export LD_LIBRARY_PATH=:/existing:/vpl/lib:/existing: ONEVPL_SEARCH_PATH=chosen LIBVA_DRIVERS_PATH=chosen VK_DRIVER_FILES=chosen`, `test "$LD_LIBRARY_PATH" = "$host/lib:/vpl/lib:/intel/lib:/vulkan/lib:/existing"; test "$ONEVPL_SEARCH_PATH:$LIBVA_DRIVERS_PATH:$VK_DRIVER_FILES" = chosen:chosen:chosen`, true, true},
		{"empty", "ID=nixos", `export ONEVPL_SEARCH_PATH= LIBVA_DRIVERS_PATH= VK_DRIVER_FILES=`, `test "$ONEVPL_SEARCH_PATH:$LIBVA_DRIVERS_PATH:$VK_DRIVER_FILES" = ::`, true, true},
		{"fallback", "ID='nixos'", "", `test "$LD_LIBRARY_PATH" = /vpl/lib:/intel/lib:/vulkan/lib; test "$LIBVA_DRIVERS_PATH" = /intel/lib/dri`, false, false},
		{"optout", "ID=nixos", `export TAILOR_NIXOS_DRIVERS=0`, `test "${LD_LIBRARY_PATH+x}${ONEVPL_SEARCH_PATH+x}${LIBVA_DRIVERS_PATH+x}${VK_DRIVER_FILES+x}" = ""`, true, true},
		{"other linux", "ID=ubuntu", "", `test "${LD_LIBRARY_PATH+x}${ONEVPL_SEARCH_PATH+x}${LIBVA_DRIVERS_PATH+x}${VK_DRIVER_FILES+x}" = ""`, true, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
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
			}
			icds := filepath.Join(root, "icds")
			writeManagedRenderFixture(t, icds, "z.json", []byte("{}"))
			writeManagedRenderFixture(t, icds, "a.json", []byte("{}"))
			if err := os.MkdirAll(filepath.Join(host, "share/vulkan"), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.Symlink(icds, filepath.Join(host, "share/vulkan/icd.d")); err != nil {
				t.Fatal(err)
			}
			script := strings.NewReplacer("/etc/os-release", release, "/run/opengl-driver", host).Replace(hook)
			script = "set -eu\nunset LD_LIBRARY_PATH ONEVPL_SEARCH_PATH LIBVA_DRIVERS_PATH VK_DRIVER_FILES TAILOR_NIXOS_DRIVERS\nhost=" + strconv.Quote(host) + "\n" + tc.initial + "\n" + script + "\n" + script + "\n" + tc.check + "\ntest \"$CGO_ENABLED\" = 1\n"
			cmd := exec.CommandContext(t.Context(), "bash")
			cmd.Stdin = strings.NewReader(script)
			output, err := cmd.CombinedOutput()
			if err != nil {
				t.Fatalf("runtime: %v\n%s", err, output)
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
