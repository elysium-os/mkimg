{
    inputs = {
        nixpkgs.url = "github:NixOS/nixpkgs/nixos-24.11";
        flake-utils.url = "github:numtide/flake-utils";
    };

    outputs = { self, nixpkgs, flake-utils, ... } @ inputs: flake-utils.lib.eachDefaultSystem (system:
        let
            pkgs = import nixpkgs { inherit system; };
            inherit (pkgs) lib mkShell buildGoModule;
        in {
            devShells.default = mkShell {
                shellHook = "export NIX_SHELL_NAME='mkimg'";
                nativeBuildInputs = with pkgs; [
                    go
                ];
            };

            defaultPackage = buildGoModule rec {
                name = "mkimg";

                src = self;

                vendorHash = "sha256-7AocOQd9Jwz7gFQZG07YYXygBDgKIbKyDU0wf4eHc3s=";

                meta = {
                    description = "mkimg is a tiny tool to simplify the process of creating partitioned disk images";
                    homepage = https://git.thenest.dev/wux/mkimg;
                    maintainers = with lib.maintainers; [ wux ];
                };
            };
        }
    );
}
