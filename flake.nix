{
  description = "Tailor flake";

  inputs = {
    nixpkgs.url = "github:nixos/nixpkgs/nixos-unstable";
    nix-packages.url = "github:wimpysworld/nix-packages";
    nix-packages.inputs.nixpkgs.follows = "nixpkgs";
  };

  outputs =
    {
      self,
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
      goFor =
        pkgs:
        pkgs.go_1_26.overrideAttrs (_: rec {
          version = "1.26.6";
          src = pkgs.fetchurl {
            url = "https://go.dev/dl/go${version}.src.tar.gz";
            hash = "sha256-oHIcVMaIkBRI13rZs+x+p8R0cwdV/4kTgukuy5P/LLE=";
          };
        });
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
                (goFor pkgs)
                gocyclo
                golangci-lint
                goreleaser
                just
              ]
              ++ (if tailorPkgs ? tailor then [ tailorPkgs.tailor ] else [ ]);
          };
        }
      );

      packages = forAllSystems (
        system:
        let
          pkgs = import nixpkgs { inherit system; };
          go = goFor pkgs;
          buildGoModule = pkgs.buildGo126Module.override { inherit go; };
          version = "0.0.0-${self.sourceInfo.shortRev or (self.sourceInfo.dirtyShortRev or "dirty")}";
          tailor = buildGoModule {
            pname = "tailor";
            inherit version;
            src = ./.;
            vendorHash = "sha256-GuGmFzx3p1k5b61NTVCFlkPz2M+Cd8cyCoop/w89gVU=";
            subPackages = [ "cmd/tailor" ];
            ldflags = [
              "-s"
              "-w"
              "-X main.version=${version}"
            ];
          };
        in
        {
          inherit tailor;
          default = tailor;
        }
      );
    };
}
