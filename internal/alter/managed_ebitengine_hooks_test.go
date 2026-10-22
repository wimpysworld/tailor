package alter

import (
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

func TestEbitengineHooksShareFFmpegRuntime(t *testing.T) {
	for _, ffmpeg := range []bool{false, true} {
		for _, x86 := range []bool{false, true} {
			t.Run("ffmpeg="+strconv.FormatBool(ffmpeg)+"/x86="+strconv.FormatBool(x86), func(t *testing.T) {
				testEbitengineHooksShareFFmpegRuntime(t, ffmpeg, x86)
			})
		}
	}
}

func testEbitengineHooksShareFFmpegRuntime(t *testing.T, ffmpeg, x86 bool) {
	t.Helper()
	nix, err := exec.LookPath("nix")
	if err != nil {
		t.Skip("nix required")
	}
	root := t.TempDir()
	cfg := ebitengineConfig()
	cfg.Languages.GoOptions.FFmpegStatigo = new(ffmpeg)
	for name, content := range renderSelectedManagedFiles(t, cfg) {
		if strings.HasPrefix(name, "nix/") {
			writeManagedRenderFixture(t, root, name, content)
		}
	}
	expression := `let names=["alsa-lib" "libGL" "libX11" "libXcursor" "libXext" "libXi" "libXinerama" "libXrandr" "libXrender" "vulkan-loader"]; pkgs=builtins.listToAttrs (map (name:{inherit name; value="/"+name;}) names) // { stdenv.isLinux=true; stdenv.hostPlatform.isx86_64=` + strconv.FormatBool(x86) + `; intel-media-driver=if pkgs.stdenv.hostPlatform.isx86_64 then "/intel-media-driver" else throw "Intel on ARM"; vpl-gpu-rt=if pkgs.stdenv.hostPlatform.isx86_64 then "/vpl-gpu-rt" else throw "VPL on ARM"; lib.optionalString = b: s: if b then s else ""; lib.optionals = b: xs: if b then xs else []; lib.makeLibraryPath = xs: builtins.concatStringsSep ":" (map (x: x+"/lib") xs); }; in import ` + strconv.Quote(filepath.Join(root, "nix/hooks.nix")) + ` {inherit pkgs;}`
	cmd := exec.CommandContext(t.Context(), nix, "eval", "--offline", "--impure", "--raw", "--expr", expression)
	var stderr strings.Builder
	cmd.Stderr = &stderr
	output, err := cmd.Output()
	if err != nil {
		t.Fatalf("hook eval: %v %s%s", err, output, stderr.String())
	}
	for _, tc := range []struct {
		name, id, initial string
		host, drivers     bool
	}{
		{"nixos", "nixos", "", true, true},
		{"missing host", "nixos", "", false, true},
		{"other linux", "ubuntu", "", true, false},
		{"opt out", "nixos", "export TAILOR_NIXOS_DRIVERS=0", true, false},
		{"empty scalars", "nixos", "export ONEVPL_SEARCH_PATH= LIBVA_DRIVERS_PATH= VK_DRIVER_FILES=", true, true},
		{"custom scalars", "nixos", "export ONEVPL_SEARCH_PATH=chosen LIBVA_DRIVERS_PATH=chosen VK_DRIVER_FILES=chosen", true, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			hostRoot := t.TempDir()
			host := filepath.Join(hostRoot, "driver")
			if tc.host {
				if err := os.MkdirAll(filepath.Join(host, "lib"), 0o755); err != nil {
					t.Fatal(err)
				}
				writeManagedRenderFixture(t, host, "share/vulkan/icd.d/a.json", []byte("{}"))
			}
			release := filepath.Join(hostRoot, "os-release")
			writeManagedRenderFixture(t, hostRoot, "os-release", []byte("ID="+tc.id+"\n"))
			hook := strings.NewReplacer("/etc/os-release", release, "/run/opengl-driver", host).Replace(string(output))
			want := "/custom"
			if ffmpeg {
				want = "/vulkan-loader/lib:" + want
				if x86 {
					want = "/vpl-gpu-rt/lib:/intel-media-driver/lib:" + want
				}
			}
			want = "/alsa-lib/lib:/libGL/lib:/libX11/lib:/libXcursor/lib:/libXext/lib:/libXi/lib:/libXinerama/lib:/libXrandr/lib:/libXrender/lib:" + want
			if tc.host && tc.drivers {
				want = host + "/lib:" + want
			}
			script := "set -eu\nunset TAILOR_NIXOS_DRIVERS ONEVPL_SEARCH_PATH LIBVA_DRIVERS_PATH VK_DRIVER_FILES\nexport LD_LIBRARY_PATH=:/custom:/libGL/lib:/custom: CGO_ENABLED=chosen\n" + tc.initial + "\n" + hook + "\nfirst=$LD_LIBRARY_PATH\n" + hook + "\ntest \"$first\" = \"$LD_LIBRARY_PATH\"\ntest \"$CGO_ENABLED\" = 0\ntest \"$LD_LIBRARY_PATH\" = " + strconv.Quote(want) + "\n"
			switch tc.name {
			case "empty scalars":
				script += "test \"$ONEVPL_SEARCH_PATH:$LIBVA_DRIVERS_PATH:$VK_DRIVER_FILES\" = ::\n"
			case "custom scalars":
				script += "test \"$ONEVPL_SEARCH_PATH:$LIBVA_DRIVERS_PATH:$VK_DRIVER_FILES\" = chosen:chosen:chosen\n"
			default:
				if tc.host && tc.drivers {
					script += "test \"$VK_DRIVER_FILES\" = " + strconv.Quote(host+"/share/vulkan/icd.d/a.json") + "\n"
				} else {
					script += "test \"${VK_DRIVER_FILES+x}\" = ''\n"
				}
				if !ffmpeg || !x86 || !tc.drivers {
					script += "test \"${ONEVPL_SEARCH_PATH+x}${LIBVA_DRIVERS_PATH+x}\" = ''\n"
				}
			}
			cmd := exec.CommandContext(t.Context(), "bash")
			cmd.Stdin = strings.NewReader(script)
			if out, err := cmd.CombinedOutput(); err != nil {
				t.Fatalf("hook runtime: %v %s", err, out)
			}
		})
	}
}
