{
  description = "nixpkgs PR tracker";

  inputs = {
    nixpkgs.url = "github:NixOS/nixpkgs/nixos-unstable";
    treefmt-nix.url = "github:numtide/treefmt-nix";
    git-hooks.url = "github:cachix/git-hooks.nix";
  };

  outputs =
    {
      self,
      nixpkgs,
      treefmt-nix,
      git-hooks,
    }:
    let
      systems = [
        "x86_64-linux"
        "aarch64-linux"
        "x86_64-darwin"
        "aarch64-darwin"
      ];
      forAllSystems =
        f:
        nixpkgs.lib.genAttrs systems (
          system:
          f {
            inherit system;
            pkgs = nixpkgs.legacyPackages.${system};
          }
        );
      treefmtEval = forAllSystems ({ pkgs, ... }: treefmt-nix.lib.evalModule pkgs ./treefmt.nix);
    in
    {
      formatter = forAllSystems ({ system, ... }: treefmtEval.${system}.config.build.wrapper);

      checks = forAllSystems (
        { system, ... }:
        {
          formatting = treefmtEval.${system}.config.build.check self;
          pre-commit-check = git-hooks.lib.${system}.run {
            src = ./.;
            hooks = {
              treefmt = {
                enable = true;
                packageOverrides.treefmt = treefmtEval.${system}.config.build.wrapper;
              };
            };
          };
        }
      );

      packages = forAllSystems (
        { pkgs, ... }:
        let
          applied = pkgs.extend self.overlays.default;
        in
        {
          default = applied.nixpkgs-pr-tracker;
        }
      );

      overlays.default = final: prev: {
        nixpkgs-pr-tracker = final.buildGoModule {
          pname = "nixpkgs-pr-tracker";
          version = "0.1.0";
          src = ./.;
          vendorHash = "sha256-JlQWPfcNpIgag1LHDcvz1wlxo/RcdN02J3zKXFd1tvc=";
        };
      };

      devShells = forAllSystems (
        { system, pkgs, ... }:
        {
          default = pkgs.mkShell {
            inherit (self.checks.${system}.pre-commit-check) shellHook;
            packages = with pkgs; [
              go
              gopls
              gotools
            ];
          };
        }
      );
    };
}
