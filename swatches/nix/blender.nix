# Managed by Tailor: nix/blender.nix
{ pkgs, ... }:
[ (import ./lib/blender.nix { inherit pkgs; }).bridge ]
