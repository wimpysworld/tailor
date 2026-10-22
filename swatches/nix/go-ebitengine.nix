# Managed by Tailor: nix/go-ebitengine.nix
{ pkgs, ... }:
let
  runtime =
    if builtins.pathExists ./lib/go-ebitengine-runtime.nix then
      import ./lib/go-ebitengine-runtime.nix { inherit pkgs; }
    else
      [ ];
  mobile = pkgs.writeShellApplication {
    name = "tailor-ebitenmobile";
    runtimeInputs = [
      pkgs.go
      pkgs.coreutils
    ];
    text = ''
      caller="$PWD"
      target=""
      next_target=0
      for arg in "$@"; do
        if [ "$next_target" = 1 ]; then target="$arg"; next_target=0; fi
        case "$arg" in
          -target) next_target=1 ;;
          -target=*) target="''${arg#-target=}" ;;
        esac
      done
      IFS=, read -r -a targets <<< "$target"
      for component in ''${targets[@]+"''${targets[@]}"}; do
        case "''${component%%/*}" in
          android)
            if [ ! -d "''${ANDROID_HOME:-}" ] || [ ! -d "''${ANDROID_NDK_HOME:-}" ]; then
              echo "Android binding requires the reviewed Android shell and SDK paths." >&2
              exit 1
            fi ;;
          ios|iossimulator)
            if [ "$(go env GOHOSTOS)" != darwin ] || ! command -v xcrun >/dev/null || ! xcrun --find clang >/dev/null; then
              echo "iOS binding requires macOS and an installed, selected, licensed Xcode." >&2
              exit 1
            fi ;;
        esac
      done
      module_dir=$(go list -mod=readonly -f '{{.Module.Dir}}' github.com/hajimehoshi/ebiten/v2) || {
        echo "Cannot resolve Ebitengine. Prepare the project's pinned module dependencies first." >&2
        exit 1
      }
      if [ -z "$module_dir" ] || [ ! -d "$module_dir/cmd/ebitenmobile" ]; then
        echo "The resolved Ebitengine module has no cmd/ebitenmobile source." >&2
        exit 1
      fi
      host_os=$(go env GOHOSTOS)
      host_arch=$(go env GOHOSTARCH)
      tmp=$(mktemp -d)
      trap 'rm -rf -- "$tmp"' EXIT
      trap 'exit 129' HUP
      trap 'exit 130' INT
      trap 'exit 143' TERM
      (
        unset GOARM GOARM64 GOAMD64 GO386 GOMIPS GOMIPS64 GOPPC64 GORISCV64 GOWASM GOEXPERIMENT
        export GOOS="$host_os" GOARCH="$host_arch" GOWORK=off GOFLAGS= CGO_ENABLED=0
        go -C "$module_dir" build -mod=readonly -o "$tmp/ebitenmobile" ./cmd/ebitenmobile
      )
      cd "$caller"
      CGO_ENABLED=1 "$tmp/ebitenmobile" "$@"
    '';
  };
in
with pkgs;
[
  bash
  coreutils
  findutils
  gnused
  gnugrep
  gawk
  git
  curl
  gzip
  zip
  unzip
  (go-tools.override { buildGoModule = pkgs.buildGoModule.override { go = pkgs.go; }; })
  mobile
  brotli
  miniserve
  ffmpeg-headless
  imagemagick
  vorbis-tools
  git-lfs
  android-tools
  nodejs
  wrangler
  butler
]
++ lib.optionals stdenv.isLinux (
  runtime
  ++ [
    mesa-demos
    pciutils
  ]
)
