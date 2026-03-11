{
  description = "Tailor flake";

  inputs = {
    nixpkgs.url = "github:nixos/nixpkgs/nixos-unstable";
  };

  outputs =
    { nixpkgs, ... }:
    let
      supportedSystems = [
        "x86_64-linux"
        "aarch64-darwin"
        "aarch64-linux"
      ];
      forAllSystems = nixpkgs.lib.genAttrs supportedSystems;
    in
    {
      devShells = forAllSystems (
        system:
        let
          pkgs = import nixpkgs { inherit system; };
        in
        {
          default = pkgs.mkShell {
            packages = with pkgs; [
              actionlint
              cosign
              gh
              go
              gocyclo
              golangci-lint
              goreleaser
              ineffassign
              just
            ];
          };
        }
      );
    }
    // (
      if builtins.pathExists ./pkgs/tailor/default.nix then
        {
          packages = forAllSystems (
            system:
            let
              pkgs = import nixpkgs { inherit system; };
              tailor = pkgs.callPackage ./pkgs/tailor/default.nix { };
            in
            {
              tailor = tailor;
              default = tailor;
            }
          );
        }
      else
        { }
    );
}
