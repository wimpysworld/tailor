{
  description = "Tailor flake";

  inputs = {
    nixpkgs.url = "github:nixos/nixpkgs/nixpkgs-26.05-darwin";
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
        "x86_64-darwin"
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
          version = "0.0.0-${self.sourceInfo.shortRev or (self.sourceInfo.dirtyShortRev or "dirty")}";
          tailor = pkgs.buildGo126Module {
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
