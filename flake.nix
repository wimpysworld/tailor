{
  description = "Tailor flake";

  inputs = {
    nixpkgs.url = "github:nixos/nixpkgs/nixos-unstable";
    nix-packages.url = "github:wimpysworld/nix-packages";
    nix-packages.inputs.nixpkgs.follows = "nixpkgs";
  };

  outputs =
    {
      nixpkgs,
      nix-packages,
      ...
    }:
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
          tailorPkgs = nix-packages.packages.${system} or { };
        in
        {
          default = pkgs.mkShell {
            packages =
              with pkgs;
              [
                actionlint
                cosign
                gh
                go_1_26
                gocyclo
                golangci-lint
                goreleaser
                ineffassign
                just
              ]
              ++ (if tailorPkgs ? tailor then [ tailorPkgs.tailor ] else [ ]);
          };
        }
      );

      packages = forAllSystems (
        system:
        let
          tailorPkgs = nix-packages.packages.${system} or { };
        in
        if tailorPkgs ? tailor then
          {
            tailor = tailorPkgs.tailor;
            default = tailorPkgs.tailor;
          }
        else
          { }
      );
    };
}
