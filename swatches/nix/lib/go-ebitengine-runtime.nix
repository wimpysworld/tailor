# Managed by Tailor: nix/lib/go-ebitengine-runtime.nix
{ pkgs, ... }:
pkgs.lib.optionals pkgs.stdenv.isLinux (
  with pkgs;
  [
    alsa-lib
    libGL
    libX11
    libXcursor
    libXext
    libXi
    libXinerama
    libXrandr
    libXrender
  ]
)
