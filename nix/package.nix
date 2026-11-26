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
  vendorHash = "sha256-cGt8r22M0Rq+kycRbh1PsxKQTtN5qQrWDOBGVikYz9M=";
  subPackages = [ "cmd/tailor" ];
  ldflags = [
    "-s"
    "-w"
    "-X main.version=${version}"
  ];
}
