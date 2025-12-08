{
    inputs = {
        nixpkgs.url = "github:NixOS/nixpkgs/nixos-25.05";
        flake-utils.url = "github:numtide/flake-utils";
    };

    outputs =
        {
            self,
            nixpkgs,
            flake-utils,
            ...
        }:
        flake-utils.lib.eachDefaultSystem (
            system:
            let
                pkgs = import nixpkgs { inherit system; };
                inherit (pkgs) lib mkShell buildGoModule;
            in
            {
                devShells.default = mkShell {
                    shellHook = "export NIX_SHELL_NAME='mkimg'";
                    nativeBuildInputs = with pkgs; [
                        go
                    ];
                };

                defaultPackage = buildGoModule rec {
                    name = "mkimg";

                    src = self;

                    vendorHash = "sha256-7jxXSbJKA/gqHMTxbTKVdZMEOYx6fyNzcL/XOpcpBMc=";

                    meta = {
                        description = "mkimg is a tiny tool to simplify the process of creating partitioned disk images";
                        homepage = "https://github.com/elysium-os/mkimg";
                        maintainers = with lib.maintainers; [ wux ];
                    };
                };
            }
        );
}
