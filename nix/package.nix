{ pkgs, self, ... }:
let
  # The release workflow writes nix/version into the commit that the latest tag names.
  version =
    if builtins.pathExists ./version then
      pkgs.lib.trim (builtins.readFile ./version)
    else
      "0.0.0-${self.sourceInfo.shortRev or (self.sourceInfo.dirtyShortRev or "dirty")}";
in
pkgs.buildGo126Module {
  pname = "tailor";
  inherit version;
  src = self;
  vendorHash = "sha256-Qn3XxM9KybGXE92RT26ldj+x8HIReXDGPBkAqv1NnNQ=";
  subPackages = [ "cmd/tailor" ];
  ldflags = [
    "-s"
    "-w"
    "-X main.version=${version}"
  ];
}
