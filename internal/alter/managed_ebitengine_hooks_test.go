package alter

import (
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

func TestEbitengineHooksShareFFmpegRuntime(t *testing.T) {
	nix, err := exec.LookPath("nix")
	if err != nil {
		t.Skip("nix required")
	}
	root := t.TempDir()
	cfg := ebitengineConfig()
	cfg.Languages.GoOptions.FFmpegStatigo = new(true)
	for name, content := range renderSelectedManagedFiles(t, cfg) {
		if strings.HasPrefix(name, "nix/") {
			writeManagedRenderFixture(t, root, name, content)
		}
	}
	expression := `let names=["alsa-lib" "libGL" "libX11" "libXcursor" "libXext" "libXi" "libXinerama" "libXrandr" "libXrender" "vulkan-loader" "intel-media-driver" "vpl-gpu-rt"]; pkgs=builtins.listToAttrs (map (name:{inherit name; value="/"+name;}) names) // { stdenv.isLinux=true; lib.optionalString = b: s: if b then s else ""; lib.optionals = b: xs: if b then xs else []; lib.makeLibraryPath = xs: builtins.concatStringsSep ":" (map (x: x+"/lib") xs); }; in import ` + strconv.Quote(filepath.Join(root, "nix/hooks.nix")) + ` {inherit pkgs;}`
	output, err := exec.CommandContext(t.Context(), nix, "eval", "--offline", "--impure", "--raw", "--expr", expression).CombinedOutput()
	if err != nil {
		t.Fatalf("hook eval: %v %s", err, output)
	}
	for _, tc := range []struct {
		name, id, initial string
		enabled           bool
	}{
		{"nixos", "nixos", "", true},
		{"other linux", "ubuntu", "", false},
		{"opt out", "nixos", "export TAILOR_NIXOS_DRIVERS=0", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			release := filepath.Join(root, "os-release")
			writeManagedRenderFixture(t, root, "os-release", []byte("ID="+tc.id+"\n"))
			hook := strings.NewReplacer("/etc/os-release", release, "/run/opengl-driver", filepath.Join(root, "missing-driver")).Replace(string(output))
			script := "set -eu\nunset TAILOR_NIXOS_DRIVERS\nexport LD_LIBRARY_PATH=/custom ONEVPL_SEARCH_PATH= LIBVA_DRIVERS_PATH= VK_DRIVER_FILES= CGO_ENABLED=chosen\n" + tc.initial + "\n" + hook + "\nfirst=$LD_LIBRARY_PATH\n" + hook + "\ntest \"$first\" = \"$LD_LIBRARY_PATH\"\ntest \"$CGO_ENABLED\" = 0\ntest \"$ONEVPL_SEARCH_PATH:$LIBVA_DRIVERS_PATH:$VK_DRIVER_FILES\" = ::\n"
			if tc.enabled {
				script += "case $LD_LIBRARY_PATH in *'/libGL/lib:'*'/custom') ;; *) exit 41;; esac\ncase $LD_LIBRARY_PATH in *'/vulkan-loader/lib:'*) ;; *) exit 42;; esac\n"
			} else {
				script += "test \"$LD_LIBRARY_PATH\" = /custom\n"
			}
			cmd := exec.CommandContext(t.Context(), "bash")
			cmd.Stdin = strings.NewReader(script)
			if out, err := cmd.CombinedOutput(); err != nil {
				t.Fatalf("hook runtime: %v %s", err, out)
			}
		})
	}
}
