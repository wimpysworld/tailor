# Managed by Tailor: nix/hooks/go-ebitengine.nix
{ pkgs, ... }:
pkgs.lib.optionalString
  (pkgs.stdenv.isLinux && builtins.pathExists ../lib/go-ebitengine-runtime.nix)
  ''
    tailor_prepend_path LD_LIBRARY_PATH "${
      pkgs.lib.makeLibraryPath (import ../lib/go-ebitengine-runtime.nix { inherit pkgs; })
    }"
  ''
