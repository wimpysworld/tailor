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

func TestEbitengineAndroidAdoptionGuidance(t *testing.T) {
	results := []SwatchResult{{Path: "flake.nix", Category: Skipped, Reason: SkipManagedRootExists}}
	report := buildReport("baste", "", nil, nil, nil, results, DryRun)
	appendManagedReporting(&report, ebitengineConfig(), results)
	for _, text := range []string{
		"For an existing `android` output",
		".overrideAttrs (old:",
		"Remove duplicate SDK, adb, Java, and shared hook declarations",
		"tailor-ebitenmobile bind",
		"-Pandroid.aapt2FromMavenOverride=",
		"tool availability alone does not validate Android builds",
	} {
		if !strings.Contains(report.Plain, text) {
			t.Errorf("Android adoption guidance lacks %q", text)
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
		"nix/go-ebitengine.nix":             `{ pkgs }: [ pkgs.go "mobile-runner" pkgs.android-tools ]`,
		"nix/lib/go-ebitengine-runtime.nix": `_: []`,
		"nix/hooks.nix":                     string(files["nix/hooks.nix"]),
		"nix/hooks/go-ebitengine.nix":       `{ pkgs }: assert pkgs.go == "selected-go"; ''export ORDER="''${ORDER-}:desktop"; tailor_prepend_path LD_LIBRARY_PATH /runtime''`,
		"nix/hooks/core.nix":                `{ pkgs }: assert pkgs.go == "selected-go"; ''export ORDER="$ORDER:core"; tailor_prepend_path LD_LIBRARY_PATH /host; export CGO_ENABLED=0''`,
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
  go = "selected-go"; just = "just"; android-tools = "standalone-adb";
  jdk21 = { outPath = "/fixture/jdk"; home = "/fixture/jdk/lib/openjdk"; };
  gradle_8.override = args: assert args.java.home == "/fixture/jdk/lib/openjdk"; { version = "8.14"; };
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
		shell.NDK != shell.Home+"/ndk/28.2.13676358" || shell.BuildTools != shell.Home+"/build-tools/35.0.0" || shell.Java != "/fixture/jdk/lib/openjdk" {
		t.Fatalf("inconsistent Android environment: %+v", shell)
	}
	if len(shell.Packages) != 8 || shell.Packages[0] != "selected-go" || shell.Packages[5] != "selected-go" || shell.Packages[6] != "selected-go" {
		t.Fatalf("caller Go override lost: %v", shell.Packages)
	}
	for _, pkg := range shell.Packages {
		if pkg == "standalone-adb" {
			t.Fatal("standalone adb entered the SDK shell")
		}
	}
	for _, tool := range []string{
		"sdk/libexec/android-sdk/platform-tools/adb",
		"sdk/libexec/android-sdk/build-tools/35.0.0/aapt2",
		"sdk/libexec/android-sdk/build-tools/35.0.0/zipalign",
		"sdk/libexec/android-sdk/build-tools/35.0.0/apksigner",
		"jdk/lib/openjdk/bin/java",
		"jdk/lib/openjdk/bin/javac",
	} {
		writeManagedRenderFixture(t, root, tool, []byte("#!/bin/sh\nexit 97\n"))
		if err := os.Chmod(filepath.Join(root, tool), 0o700); err != nil {
			t.Fatal(err)
		}
	}
	local := func(value string) string { return strings.ReplaceAll(value, "/fixture", root) }
	cmd := exec.CommandContext(t.Context(), "bash", "-eu")
	cmd.Stdin = strings.NewReader(local(shell.Hook) + `
 test "$ORDER" = :desktop:core
 test "$CGO_ENABLED" = 1
 test "$LD_LIBRARY_PATH" = /host:/runtime
 test "$PATH" = "$ANDROID_HOME/platform-tools:$ANDROID_BUILD_TOOLS:$JAVA_HOME/bin:/original/bin"
 test "$(command -v adb)" = "$ANDROID_HOME/platform-tools/adb"
 for tool in aapt2 zipalign apksigner; do
   test "$(command -v "$tool")" = "$ANDROID_BUILD_TOOLS/$tool"
 done
 for tool in java javac; do
   test "$(command -v "$tool")" = "$JAVA_HOME/bin/$tool"
 done
` + local(shell.Hook) + `
 test "$ORDER" = :desktop:core:desktop:core
 test "$LD_LIBRARY_PATH" = /host:/runtime
 test "$PATH" = "$ANDROID_HOME/platform-tools:$ANDROID_BUILD_TOOLS:$JAVA_HOME/bin:/original/bin"
`)
	cmd.Env = append(os.Environ(), "ORDER=", "LD_LIBRARY_PATH=", "PATH=/original/bin", "JAVA_HOME="+local(shell.Java), "ANDROID_HOME="+local(shell.Home), "ANDROID_BUILD_TOOLS="+local(shell.BuildTools))
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("Android shell hook: %v\n%s", err, output)
	}
	for _, host := range []string{"aarch64-linux", "aarch64-darwin", "x86_64-darwin"} {
		unsupported := strings.Replace(expr, `stdenv.hostPlatform.system = "x86_64-linux"`, `stdenv.hostPlatform.system = "`+host+`"`, 1)
		unsupported = strings.Replace(unsupported, strconv.Quote(filepath.Join(root, "sdk")), `throw "SDK evaluated on unsupported host"`, 1)
		output, err := exec.CommandContext(t.Context(), nix, "eval", "--offline", "--impure", "--json", "--expr", unsupported).CombinedOutput()
		if err == nil || !strings.Contains(string(output), "requires an x86_64-linux SDK host") {
			t.Fatalf("unsupported host %s: %v\n%s", host, err, output)
		}
	}
}
