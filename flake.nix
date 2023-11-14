# References:
# https://blog.lenny.ninja/part-1-quickly-packaging-services-using-nix-flakes.html
# https://ayats.org/blog/no-flake-utils/
# https://nixos.org/manual/nixos/stable/#sec-writing-modules
# https://nixos.org/manual/nixpkgs/stable/#ssec-language-go
{
  description = "Convert Docker Compose to Nix";

  inputs = {
    nixpkgs.url = "github:NixOS/nixpkgs/nixos-unstable";
  };

  outputs = { self, nixpkgs, ... }: let
    supportedSystems = [ "x86_64-linux" "x86_64-darwin" "aarch64-linux" "aarch64-darwin" ];
    forAllSystems = function: nixpkgs.lib.genAttrs supportedSystems (system: function nixpkgs.legacyPackages.${system});
    pname = "compose2nix";
    owner = "aksiksi";
    version = "0.1.3";
  in {
    # Nix package
    packages = forAllSystems (pkgs: {
      # TODO(aksiksi): Pull from GitHub.
      default = pkgs.buildGoModule {
        inherit pname;
        inherit version;
        src = ./.;
        vendorSha256 = "sha256-+Ht+cdaTwmfdZ2lMnSuX9EBkU2V8rXyH1qBet8pZ+dU=";
      };
    });

    # Development shell
    devShells = forAllSystems (pkgs: {
      default = pkgs.mkShell {
        buildInputs = [ pkgs.go pkgs.gopls ];
      };
    });
  };
}
