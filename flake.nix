# References:
# https://nixos.org/manual/nixos/stable/#sec-writing-modules
# https://nixos.org/manual/nixpkgs/stable/#ssec-language-go
{
  description = "A tool to automatically generate a NixOS config from a Docker Compose project.";

  inputs = {
    nixpkgs.url = "github:NixOS/nixpkgs/nixos-unstable";
    nix-pre-commit.url = "github:jmgilman/nix-pre-commit";
    nix-pre-commit.inputs.nixpkgs.follows = "nixpkgs";
    onchg.url = "github:aksiksi/onchg-rs";
    onchg.inputs.nixpkgs.follows = "nixpkgs";
  };

  outputs = { self, nixpkgs, nix-pre-commit, onchg, ... }: let
    supportedSystems = [ "x86_64-linux" "x86_64-darwin" "aarch64-linux" "aarch64-darwin" ];
    forAllSystems = nixpkgs.lib.genAttrs supportedSystems;
    pkgsFor = system: nixpkgs.legacyPackages.${system};
    pname = "compose2nix";
    owner = "aksiksi";
    # LINT.OnChange(version)
    version = "0.2.1-pre";
    # LINT.ThenChange(main.go:version)
  in {
    # Nix package
    packages = forAllSystems (system:
      let pkgs = pkgsFor system; in {
        default = pkgs.buildGoModule {
          inherit pname;
          inherit version;
          src = ./.;
          vendorHash = "sha256-5DTPG4FiSWguTmcVmys64Y1fXJHlSI/1qj1VEBJomNk=";
        };
      }
    );

    # Development shell
    devShells = forAllSystems (system:
      let pkgs = pkgsFor system; in {
        default = pkgs.mkShell {
          buildInputs = [ pkgs.go pkgs.gopls ];
          # Add a Git pre-commit hook.
          shellHook = (nix-pre-commit.lib.${system}.mkConfig {
            inherit pkgs;
            config = {
              repos = [
                {
                  repo = "local";
                  hooks = [
                    {
                      id = "onchg";
                      language = "system";
                      entry = "${onchg.packages.${system}.default}/bin/onchg repo";
                      types = [ "text" ];
                      pass_filenames = false;
                    }
                  ];
                }
              ];
            };
          }).shellHook;
        };
      }
    );

    # Run:
    # nix build .#checks.x86_64-linux.integrationTest
    # To run interactively:
    # nix build .#checks.x86_64-linux.integrationTest.driverInteractive
    # See: https://nixos.org/manual/nixos/stable/index.html#sec-running-nixos-tests-interactively
    checks.x86_64-linux = let
      pkgs = nixpkgs.legacyPackages.x86_64-linux;
    in {
      # This test is meant to be run by nixos-test/test.sh.
      # https://nixos.org/manual/nixos/stable/index.html#sec-nixos-tests
      # https://nix.dev/tutorials/nixos/integration-testing-using-virtual-machines
      integrationTest = pkgs.nixosTest (import ./nixos-test/test.nix);
    };
  };
}
