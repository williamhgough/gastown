{
  description = "Multi-agent orchestration system for Claude Code with persistent work tracking";

  inputs = {
    nixpkgs.url = "github:NixOS/nixpkgs/nixos-unstable";
    flake-utils.url = "github:numtide/flake-utils";
    beads.url = "github:gastownhall/beads/v1.3.1";
  };

  outputs =
    {
      self,
      nixpkgs,
      flake-utils,
      beads,
    }:
    flake-utils.lib.eachDefaultSystem (
      system:
      let
        pkgs = import nixpkgs {
          inherit system;
        };
        beadsPkg = beads.packages.${system}.default;
      in
      {
        packages = {
          gt = pkgs.buildGo127Module {
            pname = "gt";
            version = "1.0.0";
            src = ./.;
            vendorHash = "sha256-KMeBz8px0GfAmUmlgzctn2w12MYcZISMXOAeqJvyd+4=";

            ldflags = [
              "-X github.com/gastownhall/gastown/internal/cmd.Build=nix"
              "-X github.com/steveyegge/gastown/internal/cmd.BuiltProperly=1"
            ];

            subPackages = [ "cmd/gt" ];
            buildInputs = [ pkgs.icu ];

            meta = with pkgs.lib; {
              description = "Multi-agent orchestration system for Claude Code with persistent work tracking";
              homepage = "https://github.com/gastownhall/gastown";
              license = licenses.mit;
              mainProgram = "gt";
            };
          };
          default = self.packages.${system}.gt;
        };

        apps = {
          gt = flake-utils.lib.mkApp {
            drv = self.packages.${system}.gt;
          };
          default = self.apps.${system}.gt;
        };

        devShells.default = pkgs.mkShell {
          buildInputs = [
            beadsPkg
            pkgs.go_1_27
            pkgs.gopls
            pkgs.gotools
            pkgs.go-tools
          ];
        };
      }
    );
}
