# Managed by Tailor: nix/shells/go-ebitengine-android.nix
{ pkgs, ... }:
let
  sdkPkgs = import pkgs.path {
    system = pkgs.stdenv.hostPlatform.system;
    config = {
      android_sdk.accept_license = true;
      allowUnfreePredicate =
        package:
        builtins.elem (pkgs.lib.getName package) [
          "android-sdk-cmdline-tools"
          "android-sdk-platform-tools"
          "android-sdk-tools"
          "android-sdk-build-tools"
          "android-sdk-platforms"
          "android-sdk-ndk"
          "platform-tools"
          "tools"
          "cmdline-tools"
          "build-tools"
          "platforms"
          "ndk"
        ];
    };
  };
  sdk =
    (sdkPkgs.androidenv.composeAndroidPackages {
      platformVersions = [ "35" ];
      buildToolsVersions = [ "35.0.0" ];
      ndkVersions = [ "28.2.13676358" ];
      includeNDK = true;
      includeCmake = false;
      includeEmulator = false;
      includeSystemImages = false;
      includeSources = false;
    }).androidsdk;
  jdk = pkgs.jdk21;
  gradle = pkgs.gradle_8.override { java = jdk; };
  home = "${sdk}/libexec/android-sdk";
  complete = builtins.all builtins.pathExists [
    ../go.nix
    ../go-ebitengine.nix
    ../hooks.nix
    ../hooks/go-ebitengine.nix
    ../lib/go-ebitengine-runtime.nix
  ];
in
if pkgs.stdenv.hostPlatform.system != "x86_64-linux" then
  throw "Tailor Android requires an x86_64-linux SDK host; retain your project shell on other hosts."
else if !complete then
  throw "Tailor Android files are incomplete; reconcile the managed files before entering the shell."
else if !pkgs.lib.versionAtLeast gradle.version "8.13" then
  throw "Tailor Android requires Gradle 8 >=8.13; retain a compatible project override."
else
  pkgs.mkShell {
    packages = [
      pkgs.go
      pkgs.just
      sdk
      jdk
      gradle
    ]
    ++ import ../go.nix { inherit pkgs; }
    ++ builtins.filter (package: package != pkgs.android-tools) (
      import ../go-ebitengine.nix { inherit pkgs; }
    );
    ANDROID_HOME = home;
    ANDROID_SDK_ROOT = home;
    ANDROID_NDK_HOME = "${home}/ndk/28.2.13676358";
    ANDROID_BUILD_TOOLS = "${home}/build-tools/35.0.0";
    JAVA_HOME = jdk.home;
    shellHook =
      (import ../hooks.nix { inherit pkgs; })
      + "\n"
      + ''
        tailor_prepend_path PATH "${home}/platform-tools:${home}/build-tools/35.0.0:${jdk.home}/bin"
        export CGO_ENABLED=1
      '';
  }
