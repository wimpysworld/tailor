{
  description = "Tailor flake";

  inputs = {
    nixpkgs.url = "github:nixos/nixpkgs/nixpkgs-unstable";
    nix-packages.url = "git+https://nix.wimpys.world/packages.git";
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
            shellHook = import ./nix/hooks.nix { inherit pkgs; };
            packages =
              with pkgs;
              [
                actionlint
                cosign
                gh
                just
              ]
              ++ (if tailorPkgs ? tailor then [ tailorPkgs.tailor ] else [ ])
              ++ import ./nix/loader.nix { inherit pkgs; };
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
            vendorHash = "sha256-t4ipStCQXEPvB4/BYKbkt0Y4ikMbBG83ZtlCO7SZYrg=";
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
