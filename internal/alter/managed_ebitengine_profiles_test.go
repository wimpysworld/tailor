package alter

import (
	"encoding/json"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/wimpysworld/tailor/internal/config"
	"gopkg.in/yaml.v3"
)

func TestEbitengineDesktopWebPackages(t *testing.T) {
	nix, err := exec.LookPath("nix")
	if err != nil {
		t.Skip("nix required")
	}
	for _, declaration := range []string{"true", "{enabled: true}", "{enabled: true, android: {enabled: false}}", "{enabled: true, android: {enabled: true}}"} {
		for _, host := range []struct {
			system string
			linux  bool
		}{{"x86_64-linux", true}, {"aarch64-darwin", false}} {
			t.Run(host.system+"/"+declaration, func(t *testing.T) {
				cfg := ebitengineConfig()
				var engine config.EbitengineOptions
				if err := yaml.Unmarshal([]byte(declaration), &engine); err != nil {
					t.Fatal(err)
				}
				cfg.Languages.GoOptions.Ebitengine = &engine
				root := t.TempDir()
				files := renderSelectedManagedFiles(t, cfg)
				if (files["nix/shells/go-ebitengine-android.nix"] != nil) != cfg.EbitengineAndroidEnabled() {
					t.Fatal("Android shell does not match declaration")
				}
				for _, path := range []string{"nix/go-ebitengine.nix", "nix/lib/go-ebitengine-runtime.nix"} {
					writeManagedRenderFixture(t, root, path, files[path])
				}
				expression := `let isLinux = ` + strconv.FormatBool(host.linux) + `; names = [ "bash" "coreutils" "findutils" "gnused" "gnugrep" "gawk" "git" "curl" "gzip" "zip" "unzip" "alsa-lib" "libGL" "libX11" "libXcursor" "libXext" "libXi" "libXinerama" "libXrandr" "libXrender" "mesa-demos" "pciutils" "brotli" "miniserve" "ffmpeg-headless" "imagemagick" "vorbis-tools" "git-lfs" "android-tools" "nodejs" "wrangler" "butler" "openssh" "rsync" "openssl" "avahi" ]; pkgs = builtins.listToAttrs (map (name: { inherit name; value=if !isLinux && builtins.elem name [ "alsa-lib" "libGL" "libX11" "libXcursor" "libXext" "libXi" "libXinerama" "libXrandr" "libXrender" "mesa-demos" "pciutils" "avahi" ] then throw "Linux-only dependency on Darwin: ${name}" else name; }) names) // { python3=throw "default shell evaluated Python"; stdenv.isLinux=isLinux; path=throw "default shell evaluated SDK package set"; androidenv=throw "default shell evaluated Android SDK"; lib.optionals=b: xs: if b then xs else []; go="selected-go"; go-tools=throw "Ebitengine must use generic Go Staticcheck"; writeShellApplication = _: "runner"; }; in map toString (import ` + strconv.Quote(filepath.Join(root, "nix/go-ebitengine.nix")) + ` {inherit pkgs;})`
				output, err := exec.CommandContext(t.Context(), nix, "eval", "--offline", "--impure", "--json", "--expr", expression).CombinedOutput()
				if err != nil {
					t.Fatalf("package eval: %v %s", err, output)
				}
				var packages []string
				if err := json.Unmarshal(output, &packages); err != nil {
					t.Fatal(err)
				}
				for _, pkg := range []string{"bash", "coreutils", "findutils", "gnused", "gnugrep", "gawk", "git", "curl", "gzip", "zip", "unzip", "openssh", "rsync", "openssl", "runner", "brotli", "miniserve", "ffmpeg-headless", "imagemagick", "vorbis-tools", "git-lfs", "android-tools", "nodejs", "wrangler", "butler"} {
					if !slices.Contains(packages, pkg) {
						t.Fatalf("missing base package %s: %v", pkg, packages)
					}
				}
				for _, pkg := range []string{"alsa-lib", "libGL", "libX11", "libXcursor", "libXext", "libXi", "libXinerama", "libXrandr", "libXrender", "mesa-demos", "pciutils", "avahi"} {
					if slices.Contains(packages, pkg) != host.linux {
						t.Fatalf("%s presence != %v: %v", pkg, host.linux, packages)
					}
				}
				unique := slices.Clone(packages)
				slices.Sort(unique)
				if len(packages) != len(slices.Compact(unique)) {
					t.Fatal("duplicate packages in managed fragment")
				}
				for _, pkg := range []string{"blender", "python3", "python-with-media", "numpy", "pillow", "matplotlib", "pyright", "staticcheck"} {
					if slices.Contains(packages, pkg) {
						t.Fatalf("project-owned package %s in managed fragment", pkg)
					}
				}
				baseline := &config.Config{Languages: &config.LanguageSettings{Go: new(true), GoOptions: &config.GoOptions{CGO: new(false)}}}
				if string(files["just/tailor.just"]) != string(renderSelectedManagedFiles(t, baseline)["just/tailor.just"]) {
					t.Fatal("Ebitengine changed quality or setup commands")
				}
			})
		}
	}
}

func TestEbitenginePartialFilesAndTypedLoaders(t *testing.T) {
	nix, err := exec.LookPath("nix")
	if err != nil {
		t.Skip("nix required")
	}
	root := t.TempDir()
	files := renderSelectedManagedFiles(t, ebitengineConfig())
	for _, path := range []string{"nix/loader.nix", "nix/hooks.nix", "nix/hooks/go-ebitengine.nix"} {
		writeManagedRenderFixture(t, root, path, files[path])
	}
	writeManagedRenderFixture(t, root, "nix/shells/go-ebitengine-android.nix", []byte(`throw "shell must not enter loader"`))
	expression := `let pkgs={stdenv.isLinux=true; lib.optionalString=b: s: if b then s else "";}; in { packages=import ` + strconv.Quote(filepath.Join(root, "nix/loader.nix")) + ` {inherit pkgs;}; hooks=builtins.typeOf (import ` + strconv.Quote(filepath.Join(root, "nix/hooks.nix")) + ` {inherit pkgs;}); fragment=import ` + strconv.Quote(filepath.Join(root, "nix/hooks/go-ebitengine.nix")) + ` {inherit pkgs;}; }`
	output, err := exec.CommandContext(t.Context(), nix, "eval", "--offline", "--impure", "--json", "--expr", expression).CombinedOutput()
	if err != nil || strings.TrimSpace(string(output)) != `{"fragment":"","hooks":"string","packages":[]}` {
		t.Fatalf("partial files: %v %s", err, output)
	}
}
