package alter

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

func TestEbitengineAndroidEnablementScopesConsent(t *testing.T) {
	for _, enabled := range []bool{false, true} {
		cfg := ebitengineConfig()
		cfg.Languages.GoOptions.Ebitengine.Android.Enabled = enabled
		files := renderSelectedManagedFiles(t, cfg)
		if (files["nix/shells/go-ebitengine-android.nix"] != nil) != enabled {
			t.Fatal("Android shell does not match enablement")
		}
		for path, content := range files {
			if strings.Contains(string(content), "android_sdk.accept_license = true") && path != "nix/shells/go-ebitengine-android.nix" {
				t.Fatalf("SDK consent escaped the Android shell: %s", path)
			}
			if strings.Contains(string(content), "allowUnfree = true") {
				t.Fatalf("unrestricted unfree packages: %s", path)
			}
		}
	}
}

func TestEbitengineAndroidShellEnvironment(t *testing.T) {
	nix, err := exec.LookPath("nix")
	if err != nil {
		t.Skip("nix required")
	}
	root := t.TempDir()
	files := renderSelectedManagedFiles(t, ebitengineConfig())
	writeManagedRenderFixture(t, root, "nix/shells/go-ebitengine-android.nix", files["nix/shells/go-ebitengine-android.nix"])
	fixtures := map[string]string{
		"nix/go.nix":                        `{ pkgs }: [ pkgs.go ]`,
		"nix/go-ebitengine.nix":             `{ pkgs }: [ pkgs.go "mobile-runner" ]`,
		"nix/lib/go-ebitengine-runtime.nix": `_: []`,
		"nix/hooks.nix":                     `{ pkgs }: assert pkgs.go == "selected-go"; ''export ORDER=shared; export CGO_ENABLED=0''`,
		"nix/hooks/go-ebitengine.nix":       `{ pkgs }: assert pkgs.go == "selected-go"; ''export ORDER="$ORDER:desktop"''`,
		"nix/hooks/core.nix":                `{ pkgs }: assert pkgs.go == "selected-go"; ''export ORDER="$ORDER:core"; export CGO_ENABLED=0''`,
		"sdk/default.nix": `{ system, config }:
assert system == "x86_64-linux";
assert config.android_sdk.accept_license;
assert builtins.all (name: config.allowUnfreePredicate { inherit name; }) [
  "android-sdk-cmdline-tools" "android-sdk-platform-tools" "android-sdk-tools"
  "android-sdk-build-tools" "android-sdk-platforms" "android-sdk-ndk"
  "platform-tools" "tools" "cmdline-tools" "build-tools" "platforms" "ndk"
];
assert !(config.allowUnfreePredicate { name = "unrelated-unfree"; });
{ androidenv.composeAndroidPackages = args:
  assert args.platformVersions == [ "35" ];
  assert args.buildToolsVersions == [ "35.0.0" ];
  assert args.ndkVersions == [ "28.2.13676358" ];
  assert args.includeNDK;
  assert !(args.includeCmake || args.includeEmulator || args.includeSystemImages || args.includeSources);
  { androidsdk = "/fixture/sdk"; };
}`,
	}
	for path, content := range fixtures {
		writeManagedRenderFixture(t, root, path, []byte(content))
	}
	expr := `let pkgs = {
  path = ` + strconv.Quote(filepath.Join(root, "sdk")) + `;
  stdenv.hostPlatform.system = "x86_64-linux";
  go = "selected-go"; just = "just"; jdk21 = "/fixture/jdk";
  gradle_8.override = args: assert args.java == "/fixture/jdk"; { version = "8.14"; };
  lib.getName = p: p.name;
  lib.versionAtLeast = actual: required: actual == "8.14" && required == "8.13";
  mkShell = args: args;
}; in import ` + strconv.Quote(filepath.Join(root, "nix/shells/go-ebitengine-android.nix")) + ` { inherit pkgs; }`
	output, err := exec.CommandContext(t.Context(), nix, "eval", "--offline", "--impure", "--json", "--expr", expr).CombinedOutput()
	if err != nil {
		t.Fatalf("Android shell evaluation: %v\n%s", err, output)
	}
	var shell struct {
		Home       string `json:"ANDROID_HOME"`
		Root       string `json:"ANDROID_SDK_ROOT"`
		NDK        string `json:"ANDROID_NDK_HOME"`
		BuildTools string `json:"ANDROID_BUILD_TOOLS"`
		Java       string `json:"JAVA_HOME"`
		Hook       string `json:"shellHook"`
		Packages   []any  `json:"packages"`
	}
	if err := json.Unmarshal(output, &shell); err != nil {
		t.Fatal(err)
	}
	if shell.Home != "/fixture/sdk/libexec/android-sdk" || shell.Root != shell.Home ||
		shell.NDK != shell.Home+"/ndk/28.2.13676358" || shell.BuildTools != shell.Home+"/build-tools/35.0.0" || shell.Java != "/fixture/jdk" {
		t.Fatalf("inconsistent Android environment: %+v", shell)
	}
	if len(shell.Packages) != 8 || shell.Packages[0] != "selected-go" || shell.Packages[5] != "selected-go" || shell.Packages[6] != "selected-go" {
		t.Fatalf("caller Go override lost: %v", shell.Packages)
	}
	cmd := exec.CommandContext(t.Context(), "bash", "-eu")
	cmd.Stdin = strings.NewReader(shell.Hook + `
 test "$ORDER" = shared:desktop:core
 test "$CGO_ENABLED" = 1
 test "$PATH" = "$ANDROID_HOME/platform-tools:$ANDROID_BUILD_TOOLS:/original/bin"
`)
	cmd.Env = append(os.Environ(), "PATH=/original/bin", "ANDROID_HOME="+shell.Home, "ANDROID_BUILD_TOOLS="+shell.BuildTools)
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("Android shell hook: %v\n%s", err, output)
	}
	for _, host := range []string{"aarch64-linux", "aarch64-darwin", "x86_64-darwin"} {
		unsupported := strings.Replace(expr, `stdenv.hostPlatform.system = "x86_64-linux"`, `stdenv.hostPlatform.system = "`+host+`"`, 1)
		output, err := exec.CommandContext(t.Context(), nix, "eval", "--offline", "--impure", "--json", "--expr", unsupported).CombinedOutput()
		if err == nil || !strings.Contains(string(output), "requires an x86_64-linux SDK host") {
			t.Fatalf("unsupported host %s: %v\n%s", host, err, output)
		}
	}
}
